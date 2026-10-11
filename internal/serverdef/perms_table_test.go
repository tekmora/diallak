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
