package serverplan

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool {
	return &b
}

func raiseBot(s *Snapshot) {
	for i := range s.Roles {
		if s.Roles[i].Name == s.Caller.Role {
			s.Roles[i].Permissions = append(s.Roles[i].Permissions, "ADMINISTRATOR")
			break
		}
	}
	s.Caller.Permissions = append(s.Caller.Permissions, "ADMINISTRATOR")
}

func TestCompare_ManagedRoles_MissingHasDetail(t *testing.T) {
	def := loadDef(t)
	// Base snapshotOf currently does not have the 3 managed roles, or we construct one without them.
	s := snapshotOf(t, def)
	// Remove any managed roles
	var filtered []Role
	for _, r := range s.Roles {
		if r.Name != "Server Booster" && r.Name != "Ko-fi Bot" && r.Name != "Patreon" {
			filtered = append(filtered, r)
		}
	}
	s.Roles = filtered

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	managedMissing := make(map[string]Difference)
	for _, d := range diffs {
		if d.Kind == KindRoleMissing {
			managedMissing[d.Object] = d
		}
	}

	for _, id := range []string{"booster", "funding-ko-fi", "funding-patreon"} {
		obj := "roles[" + id + "]"
		d, ok := managedMissing[obj]
		if !ok {
			t.Errorf("missing diff for %s", obj)
			continue
		}
		wantDetail := "managed; the plan does not create it"
		if d.Detail != wantDetail {
			t.Errorf("%s detail = %q, want %q", obj, d.Detail, wantDetail)
		}
	}
}

func TestCompare_ManagedRoles_NotManagedKind(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	roleNamed(t, s, "Ko-fi Bot").Managed = false

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	var found bool
	for _, d := range diffs {
		if d.Kind == KindRoleNotManaged && d.Object == "roles[funding-ko-fi]" {
			found = true
			if d.Detail != "" {
				t.Errorf("expected empty detail, got %q", d.Detail)
			}
		}
	}
	if !found {
		t.Errorf("expected KindRoleNotManaged for roles[funding-ko-fi], got diffs: %+v", diffs)
	}
}

func TestCompare_ManagedRoles_RolePlaceIntegrationAboveModerator(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	// Move Ko-fi Bot to index 0 (above Operator and Moderator)
	var kofi Role
	var withoutKofi []Role
	for _, r := range s.Roles {
		if r.Name == "Ko-fi Bot" {
			kofi = r
		} else {
			withoutKofi = append(withoutKofi, r)
		}
	}
	s.Roles = append([]Role{kofi}, withoutKofi...)

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	var found bool
	for _, d := range diffs {
		if d.Kind == KindRolePlace && d.Object == "roles[funding-ko-fi]" {
			found = true
			wantDetail := "above roles[moderator]"
			if d.Detail != wantDetail {
				t.Errorf("detail = %q, want %q", d.Detail, wantDetail)
			}
		}
	}
	if !found {
		t.Errorf("expected KindRolePlace for roles[funding-ko-fi], got diffs: %+v", diffs)
	}
}

func TestCompare_ManagedRoles_PlatformRoleAboveModeratorNoPlaceDiff(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	// Move Booster (platform) above moderator; should NOT produce role-place
	var booster Role
	var withoutBooster []Role
	for _, r := range s.Roles {
		if r.Name == "Server Booster" {
			booster = r
		} else {
			withoutBooster = append(withoutBooster, r)
		}
	}
	s.Roles = append([]Role{booster}, withoutBooster...)

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	for _, d := range diffs {
		if d.Kind == KindRolePlace && d.Object == "roles[booster]" {
			t.Errorf("unexpected KindRolePlace for booster: %+v", d)
		}
	}
}

func TestCompare_ManagedRoles_IntegrationAboveModeratorNoModeratorOnServer(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	// Remove moderator from server and move Ko-fi Bot to top
	var kofi Role
	var withoutModOrKofi []Role
	for _, r := range s.Roles {
		if r.Name == "Ko-fi Bot" {
			kofi = r
		} else if r.Name != "Moderator" {
			withoutModOrKofi = append(withoutModOrKofi, r)
		}
	}
	s.Roles = append([]Role{kofi}, withoutModOrKofi...)

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	for _, d := range diffs {
		if d.Kind == KindRolePlace {
			t.Errorf("unexpected KindRolePlace when moderator is not on server: %+v", d)
		}
	}
}

func TestCompare_ManagedRoles_NeverPermissionsDiff(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	// Add extra permissions to Ko-fi Bot
	roleNamed(t, s, "Ko-fi Bot").Permissions = []string{"ADMINISTRATOR", "MANAGE_GUILD"}

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	for _, d := range diffs {
		if d.Kind == KindRolePermissions && d.Object == "roles[funding-ko-fi]" {
			t.Errorf("managed role should never produce role-permissions diff: %+v", d)
		}
	}
}

func TestCompare_RoleDisplay(t *testing.T) {
	def := loadDef(t)

	t.Run("shown_apart differs", func(t *testing.T) {
		s := snapshotOf(t, def)
		// helper definition has shown_apart: true. Server has shown_apart: false
		for i := range s.Roles {
			if s.Roles[i].Name == "Helper" {
				s.Roles[i].ShownApart = false
			}
		}
		diffs, err := Compare(def, s)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		var found bool
		for _, d := range diffs {
			if d.Kind == KindRoleDisplay && d.Object == "roles[helper]" {
				found = true
				want := "shown apart: server no; definition yes"
				if d.Detail != want {
					t.Errorf("detail = %q, want %q", d.Detail, want)
				}
			}
		}
		if !found {
			t.Errorf("expected KindRoleDisplay for roles[helper], got: %+v", diffs)
		}
	})

	t.Run("mentionable differs", func(t *testing.T) {
		s := snapshotOf(t, def)
		// notify-releases definition has mentionable: true. Server has mentionable: false
		for i := range s.Roles {
			if s.Roles[i].Name == "Notify: releases" {
				s.Roles[i].Mentionable = false
			}
		}
		diffs, err := Compare(def, s)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		var found bool
		for _, d := range diffs {
			if d.Kind == KindRoleDisplay && d.Object == "roles[notify-releases]" {
				found = true
				want := "mentionable: server no; definition yes"
				if d.Detail != want {
					t.Errorf("detail = %q, want %q", d.Detail, want)
				}
			}
		}
		if !found {
			t.Errorf("expected KindRoleDisplay for roles[notify-releases], got: %+v", diffs)
		}
	})

	t.Run("a field the definition does not state is not compared", func(t *testing.T) {
		s := snapshotOf(t, def)
		// helper states shown_apart only, notify-releases states mentionable
		// only, contributor states neither.
		roleNamed(t, s, "Helper").Mentionable = true
		roleNamed(t, s, "Notify: releases").ShownApart = true
		contributor := roleNamed(t, s, "Contributor")
		contributor.ShownApart, contributor.Mentionable = true, true
		diffs, err := Compare(def, s)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		for _, d := range diffs {
			if d.Kind == KindRoleDisplay {
				t.Errorf("%s %s (%s) for a field the definition does not state", d.Kind, d.Object, d.Detail)
			}
		}
	})

	t.Run("a stated no against a server yes", func(t *testing.T) {
		defCopy := deepCopy(t, def)
		for i := range defCopy.Roles {
			if defCopy.Roles[i].ID == "helper" {
				defCopy.Roles[i].ShownApart = boolPtr(false)
				defCopy.Roles[i].Mentionable = boolPtr(false)
			}
		}
		s := snapshotOf(t, defCopy)
		helper := roleNamed(t, s, "Helper")
		helper.ShownApart, helper.Mentionable = true, true
		diffs, err := Compare(defCopy, s)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		var details []string
		for _, d := range diffs {
			if d.Kind == KindRoleDisplay && d.Object == "roles[helper]" {
				details = append(details, d.Detail)
			}
		}
		want := []string{"shown apart: server yes; definition no; mentionable: server yes; definition no"}
		if !slices.Equal(details, want) {
			t.Errorf("role-display of roles[helper] = %q, want %q", details, want)
		}
	})

	t.Run("both differ joined by semicolon", func(t *testing.T) {
		defCopy := deepCopy(t, def)
		for i := range defCopy.Roles {
			if defCopy.Roles[i].ID == "helper" {
				defCopy.Roles[i].ShownApart = boolPtr(true)
				defCopy.Roles[i].Mentionable = boolPtr(true)
			}
		}
		s := snapshotOf(t, defCopy)
		for i := range s.Roles {
			if s.Roles[i].Name == "Helper" {
				s.Roles[i].ShownApart = false
				s.Roles[i].Mentionable = false
			}
		}
		diffs, err := Compare(defCopy, s)
		if err != nil {
			t.Fatalf("Compare failed: %v", err)
		}
		var found bool
		for _, d := range diffs {
			if d.Kind == KindRoleDisplay && d.Object == "roles[helper]" {
				found = true
				want := "shown apart: server no; definition yes; mentionable: server no; definition yes"
				if d.Detail != want {
					t.Errorf("detail = %q, want %q", d.Detail, want)
				}
			}
		}
		if !found {
			t.Errorf("expected KindRoleDisplay for roles[helper], got: %+v", diffs)
		}
	})
}

func TestPlan_SetRoleDisplay_CandidateAndRefusals(t *testing.T) {
	def := loadDef(t)

	t.Run("raised bot plans set-role-display", func(t *testing.T) {
		s := snapshotOf(t, def)
		raiseBot(s)
		for i := range s.Roles {
			if s.Roles[i].Name == "Notify: releases" {
				s.Roles[i].Mentionable = false
			}
		}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		var found bool
		for _, c := range res.Changes {
			if c.Action == ActionSetRoleDisplay && c.Object == "roles[notify-releases]" {
				found = true
				if !slices.Equal(c.Needs, []string{"MANAGE_ROLES"}) {
					t.Errorf("needs = %v, want [MANAGE_ROLES]", c.Needs)
				}
				if c.Detail != "mentionable: server no; definition yes" {
					t.Errorf("detail = %q", c.Detail)
				}
			}
		}
		if !found {
			t.Errorf("expected set-role-display change, got: %+v", res.Changes)
		}
	})

	t.Run("managed def role booster with display diff yields NO candidate or refusal", func(t *testing.T) {
		s := snapshotOf(t, def)
		raiseBot(s)
		for i := range s.Roles {
			if s.Roles[i].Name == "Server Booster" {
				s.Roles[i].ShownApart = false
			}
		}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		for _, c := range res.Changes {
			if c.Object == "roles[booster]" {
				t.Errorf("unexpected change for booster: %+v", c)
			}
		}
		for _, ref := range res.Refused {
			if ref.Object == "roles[booster]" {
				t.Errorf("unexpected refusal for booster: %+v", ref)
			}
		}
	})

	t.Run("own mode refused outside-own-changes", func(t *testing.T) {
		s := snapshotOf(t, def)
		for i := range s.Roles {
			if s.Roles[i].Name == "Helper" {
				s.Roles[i].ShownApart = false
			}
		}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		var found bool
		for _, ref := range res.Refused {
			if ref.Action == ActionSetRoleDisplay && ref.Object == "roles[helper]" {
				found = true
				if ref.Reason != ReasonOutsideOwnChanges {
					t.Errorf("reason = %q, want %q", ref.Reason, ReasonOutsideOwnChanges)
				}
			}
		}
		if !found {
			t.Errorf("expected set-role-display refusal, got: %+v", res.Refused)
		}
	})

	t.Run("above caller refused above-caller", func(t *testing.T) {
		s := snapshotOf(t, def)
		raiseBot(s)
		// Move Diallak below Helper
		var diallak Role
		var otherRoles []Role
		for _, r := range s.Roles {
			if r.Name == s.Caller.Role {
				diallak = r
			} else {
				otherRoles = append(otherRoles, r)
			}
		}
		for i := range otherRoles {
			if otherRoles[i].Name == "Helper" {
				otherRoles[i].ShownApart = false
			}
		}
		// diallak at bottom before everyone
		s.Roles = append(otherRoles[:len(otherRoles)-1], diallak, otherRoles[len(otherRoles)-1])

		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		var found bool
		for _, ref := range res.Refused {
			if ref.Action == ActionSetRoleDisplay && ref.Object == "roles[helper]" {
				found = true
				if ref.Reason != ReasonAboveCaller {
					t.Errorf("reason = %q, want %q", ref.Reason, ReasonAboveCaller)
				}
			}
		}
		if !found {
			t.Errorf("expected above-caller refusal, got: %+v", res.Refused)
		}
	})

	t.Run("server role managed=true refused managed-role", func(t *testing.T) {
		s := snapshotOf(t, def)
		raiseBot(s)
		for i := range s.Roles {
			if s.Roles[i].Name == "Helper" {
				s.Roles[i].ShownApart = false
				s.Roles[i].Managed = true
			}
		}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		var found bool
		for _, ref := range res.Refused {
			if ref.Action == ActionSetRoleDisplay && ref.Object == "roles[helper]" {
				found = true
				if ref.Reason != ReasonManagedRole {
					t.Errorf("reason = %q, want %q", ref.Reason, ReasonManagedRole)
				}
			}
		}
		if !found {
			t.Errorf("expected managed-role refusal, got: %+v", res.Refused)
		}
	})

	t.Run("caller lacks MANAGE_ROLES refused lacks-right", func(t *testing.T) {
		s := snapshotOf(t, def)
		// Raise bot with MANAGE_CHANNELS (not ADMINISTRATOR) so raised is true, but lacks MANAGE_ROLES
		for i := range s.Roles {
			if s.Roles[i].Name == s.Caller.Role {
				s.Roles[i].Permissions = append(s.Roles[i].Permissions, "MANAGE_CHANNELS")
				break
			}
		}
		var callerPerms []string
		for _, p := range s.Caller.Permissions {
			if p != "MANAGE_ROLES" {
				callerPerms = append(callerPerms, p)
			}
		}
		callerPerms = append(callerPerms, "MANAGE_CHANNELS")
		s.Caller.Permissions = callerPerms

		for i := range s.Roles {
			if s.Roles[i].Name == "Notify: releases" {
				s.Roles[i].Mentionable = false
			}
		}
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		var found bool
		for _, ref := range res.Refused {
			if ref.Action == ActionSetRoleDisplay && ref.Object == "roles[notify-releases]" {
				found = true
				if ref.Reason != ReasonLacksRight || !strings.Contains(ref.Detail, "MANAGE_ROLES") {
					t.Errorf("reason = %q, detail = %q", ref.Reason, ref.Detail)
				}
			}
		}
		if !found {
			t.Errorf("expected lacks-right refusal, got: %+v", res.Refused)
		}
	})
}

func TestPlan_ManagedRoles_MissingDoesNotAlterPlan(t *testing.T) {
	def := loadDef(t)
	sWith := snapshotOf(t, def)
	sWithout := snapshotOf(t, def)
	// Remove all 3 managed roles from sWithout
	var filtered []Role
	for _, r := range sWithout.Roles {
		if r.Name != "Server Booster" && r.Name != "Ko-fi Bot" && r.Name != "Patreon" {
			filtered = append(filtered, r)
		}
	}
	sWithout.Roles = filtered

	resWith, err := Plan(def, sWith)
	if err != nil {
		t.Fatalf("Plan with: %v", err)
	}
	resWithout, err := Plan(def, sWithout)
	if err != nil {
		t.Fatalf("Plan without: %v", err)
	}

	if len(resWith.Changes) != len(resWithout.Changes) {
		t.Errorf("changes len mismatch: with=%d without=%d", len(resWith.Changes), len(resWithout.Changes))
	}
	if len(resWith.Refused) != len(resWithout.Refused) {
		t.Errorf("refused len mismatch: with=%d without=%d", len(resWith.Refused), len(resWithout.Refused))
	}
}

func TestRender_SetRoleDisplay(t *testing.T) {
	res := Result{
		Raised: true,
		Changes: []Change{
			{
				Step:   1,
				Action: ActionSetRoleDisplay,
				Object: "roles[helper]",
				Needs:  []string{"MANAGE_ROLES"},
				Detail: "shown apart: server no; definition yes",
			},
		},
	}
	var buf bytes.Buffer
	if err := Render(&buf, res); err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "  1. set-role-display roles[helper] needs MANAGE_ROLES; shown apart: server no; definition yes\n"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("Render output = %q, want it to contain %q", buf.String(), want)
	}
}

func TestCompare_ManagedRoles_AtAnyPlace_NoRoleOrderDiff(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	// Move the 3 managed roles to various positions (e.g. at the top before Operator)
	var managedRoles []Role
	var otherRoles []Role
	for _, r := range s.Roles {
		if r.Name == "Server Booster" || r.Name == "Ko-fi Bot" || r.Name == "Patreon" {
			managedRoles = append(managedRoles, r)
		} else {
			otherRoles = append(otherRoles, r)
		}
	}
	// Place managed roles at the top of non-everyone roles
	s.Roles = append(managedRoles, otherRoles...)

	roDiff := compareRoleOrder(def, s)
	if roDiff != nil {
		t.Errorf("expected no role-order diff with managed roles at top, got: %+v", roDiff)
	}

	// Also verify plan set-role-order detail is unaffected and never holds a managed role
	raiseBot(s)
	// Swap two unmanaged roles to trigger a set-role-order candidate
	cIdx, sIdx := -1, -1
	for i := range s.Roles {
		if s.Roles[i].Name == "Contributor" {
			cIdx = i
		}
		if s.Roles[i].Name == "Supporter" {
			sIdx = i
		}
	}
	s.Roles[cIdx], s.Roles[sIdx] = s.Roles[sIdx], s.Roles[cIdx]

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, c := range res.Changes {
		if c.Action == ActionSetRoleOrder {
			for _, managedID := range []string{"booster", "funding-ko-fi", "funding-patreon"} {
				if strings.Contains(c.Detail, managedID) {
					t.Errorf("set-role-order detail %q should not contain managed role %q", c.Detail, managedID)
				}
			}
		}
	}
}
