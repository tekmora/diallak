package serverplan

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func roleNamed(t *testing.T, s *Snapshot, name string) *Role {
	t.Helper()
	for i := range s.Roles {
		if s.Roles[i].Name == name {
			return &s.Roles[i]
		}
	}
	t.Fatalf("role %q not in snapshot", name)
	return nil
}

func entryOf(t *testing.T, s *Snapshot, channel, role string) *Access {
	t.Helper()
	for i := range s.Channels {
		if s.Channels[i].Name != channel {
			continue
		}
		for j := range s.Channels[i].Access {
			if s.Channels[i].Access[j].Role == role {
				return &s.Channels[i].Access[j]
			}
		}
	}
	t.Fatalf("channel %q has no entry for role %q", channel, role)
	return nil
}

func unknownBitDiffs(diffs []Difference) []Difference {
	var res []Difference
	for _, d := range diffs {
		if d.Kind == KindUnknownBits {
			res = append(res, d)
		}
	}
	return res
}

func TestCheck_Negative_UnknownBits(t *testing.T) {
	tests := []struct {
		name    string
		bits    []int
		wantErr string
	}{
		{"a bit the table names", []int{46}, `unknown bit 46 in role "Bot" is the permission "SEND_VOICE_MESSAGES"`},
		{"the lowest bit the table names", []int{0}, `unknown bit 0 in role "Bot" is the permission "CREATE_INSTANT_INVITE"`},
		{"below the value", []int{-1}, `unknown bit -1 in role "Bot" is outside 0 to 63`},
		{"above the value", []int{64}, `unknown bit 64 in role "Bot" is outside 0 to 63`},
		{"falling order", []int{53, 47}, `unknown bit 47 in role "Bot" is not in rising order`},
		{"twice", []int{47, 47}, `unknown bit 47 in role "Bot" is not in rising order`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := validBaseSnapshot()
			s.Roles[0].UnknownBits = tc.bits
			err := Check(&s)
			if err == nil {
				t.Fatalf("Check accepted unknown bits %v, want an error", tc.bits)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to hold %q", err.Error(), tc.wantErr)
			}
		})
	}

	t.Run("entry sides are checked", func(t *testing.T) {
		s := validBaseSnapshot()
		s.Channels[0].Access[0].UnknownAllow = []int{46}
		s.Channels[0].Access[0].UnknownDeny = []int{64}
		err := Check(&s)
		if err == nil {
			t.Fatal("Check accepted wrong unknown bits on an entry, want an error")
		}
		for _, want := range []string{
			`unknown bit 46 in channel "general" access[@everyone].allow is the permission "SEND_VOICE_MESSAGES"`,
			`unknown bit 64 in channel "general" access[@everyone].deny is outside 0 to 63`,
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to hold %q", err.Error(), want)
			}
		}
	})
}

func TestCheck_Boundary_UnknownBitsAreTheBitsWithoutAName(t *testing.T) {
	// 64 bits, 52 of them named by the table: 12 numbers are accepted, 47 and
	// the bits from 53 on.
	want := []int{47, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63}
	var accepted []int
	for b := 0; b < 64; b++ {
		s := validBaseSnapshot()
		s.Roles[1].UnknownBits = []int{b}
		if err := Check(&s); err == nil {
			accepted = append(accepted, b)
		}
	}
	if len(accepted) != len(want) {
		t.Fatalf("accepted bits = %v, want %v", accepted, want)
	}
	for i := range want {
		if accepted[i] != want[i] {
			t.Fatalf("accepted bits = %v, want %v", accepted, want)
		}
	}

	s := validBaseSnapshot()
	s.Roles[1].UnknownBits = want
	if err := Check(&s); err != nil {
		t.Errorf("Check refused every unknown bit in rising order: %v", err)
	}
}

func TestLoadSnapshot_Positive_UnknownBitsRoundTrip(t *testing.T) {
	s := validBaseSnapshot()
	plain, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(plain), "unknown_") {
		t.Errorf("a snapshot without unknown bits names the fields: %s", plain)
	}

	s.Roles[1].UnknownBits = []int{47}
	s.Channels[0].Access[0].UnknownAllow = []int{47, 53}
	s.Channels[0].Access[0].UnknownDeny = []int{60}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"unknown_bits":[47]`, `"unknown_allow":[47,53]`, `"unknown_deny":[60]`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("snapshot %s lacks %s", data, want)
		}
	}
	got, err := LoadSnapshot(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if len(got.Roles[1].UnknownBits) != 1 || got.Roles[1].UnknownBits[0] != 47 {
		t.Errorf("role unknown bits = %v, want [47]", got.Roles[1].UnknownBits)
	}
	acc := got.Channels[0].Access[0]
	if len(acc.UnknownAllow) != 2 || acc.UnknownAllow[1] != 53 || len(acc.UnknownDeny) != 1 || acc.UnknownDeny[0] != 60 {
		t.Errorf("entry unknown bits = %v / %v, want [47 53] / [60]", acc.UnknownAllow, acc.UnknownDeny)
	}
}

func TestLoadSnapshot_Negative_UnknownBitWithAName(t *testing.T) {
	s := validBaseSnapshot()
	s.Roles[1].UnknownBits = []int{48}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := LoadSnapshot(bytes.NewReader(data)); err == nil {
		t.Fatal("LoadSnapshot accepted bit 48 as unknown, want an error")
	}
}

func TestCompare_Negative_UnknownBitsEveryCarrier(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	// A role an integration manages, above every other role, as the first server read found it.
	s.Roles = slices.DeleteFunc(s.Roles, func(r Role) bool { return r.Name == "Server Booster" })
	booster := Role{Name: "Server Booster", Managed: true, ShownApart: true, Permissions: []string{}, UnknownBits: []int{47}}
	s.Roles = append([]Role{booster}, s.Roles...)
	roleNamed(t, s, "Moderator").UnknownBits = []int{47, 53}
	roleNamed(t, s, "@everyone").UnknownBits = []int{47}
	entryOf(t, s, "rules", "@everyone").UnknownAllow = []int{47}
	entryOf(t, s, "home-lab", "Moderator").UnknownAllow = []int{47}
	entryOf(t, s, "home-lab", "Moderator").UnknownDeny = []int{53}
	s.Channels = append(s.Channels, Channel{
		Name: "old-room", Type: "text",
		Access: []Access{{Role: "@everyone", Allow: []string{}, Deny: []string{}, UnknownDeny: []int{47}}},
	})

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	want := []Difference{
		{Kind: KindUnknownBits, Object: "server.roles[Server Booster]", Detail: "bits 47"},
		{Kind: KindUnknownBits, Object: "server.roles[Moderator]", Detail: "bits 47, 53"},
		{Kind: KindUnknownBits, Object: "server.roles[@everyone]", Detail: "bits 47"},
		{Kind: KindUnknownBits, Object: "server.channels[rules].access[@everyone]", Detail: "allow 47"},
		{Kind: KindUnknownBits, Object: "server.channels[home-lab].access[Moderator]", Detail: "allow 47; deny 53"},
		{Kind: KindUnknownBits, Object: "server.channels[old-room].access[@everyone]", Detail: "deny 47"},
	}
	got := unknownBitDiffs(diffs)
	if len(got) != len(want) {
		t.Fatalf("unknown-bit differences = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("difference %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// The named permissions did not change, so nothing else is reported for
	// the roles and entries of the definition.
	for _, d := range diffs {
		switch d.Kind {
		case KindUnknownBits, KindRoleExtra, KindChannelExtra:
		default:
			t.Errorf("unexpected difference %+v", d)
		}
	}
}

func TestCompare_Boundary_NoUnknownBitsNoDifference(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	// Empty lists are no finding.
	roleNamed(t, s, "Moderator").UnknownBits = []int{}
	entryOf(t, s, "rules", "@everyone").UnknownAllow = []int{}
	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("differences = %+v, want none", diffs)
	}
}

func TestPlan_Negative_UnknownBitsRefuseTheWrite(t *testing.T) {
	def := loadDef(t)
	tests := []struct {
		name       string
		mutate     func(s *Snapshot)
		plant      func(s *Snapshot)
		wantAction string
		wantObject string
		wantDetail string
	}{
		{
			name:       "entry that differs",
			mutate:     func(s *Snapshot) { entryOf(t, s, "rules", "@everyone").Deny = []string{"SEND_MESSAGES"} },
			plant:      func(s *Snapshot) { entryOf(t, s, "rules", "@everyone").UnknownAllow = []int{47} },
			wantAction: ActionSetAccess, wantObject: "channels[rules].access[everyone]", wantDetail: "allow 47",
		},
		{
			// With no named permission the entry counts as absent, and the
			// change would still write over it.
			name: "entry that holds unknown bits only",
			mutate: func(s *Snapshot) {
				acc := entryOf(t, s, "home-lab", "Moderator")
				acc.Allow, acc.Deny = []string{}, []string{}
			},
			plant:      func(s *Snapshot) { entryOf(t, s, "home-lab", "Moderator").UnknownDeny = []int{47} },
			wantAction: ActionSetAccess, wantObject: "channels[home-lab].access[moderator]", wantDetail: "deny 47",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Without the bit the change is planned.
			control := raisedSnapshotOf(t, def)
			tc.mutate(control)
			res, err := Plan(def, control)
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			if !changeFor(res, tc.wantAction, tc.wantObject) {
				t.Fatalf("control: change (%s, %s) not planned: %+v", tc.wantAction, tc.wantObject, res)
			}

			s := raisedSnapshotOf(t, def)
			tc.mutate(s)
			tc.plant(s)
			res, err = Plan(def, s)
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			if changeFor(res, tc.wantAction, tc.wantObject) {
				t.Fatalf("change (%s, %s) planned on an object with unknown bits", tc.wantAction, tc.wantObject)
			}
			ref, ok := refusalFor(res, tc.wantAction, tc.wantObject)
			if !ok {
				t.Fatalf("no refusal for (%s, %s): %+v", tc.wantAction, tc.wantObject, res.Refused)
			}
			if ref.Reason != ReasonUnknownBits || ref.Detail != tc.wantDetail {
				t.Errorf("refusal = %s; %s, want %s; %s", ref.Reason, ref.Detail, ReasonUnknownBits, tc.wantDetail)
			}
		})
	}
}

func TestPlan_Positive_UnlistedBitsOnAChangedRoleAreClearedAndNamed(t *testing.T) {
	def := loadDef(t)
	tests := []struct {
		name, roleName, wantObject, wantDetail string
		perms                                  []string
		bits                                   []int
	}{
		{"supporter", "Supporter", "roles[supporter]", "clears unlisted bits 47", []string{"VIEW_AUDIT_LOG"}, []int{47}},
		{"everyone", "@everyone", "roles[everyone]", "clears unlisted bits 47, 53", []string{"VIEW_CHANNEL"}, []int{47, 53}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			control := raisedSnapshotOf(t, def)
			roleNamed(t, control, tc.roleName).Permissions = tc.perms
			resControl, err := Plan(def, control)
			if err != nil {
				t.Fatalf("Plan control failed: %v", err)
			}
			var controlChanges []Change
			for _, c := range resControl.Changes {
				if c.Action == ActionSetRolePermissions && c.Object == tc.wantObject {
					controlChanges = append(controlChanges, c)
				}
			}
			if len(controlChanges) != 1 || controlChanges[0].Detail != "" {
				t.Fatalf("control: want 1 change for %s with empty Detail: %+v", tc.wantObject, controlChanges)
			}

			s := raisedSnapshotOf(t, def)
			r := roleNamed(t, s, tc.roleName)
			r.Permissions = tc.perms
			r.UnknownBits = tc.bits
			res, err := Plan(def, s)
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			var changes []Change
			for _, c := range res.Changes {
				if c.Action == ActionSetRolePermissions && c.Object == tc.wantObject {
					changes = append(changes, c)
				}
			}
			if len(changes) != 1 || changes[0].Detail != tc.wantDetail {
				t.Fatalf("want 1 change for %s with detail %q, got: %+v", tc.wantObject, tc.wantDetail, changes)
			}
			for _, ref := range res.Refused {
				if ref.Object == tc.wantObject {
					t.Errorf("res.Refused holds entry for %s: %+v", tc.wantObject, ref)
				}
			}
		})
	}
}

func TestPlan_Negative_ManagedRoleWithUnlistedBitsIsNeverWritten(t *testing.T) {
	def := loadDef(t)
	s := raisedSnapshotOf(t, def)
	r := roleNamed(t, s, "Supporter")
	r.Managed = true
	r.Permissions = []string{"VIEW_AUDIT_LOG"}
	r.UnknownBits = []int{47}

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if changeFor(res, ActionSetRolePermissions, "roles[supporter]") {
		t.Fatal("change planned for managed role with unlisted bits, want none")
	}
	ref, ok := refusalFor(res, ActionSetRolePermissions, "roles[supporter]")
	if !ok || ref.Reason != ReasonManagedRole {
		t.Fatalf("refusal = %+v (found %v), want reason %s", ref, ok, ReasonManagedRole)
	}
}

func TestPlan_Boundary_UnknownBitsLeaveTheRestAlone(t *testing.T) {
	def := loadDef(t)

	t.Run("role as defined: no change, no refusal", func(t *testing.T) {
		s := raisedSnapshotOf(t, def)
		roleNamed(t, s, "Supporter").UnknownBits = []int{47}
		roleNamed(t, s, "@everyone").UnknownBits = []int{47}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		for _, ref := range res.Refused {
			if ref.Reason == ReasonUnknownBits {
				t.Errorf("refusal without a change to make: %+v", ref)
			}
		}
		if changeFor(res, ActionSetRolePermissions, "roles[supporter]") {
			t.Error("change planned for a role whose named permissions are as defined")
		}
		if changeFor(res, ActionSetRolePermissions, "roles[everyone]") {
			t.Error("change planned for everyone role whose named permissions are as defined")
		}
		if _, ok := refusalFor(res, ActionSetRolePermissions, "roles[supporter]"); ok {
			t.Error("refusal for roles[supporter] whose named permissions are as defined")
		}
		if _, ok := refusalFor(res, ActionSetRolePermissions, "roles[everyone]"); ok {
			t.Error("refusal for roles[everyone] whose named permissions are as defined")
		}
	})

	t.Run("bits on the role do not block its entry", func(t *testing.T) {
		s := raisedSnapshotOf(t, def)
		roleNamed(t, s, "Moderator").UnknownBits = []int{47}
		entryOf(t, s, "home-lab", "Moderator").Allow = []string{}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		if !changeFor(res, ActionSetAccess, "channels[home-lab].access[moderator]") {
			t.Errorf("entry change not planned: %+v", res)
		}
	})

	t.Run("bits on one entry do not block its neighbour", func(t *testing.T) {
		s := raisedSnapshotOf(t, def)
		entryOf(t, s, "home-lab", "@everyone").UnknownDeny = []int{47}
		entryOf(t, s, "home-lab", "Moderator").Allow = []string{}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		if !changeFor(res, ActionSetAccess, "channels[home-lab].access[moderator]") {
			t.Errorf("neighbour's change not planned: %+v", res)
		}
		if _, ok := refusalFor(res, ActionSetAccess, "channels[home-lab].access[everyone]"); ok {
			t.Error("refusal for an entry whose named permissions are as defined")
		}
	})

	// An unnamed bit lifts no other refusal of a role-permission change.
	t.Run("a role above the caller stays refused", func(t *testing.T) {
		s := raisedSnapshotOf(t, def)
		r := roleNamed(t, s, "Operator")
		r.Permissions, r.UnknownBits = []string{}, []int{47}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		if changeFor(res, ActionSetRolePermissions, "roles[operator]") {
			t.Error("change planned for a role above the caller")
		}
		ref, ok := refusalFor(res, ActionSetRolePermissions, "roles[operator]")
		if !ok || ref.Reason != ReasonAboveCaller {
			t.Errorf("refusal = %+v (found %v), want reason %s", ref, ok, ReasonAboveCaller)
		}
	})

	t.Run("a bot role that is not raised stays refused", func(t *testing.T) {
		s := snapshotOf(t, def)
		r := roleNamed(t, s, "Supporter")
		r.Permissions, r.UnknownBits = []string{"VIEW_AUDIT_LOG"}, []int{47}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		if changeFor(res, ActionSetRolePermissions, "roles[supporter]") {
			t.Error("change planned outside the bot's own changes")
		}
		ref, ok := refusalFor(res, ActionSetRolePermissions, "roles[supporter]")
		if !ok || ref.Reason != ReasonOutsideOwnChanges {
			t.Errorf("refusal = %+v (found %v), want reason %s", ref, ok, ReasonOutsideOwnChanges)
		}
	})
}

func TestRender_Positive_UnknownBitsInPlanAndDigest(t *testing.T) {
	def := loadDef(t)
	plain := raisedSnapshotOf(t, def)
	roleNamed(t, plain, "Supporter").Permissions = []string{"VIEW_AUDIT_LOG"}
	planted := raisedSnapshotOf(t, def)
	roleNamed(t, planted, "Supporter").Permissions = []string{"VIEW_AUDIT_LOG"}
	roleNamed(t, planted, "Supporter").UnknownBits = []int{47}

	resPlain, err := Plan(def, plain)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	resPlanted, err := Plan(def, planted)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	var bufPlanted bytes.Buffer
	if err := Render(&bufPlanted, resPlanted); err != nil {
		t.Fatalf("Render planted failed: %v", err)
	}
	var bufPlain bytes.Buffer
	if err := Render(&bufPlain, resPlain); err != nil {
		t.Fatalf("Render plain failed: %v", err)
	}

	plantedLines := strings.Split(bufPlanted.String(), "\n")
	var plantedLine string
	for _, l := range plantedLines {
		if strings.Contains(l, ". set-role-permissions roles[supporter] needs ") {
			plantedLine = l
			break
		}
	}
	if plantedLine == "" {
		t.Fatalf("planted plan text lacks line with '. set-role-permissions roles[supporter] needs ':\n%s", bufPlanted.String())
	}
	const wantEnding = "; clears unlisted bits 47"
	if !strings.HasSuffix(plantedLine, wantEnding) {
		t.Errorf("planted line = %q, want it to end with %q", plantedLine, wantEnding)
	}
	plainLine := strings.TrimSuffix(plantedLine, wantEnding)
	if !strings.Contains(bufPlain.String(), plainLine+"\n") {
		t.Errorf("plain plan text lacks %q:\n%s", plainLine, bufPlain.String())
	}

	// A plan agreed on before the bit appeared does not name the same digest.
	dPlain, err := Digest(resPlain, "sha256:definition")
	if err != nil {
		t.Fatalf("Digest failed: %v", err)
	}
	dPlanted, err := Digest(resPlanted, "sha256:definition")
	if err != nil {
		t.Fatalf("Digest failed: %v", err)
	}
	if dPlain == dPlanted {
		t.Error("the digest is the same with and without the unknown bit")
	}

	diffs, err := Compare(def, planted)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	bufPlanted.Reset()
	if err := RenderDifferences(&bufPlanted, true, diffs); err != nil {
		t.Fatalf("RenderDifferences failed: %v", err)
	}
	if want := "  unknown-permission-bits server.roles[Supporter]; bits 47\n"; !strings.Contains(bufPlanted.String(), want) {
		t.Errorf("difference text lacks %q:\n%s", want, bufPlanted.String())
	}
}
