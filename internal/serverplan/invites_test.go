package serverplan

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestInviteSnapshot(t *testing.T) {
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

func TestInviteSnapshotCheck(t *testing.T) {
	good := Invite{Code: "abc", Channel: "general", ByBot: true}
	for _, tc := range []struct {
		name   string
		read   bool
		invite Invite
		want   bool
	}{
		{"valid", true, good, false},
		{"not read", false, good, true},
		{"empty code", true, Invite{Channel: "general"}, true},
		{"empty channel", true, Invite{Code: "abc"}, true},
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

func TestInviteCompareMissing(t *testing.T) {
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

func TestInviteComparisonCases(t *testing.T) {
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
		{"match", true, "", []Invite{good}, ""},
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

func TestInvitePlan(t *testing.T) {
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
	def.Invites[0].Code = "published"
	res, err = Plan(def, s)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Changes {
		if c.Action == ActionCreateInvite {
			t.Fatalf("recorded code planned for replacement: %+v", c)
		}
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

func TestCheckInviteRight_Negative_NoGrant(t *testing.T) {
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
