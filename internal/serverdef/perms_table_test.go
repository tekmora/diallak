package serverdef

import (
	"bytes"
	"os"
	"slices"
	"testing"
)

func TestPermissions_Positive_Properties(t *testing.T) {
	perms := Permissions()
	if len(perms) != 52 {
		t.Fatalf("len(Permissions()) = %d, want 52", len(perms))
	}

	seenNames := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		if _, ok := seenNames[p]; ok {
			t.Errorf("duplicate permission name %q", p)
		}
		seenNames[p] = struct{}{}
	}
}

func TestPermissions_Positive_EveryBitDistinctAndBit47Absent(t *testing.T) {
	perms := Permissions()
	var totalMask uint64
	for _, p := range perms {
		mask, err := PermissionMask([]string{p})
		if err != nil {
			t.Fatalf("PermissionMask(%q) error: %v", p, err)
		}
		if mask == 0 || (mask&(mask-1)) != 0 {
			t.Errorf("permission %q mask 0x%x is not a single bit", p, mask)
		}
		if totalMask&mask != 0 {
			t.Errorf("permission %q bit overlaps with another permission: mask 0x%x", p, mask)
		}
		totalMask |= mask
	}

	// Bit 47 must be absent
	bit47 := uint64(1) << 47
	if totalMask&bit47 != 0 {
		t.Errorf("bit 47 is present in total permissions mask: 0x%x", totalMask)
	}
}

func TestPermissionMask_Positive_RoundTrip(t *testing.T) {
	all := Permissions()
	mask, err := PermissionMask(all)
	if err != nil {
		t.Fatalf("PermissionMask(all) error: %v", err)
	}
	decoded, unknown := PermissionNames(mask)
	if unknown != 0 {
		t.Errorf("PermissionNames(mask) unknown = 0x%x, want 0", unknown)
	}
	if !slices.Equal(decoded, all) {
		t.Errorf("roundtrip mismatch:\ngot:  %v\nwant: %v", decoded, all)
	}
}

func TestPermissionNames_Boundary_MaskZero(t *testing.T) {
	names, unknown := PermissionNames(0)
	if len(names) != 0 {
		t.Errorf("PermissionNames(0) len = %d, want empty slice", len(names))
	}
	if unknown != 0 {
		t.Errorf("PermissionNames(0) unknown = 0x%x, want 0", unknown)
	}
}

func TestPermissionNames_Boundary_Bit47Unknown(t *testing.T) {
	bit47 := uint64(1) << 47
	names, unknown := PermissionNames(bit47)
	if len(names) != 0 {
		t.Errorf("PermissionNames(1<<47) returned names %v, want empty", names)
	}
	if unknown != bit47 {
		t.Errorf("PermissionNames(1<<47) unknown = 0x%x, want 0x%x", unknown, bit47)
	}
}

func TestPermissionNames_Boundary_Bit53Unknown(t *testing.T) {
	bit53 := uint64(1) << 53
	names, unknown := PermissionNames(bit53)
	if len(names) != 0 {
		t.Errorf("PermissionNames(1<<53) returned names %v, want empty", names)
	}
	if unknown != bit53 {
		t.Errorf("PermissionNames(1<<53) unknown = 0x%x, want 0x%x", unknown, bit53)
	}
}

func TestPermissionMask_Negative_UnknownName(t *testing.T) {
	_, err := PermissionMask([]string{"NOT_A_REAL_PERMISSION"})
	if err == nil {
		t.Fatal("PermissionMask with unknown name succeeded, want error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("NOT_A_REAL_PERMISSION")) {
		t.Errorf("error %q does not name unknown permission", err.Error())
	}
}

func TestDefinition_Positive_CommittedDefinitionValidates(t *testing.T) {
	data, err := os.ReadFile("../../server/definition.json")
	if err != nil {
		t.Fatalf("read definition fixture: %v", err)
	}
	def, err := Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load definition: %v", err)
	}
	findings := Validate(def)
	if len(findings) != 0 {
		t.Fatalf("Validate(committed definition) returned %d findings: %+v", len(findings), findings)
	}
}

type pinnedPermission struct {
	name string
	bit  uint
}

// pinnedPermissionsBase returns the first half of pinned platform permissions.
func pinnedPermissionsBase() []pinnedPermission {
	return []pinnedPermission{
		{"CREATE_INSTANT_INVITE", 0},
		{"KICK_MEMBERS", 1},
		{"BAN_MEMBERS", 2},
		{"ADMINISTRATOR", 3},
		{"MANAGE_CHANNELS", 4},
		{"MANAGE_GUILD", 5},
		{"ADD_REACTIONS", 6},
		{"VIEW_AUDIT_LOG", 7},
		{"PRIORITY_SPEAKER", 8},
		{"STREAM", 9},
		{"VIEW_CHANNEL", 10},
		{"SEND_MESSAGES", 11},
		{"SEND_TTS_MESSAGES", 12},
		{"MANAGE_MESSAGES", 13},
		{"EMBED_LINKS", 14},
		{"ATTACH_FILES", 15},
		{"READ_MESSAGE_HISTORY", 16},
		{"MENTION_EVERYONE", 17},
		{"USE_EXTERNAL_EMOJIS", 18},
		{"VIEW_GUILD_INSIGHTS", 19},
		{"CONNECT", 20},
		{"SPEAK", 21},
		{"MUTE_MEMBERS", 22},
		{"DEAFEN_MEMBERS", 23},
		{"MOVE_MEMBERS", 24},
		{"USE_VAD", 25},
	}
}

// pinnedPermissionsExtended returns the second half of pinned platform permissions.
func pinnedPermissionsExtended() []pinnedPermission {
	return []pinnedPermission{
		{"CHANGE_NICKNAME", 26},
		{"MANAGE_NICKNAMES", 27},
		{"MANAGE_ROLES", 28},
		{"MANAGE_WEBHOOKS", 29},
		{"MANAGE_GUILD_EXPRESSIONS", 30},
		{"USE_APPLICATION_COMMANDS", 31},
		{"REQUEST_TO_SPEAK", 32},
		{"MANAGE_EVENTS", 33},
		{"MANAGE_THREADS", 34},
		{"CREATE_PUBLIC_THREADS", 35},
		{"CREATE_PRIVATE_THREADS", 36},
		{"USE_EXTERNAL_STICKERS", 37},
		{"SEND_MESSAGES_IN_THREADS", 38},
		{"USE_EMBEDDED_ACTIVITIES", 39},
		{"MODERATE_MEMBERS", 40},
		{"VIEW_CREATOR_MONETIZATION_ANALYTICS", 41},
		{"USE_SOUNDBOARD", 42},
		{"CREATE_GUILD_EXPRESSIONS", 43},
		{"CREATE_EVENTS", 44},
		{"USE_EXTERNAL_SOUNDS", 45},
		{"SEND_VOICE_MESSAGES", 46},
		{"SET_VOICE_CHANNEL_STATUS", 48},
		{"SEND_POLLS", 49},
		{"USE_EXTERNAL_APPS", 50},
		{"PIN_MESSAGES", 51},
		{"BYPASS_SLOWMODE", 52},
	}
}

// pinnedPermissions returns all pinned platform permission names and bit positions.
func pinnedPermissions() []pinnedPermission {
	return append(pinnedPermissionsBase(), pinnedPermissionsExtended()...)
}

// The rows below were taken from the platform's permission page (state of
// 2026-10-11), not from the table under test. Bit 47 is not assigned.
func TestPermissions_Positive_NamesAndBitsPinned(t *testing.T) {
	want := pinnedPermissions()
	got := Permissions()
	if len(got) != len(want) {
		t.Fatalf("len(Permissions()) = %d, want %d", len(got), len(want))
	}
	for i, row := range want {
		if got[i] != row.name {
			t.Errorf("Permissions()[%d] = %q, want %q", i, got[i], row.name)
		}
		mask, err := PermissionMask([]string{row.name})
		if err != nil {
			t.Errorf("PermissionMask(%q): %v", row.name, err)
			continue
		}
		if mask != uint64(1)<<row.bit {
			t.Errorf("PermissionMask(%q) = 0x%x, want bit %d", row.name, mask, row.bit)
		}
	}
}
