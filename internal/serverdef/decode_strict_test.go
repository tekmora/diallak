package serverdef

import (
	"strings"
	"testing"
)

type sampleStruct struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestDecodeStrict_Positive_Negative_Boundary(t *testing.T) {
	t.Run("positive decode into second struct", func(t *testing.T) {
		input := `{"name": "test", "count": 42}`
		var s sampleStruct
		if err := DecodeStrict(strings.NewReader(input), "sample", &s); err != nil {
			t.Fatalf("DecodeStrict failed: %v", err)
		}
		if s.Name != "test" || s.Count != 42 {
			t.Fatalf("unexpected decoded value: %+v", s)
		}
	})

	t.Run("negative unknown key", func(t *testing.T) {
		input := `{"name": "test", "unknown": true}`
		var s sampleStruct
		err := DecodeStrict(strings.NewReader(input), "sample", &s)
		if err == nil {
			t.Fatal("expected error for unknown key, got nil")
		}
		if !strings.Contains(err.Error(), "sample") {
			t.Errorf("error %q should carry what='sample'", err.Error())
		}
		if !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("error %q should mention unknown field", err.Error())
		}
	})

	t.Run("negative key twice", func(t *testing.T) {
		input := `{"name": "test", "name": "duplicate"}`
		var s sampleStruct
		err := DecodeStrict(strings.NewReader(input), "sample", &s)
		if err == nil {
			t.Fatal("expected error for duplicate key, got nil")
		}
		if !strings.Contains(err.Error(), "name") {
			t.Errorf("error %q should name duplicate key", err.Error())
		}
	})

	t.Run("negative trailing value", func(t *testing.T) {
		input := `{"name": "test", "count": 1} trailing`
		var s sampleStruct
		err := DecodeStrict(strings.NewReader(input), "sample", &s)
		if err == nil {
			t.Fatal("expected error for trailing data, got nil")
		}
	})

	t.Run("negative empty input", func(t *testing.T) {
		var s sampleStruct
		err := DecodeStrict(strings.NewReader("   \n"), "sample", &s)
		if err == nil {
			t.Fatal("expected error for empty input, got nil")
		}
	})

	t.Run("boundary size limit exceeded", func(t *testing.T) {
		oversized := strings.Repeat(" ", MaxBytes+1)
		var s sampleStruct
		err := DecodeStrict(strings.NewReader(oversized), "sample", &s)
		if err == nil {
			t.Fatal("expected error for oversized input, got nil")
		}
		if !strings.Contains(err.Error(), "sample") {
			t.Errorf("error %q should carry what='sample'", err.Error())
		}
	})
}
