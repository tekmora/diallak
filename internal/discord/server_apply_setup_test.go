package discord

import (
	"bytes"
	"cmp"
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
	fx.fake.dragInClient(t, func(shown []fakeRole) []fakeRole {
		bot := slices.IndexFunc(shown, func(r fakeRole) bool { return r.id == snowflake.ID(orderBotRoleID) })
		target := slices.IndexFunc(shown, func(r fakeRole) bool { return r.name == above })
		if bot < 0 || target < 0 || target < bot {
			t.Fatalf("bot role at %d, %s at %d; want both on the server and the bot's role above", bot, above, target)
		}
		botRole := shown[bot]
		shown = slices.Delete(shown, bot, bot+1)
		return slices.Insert(shown, target, botRole)
	})
}

// dragInClient is a hand act of the operator in the client: the roles stand
// in the order arrange returns, top first, and take the positions from their
// count down to 1. Whether the client numbers without gaps is not measured;
// the tests rest on the order only.
func (f *fakePlatform) dragInClient(t *testing.T, arrange func(shown []fakeRole) []fakeRole) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	everyone := slices.IndexFunc(f.roles, func(r fakeRole) bool { return r.id == snowflake.ID(f.guildID) })
	if everyone < 0 {
		t.Fatal("the everyone role is not on the server")
	}
	everyoneRole := f.roles[everyone]
	shown := slices.Delete(slices.Clone(f.roles), everyone, everyone+1)
	slices.SortFunc(shown, func(a, b fakeRole) int {
		if a.position != b.position {
			return b.position - a.position
		}
		return cmp.Compare(a.id, b.id)
	})
	arranged := arrange(shown)
	if len(arranged) != len(shown) {
		t.Fatalf("the drag left %d roles of %d", len(arranged), len(shown))
	}
	for i := range arranged {
		arranged[i].position = len(arranged) - i
	}
	f.roles = append(arranged, everyoneRole)
}

// assertRefusalsBecameChanges holds the second plan of a first set-up to the
// first: everything refused before is a change now, and nothing else is. The
// roles the run creates stand in the definition's order by themselves, so the
// plan holds no order step.
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
	if len(planned) != 0 {
		t.Errorf("changes the first plan did not name = %v, want none", planned)
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
	// A new server holds none of the roles another party manages; the plan
	// does not make them, and they are all compare has left to name.
	for _, d := range diffs {
		if d.Kind != serverplan.KindRoleMissing || d.Detail != managedRoleNote {
			t.Errorf("compare after the set-up holds %s %s (%q), want only managed roles that are missing", d.Kind, d.Object, d.Detail)
		}
	}
}

func TestApplyServer_Negative_FirstSetUpOnAPlatformThatMovesNoRoleUp(t *testing.T) {
	fx := newSetupFixture(t)
	rights, _ := fx.ownPlanRights(t)

	fx.setBotRights(t, rights)
	raised, in := fx.plan(t, setupDay)
	if len(raised.Refused) != 0 {
		t.Fatalf("plan refused %+v, want no refusal", raised.Refused)
	}
	if _, err := fx.apply(t, setupDay, in); err != nil {
		t.Fatalf("a plan with no refusal stopped in the apply: %v", err)
	}
}

// addRoleLike adds a copy of the definition's role id to the definition, under
// a new id and name, and puts it in the order directly above the role before.
func addRoleLike(t *testing.T, def *serverdef.Definition, id, before string) serverdef.Role {
	t.Helper()
	at := slices.IndexFunc(def.Roles, func(r serverdef.Role) bool { return r.ID == id })
	place := slices.Index(def.RoleOrder, before)
	if at < 0 || (before != "" && place < 0) {
		t.Fatalf("role %q at %d, %q at %d of the order; want both in the definition", id, at, before, place)
	}
	added := def.Roles[at]
	added.ID, added.Name = added.ID+"-2", added.Name+" 2"
	def.Roles = append(def.Roles, added)
	if before == "" {
		def.RoleOrder = append(def.RoleOrder, added.ID)
	} else {
		def.RoleOrder = slices.Insert(def.RoleOrder, place, added.ID)
	}
	return added
}

// finishSetUp runs a first set-up and leaves the server as the operator does
// after it: the bot's role directly above the roles it gives, with the rights
// of the set-up.
func (fx setupFixture) finishSetUp(t *testing.T) []string {
	t.Helper()
	rights, _ := fx.ownPlanRights(t)
	fx.setBotRights(t, rights)
	_, in := fx.plan(t, setupDay)
	if _, err := fx.apply(t, setupDay, in); err != nil {
		t.Fatalf("set-up: %v", err)
	}
	fx.lowerBotRole(t)
	fx.setBotRights(t, rights)
	return rights
}

func TestApplyServer_Negative_RoleAddedLaterOnAPlatformThatMovesNoRoleUp(t *testing.T) {
	fx := newSetupFixture(t)
	fx.finishSetUp(t)
	addRoleLike(t, fx.def, "contributor", "")

	later, in := fx.plan(t, setupDay)
	if len(later.Refused) != 0 {
		t.Fatalf("plan refused %+v, want no refusal", later.Refused)
	}
	if _, err := fx.apply(t, setupDay, in); err != nil {
		t.Fatalf("a plan with no refusal stopped in the apply: %v", err)
	}
}

// A role defined between two roles that already stand on the server lands
// below them all; sorting it in needs a position the server does not have, so
// the plan refuses the order step and names the role to place by hand.
func TestApplyServer_Negative_RoleInsertedMidOrderIsRefusedInThePlan(t *testing.T) {
	fx := newSetupFixture(t)
	fx.finishSetUp(t)
	addRoleLike(t, fx.def, "contributor", "supporter")

	later, _ := fx.plan(t, setupDay)
	var found bool
	for _, r := range later.Refused {
		if r.Action != serverplan.ActionSetRoleOrder {
			t.Errorf("%s %s refused as %q, want only the order step refused", r.Action, r.Object, r.Reason)
			continue
		}
		found = true
		if r.Reason != serverplan.ReasonTooFewPositions || !strings.Contains(r.Detail, "contributor-2") {
			t.Errorf("order step refused as %q (%q), want %q naming contributor-2",
				r.Reason, r.Detail, serverplan.ReasonTooFewPositions)
		}
	}
	if !found {
		t.Fatalf("plan refused %+v, want the order step refused", later.Refused)
	}
}

// assertPlanRefusedWithoutRight verifies that omitting a required right causes plan refusal and prevents apply.
func assertPlanRefusedWithoutRight(t *testing.T, fx setupFixture, rights []string, without string) {
	t.Helper()
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
}

func TestApplyServer_Boundary_EveryNamedRightIsNeeded(t *testing.T) {
	fx := newSetupFixture(t)
	rights, _ := fx.ownPlanRights(t)

	for _, without := range rights {
		t.Run(without, func(t *testing.T) {
			assertPlanRefusedWithoutRight(t, fx, rights, without)
		})
	}
}

// assertChannelListPlanResult validates channel creation refusals and planned changes against visibility rules.
func assertChannelListPlanResult(t *testing.T, fx setupFixture, result serverplan.Result, wantRefused bool) {
	t.Helper()
	if got := len(result.Refused) > 0; got != wantRefused {
		t.Fatalf("refused = %+v, want refusals: %t", result.Refused, wantRefused)
	}
	for _, r := range result.Refused {
		if r.Action != serverplan.ActionCreateChannel || r.Reason != serverplan.ReasonNotVisible {
			t.Errorf("%s %s refused as %q, want only %s refused as %q",
				r.Action, r.Object, r.Reason, serverplan.ActionCreateChannel, serverplan.ReasonNotVisible)
		}
	}
	for _, c := range result.Changes {
		if wantRefused && c.Action == serverplan.ActionCreateChannel {
			t.Errorf("%s is planned from a list of channels that is not the whole list", c.Object)
		}
	}
	if want := visibleChannels(fx.def); wantRefused && (len(result.Refused) != want || want == len(fx.def.Channels)) {
		t.Errorf("%d channels refused, want the %d of %d the definition lets the bot see",
			len(result.Refused), want, len(fx.def.Channels))
	}
}

// runChannelListDateSubtest sets up bot rights, computes the plan at day, and checks channel visibility behavior.
func runChannelListDateSubtest(t *testing.T, day time.Time, administrator, wantRefused bool) {
	t.Helper()
	fx := newSetupFixture(t)
	rights, _ := fx.ownPlanRights(t)
	if administrator {
		rights = append(rights, "ADMINISTRATOR")
	}
	fx.setBotRights(t, rights)

	result, _ := fx.plan(t, day.Format(time.RFC3339))
	assertChannelListPlanResult(t, fx, result, wantRefused)
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
			runChannelListDateSubtest(t, tc.day, tc.administrator, tc.wantRefused)
		})
	}
}
