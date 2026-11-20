package serverplan

import (
	"slices"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

func TestCheckInviteRight_Negative_NewChannelEveryoneDeniesView(t *testing.T) {
	ctx := inviteRightContext(t)
	ch := ctx.defChannel("rules")
	ch.Access[0].Deny = append(ch.Access[0].Deny, "VIEW_CHANNEL")
	ctx.changes = []Change{{Action: ActionCreateChannel, Object: "channels[rules]"}}
	reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"})
	if reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE: the bot cannot view the channel" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestProjectedInviteChannel_Boundary_SnapshotUnchanged(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels[0].Access = []Access{{Role: "@everyone", Deny: []string{"VIEW_CHANNEL"}}}
	before := slices.Clone(ctx.snap.Channels[0].Access)
	ctx.changes = []Change{{Action: ActionSetAccess, Object: "channels[rules].access[" + everyoneID(ctx.def) + "]"}}
	ctx.projectedInviteChannel(ctx.defChannel("rules"), &ctx.snap.Channels[0])
	if !slices.EqualFunc(ctx.snap.Channels[0].Access, before, func(a, b Access) bool {
		return a.Role == b.Role && slices.Equal(a.Allow, b.Allow) && slices.Equal(a.Deny, b.Deny) &&
			slices.Equal(a.UnknownAllow, b.UnknownAllow) && slices.Equal(a.UnknownDeny, b.UnknownDeny)
	}) {
		t.Fatalf("snapshot access changed: %+v", ctx.snap.Channels[0].Access)
	}
}

func TestCheckInviteRight_Positive_NewChannelBotAllowsView(t *testing.T) {
	ctx := inviteRightContext(t)
	ch := ctx.defChannel("rules")
	ch.Access[0].Deny = append(ch.Access[0].Deny, "VIEW_CHANNEL")
	ch.Access[1].Allow = append(ch.Access[1].Allow, "VIEW_CHANNEL")
	ctx.changes = []Change{{Action: ActionCreateChannel, Object: "channels[rules]"}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" || detail != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_SetAccessHeldRoleDeniesView(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Caller.Roles = []string{"bot", "other"}
	ctx.snap.Channels[0].Access = []Access{{Role: "other", Deny: []string{"VIEW_CHANNEL"}}}
	ctx.changes = []Change{{Action: ActionSetAccess, Object: "channels[rules].access[bot]"}}
	reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"})
	if reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE: the bot cannot view the channel" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_SetAccessGrantsView(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels[0].Access = []Access{{Role: "@everyone", Deny: []string{"VIEW_CHANNEL"}}}
	ctx.defChannel("rules").Access[1].Allow = append(ctx.defChannel("rules").Access[1].Allow, "VIEW_CHANNEL")
	ctx.changes = []Change{{Action: ActionSetAccess, Object: "channels[rules].access[bot]"}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" || detail != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Boundary_SetAccessLiftsEveryoneViewDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.snap.Channels[0].Access = []Access{{Role: "@everyone", Deny: []string{"VIEW_CHANNEL"}}}
	ctx.changes = []Change{{Action: ActionSetAccess, Object: "channels[rules].access[" + everyoneID(ctx.def) + "]"}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" || detail != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_PlannedGrantBeforeInvite(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.InvitesRead = true
	s.Invites = nil
	raiseBotRole(t, s, "ADMINISTRATOR")
	removeChannelRoleAccess(s, "rules", s.Caller.Role)
	res, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	access, create := 0, 0
	for _, c := range res.Changes {
		if c.Action == ActionSetAccess && c.Object == "channels[rules].access[bot]" {
			access = c.Step
		}
		if c.Action == ActionCreateInvite {
			create = c.Step
		}
	}
	if access == 0 || create <= access {
		t.Fatalf("invite must follow bot entry: %+v", res)
	}
}

func TestCheckInviteRight_Positive_ObservedGrant(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.InvitesRead = true
	s.Invites = nil
	result, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range result.Changes {
		if c.Action == ActionCreateInvite {
			return
		}
	}
	t.Fatalf("create missing: %+v", result)
}

func TestCheckInviteRight_Negative_NoGrantPlan(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.InvitesRead = true
	s.Invites = nil
	removeChannelRoleAccess(s, "rules", s.Caller.Role)
	result, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range result.Refused {
		if c.Action == ActionCreateInvite && c.Reason == ReasonLacksRight && c.Detail == "CREATE_INSTANT_INVITE" {
			return
		}
	}
	t.Fatalf("missing right refusal: %+v", result)
}

func TestCheckInviteRight_Positive_AdministratorOverridesMember(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["ADMINISTRATOR"] = struct{}{}
	ctx.snap.Channels[0].MemberEntries = 1
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_MemberEntryOverridesGrant(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels[0].MemberEntries = 1
	ctx.snap.Channels[0].Access = []Access{{Role: ctx.snap.Caller.Role, Allow: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight || !strings.Contains(detail, "single members") {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_ServerWide(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_ServerWideWithDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.snap.Channels[0].Access = []Access{{Role: "@everyone", Deny: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, _ := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight {
		t.Fatalf("reason: %s", reason)
	}
}

func TestCheckInviteRight_Negative_ServerWideWithDefinitionDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.defChannel("rules").Access = append(ctx.defChannel("rules").Access, serverdef.Access{Role: everyoneID(ctx.def), Deny: []string{"CREATE_INSTANT_INVITE"}})
	if reason, _ := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight {
		t.Fatalf("reason: %s", reason)
	}
}

func TestCheckInviteRight_Positive_ServerWideOtherRoleDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Caller.Roles = []string{"bot"}
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.snap.Channels[0].Access = []Access{{Role: "other", Deny: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_ServerWideOtherRoleDefinitionDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.defChannel("rules").Access = append(ctx.defChannel("rules").Access, serverdef.Access{Role: "other", Deny: []string{"CREATE_INSTANT_INVITE"}})
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_ServerWideWithBotRoleDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.snap.Channels[0].Access = []Access{{Role: ctx.snap.Caller.Role, Deny: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, _ := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight {
		t.Fatalf("reason: %s", reason)
	}
}

func TestCheckInviteRight_Positive_CreatedChannel(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels[0].MemberEntries = 1
	ctx.changes = []Change{{Action: ActionCreateChannel, Object: "channels[rules]"}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_PlannedAccess(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.changes = []Change{{Action: ActionSetAccess, Object: "channels[rules].access[bot]"}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_ObservedBotRoleAllow(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels[0].Access = []Access{{Role: ctx.snap.Caller.Role, Allow: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" || detail != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_NoGrant(t *testing.T) {
	ctx := inviteRightContext(t)
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func inviteRightContext(t *testing.T) *planContext {
	t.Helper()
	def := loadDef(t)
	ch := defChannelName(def, "rules")
	return &planContext{def: def, snap: &Snapshot{Caller: Caller{Role: "bot"}, Roles: []Role{{Name: "@everyone", Everyone: true}}, Channels: []Channel{{Name: ch}}}, botRoleID: "bot", rights: heldRights{apart: map[string]struct{}{}, everyone: map[string]struct{}{"VIEW_CHANNEL": {}}}}
}

func TestCheckInviteRight_Negative_ServerWideOtherHeldRoleDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.snap.Caller.Roles = []string{"bot", "other"}
	ctx.snap.Channels[0].Access = []Access{{Role: "other", Deny: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, _ := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight {
		t.Fatalf("reason: %s", reason)
	}
}

func TestCheckInviteRight_Positive_HeldRoleAllowOverEveryoneDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Caller.Roles = []string{"bot", "other"}
	ctx.snap.Channels[0].Access = []Access{{Role: "@everyone", Deny: []string{"CREATE_INSTANT_INVITE"}}, {Role: "other", Allow: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Boundary_HeldRoleAllowOverHeldRoleDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Caller.Roles = []string{"bot", "other"}
	ctx.snap.Channels[0].Access = []Access{{Role: "bot", Deny: []string{"CREATE_INSTANT_INVITE"}}, {Role: "other", Allow: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_EveryoneAllowWithoutServerWide(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels[0].Access = []Access{{Role: "@everyone", Allow: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_UnobservedDefinitionDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels = nil
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.defChannel("rules").Access = append(ctx.defChannel("rules").Access, serverdef.Access{Role: everyoneID(ctx.def), Deny: []string{"CREATE_INSTANT_INVITE"}})
	if reason, _ := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight {
		t.Fatalf("reason: %s", reason)
	}
}

func TestCheckInviteRight_Negative_HeldRoleNameShared(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["VIEW_CHANNEL"] = struct{}{}
	ctx.snap.Roles = append(ctx.snap.Roles, Role{Name: "other"}, Role{Name: "other"})
	ctx.snap.Caller.Roles = []string{"bot", "other"}
	ctx.snap.Channels[0].Access = []Access{{Role: "other", Allow: []string{"CREATE_INSTANT_INVITE"}}}
	reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"})
	if reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE: role name other names several roles" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_NoHeldRolesWithOtherEntry(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels[0].Access = []Access{{Role: "other", Allow: []string{"CREATE_INSTANT_INVITE"}}}
	reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"})
	if reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE: the snapshot names no held roles" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_NoHeldRolesBotEntryOnly(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["VIEW_CHANNEL"] = struct{}{}
	ctx.snap.Channels[0].Access = []Access{{Role: "bot", Allow: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_HeldRoleDeniesView(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["VIEW_CHANNEL"] = struct{}{}
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.snap.Caller.Roles = []string{"bot", "other"}
	ctx.snap.Channels[0].Access = []Access{{Role: "other", Deny: []string{"VIEW_CHANNEL"}}}
	reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"})
	if reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE: the bot cannot view the channel" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Boundary_ViewDeniedByEveryoneAllowedByHeldRole(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.snap.Caller.Roles = []string{"bot", "other"}
	ctx.snap.Channels[0].Access = []Access{{Role: "@everyone", Deny: []string{"VIEW_CHANNEL"}}, {Role: "other", Allow: []string{"VIEW_CHANNEL"}}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" {
		t.Fatalf("refused: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_NewChannelWithDefinitionDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.changes = []Change{{Action: ActionCreateChannel, Object: "channels[rules]"}}
	ctx.defChannel("rules").Access = append(ctx.defChannel("rules").Access, serverdef.Access{Role: everyoneID(ctx.def), Deny: []string{"CREATE_INSTANT_INVITE"}})
	reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"})
	if reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_SetAccessWithDefinitionDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.changes = []Change{{Action: ActionSetAccess, Object: "channels[rules].access[bot]"}}
	ctx.defChannel("rules").Access = append(ctx.defChannel("rules").Access, serverdef.Access{Role: everyoneID(ctx.def), Deny: []string{"CREATE_INSTANT_INVITE"}})
	reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"})
	if reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_NoViewRight(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.everyone = map[string]struct{}{}
	ctx.snap.Channels[0].Access = []Access{{Role: "@everyone", Allow: []string{"CREATE_INSTANT_INVITE"}}}
	reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"})
	if reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE: the bot cannot view the channel" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_ChannelNeitherObservedNorCreated(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels = nil
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonChannelNotCreated || detail != "channels[rules]" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Boundary_ChannelAbsentWithoutRight(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels = nil
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight || detail != "CREATE_INSTANT_INVITE" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Negative_AdministratorChannelNotCreated(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels = nil
	ctx.rights.apart["ADMINISTRATOR"] = struct{}{}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonChannelNotCreated || detail != "channels[rules]" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestCheckInviteRight_Positive_AdministratorChannelCreated(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.snap.Channels = nil
	ctx.rights.apart["ADMINISTRATOR"] = struct{}{}
	ctx.changes = []Change{{Action: ActionCreateChannel, Object: "channels[rules]"}}
	if reason, detail := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != "" || detail != "" {
		t.Fatalf("refusal: %s %s", reason, detail)
	}
}

func TestPlanInvites_Negative_ChannelCreateRefused(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.Channels = slices.DeleteFunc(s.Channels, func(ch Channel) bool { return ch.Name == defChannelName(def, "rules") })
	s.InvitesRead = true
	s.Invites = nil
	raiseBotRole(t, s, "CREATE_INSTANT_INVITE")
	result, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Raised {
		t.Fatal("caller is not raised")
	}
	channelRefused := slices.ContainsFunc(result.Refused, func(r Refusal) bool {
		return r.Action == ActionCreateChannel && r.Object == "channels[rules]" && r.Reason == ReasonLacksRight && strings.Contains(r.Detail, "MANAGE_CHANNELS")
	})
	inviteRefused := slices.ContainsFunc(result.Refused, func(r Refusal) bool {
		return r.Action == ActionCreateInvite && r.Object == "invites[hub]" && r.Reason == ReasonChannelNotCreated && r.Detail == "channels[rules]"
	})
	if slices.ContainsFunc(result.Changes, func(c Change) bool { return c.Action == ActionCreateInvite }) {
		t.Fatalf("unexpected invite change: %+v", result.Changes)
	}
	if !channelRefused || !inviteRefused {
		t.Fatalf("missing channel or invite refusal: %+v", result.Refused)
	}
}
