package serverplan

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

// raisedSnapshotOf is the server as defined, with the caller's role holding one
// right beyond the definition so that the plan runs in raised mode.
func raisedSnapshotOf(t *testing.T, def *serverdef.Definition) *Snapshot {
	t.Helper()
	s := snapshotOf(t, def)
	for i := range s.Roles {
		if s.Roles[i].Name == s.Caller.Role {
			s.Roles[i].Permissions = append(s.Roles[i].Permissions, "MANAGE_CHANNELS")
		}
	}
	s.Caller.Permissions = append(s.Caller.Permissions, "MANAGE_CHANNELS")
	return s
}

func duplicateRole(t *testing.T, s *Snapshot, name string) {
	t.Helper()
	for i := range s.Roles {
		if s.Roles[i].Name == name {
			s.Roles = append(s.Roles[:i+1], append([]Role{s.Roles[i]}, s.Roles[i+1:]...)...)
			return
		}
	}
	t.Fatalf("role %q not in snapshot", name)
}

func removeChannel(t *testing.T, s *Snapshot, name string) {
	t.Helper()
	kept := make([]Channel, 0, len(s.Channels))
	for i := range s.Channels {
		if s.Channels[i].Name != name {
			kept = append(kept, s.Channels[i])
		}
	}
	if len(kept) == len(s.Channels) {
		t.Fatalf("channel %q not in snapshot", name)
	}
	s.Channels = kept
}

func categoryNameOf(t *testing.T, def *serverdef.Definition, channelID string) string {
	t.Helper()
	for i := range def.Channels {
		if def.Channels[i].ID != channelID {
			continue
		}
		for j := range def.Categories {
			if def.Categories[j].ID == def.Channels[i].Category {
				return def.Categories[j].Name
			}
		}
	}
	t.Fatalf("no category for channel %q", channelID)
	return ""
}

func refusalFor(res Result, action, object string) (Refusal, bool) {
	for _, ref := range res.Refused {
		if ref.Action == action && ref.Object == object {
			return ref, true
		}
	}
	return Refusal{}, false
}

func changeFor(res Result, action, object string) bool {
	for _, c := range res.Changes {
		if c.Action == action && c.Object == object {
			return true
		}
	}
	return false
}

// countDefChannelAccessForRole counts how many access entries a role has in definition channels.
func countDefChannelAccessForRole(def *serverdef.Definition, roleID string) int {
	want := 0
	for i := range def.Channels {
		for j := range def.Channels[i].Access {
			if def.Channels[i].Access[j].Role == roleID {
				want++
			}
		}
	}
	return want
}

// runAmbiguousNameRoleEntriesTwice verifies refusals when a role exists twice.
func runAmbiguousNameRoleEntriesTwice(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := raisedSnapshotOf(t, def)
	duplicateRole(t, s, "Moderator")

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Changes) != 0 {
		t.Errorf("changes = %+v, want none", res.Changes)
	}
	entries := 0
	for _, ref := range res.Refused {
		if ref.Reason != ReasonAmbiguousName {
			t.Errorf("refusal %+v, want reason %s", ref, ReasonAmbiguousName)
		}
		if ref.Action == ActionSetAccess && strings.HasSuffix(ref.Object, ".access[moderator]") {
			entries++
		}
	}
	want := countDefChannelAccessForRole(def, "moderator")
	if want == 0 || entries != want {
		t.Errorf("refused entries of moderator = %d, want %d (> 0)", entries, want)
	}
	if _, ok := refusalFor(res, ActionResolveName, "roles[moderator]"); !ok {
		t.Errorf("resolve-name roles[moderator] not refused: %+v", res.Refused)
	}
}

// runAmbiguousNameChannelCreatedRoleTwice verifies refusal when creating channel with ambiguous role.
func runAmbiguousNameChannelCreatedRoleTwice(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := raisedSnapshotOf(t, def)
	duplicateRole(t, s, "Moderator")
	removeChannel(t, s, "mod-queue")

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	ref, ok := refusalFor(res, ActionCreateChannel, "channels[mod-queue]")
	if !ok || ref.Reason != ReasonAmbiguousName {
		t.Errorf("create-channel channels[mod-queue] = %+v (found %v), want %s", ref, ok, ReasonAmbiguousName)
	}
}

// runAmbiguousNameChannelCreatedCategoryTwice verifies refusal when creating channel in ambiguous category.
func runAmbiguousNameChannelCreatedCategoryTwice(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := raisedSnapshotOf(t, def)
	s.Categories = append(s.Categories, Category{Name: categoryNameOf(t, def, "lounge")})
	removeChannel(t, s, "lounge")

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	ref, ok := refusalFor(res, ActionCreateChannel, "channels[lounge]")
	if !ok || ref.Reason != ReasonAmbiguousName {
		t.Errorf("create-channel channels[lounge] = %+v (found %v), want %s", ref, ok, ReasonAmbiguousName)
	}
	if len(res.Changes) != 0 {
		t.Errorf("changes = %+v, want none", res.Changes)
	}
}

// runAmbiguousNameChannelMovedCategoryTwice verifies refusal when moving channel to ambiguous category.
func runAmbiguousNameChannelMovedCategoryTwice(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := raisedSnapshotOf(t, def)
	s.Categories = append(s.Categories, Category{Name: categoryNameOf(t, def, "lounge")})
	for i := range s.Channels {
		if s.Channels[i].Name == "lounge" {
			s.Channels[i].Category = categoryNameOf(t, def, "rules")
		}
	}

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	ref, ok := refusalFor(res, ActionMoveChannel, "channels[lounge]")
	if !ok || ref.Reason != ReasonAmbiguousName {
		t.Errorf("move-channel channels[lounge] = %+v (found %v), want %s", ref, ok, ReasonAmbiguousName)
	}
}

func TestPlan_Negative_AmbiguousNameBlocksWhatRestsOnIt(t *testing.T) {
	def := loadDef(t)

	t.Run("entries of a role that exists twice", func(t *testing.T) {
		runAmbiguousNameRoleEntriesTwice(t, def)
	})

	t.Run("channel created with an entry of a role that exists twice", func(t *testing.T) {
		runAmbiguousNameChannelCreatedRoleTwice(t, def)
	})

	t.Run("channel created in a category that exists twice", func(t *testing.T) {
		runAmbiguousNameChannelCreatedCategoryTwice(t, def)
	})

	t.Run("channel moved to a category that exists twice", func(t *testing.T) {
		runAmbiguousNameChannelMovedCategoryTwice(t, def)
	})
}

func TestPlan_Boundary_AmbiguousNameLeavesTheRestAlone(t *testing.T) {
	def := loadDef(t)
	s := raisedSnapshotOf(t, def)
	duplicateRole(t, s, "Moderator")
	// lounge has no entry and sits in a category that exists once.
	removeChannel(t, s, "lounge")

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if !changeFor(res, ActionCreateChannel, "channels[lounge]") {
		t.Errorf("create-channel channels[lounge] not accepted: changes %+v refused %+v", res.Changes, res.Refused)
	}
	if len(res.Changes) != 1 {
		t.Errorf("changes = %+v, want exactly the one channel", res.Changes)
	}
}

// moderatorMissing is the server as defined without the Moderator role, the
// highest role the plan may create, read by a caller that holds ADMINISTRATOR
// and may therefore create it.
func moderatorMissing(t *testing.T, def *serverdef.Definition) *Snapshot {
	t.Helper()
	s := snapshotOf(t, def)
	if s.Roles[1].Name != "Moderator" {
		t.Fatalf("second role = %q, want Moderator", s.Roles[1].Name)
	}
	s.Roles = slices.Delete(s.Roles, 1, 2)
	for i := range s.Roles {
		if s.Roles[i].Name == s.Caller.Role {
			s.Roles[i].Permissions = append(s.Roles[i].Permissions, "ADMINISTRATOR")
		}
	}
	s.Caller.Permissions = append(s.Caller.Permissions, "ADMINISTRATOR")
	return s
}

func TestPlan_Negative_CreatedRoleAboveARoleAboveTheCaller(t *testing.T) {
	def := loadDef(t)
	s := moderatorMissing(t, def)

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if !changeFor(res, ActionCreateRole, "roles[moderator]") {
		t.Fatalf("create-role roles[moderator] not accepted: %+v", res.Refused)
	}
	// The definition places the created role first, above roles that stand above
	// the caller: the caller cannot sort those.
	ref, ok := refusalFor(res, ActionSetRoleOrder, "role_order")
	if !ok || ref.Reason != ReasonAboveCaller {
		t.Errorf("set-role-order = %+v (found %v), want %s", ref, ok, ReasonAboveCaller)
	}
}

// callerOnTop moves the caller's role above every other role and numbers it
// as the client does, without gaps, plus the given free positions.
func callerOnTop(s *Snapshot, free int) {
	for i := range s.Roles {
		if s.Roles[i].Name == s.Caller.Role {
			caller := s.Roles[i]
			rest := append(append([]Role{}, s.Roles[:i]...), s.Roles[i+1:]...)
			s.Roles = append([]Role{caller}, rest...)
			break
		}
	}
	s.Caller.Position = len(s.Roles) - 1 + free
}

// One free position above the roles below the caller's is the room for one
// created role; without it the order step would stop in the apply.
// runBoundaryCreatedRoleOneFreePosition verifies role ordering when there is one free position.
func runBoundaryCreatedRoleOneFreePosition(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := moderatorMissing(t, def)
	callerOnTop(s, 1)
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) != 0 {
		t.Errorf("refused = %+v, want none", res.Refused)
	}
	found := false
	for _, c := range res.Changes {
		if c.Action == ActionSetRoleOrder {
			found = true
			if !strings.HasPrefix(c.Detail, "operator, moderator, ") {
				t.Errorf("set-role-order detail = %q, want it to start with the created role", c.Detail)
			}
		}
	}
	if !found {
		t.Errorf("set-role-order not accepted: %+v", res.Changes)
	}
}

// runBoundaryCreatedRoleNoFreePosition verifies role ordering refusal when there is no free position.
func runBoundaryCreatedRoleNoFreePosition(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := moderatorMissing(t, def)
	callerOnTop(s, 0)
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if !changeFor(res, ActionCreateRole, "roles[moderator]") {
		t.Fatalf("create-role roles[moderator] not accepted: %+v", res.Refused)
	}
	ref, ok := refusalFor(res, ActionSetRoleOrder, "role_order")
	want := fmt.Sprintf("%d roles would stand below position %d; create moderator by hand at the definition's place",
		s.Caller.Position, s.Caller.Position)
	if !ok || ref.Reason != ReasonTooFewPositions || ref.Detail != want {
		t.Errorf("set-role-order = %+v (found %v), want %s with %q", ref, ok, ReasonTooFewPositions, want)
	}
}

func TestPlan_Boundary_CreatedRoleWithTheCallerOnTop(t *testing.T) {
	def := loadDef(t)

	t.Run("one free position", func(t *testing.T) {
		runBoundaryCreatedRoleOneFreePosition(t, def)
	})

	t.Run("no free position", func(t *testing.T) {
		runBoundaryCreatedRoleNoFreePosition(t, def)
	})
}

// swapBelowCaller swaps the two roles right below the caller's role, so that
// the order differs from the definition with no role to create.
func swapBelowCaller(t *testing.T, s *Snapshot) int {
	t.Helper()
	at := slices.IndexFunc(s.Roles, func(r Role) bool { return r.Name == s.Caller.Role })
	if at < 0 || at+2 >= len(s.Roles)-1 {
		t.Fatalf("no two roles below the caller's role in %+v", s.Roles)
	}
	s.Roles[at+1], s.Roles[at+2] = s.Roles[at+2], s.Roles[at+1]
	return len(s.Roles) - 2 - at
}

// The order step numbers the roles below the caller's from 1; with as many
// roles below as the caller's position it has no room, and the plan says so
// before the apply stops.
func TestPlan_Boundary_ReorderWithoutCreation(t *testing.T) {
	def := loadDef(t)

	t.Run("one position more than roles below", func(t *testing.T) {
		s := raisedSnapshotOf(t, def)
		below := swapBelowCaller(t, s)
		s.Caller.Position = below + 1
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		if len(res.Refused) != 0 {
			t.Errorf("refused = %+v, want none", res.Refused)
		}
		if !changeFor(res, ActionSetRoleOrder, "role_order") {
			t.Errorf("set-role-order not accepted: %+v", res.Changes)
		}
	})

	t.Run("as many roles below as the position", func(t *testing.T) {
		s := raisedSnapshotOf(t, def)
		below := swapBelowCaller(t, s)
		s.Caller.Position = below
		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		ref, ok := refusalFor(res, ActionSetRoleOrder, "role_order")
		want := fmt.Sprintf("%d roles stand below position %d; sort them by hand", below, below)
		if !ok || ref.Reason != ReasonTooFewPositions || ref.Detail != want {
			t.Errorf("set-role-order = %+v (found %v), want %s with %q", ref, ok, ReasonTooFewPositions, want)
		}
	})
}

type firstReasonCase struct {
	name       string
	setup      func() *Snapshot
	wantAction string
	wantObject string
	wantReason string
}

// setRolePerms updates the permissions of a role named name in snapshot s.
func setRolePerms(s *Snapshot, name string, perms ...string) {
	for i := range s.Roles {
		if s.Roles[i].Name == name {
			s.Roles[i].Permissions = perms
		}
	}
}

// setRoleManagedAndPerms updates managed flag and permissions of a role named name.
func setRoleManagedAndPerms(s *Snapshot, name string, managed bool, perms ...string) {
	for i := range s.Roles {
		if s.Roles[i].Name == name {
			s.Roles[i].Managed = managed
			s.Roles[i].Permissions = perms
		}
	}
}

// firstReasonWinsCasesPart1 returns the first set of test cases for TestPlan_Boundary_FirstReasonWins.
func firstReasonWinsCasesPart1(t *testing.T, def *serverdef.Definition) []firstReasonCase {
	return []firstReasonCase{
		{
			name: "ambiguous-name before outside-own-changes",
			setup: func() *Snapshot {
				s := snapshotOf(t, def)
				duplicateRole(t, s, "Moderator")
				return s
			},
			wantAction: ActionSetAccess, wantObject: "channels[mod-queue].access[moderator]", wantReason: ReasonAmbiguousName,
		},
		{
			name: "not-visible before outside-own-changes",
			setup: func() *Snapshot {
				s := snapshotOf(t, def)
				s.ChannelsComplete = false
				removeChannel(t, s, "lounge")
				return s
			},
			wantAction: ActionCreateChannel, wantObject: "channels[lounge]", wantReason: ReasonNotVisible,
		},
		{
			name: "outside-own-changes before missing-feature",
			setup: func() *Snapshot {
				s := snapshotOf(t, def)
				s.Features = []string{"COMMUNITY"}
				removeChannel(t, s, "announcements")
				return s
			},
			wantAction: ActionCreateChannel, wantObject: "channels[announcements]", wantReason: ReasonOutsideOwnChanges,
		},
	}
}

// firstReasonWinsCasesPart2 returns the second set of test cases for TestPlan_Boundary_FirstReasonWins.
func firstReasonWinsCasesPart2(t *testing.T, def *serverdef.Definition) []firstReasonCase {
	return []firstReasonCase{
		{
			name: "missing-feature before lacks-right",
			setup: func() *Snapshot {
				s := snapshotOf(t, def)
				// Raised by a right that creates no channel.
				setRolePerms(s, s.Caller.Role, "MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "VIEW_AUDIT_LOG")
				s.Caller.Permissions = append(s.Caller.Permissions, "VIEW_AUDIT_LOG")
				s.Features = []string{"COMMUNITY"}
				removeChannel(t, s, "announcements")
				return s
			},
			wantAction: ActionCreateChannel, wantObject: "channels[announcements]", wantReason: ReasonMissingFeature,
		},
		{
			name: "managed-role before above-caller",
			setup: func() *Snapshot {
				s := raisedSnapshotOf(t, def)
				setRoleManagedAndPerms(s, "Moderator", true, "MANAGE_MESSAGES")
				return s
			},
			wantAction: ActionSetRolePermissions, wantObject: "roles[moderator]", wantReason: ReasonManagedRole,
		},
		{
			name: "made-by-hand before above-caller",
			setup: func() *Snapshot {
				s := raisedSnapshotOf(t, def)
				// The role grants ADMINISTRATOR and stands above the caller.
				setRolePerms(s, "Operator")
				return s
			},
			wantAction: ActionSetRolePermissions, wantObject: "roles[operator]", wantReason: ReasonMadeByHand,
		},
		{
			name: "above-caller before lacks-right",
			setup: func() *Snapshot {
				s := raisedSnapshotOf(t, def)
				// The caller holds no MANAGE_ROLES and the role stands above it.
				setRolePerms(s, "Moderator", "MANAGE_MESSAGES")
				s.Caller.Permissions = slices.DeleteFunc(s.Caller.Permissions, func(p string) bool { return p == "MANAGE_ROLES" })
				return s
			},
			wantAction: ActionSetRolePermissions, wantObject: "roles[moderator]", wantReason: ReasonAboveCaller,
		},
	}
}

func TestPlan_Boundary_FirstReasonWins(t *testing.T) {
	def := loadDef(t)
	tests := append(firstReasonWinsCasesPart1(t, def), firstReasonWinsCasesPart2(t, def)...)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Plan(def, tc.setup())
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			ref, ok := refusalFor(res, tc.wantAction, tc.wantObject)
			if !ok || ref.Reason != tc.wantReason {
				t.Errorf("%s %s = %+v (found %v), want reason %s", tc.wantAction, tc.wantObject, ref, ok, tc.wantReason)
			}
		})
	}
}

// An order step without room and without MANAGE_ROLES is refused for the
// positions, the check that comes first; with room it is refused for the
// right, which shows the caller in this case lacks it.
func TestPlan_Boundary_TooFewPositionsBeforeLacksRight(t *testing.T) {
	def := loadDef(t)
	for _, tc := range []struct {
		name       string
		room       int
		wantReason string
	}{
		{name: "no room", room: 0, wantReason: ReasonTooFewPositions},
		{name: "room", room: 1, wantReason: ReasonLacksRight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := raisedSnapshotOf(t, def)
			below := swapBelowCaller(t, s)
			s.Caller.Position = below + tc.room
			s.Caller.Permissions = slices.DeleteFunc(s.Caller.Permissions, func(p string) bool {
				return p == "MANAGE_ROLES" || p == "ADMINISTRATOR"
			})
			res, err := Plan(def, s)
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			ref, ok := refusalFor(res, ActionSetRoleOrder, "role_order")
			if !ok || ref.Reason != tc.wantReason {
				t.Errorf("set-role-order = %+v (found %v), want reason %s", ref, ok, tc.wantReason)
			}
		})
	}
}
