package discord

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
)

// setupDay is a day before channelListCutoff: the list of channels is still
// the whole list for a reader without administrator rights.
const setupDay = "2026-10-11T12:00:00Z"

// setupFixture is a fresh server on which the bot's role holds its own three
// rights, and on which no other role gives the bot a right.
type setupFixture struct {
	fake *fakePlatform
	url  string
	def  *serverdef.Definition
}

func newSetupFixture(t *testing.T) setupFixture {
	t.Helper()
	fake := setupFreshServerFake(t, orderGuildID, orderBotUserID, orderBotRoleID, false)
	fake.createShifts = true
	fake.roles[0].permissions = "0"
	srv := fake.server()
	t.Cleanup(srv.Close)
	return setupFixture{fake: fake, url: srv.URL, def: mustDefinition(t)}
}

func (fx setupFixture) options(t *testing.T, day string) clientOptions {
	t.Helper()
	at, err := time.Parse(time.RFC3339, day)
	if err != nil {
		t.Fatalf("parse %q: %v", day, err)
	}
	return clientOptions{restURL: fx.url, now: func() time.Time { return at }}
}

// setBotRights gives the bot's role exactly the named rights, as the operator
// does by hand before and after a first set-up.
func (fx setupFixture) setBotRights(t *testing.T, rights []string) {
	t.Helper()
	mask, err := serverdef.PermissionMask(rights)
	if err != nil {
		t.Fatalf("PermissionMask(%v): %v", rights, err)
	}
	fx.fake.mu.Lock()
	defer fx.fake.mu.Unlock()
	for i := range fx.fake.roles {
		if fx.fake.roles[i].id == snowflake.ID(orderBotRoleID) {
			fx.fake.roles[i].permissions = strconv.FormatUint(mask, 10)
		}
	}
}

// plan reads the server on the given day and plans, as -server=plan does.
func (fx setupFixture) plan(t *testing.T, day string) (serverplan.Result, ApplyInput) {
	t.Helper()
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(orderBotUserID), orderGuildID, fx.options(t, day))
	if err != nil {
		t.Fatalf("read the server: %v", err)
	}
	result, err := serverplan.Plan(fx.def, snap)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return result, gateInput(t, fx.def, result)
}

func (fx setupFixture) apply(t *testing.T, day string, in ApplyInput) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := applyServerWithOptions(context.Background(), makeFakeToken(orderBotUserID), orderGuildID, in, &out, fx.options(t, day))
	return out.String(), err
}

// ownPlanRights is the first step of a first set-up: the plan made with the
// bot's own rights changes nothing and names the rights the set-up needs.
func (fx setupFixture) ownPlanRights(t *testing.T) (rights []string, refused []serverplan.Refusal) {
	t.Helper()
	own, _ := fx.plan(t, setupDay)
	if own.Raised || len(own.Changes) != 0 || len(own.Refused) == 0 {
		t.Fatalf("plan with the bot's own rights: raised %t, %d changes, %d refused; want not raised, 0 changes, refusals",
			own.Raised, len(own.Changes), len(own.Refused))
	}
	for _, r := range own.Refused {
		if r.Reason != serverplan.ReasonOutsideOwnChanges {
			t.Errorf("%s %s refused as %q, want %q", r.Action, r.Object, r.Reason, serverplan.ReasonOutsideOwnChanges)
		}
	}
	rights = own.Rights()
	if len(rights) == 0 || slices.Contains(rights, "ADMINISTRATOR") {
		t.Fatalf("rights needed = %v, want a list without ADMINISTRATOR", rights)
	}
	return rights, own.Refused
}

// roleAboveBot is the name of the role the definition puts directly above the
// bot's role.
func roleAboveBot(t *testing.T, def *serverdef.Definition) string {
	t.Helper()
	at := slices.Index(def.RoleOrder, "bot")
	if at < 1 {
		t.Fatalf("the bot's role is at %d of the definition's order, want a role above it", at)
	}
	for i := range def.Roles {
		if def.Roles[i].ID == def.RoleOrder[at-1] {
			return def.Roles[i].Name
		}
	}
	t.Fatalf("role %q of the order is no role of the definition", def.RoleOrder[at-1])
	return ""
}

// lowerBotRole is the operator's last act: the bot's role gets the rights the
// definition gives it and moves down to its place in the definition's order.
// No other role changes its place against another.
func (fx setupFixture) lowerBotRole(t *testing.T) {
	t.Helper()
	for i := range fx.def.Roles {
		if fx.def.Roles[i].Kind == "bot" {
			fx.setBotRights(t, fx.def.Roles[i].Permissions)
		}
	}
	above := roleAboveBot(t, fx.def)
	fx.fake.mu.Lock()
	defer fx.fake.mu.Unlock()
	roles := fx.fake.roles
	bot := slices.IndexFunc(roles, func(r fakeRole) bool { return r.id == snowflake.ID(orderBotRoleID) })
	target := slices.IndexFunc(roles, func(r fakeRole) bool { return r.name == above })
	if bot < 0 || target < 0 || roles[target].position >= roles[bot].position {
		t.Fatalf("bot role at %d, %s at %d; want both on the server and the bot's role above", bot, above, target)
	}
	from, to := roles[target].position, roles[bot].position
	for i := range roles {
		if roles[i].position >= from && roles[i].position < to {
			roles[i].position++
		}
	}
	roles[bot].position = from
}

// assertRefusalsBecameChanges holds the second plan of a first set-up to the
// first: everything refused before is a change now, and the one change on top
// is the order of the roles, which is planned only when roles are created.
func assertRefusalsBecameChanges(t *testing.T, refused []serverplan.Refusal, changes []serverplan.Change) {
	t.Helper()
	planned := make(map[string]bool, len(changes))
	for _, c := range changes {
		planned[c.Action+" "+c.Object] = true
	}
	for _, r := range refused {
		if !planned[r.Action+" "+r.Object] {
			t.Errorf("%s %s was refused in the first plan and is no change in the second", r.Action, r.Object)
		}
		delete(planned, r.Action+" "+r.Object)
	}
	if len(planned) != 1 || !planned[serverplan.ActionSetRoleOrder+" role_order"] {
		t.Errorf("changes the first plan did not name = %v, want only %s role_order", planned, serverplan.ActionSetRoleOrder)
	}
}

// visibleChannels counts the channels the definition does not hide from the
// bot. A hidden one, as the ticket channel, is never planned from a list of
// channels that is not the whole list.
func visibleChannels(def *serverdef.Definition) int {
	visible := 0
	for i := range def.Channels {
		hidden := false
		for _, a := range def.Channels[i].Access {
			hidden = hidden || (a.Role == "bot" && slices.Contains(a.Deny, "VIEW_CHANNEL"))
		}
		if !hidden {
			visible++
		}
	}
	return visible
}

func TestApplyServer_Positive_FirstSetUpWithTheRightsThePlanNames(t *testing.T) {
	fx := newSetupFixture(t)
	rights, refused := fx.ownPlanRights(t)

	fx.setBotRights(t, rights)
	raised, in := fx.plan(t, setupDay)
	if !raised.Raised || len(raised.Refused) != 0 {
		t.Fatalf("plan with the named rights: raised %t, refused %+v; want raised and no refusal", raised.Raised, raised.Refused)
	}
	assertRefusalsBecameChanges(t, refused, raised.Changes)
	if got := raised.Rights(); !slices.Equal(got, rights) {
		t.Errorf("rights needed with the raised role = %v, want the list of the first plan %v", got, rights)
	}

	out, err := fx.apply(t, setupDay, in)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if last := fmt.Sprintf("applied: %d of %d\n", len(raised.Changes), len(raised.Changes)); !strings.HasSuffix(out, last) {
		t.Errorf("output ends %q, want %q", out[max(0, len(out)-40):], last)
	}

	fx.lowerBotRole(t)
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(orderBotUserID), orderGuildID, fx.options(t, setupDay))
	if err != nil {
		t.Fatalf("read the server after the set-up: %v", err)
	}
	if serverplan.Raised(fx.def, snap) {
		t.Error("the bot's role still counts as raised after the operator lowered it")
	}
	diffs, err := serverplan.Compare(fx.def, snap)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("compare after the set-up holds %d differences, want 0: %+v", len(diffs), diffs)
	}
}

func TestApplyServer_Boundary_EveryNamedRightIsNeeded(t *testing.T) {
	fx := newSetupFixture(t)
	rights, _ := fx.ownPlanRights(t)

	for _, without := range rights {
		t.Run(without, func(t *testing.T) {
			fx.setBotRights(t, slices.DeleteFunc(slices.Clone(rights), func(r string) bool { return r == without }))
			result, in := fx.plan(t, setupDay)
			if len(result.Refused) == 0 {
				t.Fatalf("no refusal without %s: the list names a right the set-up does not need", without)
			}
			for _, r := range result.Refused {
				if r.Reason != serverplan.ReasonLacksRight || !slices.Contains(strings.Split(r.Detail, ", "), without) {
					t.Errorf("%s %s refused as %q (%q), want %q naming %s",
						r.Action, r.Object, r.Reason, r.Detail, serverplan.ReasonLacksRight, without)
				}
			}

			fx.fake.resetRecorded()
			out, err := fx.apply(t, setupDay, in)
			want := fmt.Sprintf("the plan holds %d refused changes; a plan with a refusal is not applied", len(result.Refused))
			if err == nil || err.Error() != want {
				t.Errorf("apply error = %v, want %q", err, want)
			}
			if out != "" {
				t.Errorf("output = %q, want none", out)
			}
			assertRequestCounts(t, fx.fake.getRecorded(), 4, 0)
		})
	}
}

func TestApplyServer_Boundary_FirstSetUpAndTheChannelListDate(t *testing.T) {
	cutoff, err := time.Parse(time.RFC3339, channelListCutoff)
	if err != nil {
		t.Fatalf("parse the cutoff: %v", err)
	}
	cases := []struct {
		name          string
		day           time.Time
		administrator bool
		wantRefused   bool
	}{
		{"last second before the date", cutoff.Add(-time.Second), false, false},
		{"on the date", cutoff, false, true},
		{"on the date with administrator", cutoff, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSetupFixture(t)
			rights, _ := fx.ownPlanRights(t)
			if tc.administrator {
				rights = append(rights, "ADMINISTRATOR")
			}
			fx.setBotRights(t, rights)

			result, _ := fx.plan(t, tc.day.Format(time.RFC3339))
			if got := len(result.Refused) > 0; got != tc.wantRefused {
				t.Fatalf("refused = %+v, want refusals: %t", result.Refused, tc.wantRefused)
			}
			for _, r := range result.Refused {
				if r.Action != serverplan.ActionCreateChannel || r.Reason != serverplan.ReasonNotVisible {
					t.Errorf("%s %s refused as %q, want only %s refused as %q",
						r.Action, r.Object, r.Reason, serverplan.ActionCreateChannel, serverplan.ReasonNotVisible)
				}
			}
			for _, c := range result.Changes {
				if tc.wantRefused && c.Action == serverplan.ActionCreateChannel {
					t.Errorf("%s is planned from a list of channels that is not the whole list", c.Object)
				}
			}
			if want := visibleChannels(fx.def); tc.wantRefused && (len(result.Refused) != want || want == len(fx.def.Channels)) {
				t.Errorf("%d channels refused, want the %d of %d the definition lets the bot see",
					len(result.Refused), want, len(fx.def.Channels))
			}
		})
	}
}
