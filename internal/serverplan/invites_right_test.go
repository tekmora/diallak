package serverplan

import (
	"slices"
	"testing"
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
