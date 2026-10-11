package serverplan

import (
	"bytes"
	"os"
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

func snapshotOf(t *testing.T, def *serverdef.Definition) *Snapshot {
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

	snapRoles := make([]Role, 0, len(def.RoleOrder)+1)
	for _, id := range def.RoleOrder {
		r, ok := rolesByID[id]
		if !ok {
			t.Fatalf("role_order id %q not in roles", id)
		}
		snapRoles = append(snapRoles, Role{
			Name:        r.Name,
			Everyone:    false,
			Managed:     r.Kind == "bot",
			Permissions: append([]string(nil), r.Permissions...),
		})
	}
	snapRoles = append(snapRoles, Role{
		Name:        everyoneRole.Name,
		Everyone:    true,
		Managed:     false,
		Permissions: append([]string(nil), everyoneRole.Permissions...),
	})

	callerPerms := append([]string(nil), botRole.Permissions...)
	callerPerms = append(callerPerms, everyoneRole.Permissions...)

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

	return &Snapshot{
		Features: []string{"COMMUNITY", "NEWS"},
		Caller: Caller{
			Role:        botRole.Name,
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

func TestCompare_Negative_EachKind(t *testing.T) {
	def := loadDef(t)

	tests := []struct {
		name       string
		mutate     func(*Snapshot)
		wantKind   string
		wantObject string
		wantWord   string
	}{
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
				for i := range s.Roles {
					if s.Roles[i].Name == "Diallak" {
						s.Roles[i].Name = "RenamedBot"
					}
				}
				s.Caller.Role = "RenamedBot"
				for i := range s.Channels {
					for j := range s.Channels[i].Access {
						if s.Channels[i].Access[j].Role == "Diallak" {
							s.Channels[i].Access[j].Role = "RenamedBot"
						}
					}
				}
			},
			wantKind:   KindRoleName,
			wantObject: "roles[bot]",
			wantWord:   "RenamedBot",
		},
		{
			name: "role-permissions lacks",
			mutate: func(s *Snapshot) {
				for i := range s.Roles {
					if s.Roles[i].Name == "Moderator" {
						s.Roles[i].Permissions = []string{"MANAGE_MESSAGES"} // lacks MODERATE_MEMBERS
					}
				}
			},
			wantKind:   KindRolePermissions,
			wantObject: "roles[moderator]",
			wantWord:   "lacks",
		},
		{
			name: "role-permissions beyond",
			mutate: func(s *Snapshot) {
				for i := range s.Roles {
					if s.Roles[i].Name == "Subject guide: coding" {
						s.Roles[i].Permissions = []string{"VIEW_AUDIT_LOG"}
					}
				}
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
		{
			name: "channel-missing",
			mutate: func(s *Snapshot) {
				newCh := make([]Channel, 0, len(s.Channels)-1)
				for _, ch := range s.Channels {
					if ch.Name != "rules" {
						newCh = append(newCh, ch)
					}
				}
				s.Channels = newCh
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
				for i := range s.Channels {
					if s.Channels[i].Name == "rules" {
						for j := range s.Channels[i].Access {
							if s.Channels[i].Access[j].Role == "@everyone" {
								s.Channels[i].Access[j].Allow = append(s.Channels[i].Access[j].Allow, "SEND_MESSAGES")
							}
						}
					}
				}
			},
			wantKind:   KindAccessDiffers,
			wantObject: "channels[rules].access[everyone]",
			wantWord:   "allow",
		},
		{
			name: "access-differs deny",
			mutate: func(s *Snapshot) {
				for i := range s.Channels {
					if s.Channels[i].Name == "rules" {
						for j := range s.Channels[i].Access {
							if s.Channels[i].Access[j].Role == "@everyone" {
								s.Channels[i].Access[j].Deny = []string{"ATTACH_FILES"}
							}
						}
					}
				}
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
			d := diffs[0]
			if d.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", d.Kind, tc.wantKind)
			}
			if d.Object != tc.wantObject {
				t.Errorf("Object = %q, want %q", d.Object, tc.wantObject)
			}
			if tc.wantWord != "" && !strings.Contains(d.Detail, tc.wantWord) {
				t.Errorf("Detail %q does not contain word %q", d.Detail, tc.wantWord)
			}
		})
	}
}

func TestCompare_Boundary_Visibility(t *testing.T) {
	def := loadDef(t)

	t.Run("tickets removed + complete -> channel-missing", func(t *testing.T) {
		s := snapshotOf(t, def)
		s.ChannelsComplete = true
		newCh := make([]Channel, 0, len(s.Channels)-1)
		for _, ch := range s.Channels {
			if ch.Name != "tickets" {
				newCh = append(newCh, ch)
			}
		}
		s.Channels = newCh

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
	})

	t.Run("tickets removed + not complete -> channel-not-visible hidden from the bot", func(t *testing.T) {
		s := snapshotOf(t, def)
		s.ChannelsComplete = false
		newCh := make([]Channel, 0, len(s.Channels)-1)
		for _, ch := range s.Channels {
			if ch.Name != "tickets" {
				newCh = append(newCh, ch)
			}
		}
		s.Channels = newCh

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
	})

	t.Run("lounge removed + not complete -> channel-not-visible missing or hidden", func(t *testing.T) {
		s := snapshotOf(t, def)
		s.ChannelsComplete = false
		newCh := make([]Channel, 0, len(s.Channels)-1)
		for _, ch := range s.Channels {
			if ch.Name != "lounge" {
				newCh = append(newCh, ch)
			}
		}
		s.Channels = newCh

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
	})

	t.Run("entry with empty allow and deny added -> 0 differences", func(t *testing.T) {
		s := snapshotOf(t, def)
		// Add empty entry to first channel
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
