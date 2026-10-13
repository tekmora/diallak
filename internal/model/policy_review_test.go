package model

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const reviewBody = `{"choices":[{"message":{"content":"model-text"}}]}`

func completeWithHeader(t *testing.T, policy, header string) (string, error) {
	t.Helper()
	server := runPolicyServer(http.Header{PolicyHeader: []string{header}}, http.StatusOK, reviewBody)
	defer server.Close()
	c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: policy}, server.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c.Complete(context.Background(), "sys", "usr")
}

func TestComplete_Boundary_PolicyComparedWithTheWholeName(t *testing.T) {
	policy := strings.Repeat("a", maxPolicyNameBytes)

	out, err := completeWithHeader(t, policy, policy)
	if err != nil || out != "model-text" {
		t.Fatalf("name of %d bytes: out=%q err=%v, want the reply", maxPolicyNameBytes, out, err)
	}

	// One byte longer: the first 128 bytes equal the policy, the name does not.
	out, err = completeWithHeader(t, policy, policy+"b")
	var polErr *PolicyError
	if out != "" || !errors.As(err, &polErr) {
		t.Fatalf("name of %d bytes: out=%q err=%v, want a PolicyError", maxPolicyNameBytes+1, out, err)
	}
	if len(polErr.Applied) != 1 || len(polErr.Applied[0]) != maxPolicyNameBytes {
		t.Errorf("Applied = %d names, first of %d bytes; want 1 name cut to %d bytes",
			len(polErr.Applied), len(polErr.Applied[0]), maxPolicyNameBytes)
	}
}

func TestComplete_Boundary_EmptyEntriesCountAgainstTheBound(t *testing.T) {
	const policy = "example-policy"

	// 63 empty entries, then the name: entry 64, the last one read.
	out, err := completeWithHeader(t, policy, strings.Repeat(",", maxAppliedPolicies-1)+policy)
	if err != nil || out != "model-text" {
		t.Fatalf("name as entry %d: out=%q err=%v, want the reply", maxAppliedPolicies, out, err)
	}

	// 64 empty entries, then the name: entry 65, never read.
	out, err = completeWithHeader(t, policy, strings.Repeat(",", maxAppliedPolicies)+policy)
	var polErr *PolicyError
	if out != "" || !errors.As(err, &polErr) {
		t.Fatalf("name as entry %d: out=%q err=%v, want a PolicyError", maxAppliedPolicies+1, out, err)
	}
	if len(polErr.Applied) != 0 {
		t.Errorf("Applied = %v, want no name", polErr.Applied)
	}
}

func TestNew_Negative_PolicyErrorDoesNotEchoTheValue(t *testing.T) {
	const bad = "two words"
	_, err := New(Config{BaseURL: "http://127.0.0.1:1", Alias: "test-alias", Policy: bad}, nil)
	if err == nil {
		t.Fatal("New accepted a policy with a space")
	}
	if strings.Contains(err.Error(), bad) {
		t.Errorf("error %q holds the refused value", err.Error())
	}
}

// verifyPolicyLatchesCancelledContextPath tests that policy failure latches even if caller context was cancelled.
func verifyPolicyLatchesCancelledContextPath(t *testing.T, path string) {
	t.Helper()
	clock := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ep := &fakeEndpoint{}
	if path == "trial" {
		ep.probeErr = &StatusError{Code: 503}
	}
	g, err := NewGate(GateConfig{Endpoint: ep, RetryInterval: 30 * time.Second, Now: clock.Now})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	clock.Advance(30 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	ep.completeErr = &PolicyError{}
	ep.onComplete = cancel // the caller gives up while the reply is read
	res, err := g.Complete(ctx, "s", "u")
	if res != "" || !errors.Is(err, ErrPolicyMissing) {
		t.Fatalf("Complete: res=%q err=%v, want ErrPolicyMissing", res, err)
	}
	if g.State() != StateNotReady || g.Reason() != "policy" {
		t.Fatalf("state=%q reason=%q, want not ready for policy", g.State(), g.Reason())
	}

	ep.onComplete = nil
	clock.Advance(time.Hour)
	if _, err := g.Complete(context.Background(), "s", "u"); !errors.Is(err, ErrPolicyMissing) {
		t.Errorf("later Complete: err=%v, want ErrPolicyMissing", err)
	}
	if got := ep.completeCalls.Load(); got != 1 {
		t.Errorf("endpoint calls = %d, want 1", got)
	}
}

func TestGate_Negative_PolicyLatchesWithACancelledContext(t *testing.T) {
	for _, path := range []string{"ready", "trial"} {
		t.Run(path, func(t *testing.T) {
			verifyPolicyLatchesCancelledContextPath(t, path)
		})
	}
}

// scriptedEndpoint answers each Complete call from its script, in the order
// the calls arrive, and holds every call until its release channel is closed.
type scriptedEndpoint struct {
	calls   atomic.Int32
	entered chan int
	steps   []scriptedStep
}

type scriptedStep struct {
	release chan struct{}
	res     string
	err     error
}

func (e *scriptedEndpoint) Probe(_ context.Context) error { return nil }

func (e *scriptedEndpoint) Complete(_ context.Context, _, _ string) (string, error) {
	i := int(e.calls.Add(1)) - 1
	if i >= len(e.steps) {
		return "", errors.New("call beyond the script")
	}
	e.entered <- i
	<-e.steps[i].release
	return e.steps[i].res, e.steps[i].err
}

type completeResult struct {
	res string
	err error
}

// TestGate_Boundary_TrialInFlightDoesNotReopenAfterTheLatch covers a trial
// that was sent before the latch closed and succeeds after it: the gate stays
// not ready and no further call reaches the endpoint.
func TestGate_Boundary_TrialInFlightDoesNotReopenAfterTheLatch(t *testing.T) {
	clock := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ep := &scriptedEndpoint{
		entered: make(chan int),
		steps: []scriptedStep{
			{release: make(chan struct{}), err: &PolicyError{}},          // call A, ready path
			{release: make(chan struct{}), err: &StatusError{Code: 503}}, // call B, ready path
			{release: make(chan struct{}), res: "trial-text"},            // call C, trial
		},
	}
	logs := &logCapture{}
	g, err := NewGate(GateConfig{
		Endpoint:      ep,
		RetryInterval: 30 * time.Second,
		Now:           clock.Now,
		Logger:        slog.New(slog.NewTextHandler(logs, nil)),
	})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	results := make([]chan completeResult, len(ep.steps))
	start := func(i int) {
		results[i] = make(chan completeResult, 1)
		go func() {
			res, err := g.Complete(context.Background(), "s", "u")
			results[i] <- completeResult{res, err}
		}()
		if got := <-ep.entered; got != i {
			t.Fatalf("call %d entered as %d", i, got)
		}
	}

	start(0)
	start(1)
	close(ep.steps[1].release) // B: unavailable, the gate is not ready
	if r := <-results[1]; r.err == nil {
		t.Fatal("call B succeeded, want the status error")
	}
	clock.Advance(30 * time.Second)
	start(2)                   // C: the trial is on its way
	close(ep.steps[0].release) // A: reply without the policy, the latch closes
	if r := <-results[0]; !errors.Is(r.err, ErrPolicyMissing) {
		t.Fatalf("call A: err=%v, want ErrPolicyMissing", r.err)
	}
	close(ep.steps[2].release) // C: succeeds after the latch
	<-results[2]

	if g.State() != StateNotReady || g.Reason() != "policy" {
		t.Fatalf("state=%q reason=%q after the trial, want not ready for policy", g.State(), g.Reason())
	}
	if n := strings.Count(logs.String(), "model ready"); n != 1 {
		t.Errorf("\"model ready\" logged %d times, want 1 (the start)", n)
	}
	clock.Advance(time.Hour)
	if _, err := g.Complete(context.Background(), "s", "u"); !errors.Is(err, ErrPolicyMissing) {
		t.Errorf("later Complete: err=%v, want ErrPolicyMissing", err)
	}
	if got := ep.calls.Load(); got != 3 {
		t.Errorf("endpoint calls = %d, want 3", got)
	}
}
