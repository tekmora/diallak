package serverplan

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// assertMadeByHand checks that res refuses action on roles[operator] as
// made-by-hand with detail, and that no right of the plan comes from it.
func assertMadeByHand(t *testing.T, res Result, action, detail string) {
	t.Helper()
	ref, ok := refusalFor(res, action, "roles[operator]")
	if !ok || ref.Reason != ReasonMadeByHand || ref.Detail != detail || len(ref.Needs) != 0 {
		t.Errorf("%s roles[operator] = %+v (found %v), want %s (%q) needing nothing", action, ref, ok, ReasonMadeByHand, detail)
	}
	if changeFor(res, action, "roles[operator]") {
		t.Errorf("%s roles[operator] planned as a change", action)
	}
	if slices.Contains(res.Rights(), "ADMINISTRATOR") {
		t.Errorf("rights needed = %v, want no ADMINISTRATOR", res.Rights())
	}
}

func TestPlan_Negative_MissingOperatorRoleIsMadeByHand(t *testing.T) {
	def := loadDef(t)
	const detail = "make the role by hand, with ADMINISTRATOR, above the bot's role"

	t.Run("caller without ADMINISTRATOR", func(t *testing.T) {
		s := raisedFirstSetupSnapshot()
		s.Roles = s.Roles[1:]
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		if len(res.Refused) != 1 {
			t.Errorf("refused = %+v, want only the Operator role", res.Refused)
		}
		assertMadeByHand(t, res, ActionCreateRole, detail)

		var out bytes.Buffer
		if err := Render(&out, res); err != nil {
			t.Fatalf("Render failed: %v", err)
		}
		want := "  - create-role roles[operator]: made-by-hand; " + detail + "\n"
		if !strings.Contains(out.String(), want) {
			t.Errorf("rendered plan lacks %q:\n%s", want, out.String())
		}
	})

	t.Run("caller with ADMINISTRATOR", func(t *testing.T) {
		s := raisedFirstSetupSnapshot()
		s.Roles = s.Roles[1:]
		s.Caller.Permissions = append(s.Caller.Permissions, "ADMINISTRATOR")
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		assertMadeByHand(t, res, ActionCreateRole, detail)
	})
}

func TestPlan_Positive_OperatorRoleInPlacePlansAsBefore(t *testing.T) {
	def := loadDef(t)
	res, err := Plan(def, raisedFirstSetupSnapshot())
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) != 0 {
		t.Errorf("refused = %+v, want none", res.Refused)
	}
	for _, c := range res.Changes {
		if c.Object == "roles[operator]" {
			t.Errorf("step %d %s roles[operator], want no change to the Operator role", c.Step, c.Action)
		}
	}
}

func TestPlan_Boundary_OperatorRoleWithoutItsRightIsMadeByHand(t *testing.T) {
	def := loadDef(t)
	s := raisedFirstSetupSnapshot()
	roleNamed(t, s, "Operator").Permissions = []string{}
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	assertMadeByHand(t, res, ActionSetRolePermissions, "give the role its rights by hand, ADMINISTRATOR among them")
}
