package health

import (
	"testing"
)

func TestReady_Negative_NilStateReportsFalse(t *testing.T) {
	var s *State
	if s.Ready() {
		t.Fatal("nil State.Ready() = true, want false")
	}
}

func TestReady_Boundary_ZeroStateAndToggledReadiness(t *testing.T) {
	s := &State{}
	if s.Ready() {
		t.Fatal("zero State.Ready() = true, want false")
	}
	s.SetReady(true)
	if !s.Ready() {
		t.Fatal("State.Ready() after SetReady(true) = false, want true")
	}
}

func TestSetReady_Negative_NilReceiverDoesNotPanic(t *testing.T) {
	var s *State
	s.SetReady(true)
}

func TestSetReady_Boundary_StoresFalseAfterTrue(t *testing.T) {
	s := &State{}
	s.SetReady(true)
	s.SetReady(false)
	if s.Ready() {
		t.Fatal("State.Ready() after SetReady(false) = true, want false")
	}
}

func TestSetModelState_Negative_NilReceiverDoesNotPanic(t *testing.T) {
	var s *State
	s.SetModelState(func() string { return "ok" })
}

func TestSetModelState_Boundary_NilCallbackClearsState(t *testing.T) {
	s := &State{}
	s.SetModelState(func() string { return "active" })
	s.SetModelState(nil)
	if got := s.ModelState(); got != "not configured" {
		t.Fatalf("ModelState() after SetModelState(nil) = %q, want 'not configured'", got)
	}
}

func TestModelState_Negative_NilReceiverReportsNotConfigured(t *testing.T) {
	var s *State
	if got := s.ModelState(); got != "not configured" {
		t.Fatalf("nil State.ModelState() = %q, want 'not configured'", got)
	}
}

func TestModelState_Boundary_NilReportFunctionReturnsNotConfigured(t *testing.T) {
	s := &State{}
	s.modelReport.Store(&modelReport{fn: nil})
	if got := s.ModelState(); got != "not configured" {
		t.Fatalf("ModelState() with nil fn = %q, want 'not configured'", got)
	}
}
