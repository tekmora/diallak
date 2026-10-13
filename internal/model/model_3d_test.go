package model

import (
	"errors"
	"strings"
	"testing"
)

func TestUnwrap_Positive_ReturnsUnderlyingError(t *testing.T) {
	base := errors.New("underlying transport failure")
	te := &TransportError{Err: base}
	if got := te.Unwrap(); !errors.Is(got, base) {
		t.Fatalf("Unwrap() = %v, want %v", got, base)
	}
}

func TestUnwrap_Negative_PreservesExactErrorIdentity(t *testing.T) {
	base := errors.New("underlying network down")
	te := &TransportError{Err: base}
	got := te.Unwrap()
	if !errors.Is(got, base) {
		t.Fatalf("Unwrap() = %v, want exact original %v", got, base)
	}
	if copyOfBase := errors.New(base.Error()); errors.Is(got, copyOfBase) {
		t.Fatalf("Unwrap() = %v matches a different error with the same text", got)
	}
}

func TestClassify_Positive_PolicyErrorClassifiedAsPolicy(t *testing.T) {
	pe := &PolicyError{Applied: []string{"other-policy"}}
	if got := Classify(pe); got != ClassPolicy {
		t.Fatalf("Classify(&PolicyError{}) = %v, want ClassPolicy", got)
	}
}

func TestValidPolicyName_Positive_ValidStandardPolicy(t *testing.T) {
	if !ValidPolicyName("standard-policy.v1") {
		t.Fatal("ValidPolicyName('standard-policy.v1') = false, want true")
	}
}

func TestValidPolicyName_Negative_RejectsEmbeddedSpaces(t *testing.T) {
	if ValidPolicyName("policy with spaces") {
		t.Fatal("ValidPolicyName('policy with spaces') = true, want false")
	}
}

func TestValidPolicyName_Boundary_ExactOneHundredTwentyEightCharacters(t *testing.T) {
	name := strings.Repeat("a", 128)
	if !ValidPolicyName(name) {
		t.Fatal("ValidPolicyName(128 chars) = false, want true")
	}
}
