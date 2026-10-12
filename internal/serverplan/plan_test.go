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

func TestPlan_Positive_OwnModeAreaEntry(t *testing.T) {
	def := loadDef(t)

	t.Run("home-lab / interest-home-lab", func(t *testing.T) {
		s := snapshotOf(t, def)
		// remove entry of interest-home-lab from home-lab
		for i := range s.Channels {
			if s.Channels[i].Name == "home-lab" {
				var newAccess []Access
				for _, acc := range s.Channels[i].Access {
					if acc.Role != "Interest: home lab and self-hosting" {
						newAccess = append(newAccess, acc)
					}
				}
				s.Channels[i].Access = newAccess
			}
		}

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
	})

	t.Run("deutsch / lang-de", func(t *testing.T) {
		s := snapshotOf(t, def)
		// remove entry of lang-de from deutsch
		for i := range s.Channels {
			if s.Channels[i].Name == "deutsch" {
				var newAccess []Access
				for _, acc := range s.Channels[i].Access {
					if acc.Role != "Language: Deutsch" {
						newAccess = append(newAccess, acc)
					}
				}
				s.Channels[i].Access = newAccess
			}
		}

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
	})
}

func TestPlan_Negative_OwnModeAnythingElse(t *testing.T) {
	baseDef := loadDef(t)

	tests := []struct {
		name       string
		setup      func() (*serverdef.Definition, *Snapshot)
		wantObject string
	}{
		{
			name: "entry of moderator on home-lab removed",
			setup: func() (*serverdef.Definition, *Snapshot) {
				s := snapshotOf(t, baseDef)
				for i := range s.Channels {
					if s.Channels[i].Name == "home-lab" {
						var newAccess []Access
						for _, acc := range s.Channels[i].Access {
							if acc.Role != "Moderator" {
								newAccess = append(newAccess, acc)
							}
						}
						s.Channels[i].Access = newAccess
					}
				}
				return baseDef, s
			},
			wantObject: "channels[home-lab].access[moderator]",
		},
		{
			name: "entry on lounge differs",
			setup: func() (*serverdef.Definition, *Snapshot) {
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
			},
			wantObject: "channels[lounge].access[everyone]",
		},
		{
			name: "a role missing",
			setup: func() (*serverdef.Definition, *Snapshot) {
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
		{
			name: "a category missing",
			setup: func() (*serverdef.Definition, *Snapshot) {
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
			setup: func() (*serverdef.Definition, *Snapshot) {
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
			setup: func() (*serverdef.Definition, *Snapshot) {
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
			setup: func() (*serverdef.Definition, *Snapshot) {
				s := snapshotOf(t, baseDef)
				// Contributor (index 5) and Supporter (index 6) swapped
				s.Roles[5], s.Roles[6] = s.Roles[6], s.Roles[5]
				return baseDef, s
			},
			wantObject: "role_order",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def, s := tc.setup()
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
			found := false
			for _, ref := range res.Refused {
				if ref.Reason == ReasonOutsideOwnChanges && ref.Object == tc.wantObject {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected refusal with reason outside-own-changes and object %q, got: %+v", tc.wantObject, res.Refused)
			}
		})
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

	// Verify order: create-role before set-role-order before create-category before create-channel
	phase := 1 // 1: create-role, 2: set-role-order, 3: create-category, 4: create-channel
	for i, c := range res.Changes {
		if c.Step != i+1 {
			t.Errorf("step %d != index %d", c.Step, i+1)
		}
		if c.Object == "roles[bot]" || c.Object == "roles[operator]" {
			t.Errorf("unexpected change for bot/operator role: %+v", c)
		}
		if c.Action == ActionSetAccess {
			t.Errorf("unexpected set-access in first setup: %+v", c)
		}

		switch c.Action {
		case ActionCreateRole:
			if phase > 1 {
				t.Errorf("create-role in phase %d", phase)
			}
		case ActionSetRoleOrder:
			if phase > 2 {
				t.Errorf("set-role-order in phase %d", phase)
			}
			phase = 2
		case ActionCreateCategory:
			if phase > 3 {
				t.Errorf("create-category in phase %d", phase)
			}
			phase = 3
		case ActionCreateChannel:
			phase = 4
		default:
			t.Errorf("unexpected action %q", c.Action)
		}
	}

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
	rights := res.Rights()
	if !slices.Equal(rights, wantRights) {
		t.Errorf("Rights() = %v, want %v", rights, wantRights)
	}

}

func TestPlan_Negative_RaisedLacksRight(t *testing.T) {
	def := loadDef(t)

	t.Run("without Operator role -> create-role roles[operator] refused lacks-right ADMINISTRATOR", func(t *testing.T) {
		s := raisedFirstSetupSnapshot()
		// Remove Operator role so it is missing
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
	})

	t.Run("caller with ADMINISTRATOR -> 0 refused", func(t *testing.T) {
		s := raisedFirstSetupSnapshot()
		s.Roles = s.Roles[1:] // missing operator role
		s.Caller.Permissions = append(s.Caller.Permissions, "ADMINISTRATOR")

		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		if len(res.Refused) != 0 {
			t.Fatalf("expected 0 refused, got %d: %+v", len(res.Refused), res.Refused)
		}
	})

	t.Run("caller without MANAGE_CHANNELS -> every create-category and create-channel refused lacks-right", func(t *testing.T) {
		s := raisedFirstSetupSnapshot()
		// Remove MANAGE_CHANNELS from caller permissions
		var newPerms []string
		for _, p := range s.Caller.Permissions {
			if p != "MANAGE_CHANNELS" {
				newPerms = append(newPerms, p)
			}
		}
		s.Caller.Permissions = newPerms

		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		for _, c := range res.Changes {
			if c.Action == ActionCreateCategory || c.Action == ActionCreateChannel {
				t.Errorf("unexpected accepted channel/category change: %+v", c)
			}
		}
		for _, ref := range res.Refused {
			if ref.Action == ActionCreateCategory || ref.Action == ActionCreateChannel {
				if ref.Reason != ReasonLacksRight || !strings.Contains(ref.Detail, "MANAGE_CHANNELS") {
					t.Errorf("refusal for %s not lacks-right MANAGE_CHANNELS: %+v", ref.Object, ref)
				}
			}
		}
	})
}

func TestPlan_Boundary_OnePermissionShort(t *testing.T) {
	def := loadDef(t)

	t.Run("caller without CONNECT -> create-channel channels[events-voice] lacks-right CONNECT", func(t *testing.T) {
		s := raisedFirstSetupSnapshot()
		var newPerms []string
		for _, p := range s.Caller.Permissions {
			if p != "CONNECT" {
				newPerms = append(newPerms, p)
			}
		}
		s.Caller.Permissions = newPerms

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
	})

	t.Run("caller without CREATE_PRIVATE_THREADS -> create-channel channels[tickets] lacks-right CREATE_PRIVATE_THREADS", func(t *testing.T) {
		s := raisedFirstSetupSnapshot()
		var newPerms []string
		for _, p := range s.Caller.Permissions {
			if p != "CREATE_PRIVATE_THREADS" {
				newPerms = append(newPerms, p)
			}
		}
		s.Caller.Permissions = newPerms

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
	})
}

func TestPlan_Boundary_NeighboursOfTheCaller(t *testing.T) {
	def := loadDef(t)

	t.Run("role directly below caller with wrong permissions -> accepted", func(t *testing.T) {
		s := snapshotOf(t, def)
		raiseBotRole(t, s, "MANAGE_CHANNELS")
		roleNamed(t, s, "Contributor").Permissions = []string{"VIEW_AUDIT_LOG"}

		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		found := false
		for _, c := range res.Changes {
			if c.Action == ActionSetRolePermissions && c.Object == "roles[contributor]" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected set-role-permissions roles[contributor] in changes: %+v", res.Changes)
		}
	})

	t.Run("role directly above caller with wrong permissions -> refused above-caller", func(t *testing.T) {
		s := snapshotOf(t, def)
		raiseBotRole(t, s, "MANAGE_CHANNELS")
		roleNamed(t, s, "Subject guide: video").Permissions = []string{"VIEW_AUDIT_LOG"}

		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		found := false
		for _, ref := range res.Refused {
			if ref.Action == ActionSetRolePermissions && ref.Object == "roles[guide-video]" && ref.Reason == ReasonAboveCaller {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected refusal above-caller for roles[guide-video]: %+v", res.Refused)
		}
	})

	t.Run("two roles below caller swapped -> set-role-order accepted", func(t *testing.T) {
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
		found := false
		for _, c := range res.Changes {
			if c.Action == ActionSetRoleOrder {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected set-role-order in changes: %+v", res.Changes)
		}
	})

	t.Run("two roles above caller swapped -> set-role-order refused above-caller", func(t *testing.T) {
		s := snapshotOf(t, def)
		raiseBotRole(t, s, "MANAGE_CHANNELS")
		// Swap operator (0) and moderator (1)
		s.Roles[0], s.Roles[1] = s.Roles[1], s.Roles[0]

		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		found := false
		for _, ref := range res.Refused {
			if ref.Action == ActionSetRoleOrder && ref.Reason == ReasonAboveCaller {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected refusal above-caller for set-role-order: %+v", res.Refused)
		}
	})
}

func TestPlan_Negative_Reasons(t *testing.T) {
	def := loadDef(t)

	tests := []struct {
		name       string
		setup      func() *Snapshot
		wantAction string
		wantObject string
		wantReason string
		wantDetail string
	}{
		{
			name: "announcements missing + no NEWS -> missing-feature",
			setup: func() *Snapshot {
				s := snapshotOf(t, def)
				raiseBotRole(t, s, "MANAGE_CHANNELS")
				s.Features = []string{"COMMUNITY"} // lacks NEWS
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
			setup: func() *Snapshot {
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
			setup: func() *Snapshot {
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
		{
			name: "two roles named Moderator -> resolve-name ambiguous-name",
			setup: func() *Snapshot {
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
			setup: func() *Snapshot {
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
			setup: func() *Snapshot {
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.setup()
			res, err := Plan(def, s)
			if err != nil {
				t.Fatalf("Plan failed: %v", err)
			}
			found := false
			for _, ref := range res.Refused {
				if ref.Action == tc.wantAction && ref.Object == tc.wantObject && ref.Reason == tc.wantReason {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("refusal (%s, %s, %s) not found in: %+v", tc.wantAction, tc.wantObject, tc.wantReason, res.Refused)
			}
		})
	}

	t.Run("tickets missing + not complete -> NO candidate at all", func(t *testing.T) {
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
