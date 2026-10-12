package discord

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverplan"
)

const managedRoleNote = "managed; the plan does not create it"

// auditReasonOf is the audit reason of a run as the library puts it in the
// header.
func auditReasonOf(digest string) string {
	return strings.ReplaceAll(url.QueryEscape("diallak apply "+digest), "+", " ")
}

// firstSetUp runs a first set-up up to the apply and leaves the bot's role
// raised, as the server stands before the operator's last act.
func (fx setupFixture) firstSetUp(t *testing.T) {
	t.Helper()
	rights, _ := fx.ownPlanRights(t)
	fx.setBotRights(t, rights)
	raised, in := fx.plan(t, setupDay)
	if len(raised.Refused) != 0 {
		t.Fatalf("plan of the first set-up refuses %+v, want no refusal", raised.Refused)
	}
	if _, err := fx.apply(t, setupDay, in); err != nil {
		t.Fatalf("apply of the first set-up: %v", err)
	}
}

// differences reads the server and compares it, as -server=compare does, and
// returns each difference as "kind object".
func (fx setupFixture) differences(t *testing.T) (lines []string, diffs []serverplan.Difference) {
	t.Helper()
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(orderBotUserID), orderGuildID, fx.options(t, setupDay))
	if err != nil {
		t.Fatalf("read the server: %v", err)
	}
	diffs, err = serverplan.Compare(fx.def, snap)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	for _, d := range diffs {
		lines = append(lines, d.Kind+" "+d.Object)
	}
	slices.Sort(lines)
	return lines, diffs
}

// setDisplay changes how a role is shown, as a person does in the settings.
func (fx setupFixture) setDisplay(t *testing.T, name string, hoist, mentionable bool) {
	t.Helper()
	fx.fake.mu.Lock()
	defer fx.fake.mu.Unlock()
	i := slices.IndexFunc(fx.fake.roles, func(r fakeRole) bool { return r.name == name })
	if i < 0 {
		t.Fatalf("role %q is not on the server", name)
	}
	fx.fake.roles[i].hoist, fx.fake.roles[i].mentionable = hoist, mentionable
}

func (fx setupFixture) display(t *testing.T, name string) (hoist, mentionable bool) {
	t.Helper()
	fx.fake.mu.Lock()
	defer fx.fake.mu.Unlock()
	i := slices.IndexFunc(fx.fake.roles, func(r fakeRole) bool { return r.name == name })
	if i < 0 {
		t.Fatalf("role %q is not on the server", name)
	}
	return fx.fake.roles[i].hoist, fx.fake.roles[i].mentionable
}

// dragToBottom is a hand act of the operator: the named roles move below
// every other role, the first name lowest, and no other role changes its
// place against another.
func (fx setupFixture) dragToBottom(t *testing.T, names ...string) {
	t.Helper()
	fx.fake.dragInClient(t, func(shown []fakeRole) []fakeRole {
		rest := slices.DeleteFunc(slices.Clone(shown), func(r fakeRole) bool { return slices.Contains(names, r.name) })
		for i := len(names) - 1; i >= 0; i-- {
			at := slices.IndexFunc(shown, func(r fakeRole) bool { return r.name == names[i] })
			if at < 0 {
				t.Fatalf("role %q is not on the server", names[i])
			}
			rest = append(rest, shown[at])
		}
		return rest
	})
}

func managedRoleLines(fx setupFixture, kind string) []string {
	var lines []string
	for i := range fx.def.Roles {
		if fx.def.Roles[i].Kind == "managed" {
			lines = append(lines, kind+" roles["+fx.def.Roles[i].ID+"]")
		}
	}
	slices.Sort(lines)
	return lines
}

// A new server holds no role of another party: no member boosts it and no
// funding integration is connected. The first set-up runs without them and
// compare names each one afterwards.
func TestApplyServer_Boundary_FirstSetUpWithoutTheManagedRoles(t *testing.T) {
	fx := newSetupFixture(t)
	fx.firstSetUp(t)
	fx.lowerBotRole(t)

	lines, diffs := fx.differences(t)
	want := managedRoleLines(fx, serverplan.KindRoleMissing)
	if len(want) != 3 {
		t.Fatalf("the definition holds %d managed roles, the test is written for 3", len(want))
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("differences after the set-up = %v, want %v", lines, want)
	}
	for _, d := range diffs {
		if d.Detail != managedRoleNote {
			t.Errorf("%s %s has the detail %q, want %q", d.Kind, d.Object, d.Detail, managedRoleNote)
		}
	}

	result, _ := fx.plan(t, setupDay)
	if len(result.Changes) != 0 || len(result.Refused) != 0 {
		t.Errorf("plan for the missing managed roles: %d changes, %d refused; want neither", len(result.Changes), len(result.Refused))
	}
}

// The fake puts a new role at the bottom, so the roles the server held before
// stand above every role the run made. The run leaves a managed role where it
// is: the two funding roles end above the Moderator role until the operator
// drags them down.
func TestApplyServer_Boundary_FirstSetUpLeavesFundingRolesWhereTheyStand(t *testing.T) {
	fx := newSetupFixture(t)
	addManagedRoles(fx.fake)
	fx.firstSetUp(t)
	fx.lowerBotRole(t)

	lines, _ := fx.differences(t)
	want := []string{
		serverplan.KindRolePlace + " roles[funding-ko-fi]",
		serverplan.KindRolePlace + " roles[funding-patreon]",
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("differences after the set-up = %v, want %v", lines, want)
	}
	result, _ := fx.plan(t, setupDay)
	if len(result.Changes) != 0 || len(result.Refused) != 0 {
		t.Errorf("plan for the place of a managed role: %d changes, %d refused; want neither", len(result.Changes), len(result.Refused))
	}

	fx.dragToBottom(t, "Ko-fi Bot", "Patreon")
	if lines, _ = fx.differences(t); len(lines) != 0 {
		t.Errorf("differences after the operator dragged the funding roles down = %v, want none", lines)
	}
}

// A display someone changed by hand goes back through plan and apply while the
// bot's role is raised. A field the definition does not state stays as it is.
func TestApplyServer_Positive_DisplaySetBackByPlanAndApply(t *testing.T) {
	fx := newSetupFixture(t)
	fx.firstSetUp(t)
	fx.setDisplay(t, "Helper", false, false)
	fx.setDisplay(t, "Notify: releases", false, false)
	fx.setDisplay(t, "Contributor", true, true)

	result, in := fx.plan(t, setupDay)
	if len(result.Refused) != 0 {
		t.Fatalf("refused = %+v, want no refusal", result.Refused)
	}
	var planned []string
	for _, c := range result.Changes {
		planned = append(planned, c.Action+" "+c.Object)
	}
	want := []string{
		serverplan.ActionSetRoleDisplay + " roles[helper]",
		serverplan.ActionSetRoleDisplay + " roles[notify-releases]",
	}
	if !slices.Equal(planned, want) {
		t.Fatalf("changes = %v, want %v", planned, want)
	}

	fx.fake.resetRecorded()
	if _, err := fx.apply(t, setupDay, in); err != nil {
		t.Fatalf("apply: %v", err)
	}
	writes := 0
	for _, call := range fx.fake.getRecorded() {
		if call.method == http.MethodGet {
			continue
		}
		writes++
		if call.method != http.MethodPatch || call.auditReason != auditReasonOf(in.PlanDigest) {
			t.Errorf("%s %s carries the audit reason %q, want a PATCH with %q",
				call.method, call.path, call.auditReason, auditReasonOf(in.PlanDigest))
		}
	}
	if writes != len(want) {
		t.Errorf("%d writes, want %d", writes, len(want))
	}

	if hoist, mentionable := fx.display(t, "Helper"); !hoist || mentionable {
		t.Errorf("Helper: shown apart %t, mentionable %t; want yes and no", hoist, mentionable)
	}
	if hoist, mentionable := fx.display(t, "Notify: releases"); hoist || !mentionable {
		t.Errorf("Notify: releases: shown apart %t, mentionable %t; want no and yes", hoist, mentionable)
	}
	if hoist, mentionable := fx.display(t, "Contributor"); !hoist || !mentionable {
		t.Errorf("Contributor: shown apart %t, mentionable %t; the definition states neither, want both left on", hoist, mentionable)
	}
	lines, _ := fx.differences(t)
	for _, line := range lines {
		if strings.HasPrefix(line, serverplan.KindRoleDisplay+" ") {
			t.Errorf("compare after the apply still holds %s", line)
		}
	}
}

// With its own rights the bot changes no display: the plan refuses the change
// and names it.
func TestApplyServer_Negative_DisplayIsNoOwnChange(t *testing.T) {
	fx := newSetupFixture(t)
	fx.firstSetUp(t)
	fx.lowerBotRole(t)
	fx.setDisplay(t, "Notify: releases", false, false)

	result, _ := fx.plan(t, setupDay)
	if len(result.Changes) != 0 || len(result.Refused) != 1 {
		t.Fatalf("%d changes, refused %+v; want no change and one refusal", len(result.Changes), result.Refused)
	}
	r := result.Refused[0]
	if r.Action != serverplan.ActionSetRoleDisplay || r.Object != "roles[notify-releases]" || r.Reason != serverplan.ReasonOutsideOwnChanges {
		t.Errorf("refused %s %s as %q, want %s roles[notify-releases] as %q",
			r.Action, r.Object, r.Reason, serverplan.ActionSetRoleDisplay, serverplan.ReasonOutsideOwnChanges)
	}
}
