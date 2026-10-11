package serverdef

import (
	"testing"
)

func sampleDefinition() *Definition {
	return &Definition{
		Version: 1,
		Roles: []Role{
			{
				ID:          "everyone",
				Kind:        "everyone",
				Permissions: []string{"VIEW_CHANNEL", "SEND_MESSAGES"},
			},
			{
				ID:          "admin-role",
				Kind:        "operator",
				Permissions: []string{"ADMINISTRATOR"},
			},
			{
				ID:          "helper",
				Kind:        "moderator",
				Permissions: []string{"READ_MESSAGE_HISTORY"},
			},
			{
				ID:          "plain-role",
				Kind:        "interest",
				Permissions: []string{},
			},
		},
		Channels: []Channel{
			{
				ID:   "standard",
				Type: "text",
				Access: []Access{
					{
						Role:  "everyone",
						Allow: []string{},
						Deny:  []string{"SEND_MESSAGES"},
					},
					{
						Role:  "helper",
						Allow: []string{"SEND_MESSAGES"},
						Deny:  []string{"READ_MESSAGE_HISTORY"},
					},
				},
			},
			{
				ID:   "hidden",
				Type: "text",
				Access: []Access{
					{
						Role:  "everyone",
						Allow: []string{},
						Deny:  []string{"VIEW_CHANNEL"},
					},
				},
			},
		},
	}
}

func TestEffective_Positive_AdministratorGetsAll(t *testing.T) {
	def := sampleDefinition()
	eff := Effective(def, "admin-role", "standard")
	all := Permissions()
	if len(eff) != len(all) {
		t.Fatalf("len(eff) = %d, want all %d permissions", len(eff), len(all))
	}
	for _, perm := range all {
		if _, ok := eff[perm]; !ok {
			t.Errorf("missing administrator permission %q", perm)
		}
	}
}

func TestEffective_Positive_EveryoneDenyThenRoleAllow(t *testing.T) {
	def := sampleDefinition()
	// In channel "standard", everyone denies SEND_MESSAGES.
	// Helper role has allow SEND_MESSAGES.
	eff := Effective(def, "helper", "standard")
	if _, ok := eff["SEND_MESSAGES"]; !ok {
		t.Errorf("helper should have SEND_MESSAGES allowed after everyone deny")
	}
}

func TestEffective_Positive_RoleDenyBeatsBase(t *testing.T) {
	def := sampleDefinition()
	// Helper has READ_MESSAGE_HISTORY in base, but "standard" channel access denies it.
	eff := Effective(def, "helper", "standard")
	if _, ok := eff["READ_MESSAGE_HISTORY"]; ok {
		t.Errorf("READ_MESSAGE_HISTORY should have been denied by helper overwrite")
	}
}

func TestEffective_Positive_NoViewChannelYieldsEmpty(t *testing.T) {
	def := sampleDefinition()
	// In channel "hidden", everyone has VIEW_CHANNEL denied.
	// Plain-role has base permissions (VIEW_CHANNEL, SEND_MESSAGES) from everyone,
	// but everyone overwrite removes VIEW_CHANNEL, so result should be completely empty.
	eff := Effective(def, "plain-role", "hidden")
	if len(eff) != 0 {
		t.Errorf("len(eff) = %d, want 0 when VIEW_CHANNEL is absent", len(eff))
	}
}

func TestEffective_Negative_NilDefinitionOrMissingRoleChannel(t *testing.T) {
	if eff := Effective(nil, "everyone", "standard"); len(eff) != 0 {
		t.Errorf("Effective(nil) = %v, want empty", eff)
	}
	def := sampleDefinition()
	// A non-existent role still inherits base permissions from everyone.
	effRole := Effective(def, "nonexistent-role", "standard")
	if len(effRole) == 0 {
		t.Errorf("Effective with unknown role in standard channel = %v, want base perms", effRole)
	}
	if eff := Effective(def, "everyone", "nonexistent-channel"); len(eff) != 0 {
		t.Errorf("Effective on nonexistent channel = %v, want empty", eff)
	}
}
