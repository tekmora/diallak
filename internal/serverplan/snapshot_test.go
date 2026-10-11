package serverplan

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestLoadSnapshot_Positive_FreshServer(t *testing.T) {
	data, err := os.ReadFile("testdata/fresh-server.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	s, err := LoadSnapshot(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}
	if s.Caller.Role != "Diallak" {
		t.Errorf("Caller.Role = %q, want 'Diallak'", s.Caller.Role)
	}
	if len(s.Roles) != 2 {
		t.Errorf("len(Roles) = %d, want 2", len(s.Roles))
	}
}

func TestLoadSnapshot_Negative_Shape(t *testing.T) {
	t.Run("unknown key", func(t *testing.T) {
		input := `{"unknown_key": 123}`
		_, err := LoadSnapshot(strings.NewReader(input))
		if err == nil {
			t.Fatal("expected error for unknown key, got nil")
		}
		if !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("error %q should mention unknown field", err.Error())
		}
	})

	t.Run("key twice", func(t *testing.T) {
		input := `{"channels_complete": true, "channels_complete": false}`
		_, err := LoadSnapshot(strings.NewReader(input))
		if err == nil {
			t.Fatal("expected error for duplicate key, got nil")
		}
		if !strings.Contains(err.Error(), "channels_complete") {
			t.Errorf("error %q should name duplicate key", err.Error())
		}
	})

	t.Run("empty input", func(t *testing.T) {
		_, err := LoadSnapshot(strings.NewReader("   \n\t"))
		if err == nil {
			t.Fatal("expected error for empty input, got nil")
		}
	})
}

func validBaseSnapshot() Snapshot {
	return Snapshot{
		Features: []string{"COMMUNITY"},
		Caller: Caller{
			Role:        "Bot",
			Permissions: []string{"VIEW_CHANNEL", "SEND_MESSAGES"},
		},
		Roles: []Role{
			{Name: "Bot", Everyone: false, Managed: true, Permissions: []string{"VIEW_CHANNEL", "SEND_MESSAGES"}},
			{Name: "@everyone", Everyone: true, Managed: false, Permissions: []string{"VIEW_CHANNEL"}},
		},
		Categories: []Category{
			{Name: "General"},
		},
		Channels: []Channel{
			{
				Name:     "general",
				Type:     "text",
				Category: "General",
				Access: []Access{
					{Role: "@everyone", Allow: []string{"VIEW_CHANNEL"}, Deny: []string{}},
				},
			},
		},
		ChannelsComplete: true,
	}
}

func TestCheck_Negative_Conditions(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Snapshot)
		wantErr string
	}{
		{
			name: "no everyone role",
			mutate: func(s *Snapshot) {
				s.Roles = []Role{
					{Name: "Bot", Everyone: false, Permissions: []string{"VIEW_CHANNEL"}},
				}
			},
			wantErr: "exactly one role with everyone=true, found 0",
		},
		{
			name: "two everyone roles",
			mutate: func(s *Snapshot) {
				s.Roles = []Role{
					{Name: "Bot", Everyone: false, Permissions: []string{"VIEW_CHANNEL"}},
					{Name: "Second", Everyone: true, Permissions: []string{"VIEW_CHANNEL"}},
					{Name: "@everyone", Everyone: true, Permissions: []string{"VIEW_CHANNEL"}},
				}
			},
			wantErr: "exactly one role with everyone=true, found 2",
		},
		{
			name: "everyone not last",
			mutate: func(s *Snapshot) {
				s.Roles = []Role{
					{Name: "@everyone", Everyone: true, Permissions: []string{"VIEW_CHANNEL"}},
					{Name: "Bot", Everyone: false, Permissions: []string{"VIEW_CHANNEL"}},
				}
			},
			wantErr: "last",
		},
		{
			name: "caller role names no role",
			mutate: func(s *Snapshot) {
				s.Caller.Role = "NonExistent"
			},
			wantErr: "caller",
		},
		{
			name: "caller role names two roles",
			mutate: func(s *Snapshot) {
				s.Roles = []Role{
					{Name: "Bot", Everyone: false, Permissions: []string{"VIEW_CHANNEL"}},
					{Name: "Bot", Everyone: false, Permissions: []string{"VIEW_CHANNEL"}},
					{Name: "@everyone", Everyone: true, Permissions: []string{"VIEW_CHANNEL"}},
				}
			},
			wantErr: "caller",
		},
		{
			name: "caller role is everyone",
			mutate: func(s *Snapshot) {
				s.Caller.Role = "@everyone"
			},
			wantErr: "caller",
		},
		{
			name: "unknown permission in caller",
			mutate: func(s *Snapshot) {
				s.Caller.Permissions = []string{"UNKNOWN_PERMISSION"}
			},
			wantErr: "permission",
		},
		{
			name: "unknown permission in role",
			mutate: func(s *Snapshot) {
				s.Roles[0].Permissions = []string{"UNKNOWN_PERMISSION"}
			},
			wantErr: "permission",
		},
		{
			name: "unknown permission in entry",
			mutate: func(s *Snapshot) {
				s.Channels[0].Access[0].Allow = []string{"UNKNOWN_PERMISSION"}
			},
			wantErr: "permission",
		},
		{
			name: "permission twice",
			mutate: func(s *Snapshot) {
				s.Roles[0].Permissions = []string{"VIEW_CHANNEL", "VIEW_CHANNEL"}
			},
			wantErr: "duplicate",
		},
		{
			name: "unknown channel type",
			mutate: func(s *Snapshot) {
				s.Channels[0].Type = "stage"
			},
			wantErr: "channel type",
		},
		{
			name: "empty channel name",
			mutate: func(s *Snapshot) {
				s.Channels[0].Name = ""
			},
			wantErr: "channel name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := validBaseSnapshot()
			tc.mutate(&s)
			err := Check(&s)
			if err == nil {
				t.Fatalf("Check succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErr)) {
				t.Errorf("Check error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestCheck_Boundary_SameNameTwiceIsNoError(t *testing.T) {
	s := validBaseSnapshot()
	// Two roles with the same name (not caller, not everyone)
	s.Roles = []Role{
		{Name: "Bot", Everyone: false, Permissions: []string{"VIEW_CHANNEL"}},
		{Name: "Member", Everyone: false, Permissions: []string{}},
		{Name: "Member", Everyone: false, Permissions: []string{}},
		{Name: "@everyone", Everyone: true, Permissions: []string{"VIEW_CHANNEL"}},
	}
	// Two categories with the same name
	s.Categories = []Category{
		{Name: "Cat"},
		{Name: "Cat"},
	}
	// Two channels with the same name
	s.Channels = []Channel{
		{Name: "room", Type: "text", Category: "Cat"},
		{Name: "room", Type: "voice", Category: "Cat"},
	}
	if err := Check(&s); err != nil {
		t.Errorf("Check failed for duplicate names: %v, want nil", err)
	}
}
