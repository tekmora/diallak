package serverplan

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

func raiseBotRole(t *testing.T, s *Snapshot, extraPerm string) {
	t.Helper()
	r := roleNamed(t, s, s.Caller.Role)
	r.Permissions = append(r.Permissions, extraPerm)
	s.Caller.Permissions = append(s.Caller.Permissions, extraPerm)
}

func raisedFirstSetupSnapshot() *Snapshot {
	callerPerms := []string{
		"MANAGE_ROLES",
		"MANAGE_MESSAGES",
		"MODERATE_MEMBERS",
		"MANAGE_GUILD",
		"MANAGE_CHANNELS",
		"CREATE_PRIVATE_THREADS",
		"CONNECT",
		// everyone permissions
		"VIEW_CHANNEL",
		"SEND_MESSAGES",
		"SEND_MESSAGES_IN_THREADS",
		"CREATE_PUBLIC_THREADS",
		"READ_MESSAGE_HISTORY",
		"ADD_REACTIONS",
		"EMBED_LINKS",
		"ATTACH_FILES",
		"USE_APPLICATION_COMMANDS",
		"SPEAK",
	}

	return &Snapshot{
		Features: []string{"COMMUNITY", "NEWS"},
		Caller: Caller{
			Role:        "Diallak",
			Position:    1,
			Permissions: callerPerms,
		},
		Roles: []Role{
			{Name: "Operator", Everyone: false, Managed: false, Permissions: []string{"ADMINISTRATOR"}},
			{
				Name:        "Diallak",
				Everyone:    false,
				Managed:     true,
				Permissions: []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "MANAGE_GUILD", "MANAGE_CHANNELS", "CREATE_PRIVATE_THREADS", "CONNECT"},
			},
			{
				Name:     "@everyone",
				Everyone: true,
				Managed:  false,
				Permissions: []string{
					"VIEW_CHANNEL",
					"SEND_MESSAGES",
					"SEND_MESSAGES_IN_THREADS",
					"CREATE_PUBLIC_THREADS",
					"READ_MESSAGE_HISTORY",
					"ADD_REACTIONS",
					"EMBED_LINKS",
					"ATTACH_FILES",
					"USE_APPLICATION_COMMANDS",
					"SPEAK",
				},
			},
		},
		Categories:       []Category{},
		Channels:         []Channel{},
		ChannelsComplete: true,
	}
}

func TestPlan_Positive_OwnModeNothingToDo(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if res.Raised {
		t.Errorf("res.Raised = true, want false")
	}
	if len(res.Changes) != 0 {
		t.Errorf("len(Changes) = %d, want 0", len(res.Changes))
	}
	if len(res.Refused) != 0 {
		t.Errorf("len(Refused) = %d, want 0", len(res.Refused))
	}
	if len(res.Rights()) != 0 {
		t.Errorf("len(Rights()) = %d, want 0", len(res.Rights()))
	}
}

// removeChannelRoleAccess drops a named role from a named channel's access list.
func removeChannelRoleAccess(s *Snapshot, channelName, roleName string) {
	for i := range s.Channels {
		if s.Channels[i].Name == channelName {
			var newAccess []Access
			for _, acc := range s.Channels[i].Access {
				if acc.Role != roleName {
					newAccess = append(newAccess, acc)
				}
			}
			s.Channels[i].Access = newAccess
		}
	}
}

// runOwnModeAreaEntryHomeLab tests own-mode area entry for home-lab.
func runOwnModeAreaEntryHomeLab(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	removeChannelRoleAccess(s, "home-lab", "Interest: home lab and self-hosting")

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if res.Raised {
		t.Errorf("res.Raised = true, want false")
	}
	if len(res.Refused) != 0 {
		t.Fatalf("expected 0 refused, got %d: %+v", len(res.Refused), res.Refused)
	}
	if len(res.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(res.Changes), res.Changes)
	}
	c := res.Changes[0]
	if c.Action != ActionSetAccess {
		t.Errorf("Action = %q, want %q", c.Action, ActionSetAccess)
	}
	if c.Object != "channels[home-lab].access[interest-home-lab]" {
		t.Errorf("Object = %q, want 'channels[home-lab].access[interest-home-lab]'", c.Object)
	}
	wantNeeds := []string{"MANAGE_ROLES", "VIEW_CHANNEL"}
	if !slices.Equal(c.Needs, wantNeeds) {
		t.Errorf("Needs = %v, want %v", c.Needs, wantNeeds)
	}
}

// runOwnModeAreaEntryDeutsch tests own-mode area entry for deutsch.
func runOwnModeAreaEntryDeutsch(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	removeChannelRoleAccess(s, "deutsch", "Language: Deutsch")

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) != 0 || len(res.Changes) != 1 {
		t.Fatalf("expected 0 refused, 1 change; got %d refused, %d changes", len(res.Refused), len(res.Changes))
	}
	c := res.Changes[0]
	if c.Action != ActionSetAccess || c.Object != "channels[room-de].access[lang-de]" {
		t.Errorf("unexpected change: %+v", c)
	}
}

func TestPlan_Positive_OwnModeAreaEntry(t *testing.T) {
	def := loadDef(t)

	t.Run("home-lab / interest-home-lab", func(t *testing.T) {
		runOwnModeAreaEntryHomeLab(t, def)
	})

	t.Run("deutsch / lang-de", func(t *testing.T) {
		runOwnModeAreaEntryDeutsch(t, def)
	})
}

type ownModeNegativeCase struct {
	name       string
	setup      func(t *testing.T, baseDef *serverdef.Definition) (*serverdef.Definition, *Snapshot)
	wantObject string
}

// setupOwnModeLoungeDiffers creates a snapshot where lounge access allows SEND_MESSAGES.
func setupOwnModeLoungeDiffers(t *testing.T, baseDef *serverdef.Definition) (*serverdef.Definition, *Snapshot) {
	t.Helper()
	defCopy := deepCopy(t, baseDef)
	for i := range defCopy.Channels {
		if defCopy.Channels[i].Name == "lounge" {
			defCopy.Channels[i].Access = []serverdef.Access{
				{Role: "everyone", Allow: []string{"VIEW_CHANNEL"}},
			}
		}
	}
	s := snapshotOf(t, defCopy)
	for i := range s.Channels {
		if s.Channels[i].Name == "lounge" {
			s.Channels[i].Access[0].Allow = append(s.Channels[i].Access[0].Allow, "SEND_MESSAGES")
		}
	}
	return defCopy, s
}

// ownModeNegativeCasesPart1 returns the first part of own-mode negative test cases.
func ownModeNegativeCasesPart1() []ownModeNegativeCase {
	return []ownModeNegativeCase{
		{
			name: "entry of moderator on home-lab removed",
			setup: func(t *testing.T, baseDef *serverdef.Definition) (*serverdef.Definition, *Snapshot) {
				t.Helper()
				s := snapshotOf(t, baseDef)
				removeChannelRoleAccess(s, "home-lab", "Moderator")
				return baseDef, s
			},
			wantObject: "channels[home-lab].access[moderator]",
		},
		{
			name:       "entry on lounge differs",
			setup:      setupOwnModeLoungeDiffers,
			wantObject: "channels[lounge].access[everyone]",
		},
		{
			name: "a role missing",
			setup: func(t *testing.T, baseDef *serverdef.Definition) (*serverdef.Definition, *Snapshot) {
				t.Helper()
				s := snapshotOf(t, baseDef)
				var newRoles []Role
				for _, r := range s.Roles {
					if r.Name != "Subject guide: coding" {
						newRoles = append(newRoles, r)
					}
				}
				s.Roles = newRoles
				return baseDef, s
			},
			wantObject: "roles[guide-coding]",
		},
	}
}

// ownModeNegativeCasesPart2 returns the second part of own-mode negative test cases.
func ownModeNegativeCasesPart2() []ownModeNegativeCase {
	return []ownModeNegativeCase{
		{
			name: "a category missing",
			setup: func(t *testing.T, baseDef *serverdef.Definition) (*serverdef.Definition, *Snapshot) {
				t.Helper()
				s := snapshotOf(t, baseDef)
				var newCats []Category
				for _, c := range s.Categories {
					if c.Name != "Staff" {
						newCats = append(newCats, c)
					}
				}
				s.Categories = newCats
				return baseDef, s
			},
			wantObject: "categories[staff]",
		},
		{
			name: "lounge missing",
			setup: func(t *testing.T, baseDef *serverdef.Definition) (*serverdef.Definition, *Snapshot) {
				t.Helper()
				s := snapshotOf(t, baseDef)
				var newCh []Channel
				for _, ch := range s.Channels {
					if ch.Name != "lounge" {
						newCh = append(newCh, ch)
					}
				}
				s.Channels = newCh
				return baseDef, s
			},
			wantObject: "channels[lounge]",
		},
		{
			name: "a role's permissions differ",
			setup: func(t *testing.T, baseDef *serverdef.Definition) (*serverdef.Definition, *Snapshot) {
				t.Helper()
				s := snapshotOf(t, baseDef)
				for i := range s.Roles {
					if s.Roles[i].Name == "Moderator" {
						s.Roles[i].Permissions = []string{"MANAGE_MESSAGES"}
					}
				}
				return baseDef, s
			},
			wantObject: "roles[moderator]",
		},
		{
			name: "two roles below the bot swapped",
			setup: func(t *testing.T, baseDef *serverdef.Definition) (*serverdef.Definition, *Snapshot) {
				t.Helper()
				s := snapshotOf(t, baseDef)
				s.Roles[5], s.Roles[6] = s.Roles[6], s.Roles[5]
				return baseDef, s
			},
			wantObject: "role_order",
		},
	}
}

// runOwnModeNegativeCase executes an own-mode negative test case.
func runOwnModeNegativeCase(t *testing.T, tc ownModeNegativeCase, baseDef *serverdef.Definition) {
	t.Helper()
	def, s := tc.setup(t, baseDef)
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Changes) != 0 {
		t.Errorf("len(Changes) = %d, want 0: %+v", len(res.Changes), res.Changes)
	}
	if len(res.Refused) == 0 {
		t.Fatalf("expected refusals, got 0")
	}
	for _, ref := range res.Refused {
		if ref.Reason == ReasonOutsideOwnChanges && ref.Object == tc.wantObject {
			return
		}
	}
	t.Errorf("expected refusal with reason outside-own-changes and object %q, got: %+v", tc.wantObject, res.Refused)
}

func TestPlan_Negative_OwnModeAnythingElse(t *testing.T) {
	baseDef := loadDef(t)
	var tests []ownModeNegativeCase
	tests = append(tests, ownModeNegativeCasesPart1()...)
	tests = append(tests, ownModeNegativeCasesPart2()...)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runOwnModeNegativeCase(t, tc, baseDef)
		})
	}
}

// checkRaisedFirstSetupAction updates and validates the lifecycle phase for an action.
func checkRaisedFirstSetupAction(t *testing.T, action string, phase *int) {
	t.Helper()
	switch action {
	case ActionCreateRole:
		if *phase > 1 {
			t.Errorf("create-role in phase %d", *phase)
		}
	case ActionSetRoleOrder:
		if *phase > 2 {
			t.Errorf("set-role-order in phase %d", *phase)
		}
		*phase = 2
	case ActionCreateCategory:
		if *phase > 3 {
			t.Errorf("create-category in phase %d", *phase)
		}
		*phase = 3
	case ActionCreateChannel:
		*phase = 4
	default:
		t.Errorf("unexpected action %q", action)
	}
}

// verifyRaisedFirstSetupChanges checks order and properties of first setup changes.
func verifyRaisedFirstSetupChanges(t *testing.T, changes []Change) {
	t.Helper()
	phase := 1
	for i, c := range changes {
		if c.Step != i+1 {
			t.Errorf("step %d != index %d", c.Step, i+1)
		}
		if c.Object == "roles[bot]" || c.Object == "roles[operator]" {
			t.Errorf("unexpected change for bot/operator role: %+v", c)
		}
		if c.Action == ActionSetAccess {
			t.Errorf("unexpected set-access in first setup: %+v", c)
		}
		checkRaisedFirstSetupAction(t, c.Action, &phase)
	}
}

// verifyRaisedFirstSetupRights checks expected rights against planned rights.
func verifyRaisedFirstSetupRights(t *testing.T, rights []string) {
	t.Helper()
	wantRights := []string{
		"ADD_REACTIONS",
		"CONNECT",
		"CREATE_PRIVATE_THREADS",
		"CREATE_PUBLIC_THREADS",
		"MANAGE_CHANNELS",
		"MANAGE_ROLES",
		"READ_MESSAGE_HISTORY",
		"SEND_MESSAGES",
		"SEND_MESSAGES_IN_THREADS",
		"VIEW_CHANNEL",
	}
	if !slices.Equal(rights, wantRights) {
		t.Errorf("Rights() = %v, want %v", rights, wantRights)
	}
}

func TestPlan_Positive_RaisedFirstSetUp(t *testing.T) {
	def := loadDef(t)
	s := raisedFirstSetupSnapshot()

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if !res.Raised {
		t.Errorf("res.Raised = false, want true")
	}
	if len(res.Refused) != 0 {
		t.Fatalf("expected 0 refused, got %d: %+v", len(res.Refused), res.Refused)
	}

	verifyRaisedFirstSetupChanges(t, res.Changes)
	verifyRaisedFirstSetupRights(t, res.Rights())
}

// runRaisedLacksRightWithoutOperator tests missing operator role produces lacks-right refusal.
func runRaisedLacksRightWithoutOperator(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := raisedFirstSetupSnapshot()
	s.Roles = s.Roles[1:]

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) != 1 {
		t.Fatalf("expected exactly 1 refusal, got %d: %+v", len(res.Refused), res.Refused)
	}
	ref := res.Refused[0]
	if ref.Action != ActionCreateRole || ref.Object != "roles[operator]" || ref.Reason != ReasonLacksRight || !strings.Contains(ref.Detail, "ADMINISTRATOR") {
		t.Errorf("unexpected refusal: %+v", ref)
	}
}

// runRaisedLacksRightWithAdministrator tests caller with ADMINISTRATOR has zero refusals.
func runRaisedLacksRightWithAdministrator(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := raisedFirstSetupSnapshot()
	s.Roles = s.Roles[1:]
	s.Caller.Permissions = append(s.Caller.Permissions, "ADMINISTRATOR")

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) != 0 {
		t.Fatalf("expected 0 refused, got %d: %+v", len(res.Refused), res.Refused)
	}
}

// snapshotWithoutCallerPerm returns a raised first setup snapshot missing a specific caller permission.
func snapshotWithoutCallerPerm(perm string) *Snapshot {
	s := raisedFirstSetupSnapshot()
	var newPerms []string
	for _, p := range s.Caller.Permissions {
		if p != perm {
			newPerms = append(newPerms, p)
		}
	}
	s.Caller.Permissions = newPerms
	return s
}

// assertNoAcceptedChannelChanges checks that no create category or channel changes exist.
func assertNoAcceptedChannelChanges(t *testing.T, changes []Change) {
	t.Helper()
	for _, c := range changes {
		if c.Action == ActionCreateCategory || c.Action == ActionCreateChannel {
			t.Errorf("unexpected accepted channel/category change: %+v", c)
		}
	}
}

// assertChannelRefusalsLackManageChannels checks category/channel refusals cite MANAGE_CHANNELS.
func assertChannelRefusalsLackManageChannels(t *testing.T, refused []Refusal) {
	t.Helper()
	for _, ref := range refused {
		if ref.Action == ActionCreateCategory || ref.Action == ActionCreateChannel {
			if ref.Reason != ReasonLacksRight || !strings.Contains(ref.Detail, "MANAGE_CHANNELS") {
				t.Errorf("refusal for %s not lacks-right MANAGE_CHANNELS: %+v", ref.Object, ref)
			}
		}
	}
}

// runRaisedLacksRightWithoutManageChannels tests missing MANAGE_CHANNELS refuses channel operations.
func runRaisedLacksRightWithoutManageChannels(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotWithoutCallerPerm("MANAGE_CHANNELS")

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	assertNoAcceptedChannelChanges(t, res.Changes)
	assertChannelRefusalsLackManageChannels(t, res.Refused)
}

func TestPlan_Negative_RaisedLacksRight(t *testing.T) {
	def := loadDef(t)

	t.Run("without Operator role -> create-role roles[operator] refused lacks-right ADMINISTRATOR", func(t *testing.T) {
		runRaisedLacksRightWithoutOperator(t, def)
	})

	t.Run("caller with ADMINISTRATOR -> 0 refused", func(t *testing.T) {
		runRaisedLacksRightWithAdministrator(t, def)
	})

	t.Run("caller without MANAGE_CHANNELS -> every create-category and create-channel refused lacks-right", func(t *testing.T) {
		runRaisedLacksRightWithoutManageChannels(t, def)
	})
}

// runOnePermissionShortWithoutConnect tests refusal when caller lacks CONNECT.
func runOnePermissionShortWithoutConnect(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotWithoutCallerPerm("CONNECT")
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) != 1 {
		t.Fatalf("expected 1 refusal, got %d: %+v", len(res.Refused), res.Refused)
	}
	ref := res.Refused[0]
	if ref.Action != ActionCreateChannel || ref.Object != "channels[events-voice]" || ref.Reason != ReasonLacksRight || ref.Detail != "CONNECT" {
		t.Errorf("unexpected refusal: %+v", ref)
	}
}

// runOnePermissionShortWithoutCreatePrivateThreads tests refusal when caller lacks CREATE_PRIVATE_THREADS.
func runOnePermissionShortWithoutCreatePrivateThreads(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotWithoutCallerPerm("CREATE_PRIVATE_THREADS")
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Refused) != 1 {
		t.Fatalf("expected 1 refusal, got %d: %+v", len(res.Refused), res.Refused)
	}
	ref := res.Refused[0]
	if ref.Action != ActionCreateChannel || ref.Object != "channels[tickets]" || ref.Reason != ReasonLacksRight || ref.Detail != "CREATE_PRIVATE_THREADS" {
		t.Errorf("unexpected refusal: %+v", ref)
	}
}

func TestPlan_Boundary_OnePermissionShort(t *testing.T) {
	def := loadDef(t)

	t.Run("caller without CONNECT -> create-channel channels[events-voice] lacks-right CONNECT", func(t *testing.T) {
		runOnePermissionShortWithoutConnect(t, def)
	})

	t.Run("caller without CREATE_PRIVATE_THREADS -> create-channel channels[tickets] lacks-right CREATE_PRIVATE_THREADS", func(t *testing.T) {
		runOnePermissionShortWithoutCreatePrivateThreads(t, def)
	})
}

// runNeighboursRoleDirectlyBelowCaller tests role below caller with wrong permissions is accepted.
func runNeighboursRoleDirectlyBelowCaller(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	raiseBotRole(t, s, "MANAGE_CHANNELS")
	roleNamed(t, s, "Contributor").Permissions = []string{"VIEW_AUDIT_LOG"}

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	for _, c := range res.Changes {
		if c.Action == ActionSetRolePermissions && c.Object == "roles[contributor]" {
			return
		}
	}
	t.Errorf("expected set-role-permissions roles[contributor] in changes: %+v", res.Changes)
}

// runNeighboursRoleDirectlyAboveCaller tests role above caller with wrong permissions is refused above-caller.
func runNeighboursRoleDirectlyAboveCaller(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	raiseBotRole(t, s, "MANAGE_CHANNELS")
	roleNamed(t, s, "Subject guide: video").Permissions = []string{"VIEW_AUDIT_LOG"}

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	for _, ref := range res.Refused {
		if ref.Action == ActionSetRolePermissions && ref.Object == "roles[guide-video]" && ref.Reason == ReasonAboveCaller {
			return
		}
	}
	t.Errorf("expected refusal above-caller for roles[guide-video]: %+v", res.Refused)
}

// runNeighboursTwoRolesBelowCallerSwapped tests swapping two roles below caller is accepted.
func runNeighboursTwoRolesBelowCallerSwapped(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	raiseBotRole(t, s, "MANAGE_CHANNELS")
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
		t.Fatalf("Plan failed: %v", err)
	}
	for _, c := range res.Changes {
		if c.Action == ActionSetRoleOrder {
			return
		}
	}
	t.Errorf("expected set-role-order in changes: %+v", res.Changes)
}

// runNeighboursTwoRolesAboveCallerSwapped tests swapping two roles above caller is refused above-caller.
func runNeighboursTwoRolesAboveCallerSwapped(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	raiseBotRole(t, s, "MANAGE_CHANNELS")
	s.Roles[0], s.Roles[1] = s.Roles[1], s.Roles[0]

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	for _, ref := range res.Refused {
		if ref.Action == ActionSetRoleOrder && ref.Reason == ReasonAboveCaller {
			return
		}
	}
	t.Errorf("expected refusal above-caller for set-role-order: %+v", res.Refused)
}

func TestPlan_Boundary_NeighboursOfTheCaller(t *testing.T) {
	def := loadDef(t)

	t.Run("role directly below caller with wrong permissions -> accepted", func(t *testing.T) {
		runNeighboursRoleDirectlyBelowCaller(t, def)
	})

	t.Run("role directly above caller with wrong permissions -> refused above-caller", func(t *testing.T) {
		runNeighboursRoleDirectlyAboveCaller(t, def)
	})

	t.Run("two roles below caller swapped -> set-role-order accepted", func(t *testing.T) {
		runNeighboursTwoRolesBelowCallerSwapped(t, def)
	})

	t.Run("two roles above caller swapped -> set-role-order refused above-caller", func(t *testing.T) {
		runNeighboursTwoRolesAboveCallerSwapped(t, def)
	})
}

type planNegativeReasonCase struct {
	name       string
	setup      func(t *testing.T, def *serverdef.Definition) *Snapshot
	wantAction string
	wantObject string
	wantReason string
}

// planNegativeReasonsCasesPart1 returns the first half of negative refusal reason test cases.
func planNegativeReasonsCasesPart1() []planNegativeReasonCase {
	return []planNegativeReasonCase{
		{
			name: "announcements missing + no NEWS -> missing-feature",
			setup: func(t *testing.T, def *serverdef.Definition) *Snapshot {
				t.Helper()
				s := snapshotOf(t, def)
				raiseBotRole(t, s, "MANAGE_CHANNELS")
				s.Features = []string{"COMMUNITY"}
				var newCh []Channel
				for _, ch := range s.Channels {
					if ch.Name != "announcements" {
						newCh = append(newCh, ch)
					}
				}
				s.Channels = newCh
				return s
			},
			wantAction: ActionCreateChannel,
			wantObject: "channels[announcements]",
			wantReason: ReasonMissingFeature,
		},
		{
			name: "lounge observed as voice -> convert-channel type-differs",
			setup: func(t *testing.T, def *serverdef.Definition) *Snapshot {
				t.Helper()
				s := snapshotOf(t, def)
				raiseBotRole(t, s, "MANAGE_CHANNELS")
				for i := range s.Channels {
					if s.Channels[i].Name == "lounge" {
						s.Channels[i].Type = "voice"
					}
				}
				return s
			},
			wantAction: ActionConvertChannel,
			wantObject: "channels[lounge]",
			wantReason: ReasonTypeDiffers,
		},
		{
			name: "two channels named lounge -> resolve-name ambiguous-name",
			setup: func(t *testing.T, def *serverdef.Definition) *Snapshot {
				t.Helper()
				s := snapshotOf(t, def)
				raiseBotRole(t, s, "MANAGE_CHANNELS")
				s.Channels = append(s.Channels, Channel{
					Name:     "lounge",
					Type:     "text",
					Category: "Community",
				})
				return s
			},
			wantAction: ActionResolveName,
			wantObject: "channels[lounge]",
			wantReason: ReasonAmbiguousName,
		},
	}
}

// planNegativeReasonsCasesPart2 returns the second half of negative refusal reason test cases.
func planNegativeReasonsCasesPart2() []planNegativeReasonCase {
	return []planNegativeReasonCase{
		{
			name: "two roles named Moderator -> resolve-name ambiguous-name",
			setup: func(t *testing.T, def *serverdef.Definition) *Snapshot {
				t.Helper()
				s := snapshotOf(t, def)
				raiseBotRole(t, s, "MANAGE_CHANNELS")
				for i, r := range s.Roles {
					if r.Name == "Moderator" {
						s.Roles = append(s.Roles[:i+1], append([]Role{r}, s.Roles[i+1:]...)...)
						break
					}
				}
				return s
			},
			wantAction: ActionResolveName,
			wantObject: "roles[moderator]",
			wantReason: ReasonAmbiguousName,
		},
		{
			name: "Supporter managed with other permissions -> managed-role",
			setup: func(t *testing.T, def *serverdef.Definition) *Snapshot {
				t.Helper()
				s := snapshotOf(t, def)
				raiseBotRole(t, s, "MANAGE_CHANNELS")
				for i := range s.Roles {
					if s.Roles[i].Name == "Supporter" {
						s.Roles[i].Managed = true
						s.Roles[i].Permissions = []string{"VIEW_AUDIT_LOG"}
					}
				}
				return s
			},
			wantAction: ActionSetRolePermissions,
			wantObject: "roles[supporter]",
			wantReason: ReasonManagedRole,
		},
		{
			name: "lounge missing + not complete -> not-visible",
			setup: func(t *testing.T, def *serverdef.Definition) *Snapshot {
				t.Helper()
				s := snapshotOf(t, def)
				raiseBotRole(t, s, "MANAGE_CHANNELS")
				s.ChannelsComplete = false
				var newCh []Channel
				for _, ch := range s.Channels {
					if ch.Name != "lounge" {
						newCh = append(newCh, ch)
					}
				}
				s.Channels = newCh
				return s
			},
			wantAction: ActionCreateChannel,
			wantObject: "channels[lounge]",
			wantReason: ReasonNotVisible,
		},
	}
}

// runPlanNegativeReasonCase executes a single refusal reason test case.
func runPlanNegativeReasonCase(t *testing.T, tc planNegativeReasonCase, def *serverdef.Definition) {
	t.Helper()
	s := tc.setup(t, def)
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	for _, ref := range res.Refused {
		if ref.Action == tc.wantAction && ref.Object == tc.wantObject && ref.Reason == tc.wantReason {
			return
		}
	}
	t.Errorf("refusal (%s, %s, %s) not found in: %+v", tc.wantAction, tc.wantObject, tc.wantReason, res.Refused)
}

// runNegativeReasonsTicketsNotComplete tests when tickets channel is missing and channels not complete.
func runNegativeReasonsTicketsNotComplete(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	raiseBotRole(t, s, "MANAGE_CHANNELS")
	s.ChannelsComplete = false
	var newCh []Channel
	for _, ch := range s.Channels {
		if ch.Name != "tickets" {
			newCh = append(newCh, ch)
		}
	}
	s.Channels = newCh

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	for _, c := range res.Changes {
		if strings.Contains(c.Object, "tickets") {
			t.Errorf("unexpected change for tickets: %+v", c)
		}
	}
	for _, ref := range res.Refused {
		if strings.Contains(ref.Object, "tickets") {
			t.Errorf("unexpected refusal for tickets: %+v", ref)
		}
	}
}

func TestPlan_Negative_Reasons(t *testing.T) {
	def := loadDef(t)
	var tests []planNegativeReasonCase
	tests = append(tests, planNegativeReasonsCasesPart1()...)
	tests = append(tests, planNegativeReasonsCasesPart2()...)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runPlanNegativeReasonCase(t, tc, def)
		})
	}

	t.Run("tickets missing + not complete -> NO candidate at all", func(t *testing.T) {
		runNegativeReasonsTicketsNotComplete(t, def)
	})
}

func TestPlan_Boundary_ExtrasAreLeftAlone(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	// Extra role, category, channel, entry
	s.Roles = append([]Role{{Name: "ExtraRole", Everyone: false}}, s.Roles...)
	s.Categories = append(s.Categories, Category{Name: "ExtraCategory"})
	s.Channels = append(s.Channels, Channel{Name: "extra-channel", Type: "text", Category: "Start"})
	s.Channels[1].Access = append(s.Channels[1].Access, Access{Role: "ExtraRole", Allow: []string{"VIEW_CHANNEL"}})

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) == 0 {
		t.Fatalf("expected differences reported for extras, got 0")
	}

	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(res.Changes) != 0 {
		t.Errorf("len(Changes) = %d, want 0: %+v", len(res.Changes), res.Changes)
	}
	if len(res.Refused) != 0 {
		t.Errorf("len(Refused) = %d, want 0: %+v", len(res.Refused), res.Refused)
	}
}

func TestPlan_Boundary_ClosedActionList(t *testing.T) {
	actions := Actions()
	if len(actions) != 10 {
		t.Fatalf("len(Actions()) = %d, want 10", len(actions))
	}
	actionSet := make(map[string]struct{}, len(actions))
	for _, a := range actions {
		if strings.Contains(a, "delete") || strings.Contains(a, "remove") {
			t.Errorf("action %q contains delete/remove", a)
		}
		actionSet[a] = struct{}{}
	}

	def := loadDef(t)
	s := raisedFirstSetupSnapshot()
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	for _, c := range res.Changes {
		if _, ok := actionSet[c.Action]; !ok {
			t.Errorf("change action %q not in Actions()", c.Action)
		}
	}
	for _, ref := range res.Refused {
		if _, ok := actionSet[ref.Action]; !ok {
			t.Errorf("refusal action %q not in Actions()", ref.Action)
		}
	}
}

func deepCopy[T any](t *testing.T, src T) T {
	t.Helper()
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal deepCopy: %v", err)
	}
	var dst T
	if err := json.Unmarshal(data, &dst); err != nil {
		t.Fatalf("unmarshal deepCopy: %v", err)
	}
	return dst
}

func TestPlan_Boundary_DeterministicAndPure(t *testing.T) {
	def := loadDef(t)
	s := raisedFirstSetupSnapshot()

	defBefore := deepCopy(t, def)
	sBefore := deepCopy(t, s)

	res1, err1 := Plan(def, s)
	if err1 != nil {
		t.Fatalf("Plan 1 failed: %v", err1)
	}
	res2, err2 := Plan(def, s)
	if err2 != nil {
		t.Fatalf("Plan 2 failed: %v", err2)
	}

	if !reflect.DeepEqual(res1, res2) {
		t.Errorf("Plan results differ between invocations")
	}
	if !reflect.DeepEqual(def, defBefore) {
		t.Errorf("Plan mutated def")
	}
	if !reflect.DeepEqual(s, sBefore) {
		t.Errorf("Plan mutated snapshot")
	}
}
