package serverplan

import (
	"fmt"
	"slices"
	"testing"
)

// everyoneHoldsVoiceAndPrivateThreads returns the first set-up snapshot with
// CONNECT and CREATE_PRIVATE_THREADS on @everyone, as a new server gives them,
// and on the bot's role only when botHoldsThem.
func everyoneHoldsVoiceAndPrivateThreads(botHoldsThem bool) *Snapshot {
	s := raisedFirstSetupSnapshot()
	for i := range s.Roles {
		if s.Roles[i].Name == "Diallak" && !botHoldsThem {
			s.Roles[i].Permissions = []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "MANAGE_CHANNELS"}
		}
		if s.Roles[i].Everyone {
			s.Roles[i].Permissions = append(s.Roles[i].Permissions, "CONNECT", "CREATE_PRIVATE_THREADS")
		}
	}
	return s
}

func everyoneStep(t *testing.T, res Result) int {
	t.Helper()
	for _, c := range res.Changes {
		if c.Action == ActionSetRolePermissions && c.Object == "roles[everyone]" {
			return c.Step
		}
	}
	t.Fatalf("plan holds no %s roles[everyone]: %+v", ActionSetRolePermissions, res.Changes)
	return 0
}

func TestPlan_Negative_RightHeldOnlyThroughEveryoneIsGoneAfterItsStep(t *testing.T) {
	def := loadDef(t)
	s := everyoneHoldsVoiceAndPrivateThreads(false)

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) == 0 {
		t.Fatalf("expected the steps that need CONNECT or CREATE_PRIVATE_THREADS to be refused, got %d changes and no refusal", len(res.Changes))
	}

	step := everyoneStep(t, res)
	want := map[string]string{
		"channels[tickets]":      fmt.Sprintf("CREATE_PRIVATE_THREADS; step %d takes CREATE_PRIVATE_THREADS from @everyone", step),
		"channels[events-voice]": fmt.Sprintf("CONNECT; step %d takes CONNECT from @everyone", step),
	}
	for _, ref := range res.Refused {
		detail, ok := want[ref.Object]
		if !ok || ref.Action != ActionCreateChannel || ref.Reason != ReasonLacksRight || ref.Detail != detail {
			t.Errorf("refused %s %s: %s (%q), want %s %s: %s (%q)",
				ref.Action, ref.Object, ref.Reason, ref.Detail, ActionCreateChannel, ref.Object, ReasonLacksRight, detail)
		}
		delete(want, ref.Object)
	}
	for object := range want {
		t.Errorf("%s not refused", object)
	}
}

func TestPlan_Positive_BotRoleHoldingTheRightsKeepsThemAfterTheEveryoneStep(t *testing.T) {
	def := loadDef(t)
	s := everyoneHoldsVoiceAndPrivateThreads(true)

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) != 0 {
		t.Fatalf("refused %+v, want none: the bot's role holds every right itself", res.Refused)
	}
	step := everyoneStep(t, res)
	for _, object := range []string{"channels[tickets]", "channels[events-voice]"} {
		idx := slices.IndexFunc(res.Changes, func(c Change) bool { return c.Object == object })
		if idx < 0 || res.Changes[idx].Step <= step {
			t.Errorf("%s not planned after step %d: %+v", object, step, res.Changes)
		}
	}
}

// A right the @everyone step adds is not held before that step: the step
// itself needs it and is refused, so no later step counts on it either.
func TestPlan_Boundary_RightTheEveryoneStepAddsDoesNotCountBeforeIt(t *testing.T) {
	def := loadDef(t)
	s := raisedFirstSetupSnapshot()
	without := func(perms []string) []string {
		return slices.DeleteFunc(slices.Clone(perms), func(p string) bool { return p == "SPEAK" })
	}
	s.Caller.Permissions = without(s.Caller.Permissions)
	for i := range s.Roles {
		s.Roles[i].Permissions = without(s.Roles[i].Permissions)
	}

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	idx := slices.IndexFunc(res.Refused, func(r Refusal) bool { return r.Object == "roles[everyone]" })
	if idx < 0 || res.Refused[idx].Reason != ReasonLacksRight || res.Refused[idx].Detail != "SPEAK" {
		t.Fatalf("refused %+v, want %s roles[everyone] refused as %s (\"SPEAK\")", res.Refused, ActionSetRolePermissions, ReasonLacksRight)
	}
	for _, c := range res.Changes {
		if slices.Contains(c.Needs, "SPEAK") {
			t.Errorf("step %d %s %s needs SPEAK, which the caller never holds", c.Step, c.Action, c.Object)
		}
	}
}
