package serverplan

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

func loadDef(t *testing.T) *serverdef.Definition {
	t.Helper()
	data, err := os.ReadFile("../../server/definition.json")
	if err != nil {
		t.Fatalf("read definition: %v", err)
	}
	def, err := serverdef.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("load definition: %v", err)
	}
	return def
}

// boolVal returns the dereferenced boolean pointer value or false if nil.
func boolVal(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// findSpecialRoles indexes definition roles and identifies bot and everyone roles.
func findSpecialRoles(t *testing.T, def *serverdef.Definition) (map[string]serverdef.Role, *serverdef.Role, *serverdef.Role) {
	t.Helper()
	var botRole, everyoneRole *serverdef.Role
	rolesByID := make(map[string]serverdef.Role, len(def.Roles))
	for i := range def.Roles {
		rolesByID[def.Roles[i].ID] = def.Roles[i]
		if def.Roles[i].Kind == "bot" {
			botRole = &def.Roles[i]
		}
		if def.Roles[i].Kind == "everyone" {
			everyoneRole = &def.Roles[i]
		}
	}
	if botRole == nil || everyoneRole == nil {
		t.Fatalf("missing bot or everyone role in definition")
	}
	return rolesByID, botRole, everyoneRole
}

// buildSnapshotRoles converts definition roles into the snapshot role slice.
func buildSnapshotRoles(t *testing.T, def *serverdef.Definition, rolesByID map[string]serverdef.Role, everyoneRole *serverdef.Role) []Role {
	t.Helper()
	snapRoles := make([]Role, 0, len(def.RoleOrder)+4)
	for _, id := range def.RoleOrder {
		r, ok := rolesByID[id]
		if !ok {
			t.Fatalf("role_order id %q not in roles", id)
		}
		snapRoles = append(snapRoles, Role{
			Name:        r.Name,
			Everyone:    false,
			Managed:     r.Kind == "bot",
			ShownApart:  boolVal(r.ShownApart),
			Mentionable: boolVal(r.Mentionable),
			Permissions: append([]string(nil), r.Permissions...),
		})
	}
	for i := range def.Roles {
		r := &def.Roles[i]
		if r.Kind == "managed" {
			snapRoles = append(snapRoles, Role{
				Name:        r.Name,
				Everyone:    false,
				Managed:     true,
				ShownApart:  boolVal(r.ShownApart),
				Mentionable: boolVal(r.Mentionable),
				Permissions: append([]string(nil), r.Permissions...),
			})
		}
	}
	snapRoles = append(snapRoles, Role{
		Name:        everyoneRole.Name,
		Everyone:    true,
		Managed:     false,
		Permissions: append([]string(nil), everyoneRole.Permissions...),
	})
	return snapRoles
}

// buildSnapshotCategoriesAndChannels converts definition categories and channels.
func buildSnapshotCategoriesAndChannels(def *serverdef.Definition, rolesByID map[string]serverdef.Role) ([]Category, []Channel) {
	cats := make([]Category, 0, len(def.Categories))
	catsByID := make(map[string]string, len(def.Categories))
	for _, c := range def.Categories {
		cats = append(cats, Category{Name: c.Name})
		catsByID[c.ID] = c.Name
	}
	channels := make([]Channel, 0, len(def.Channels))
	for _, ch := range def.Channels {
		catName := catsByID[ch.Category]
		access := make([]Access, 0, len(ch.Access))
		for _, acc := range ch.Access {
			rName := rolesByID[acc.Role].Name
			access = append(access, Access{
				Role:  rName,
				Allow: append([]string(nil), acc.Allow...),
				Deny:  append([]string(nil), acc.Deny...),
			})
		}
		channels = append(channels, Channel{
			Name:     ch.Name,
			Type:     ch.Type,
			Category: catName,
			Access:   access,
		})
	}
	return cats, channels
}

func snapshotOf(t *testing.T, def *serverdef.Definition) *Snapshot {
	t.Helper()
	rolesByID, botRole, everyoneRole := findSpecialRoles(t, def)
	snapRoles := buildSnapshotRoles(t, def, rolesByID, everyoneRole)

	callerPerms := append([]string(nil), botRole.Permissions...)
	callerPerms = append(callerPerms, everyoneRole.Permissions...)

	cats, channels := buildSnapshotCategoriesAndChannels(def, rolesByID)

	level, rules := serverSettingsOf(t, def)
	return &Snapshot{
		VerificationLevel: level,
		FilterRulesRead:   true,
		FilterRules:       rules,
		Features:          []string{"COMMUNITY", "NEWS"},
		Caller: Caller{
			Role: botRole.Name,
			// Numbered without gaps: one more than the roles below the bot's.
			Position:    len(snapRoles) - 1 - slices.IndexFunc(snapRoles, func(r Role) bool { return r.Name == botRole.Name }),
			Permissions: callerPerms,
		},
		Roles:            snapRoles,
		Categories:       cats,
		Channels:         channels,
		ChannelsComplete: true,
	}
}

func TestCompare_Positive_ServerAsDefined(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 0 {
		t.Fatalf("expected 0 differences, got %d: %+v", len(diffs), diffs)
	}
}

type compareNegativeKindCase struct {
	name       string
	mutate     func(*Snapshot)
	wantKind   string
	wantObject string
	wantWord   string
}

// renameSnapshotRole renames a role and updates references across snapshot channels.
func renameSnapshotRole(s *Snapshot, oldName, newName string) {
	for i := range s.Roles {
		if s.Roles[i].Name == oldName {
			s.Roles[i].Name = newName
		}
	}
	s.Caller.Role = newName
	for i := range s.Channels {
		for j := range s.Channels[i].Access {
			if s.Channels[i].Access[j].Role == oldName {
				s.Channels[i].Access[j].Role = newName
			}
		}
	}
}

// updateEveryoneAccessOnRules applies a mutate function to @everyone access on the rules channel.
func updateEveryoneAccessOnRules(s *Snapshot, fn func(a *Access)) {
	for i := range s.Channels {
		if s.Channels[i].Name != "rules" {
			continue
		}
		for j := range s.Channels[i].Access {
			if s.Channels[i].Access[j].Role == "@everyone" {
				fn(&s.Channels[i].Access[j])
			}
		}
	}
}

// filterOutChannel returns channels excluding the specified channel name.
func filterOutChannel(channels []Channel, name string) []Channel {
	newCh := make([]Channel, 0, len(channels)-1)
	for _, ch := range channels {
		if ch.Name != name {
			newCh = append(newCh, ch)
		}
	}
	return newCh
}

// setRolePermissions updates the permissions of a named role in the snapshot.
func setRolePermissions(s *Snapshot, roleName string, perms []string) {
	for i := range s.Roles {
		if s.Roles[i].Name == roleName {
			s.Roles[i].Permissions = perms
		}
	}
}

// compareNegativeKindRoleBasicCases returns negative role test cases for missing, ambiguous, and renamed roles.
func compareNegativeKindRoleBasicCases() []compareNegativeKindCase {
	return []compareNegativeKindCase{
		{
			name: "role-missing",
			mutate: func(s *Snapshot) {
				newRoles := make([]Role, 0, len(s.Roles)-1)
				for _, r := range s.Roles {
					if r.Name != "Subject guide: coding" {
						newRoles = append(newRoles, r)
					}
				}
				s.Roles = newRoles
			},
			wantKind:   KindRoleMissing,
			wantObject: "roles[guide-coding]",
			wantWord:   "",
		},
		{
			name: "role-ambiguous",
			mutate: func(s *Snapshot) {
				for i, r := range s.Roles {
					if r.Name == "Subject guide: coding" {
						s.Roles = append(s.Roles[:i+1], append([]Role{r}, s.Roles[i+1:]...)...)
						break
					}
				}
			},
			wantKind:   KindRoleAmbiguous,
			wantObject: "roles[guide-coding]",
			wantWord:   "2",
		},
		{
			name: "role-name",
			mutate: func(s *Snapshot) {
				renameSnapshotRole(s, "Diallak", "RenamedBot")
			},
			wantKind:   KindRoleName,
			wantObject: "roles[bot]",
			wantWord:   "RenamedBot",
		},
	}
}

// compareNegativeKindRolePermsCases returns negative role test cases for permissions and ordering.
func compareNegativeKindRolePermsCases() []compareNegativeKindCase {
	return []compareNegativeKindCase{
		{
			name: "role-permissions lacks",
			mutate: func(s *Snapshot) {
				setRolePermissions(s, "Diallak", []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS"})
			},
			wantKind:   KindRolePermissions,
			wantObject: "roles[bot]",
			wantWord:   "lacks",
		},
		{
			name: "role-permissions beyond",
			mutate: func(s *Snapshot) {
				setRolePermissions(s, "Subject guide: coding", []string{"VIEW_AUDIT_LOG"})
			},
			wantKind:   KindRolePermissions,
			wantObject: "roles[guide-coding]",
			wantWord:   "beyond",
		},
		{
			name: "role-order",
			mutate: func(s *Snapshot) {
				// Swap guide-coding (index 2) and guide-video (index 3)
				s.Roles[2], s.Roles[3] = s.Roles[3], s.Roles[2]
			},
			wantKind:   KindRoleOrder,
			wantObject: "role_order",
			wantWord:   "server:",
		},
	}
}

// compareNegativeKindCategoryCases returns negative category test cases for Compare.
func compareNegativeKindCategoryCases() []compareNegativeKindCase {
	return []compareNegativeKindCase{
		{
			name: "category-missing",
			mutate: func(s *Snapshot) {
				newCats := make([]Category, 0, len(s.Categories)-1)
				for _, c := range s.Categories {
					if c.Name != "Staff" {
						newCats = append(newCats, c)
					}
				}
				s.Categories = newCats
			},
			wantKind:   KindCategoryMissing,
			wantObject: "categories[staff]",
			wantWord:   "",
		},
		{
			name: "category-ambiguous",
			mutate: func(s *Snapshot) {
				s.Categories = append(s.Categories, Category{Name: "Staff"})
			},
			wantKind:   KindCategoryAmbiguous,
			wantObject: "categories[staff]",
			wantWord:   "2",
		},
	}
}

// compareNegativeKindChannelCases returns negative channel test cases for Compare.
func compareNegativeKindChannelCases() []compareNegativeKindCase {
	return []compareNegativeKindCase{
		{
			name: "channel-missing",
			mutate: func(s *Snapshot) {
				s.Channels = filterOutChannel(s.Channels, "rules")
			},
			wantKind:   KindChannelMissing,
			wantObject: "channels[rules]",
			wantWord:   "",
		},
		{
			name: "channel-ambiguous",
			mutate: func(s *Snapshot) {
				s.Channels = append(s.Channels, Channel{
					Name:     "rules",
					Type:     "text",
					Category: "Start",
				})
			},
			wantKind:   KindChannelAmbiguous,
			wantObject: "channels[rules]",
			wantWord:   "2",
		},
		{
			name: "channel-type",
			mutate: func(s *Snapshot) {
				for i := range s.Channels {
					if s.Channels[i].Name == "rules" {
						s.Channels[i].Type = "voice"
					}
				}
			},
			wantKind:   KindChannelType,
			wantObject: "channels[rules]",
			wantWord:   "voice",
		},
		{
			name: "channel-category",
			mutate: func(s *Snapshot) {
				for i := range s.Channels {
					if s.Channels[i].Name == "rules" {
						s.Channels[i].Category = "Community"
					}
				}
			},
			wantKind:   KindChannelCategory,
			wantObject: "channels[rules]",
			wantWord:   "Community",
		},
	}
}

// compareNegativeKindAccessCases returns negative access test cases for Compare.
func compareNegativeKindAccessCases() []compareNegativeKindCase {
	return []compareNegativeKindCase{
		{
			name: "access-missing",
			mutate: func(s *Snapshot) {
				for i := range s.Channels {
					if s.Channels[i].Name == "rules" {
						s.Channels[i].Access = []Access{} // removes @everyone access
					}
				}
			},
			wantKind:   KindAccessMissing,
			wantObject: "channels[rules].access[everyone]",
			wantWord:   "",
		},
		{
			name: "access-differs allow",
			mutate: func(s *Snapshot) {
				updateEveryoneAccessOnRules(s, func(a *Access) {
					a.Allow = append(a.Allow, "SEND_MESSAGES")
				})
			},
			wantKind:   KindAccessDiffers,
			wantObject: "channels[rules].access[everyone]",
			wantWord:   "allow",
		},
		{
			name: "access-differs deny",
			mutate: func(s *Snapshot) {
				updateEveryoneAccessOnRules(s, func(a *Access) {
					a.Deny = []string{"ATTACH_FILES"}
				})
			},
			wantKind:   KindAccessDiffers,
			wantObject: "channels[rules].access[everyone]",
			wantWord:   "deny",
		},
		{
			name: "access-extra",
			mutate: func(s *Snapshot) {
				for i := range s.Channels {
					if s.Channels[i].Name == "rules" {
						s.Channels[i].Access = append(s.Channels[i].Access, Access{
							Role:  "Operator",
							Allow: []string{"VIEW_CHANNEL"},
							Deny:  []string{},
						})
					}
				}
			},
			wantKind:   KindAccessExtra,
			wantObject: "channels[rules].access[Operator]",
			wantWord:   "",
		},
	}
}

// compareNegativeKindExtraCases returns negative extra entity test cases for Compare.
func compareNegativeKindExtraCases() []compareNegativeKindCase {
	return []compareNegativeKindCase{
		{
			name: "role-extra",
			mutate: func(s *Snapshot) {
				extra := Role{Name: "ExtraRole", Everyone: false, Managed: false, Permissions: []string{}}
				s.Roles = append([]Role{extra}, s.Roles...)
			},
			wantKind:   KindRoleExtra,
			wantObject: "server.roles[ExtraRole]",
			wantWord:   "",
		},
		{
			name: "role-extra managed",
			mutate: func(s *Snapshot) {
				extra := Role{Name: "ExtraManaged", Everyone: false, Managed: true, Permissions: []string{}}
				s.Roles = append([]Role{extra}, s.Roles...)
			},
			wantKind:   KindRoleExtra,
			wantObject: "server.roles[ExtraManaged]",
			wantWord:   "managed",
		},
		{
			name: "category-extra",
			mutate: func(s *Snapshot) {
				s.Categories = append(s.Categories, Category{Name: "ExtraCat"})
			},
			wantKind:   KindCategoryExtra,
			wantObject: "server.categories[ExtraCat]",
			wantWord:   "",
		},
		{
			name: "channel-extra",
			mutate: func(s *Snapshot) {
				s.Channels = append(s.Channels, Channel{
					Name:     "extra-chan",
					Type:     "text",
					Category: "Start",
				})
			},
			wantKind:   KindChannelExtra,
			wantObject: "server.channels[extra-chan]",
			wantWord:   "",
		},
		{
			name: "feature-missing",
			mutate: func(s *Snapshot) {
				s.Features = []string{"COMMUNITY"} // lacks NEWS
			},
			wantKind:   KindFeatureMissing,
			wantObject: "features[NEWS]",
			wantWord:   "",
		},
	}
}

// assertCompareNegativeKindDiff verifies the single difference matches expected kind, object, and word.
func assertCompareNegativeKindDiff(t *testing.T, d Difference, tc compareNegativeKindCase) {
	t.Helper()
	if d.Kind != tc.wantKind {
		t.Errorf("Kind = %q, want %q", d.Kind, tc.wantKind)
	}
	if d.Object != tc.wantObject {
		t.Errorf("Object = %q, want %q", d.Object, tc.wantObject)
	}
	if tc.wantWord != "" && !strings.Contains(d.Detail, tc.wantWord) {
		t.Errorf("Detail %q does not contain word %q", d.Detail, tc.wantWord)
	}
}

// runBoundaryVisibilityTicketsMissing verifies missing channel diff when channels are complete.
func runBoundaryVisibilityTicketsMissing(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	s.ChannelsComplete = true
	s.Channels = filterOutChannel(s.Channels, "tickets")

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	if diffs[0].Kind != KindChannelMissing || diffs[0].Object != "channels[tickets]" {
		t.Errorf("unexpected diff: %+v", diffs[0])
	}
}

// runBoundaryVisibilityTicketsHidden verifies channel-not-visible when tickets channel is hidden.
func runBoundaryVisibilityTicketsHidden(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	s.ChannelsComplete = false
	s.Channels = filterOutChannel(s.Channels, "tickets")

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	d := diffs[0]
	if d.Kind != KindChannelNotVisible || d.Object != "channels[tickets]" {
		t.Errorf("unexpected diff: %+v", d)
	}
	if d.Detail != "hidden from the bot by the definition" {
		t.Errorf("Detail = %q, want 'hidden from the bot by the definition'", d.Detail)
	}
}

// runBoundaryVisibilityLoungeHidden verifies channel-not-visible when lounge is missing or hidden.
func runBoundaryVisibilityLoungeHidden(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	s.ChannelsComplete = false
	s.Channels = filterOutChannel(s.Channels, "lounge")

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(diffs), diffs)
	}
	d := diffs[0]
	if d.Kind != KindChannelNotVisible || d.Object != "channels[lounge]" {
		t.Errorf("unexpected diff: %+v", d)
	}
	if d.Detail != "missing or hidden" {
		t.Errorf("Detail = %q, want 'missing or hidden'", d.Detail)
	}
}

// runBoundaryVisibilityEmptyAccessEntry verifies that empty allow/deny access entries produce no diffs.
func runBoundaryVisibilityEmptyAccessEntry(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	s := snapshotOf(t, def)
	s.Channels[0].Access = append(s.Channels[0].Access, Access{
		Role:  "ExtraRole",
		Allow: []string{},
		Deny:  []string{},
	})
	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 0 {
		t.Fatalf("expected 0 differences, got %d: %+v", len(diffs), diffs)
	}
}

func TestCompare_Negative_EachKind(t *testing.T) {
	def := loadDef(t)
	tests := append(compareNegativeKindRoleBasicCases(), compareNegativeKindRolePermsCases()...)
	tests = append(tests, compareNegativeKindCategoryCases()...)
	tests = append(tests, compareNegativeKindChannelCases()...)
	tests = append(tests, compareNegativeKindAccessCases()...)
	tests = append(tests, compareNegativeKindExtraCases()...)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := snapshotOf(t, def)
			tc.mutate(s)
			diffs, err := Compare(def, s)
			if err != nil {
				t.Fatalf("Compare failed: %v", err)
			}
			if len(diffs) != 1 {
				t.Fatalf("expected exactly 1 difference, got %d: %+v", len(diffs), diffs)
			}
			assertCompareNegativeKindDiff(t, diffs[0], tc)
		})
	}
}

func TestCompare_Boundary_Visibility(t *testing.T) {
	def := loadDef(t)

	t.Run("tickets removed + complete -> channel-missing", func(t *testing.T) {
		runBoundaryVisibilityTicketsMissing(t, def)
	})

	t.Run("tickets removed + not complete -> channel-not-visible hidden from the bot", func(t *testing.T) {
		runBoundaryVisibilityTicketsHidden(t, def)
	})

	t.Run("lounge removed + not complete -> channel-not-visible missing or hidden", func(t *testing.T) {
		runBoundaryVisibilityLoungeHidden(t, def)
	})

	t.Run("entry with empty allow and deny added -> 0 differences", func(t *testing.T) {
		runBoundaryVisibilityEmptyAccessEntry(t, def)
	})
}

func TestCompare_Negative_RaisedCallerIsReported(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	// caller role given MANAGE_CHANNELS and moved to the top
	var botRole Role
	var remainingRoles []Role
	for _, r := range s.Roles {
		if r.Name == "Diallak" {
			botRole = r
		} else {
			remainingRoles = append(remainingRoles, r)
		}
	}
	botRole.Permissions = append(botRole.Permissions, "MANAGE_CHANNELS")
	s.Roles = append([]Role{botRole}, remainingRoles...)
	s.Caller.Permissions = append(s.Caller.Permissions, "MANAGE_CHANNELS")

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 differences, got %d: %+v", len(diffs), diffs)
	}

	// 1: role-permissions roles[bot] "beyond MANAGE_CHANNELS"
	// 2: role-order
	d0 := diffs[0]
	if d0.Kind != KindRolePermissions || d0.Object != "roles[bot]" || !strings.Contains(d0.Detail, "beyond MANAGE_CHANNELS") {
		t.Errorf("diff[0] = %+v, want role-permissions roles[bot] beyond MANAGE_CHANNELS", d0)
	}
	d1 := diffs[1]
	if d1.Kind != KindRoleOrder || d1.Object != "role_order" {
		t.Errorf("diff[1] = %+v, want role-order role_order", d1)
	}
}

func TestCompare_Negative_Refuses(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)

	t.Run("definition with a finding -> error naming count", func(t *testing.T) {
		invalidDef := *def
		invalidDef.Version = 99
		_, err := Compare(&invalidDef, s)
		if err == nil {
			t.Fatal("expected error for invalid definition, got nil")
		}
		if !strings.Contains(err.Error(), "1") {
			t.Errorf("error %q should name the finding count", err.Error())
		}
	})

	t.Run("snapshot failing Check -> error", func(t *testing.T) {
		invalidSnap := *s
		invalidSnap.Roles = nil
		_, err := Compare(def, &invalidSnap)
		if err == nil {
			t.Fatal("expected error for invalid snapshot, got nil")
		}
	})
}
