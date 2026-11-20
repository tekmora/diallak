package serverplan

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

func TestInviteSnapshot_Positive_RoundTrip(t *testing.T) {
	s := validBaseSnapshot()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	data = append([]byte(`{"invites_read":true,"invites":[{"code":"abc","channel":"general","max_age":0,"max_uses":0,"temporary":false,"by_bot":true}],`), data[1:]...)
	var got Snapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == string(mustJSON(t, s)) {
		t.Fatal("snapshot discarded invites")
	}
}
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInviteSnapshotCheck_Positive_Valid(t *testing.T) {
	s := validBaseSnapshot()
	s.InvitesRead = true
	s.Invites = []Invite{{Code: "abc", Channel: "general", ByBot: true}}
	if err := Check(&s); err != nil {
		t.Fatal(err)
	}
}

func TestInviteSnapshotCheck_Negative_Invalid(t *testing.T) {
	good := Invite{Code: "abc", Channel: "general", ByBot: true}
	for _, tc := range []struct {
		name   string
		read   bool
		invite Invite
		want   bool
	}{
		{"not read", false, good, true},
		{"empty code", true, Invite{Channel: "general"}, true},
		{"empty channel", true, Invite{Code: "abc", ByBot: true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := validBaseSnapshot()
			s.InvitesRead = tc.read
			s.Invites = []Invite{tc.invite}
			if got := Check(&s) != nil; got != tc.want {
				t.Errorf("check error = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInviteCompare_Negative_Missing(t *testing.T) {
	def := loadDef(t)
	snap := snapshotOf(t, def)
	snap.InvitesRead = true
	snap.Invites = nil
	diffs, err := Compare(def, snap)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range diffs {
		if d.Object == "invites[hub]" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing invite is not reported: %+v", diffs)
	}
}

func TestInviteComparisonCases_Positive_Match(t *testing.T) {
	def := loadDef(t)
	diffs := compareInvites(def, &Snapshot{InvitesRead: true, Invites: []Invite{{Code: "abc", Channel: "rules", ByBot: true}}})
	if len(diffs) != 0 {
		t.Fatalf("unexpected differences: %+v", diffs)
	}
}

func TestInviteComparisonCases_Negative_Differences(t *testing.T) {
	def := loadDef(t)
	want := def.Invites[0]
	good := Invite{Code: "abc", Channel: "rules", ByBot: true}
	cases := []struct {
		name     string
		read     bool
		code     string
		invites  []Invite
		wantKind string
	}{
		{"unread", false, "", nil, KindInvitesNotRead},
		{"other ignored", true, "", []Invite{{Code: "other", Channel: "rules"}}, KindInviteCreate},
		{"missing recorded", true, "published", nil, KindInviteMissing},
		{"recorded other ignored", true, "published", []Invite{{Code: "published", Channel: "rules"}}, KindInviteMissing},
		{"different age", true, "abc", []Invite{{Code: "abc", Channel: "rules", MaxAge: 5, ByBot: true}}, KindInviteDiffers},
		{"extra bot", true, "", []Invite{good, {Code: "extra", Channel: "elsewhere", ByBot: true}}, KindInviteExtra},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def.Invites[0] = want
			def.Invites[0].Code = tc.code
			s := &Snapshot{InvitesRead: tc.read, Invites: tc.invites}
			diffs := compareInvites(def, s)
			if tc.wantKind == "" && len(diffs) != 0 {
				t.Fatalf("unexpected differences: %+v", diffs)
			}
			if tc.wantKind != "" && !slices.ContainsFunc(diffs, func(d Difference) bool { return d.Kind == tc.wantKind }) {
				t.Fatalf("missing %s: %+v", tc.wantKind, diffs)
			}
		})
	}
}

func TestInvitePlan_Positive_Create(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.InvitesRead = true
	s.Invites = nil
	res, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range res.Changes {
		if c.Action == ActionCreateInvite && c.Object == "invites[hub]" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing create step: %+v", res)
	}
}

func TestInvitePlan_Negative_RecordedReplacement(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.InvitesRead = true
	s.Invites = nil
	def.Invites[0].Code = "published"
	res, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Changes {
		if c.Action == ActionCreateInvite {
			t.Fatalf("recorded code planned for replacement: %+v", c)
		}
	}
}

func TestInvitePlan_Positive_NewChannelAndEmptyRead(t *testing.T) {
	def := loadDef(t)
	s := raisedFirstSetupSnapshot()
	s.Invites = nil
	result, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	channel, invite := 0, 0
	for _, change := range result.Changes {
		if change.Action == ActionCreateChannel && change.Object == "channels[rules]" {
			channel = change.Step
		}
		if change.Action == ActionCreateInvite && change.Object == "invites[hub]" {
			invite = change.Step
		}
	}
	if channel == 0 || invite <= channel || len(result.Refused) != 0 {
		t.Fatalf("expected channel then invite without refusals: %+v", result)
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

func TestInvitePlan_Negative_RecordedDifference(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.InvitesRead = true
	def.Invites[0].Code = "published"
	s.Invites = []Invite{{Code: "published", Channel: "rules", MaxAge: 1, ByBot: true}}
	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(diffs, func(d Difference) bool { return d.Kind == KindInviteDiffers && d.Detail == "max_age" }) {
		t.Fatalf("missing age difference: %+v", diffs)
	}
	res, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(res.Changes, func(c Change) bool { return c.Action == ActionCreateInvite }) {
		t.Fatalf("recorded invite planned: %+v", res.Changes)
	}
}

func TestCompareInvites_Positive_Unread(t *testing.T) {
	def := loadDef(t)
	got := compareInvites(def, &Snapshot{})
	if len(got) != 1 || got[0].Kind != KindInvitesNotRead || got[0].Object != "invites" {
		t.Fatalf("differences = %+v", got)
	}
}

func TestCompareInvites_Boundary_UnreadWithoutEntries(t *testing.T) {
	def := loadDef(t)
	def.Invites = nil
	if got := compareInvites(def, &Snapshot{}); len(got) != 0 {
		t.Fatalf("differences = %+v", got)
	}
}

func TestPlanInvites_Negative_UnreadRecordedCode(t *testing.T) {
	def := loadDef(t)
	def.Invites[0].Code = "published"
	s := snapshotOf(t, def)
	s.InvitesRead = false
	s.Invites = nil
	result, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range result.Changes {
		if c.Action == ActionCreateInvite {
			t.Fatalf("unexpected change: %+v", c)
		}
	}
	for _, c := range result.Refused {
		if c.Action == ActionCreateInvite {
			t.Fatalf("unexpected candidate: %+v", c)
		}
	}
}

func TestPlanInvites_Positive_UnreadCandidate(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.InvitesRead = false
	s.Invites = nil
	result, err := Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range result.Refused {
		if c.Action == ActionCreateInvite && c.Object == "invites[hub]" {
			return
		}
	}
	t.Fatalf("unread candidate missing: %+v", result)
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

func TestCompareInvites_Negative_OneToOne(t *testing.T) {
	def := loadDef(t)
	first := def.Invites[0]
	second := first
	second.ID = "second"
	good := Invite{Code: "abc", Channel: "rules", ByBot: true}
	cases := []struct {
		name         string
		entries      []serverdef.Invite
		invites      []Invite
		kind, object string
	}{
		{"extra equal", []serverdef.Invite{first}, []Invite{good, {Code: "other", Channel: "rules", ByBot: true}}, KindInviteExtra, "server.invites[other]"},
		{"only one", []serverdef.Invite{first, second}, []Invite{good}, KindInviteCreate, "invites[second]"},
		{"recorded first", []serverdef.Invite{first, second}, []Invite{good}, KindInviteCreate, "invites[hub]"},
	}
	cases[2].entries[1].Code = "abc"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def.Invites = tc.entries
			diffs := compareInvites(def, &Snapshot{InvitesRead: true, Invites: tc.invites})
			if len(diffs) != 1 || diffs[0].Kind != tc.kind || diffs[0].Object != tc.object {
				t.Fatalf("differences: %+v", diffs)
			}
		})
	}
}

func TestCheckInvites_Negative_BotWithoutChannel(t *testing.T) {
	if len(checkInvites(&Snapshot{InvitesRead: true, Invites: []Invite{{Code: "abc", ByBot: true}}})) == 0 {
		t.Fatal("bot invite without channel accepted")
	}
}
func TestCheckInvites_Positive_OtherWithoutChannel(t *testing.T) {
	if errs := checkInvites(&Snapshot{InvitesRead: true, Invites: []Invite{{Code: "abc"}}}); len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
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
	ctx.snap.Channels[0].Access = []Access{{Role: "other", Deny: []string{"CREATE_INSTANT_INVITE"}}}
	if reason, _ := ctx.checkInviteRight(candidate{channelID: "rules"}); reason != ReasonLacksRight {
		t.Fatalf("reason: %s", reason)
	}
}
func TestCheckInviteRight_Negative_ServerWideWithDefinitionDeny(t *testing.T) {
	ctx := inviteRightContext(t)
	ctx.rights.apart["CREATE_INSTANT_INVITE"] = struct{}{}
	ctx.defChannel("rules").Access = append(ctx.defChannel("rules").Access, serverdef.Access{Role: "other", Deny: []string{"CREATE_INSTANT_INVITE"}})
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
	return &planContext{def: def, snap: &Snapshot{Caller: Caller{Role: "bot"}, Channels: []Channel{{Name: ch}}}, botRoleID: "bot", rights: heldRights{apart: map[string]struct{}{}, everyone: map[string]struct{}{}}}
}
