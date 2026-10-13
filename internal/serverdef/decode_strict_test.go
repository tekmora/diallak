package serverdef

import (
	"strings"
	"testing"
)

type sampleStruct struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// verifyDecodeStrictPositive tests valid JSON decode into a struct.
func verifyDecodeStrictPositive(t *testing.T) {
	t.Helper()
	input := `{"name": "test", "count": 42}`
	var s sampleStruct
	if err := DecodeStrict(strings.NewReader(input), "sample", &s); err != nil {
		t.Fatalf("DecodeStrict failed: %v", err)
	}
	if s.Name != "test" || s.Count != 42 {
		t.Fatalf("unexpected decoded value: %+v", s)
	}
}

// verifyDecodeStrictUnknownKey tests that an unknown key produces an error.
func verifyDecodeStrictUnknownKey(t *testing.T) {
	t.Helper()
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
}

// verifyDecodeStrictKeyTwice tests that a duplicated key produces an error.
func verifyDecodeStrictKeyTwice(t *testing.T) {
	t.Helper()
	input := `{"name": "test", "name": "duplicate"}`
	var s sampleStruct
	err := DecodeStrict(strings.NewReader(input), "sample", &s)
	if err == nil {
		t.Fatal("expected error for duplicate key, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error %q should name duplicate key", err.Error())
	}
}

// verifyDecodeStrictTrailingValue tests that trailing content produces an error.
func verifyDecodeStrictTrailingValue(t *testing.T) {
	t.Helper()
	input := `{"name": "test", "count": 1} trailing`
	var s sampleStruct
	err := DecodeStrict(strings.NewReader(input), "sample", &s)
	if err == nil {
		t.Fatal("expected error for trailing data, got nil")
	}
}

// verifyDecodeStrictEmptyInput tests that empty or whitespace input produces an error.
func verifyDecodeStrictEmptyInput(t *testing.T) {
	t.Helper()
	var s sampleStruct
	err := DecodeStrict(strings.NewReader("   \n"), "sample", &s)
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

// verifyDecodeStrictBoundarySizeLimit tests that exceeding MaxBytes produces an error.
func verifyDecodeStrictBoundarySizeLimit(t *testing.T) {
	t.Helper()
	oversized := strings.Repeat(" ", MaxBytes+1)
	var s sampleStruct
	err := DecodeStrict(strings.NewReader(oversized), "sample", &s)
	if err == nil {
		t.Fatal("expected error for oversized input, got nil")
	}
	if !strings.Contains(err.Error(), "sample") {
		t.Errorf("error %q should carry what='sample'", err.Error())
	}
}

func TestDecodeStrict_Positive_Negative_Boundary(t *testing.T) {
	t.Run("positive decode into second struct", verifyDecodeStrictPositive)
	t.Run("negative unknown key", verifyDecodeStrictUnknownKey)
	t.Run("negative key twice", verifyDecodeStrictKeyTwice)
	t.Run("negative trailing value", verifyDecodeStrictTrailingValue)
	t.Run("negative empty input", verifyDecodeStrictEmptyInput)
	t.Run("boundary size limit exceeded", verifyDecodeStrictBoundarySizeLimit)
}
