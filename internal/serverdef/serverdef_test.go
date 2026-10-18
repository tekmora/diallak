package serverdef

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLoad_Positive_ValidFileAccepted(t *testing.T) {
	data, err := os.ReadFile("../../server/definition.json")
	if err != nil {
		t.Fatalf("read definition fixture: %v", err)
	}
	def, err := Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load valid definition: %v", err)
	}
	if def.Version != 1 {
		t.Errorf("Version = %d, want 1", def.Version)
	}
	if len(def.Roles) == 0 {
		t.Errorf("len(Roles) = 0, want populated list")
	}
}

func TestLoad_Positive_RoleOrderKeyAccepted(t *testing.T) {
	data, err := os.ReadFile("../../server/definition.json")
	if err != nil {
		t.Fatalf("read definition fixture: %v", err)
	}
	def, err := Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load valid definition with role_order: %v", err)
	}
	if len(def.RoleOrder) != 28 {
		t.Errorf("len(RoleOrder) = %d, want 28", len(def.RoleOrder))
	}
}

func TestLoad_Negative_UnknownFieldRefused(t *testing.T) {
	input := `{"version": 1, "applied": false, "unknown_field": "disallowed"}`
	_, err := Load(strings.NewReader(input))
	if err == nil {
		t.Fatal("Load with unknown field succeeded, want error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("error %q does not mention 'unknown field'", err.Error())
	}
}

func TestLoad_Negative_DuplicateKeyRefused(t *testing.T) {
	t.Run("top level duplicate key", func(t *testing.T) {
		input := `{"version": 1, "version": 1, "applied": false}`
		_, err := Load(strings.NewReader(input))
		if err == nil {
			t.Fatal("Load with top-level duplicate key succeeded, want error")
		}
		if !strings.Contains(err.Error(), "version") {
			t.Errorf("error %q does not name duplicate key 'version'", err.Error())
		}
	})

	t.Run("nested duplicate key", func(t *testing.T) {
		input := `{
			"version": 1,
			"applied": false,
			"summary": "ok",
			"categories": [
				{"id": "cat1", "name": "Cat", "name": "Duplicate", "purpose": "ok"}
			]
		}`
		_, err := Load(strings.NewReader(input))
		if err == nil {
			t.Fatal("Load with nested duplicate key succeeded, want error")
		}
		if !strings.Contains(err.Error(), "name") {
			t.Errorf("error %q does not name duplicate key 'name'", err.Error())
		}
	})
}

func TestLoad_Negative_TrailingDataRefused(t *testing.T) {
	input := `{"version": 1, "applied": false, "summary": "ok"} trailing garbage`
	_, err := Load(strings.NewReader(input))
	if err == nil {
		t.Fatal("Load with trailing data succeeded, want error")
	}
}

func TestLoad_Negative_EmptyInputRefused(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"whitespace only", "   \t\n  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tc.input))
			if err == nil {
				t.Fatalf("Load(%q) succeeded, want error", tc.input)
			}
		})
	}
}

func TestLoad_Boundary_MaxBytesAcceptedAndOverLimitRefused(t *testing.T) {
	t.Run("MaxBytes limit exceeded", func(t *testing.T) {
		oversized := strings.Repeat(" ", MaxBytes+1)
		_, err := Load(strings.NewReader(oversized))
		if err == nil {
			t.Fatal("Load over MaxBytes succeeded, want error")
		}
	})

	t.Run("MaxBytes exact size valid JSON", func(t *testing.T) {
		prefix := `{"version": 1, "applied": false, "summary": "`
		suffix := `"}`
		paddingLen := MaxBytes - len(prefix) - len(suffix)
		if paddingLen < 0 {
			t.Fatalf("paddingLen %d < 0", paddingLen)
		}
		input := prefix + strings.Repeat("a", paddingLen) + suffix
		if len(input) != MaxBytes {
			t.Fatalf("len(input) = %d, want MaxBytes %d", len(input), MaxBytes)
		}
		def, err := Load(strings.NewReader(input))
		if err != nil {
			t.Fatalf("Load at MaxBytes: %v", err)
		}
		if def.Version != 1 {
			t.Errorf("def.Version = %d, want 1", def.Version)
		}
	})
}

func TestLoad_Boundary_DepthLimit(t *testing.T) {
	buildNested := func(depth int) string {
		var sb strings.Builder
		for i := 0; i < depth; i++ {
			sb.WriteString(`{"a":`)
		}
		sb.WriteString(`1`)
		for i := 0; i < depth; i++ {
			sb.WriteString(`}`)
		}
		return sb.String()
	}

	t.Run("depth MaxDepth accepted", func(t *testing.T) {
		input := buildNested(MaxDepth)
		// It will fail unmarshaling into Definition because of structure,
		// but token walk should accept depth 16 without depth error.
		_, err := Load(strings.NewReader(input))
		if err != nil && strings.Contains(err.Error(), fmt.Sprintf("depth exceeds maximum depth of %d", MaxDepth)) {
			t.Fatalf("depth %d rejected for depth: %v", MaxDepth, err)
		}
	})

	t.Run("depth MaxDepth+1 refused", func(t *testing.T) {
		input := buildNested(MaxDepth + 1)
		_, err := Load(strings.NewReader(input))
		if err == nil {
			t.Fatalf("depth %d accepted, want depth error", MaxDepth+1)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("depth exceeds maximum depth of %d", MaxDepth)) {
			t.Errorf("error %q does not mention depth limit", err.Error())
		}
	})
}
