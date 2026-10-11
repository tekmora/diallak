package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type fakeEndpoint struct {
	probeCalls    atomic.Int32
	completeCalls atomic.Int32
	probeErr      error
	completeRes   string
	completeErr   error
	onComplete    func()
}

func (f *fakeEndpoint) Probe(_ context.Context) error {
	f.probeCalls.Add(1)
	return f.probeErr
}

func (f *fakeEndpoint) Complete(_ context.Context, _, _ string) (string, error) {
	f.completeCalls.Add(1)
	if f.onComplete != nil {
		f.onComplete()
	}
	return f.completeRes, f.completeErr
}

func TestGate_Positive_NewGateAndFirstComplete(t *testing.T) {
	ep := &fakeEndpoint{completeRes: "ok"}
	g, err := NewGate(GateConfig{Endpoint: ep})
	if err != nil {
		t.Fatalf("NewGate unexpected error: %v", err)
	}

	if got := g.State(); got != StateNotReady {
		t.Fatalf("initial state = %q, want %q", got, StateNotReady)
	}

	res, err := g.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Complete unexpected error: %v", err)
	}
	if res != "ok" {
		t.Errorf("Complete result = %q, want ok", res)
	}
	if ep.completeCalls.Load() != 1 {
		t.Errorf("complete calls = %d, want 1", ep.completeCalls.Load())
	}
	if got := g.State(); got != StateReady {
		t.Errorf("state after success = %q, want %q", got, StateReady)
	}
}

func TestGate_Positive_StartNil(t *testing.T) {
	ep := &fakeEndpoint{probeErr: nil}
	g, err := NewGate(GateConfig{Endpoint: ep})
	if err != nil {
		t.Fatalf("NewGate unexpected error: %v", err)
	}

	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start unexpected error: %v", err)
	}
	if got := g.State(); got != StateReady {
		t.Errorf("state after Start nil = %q, want %q", got, StateReady)
	}
}

func TestGate_Positive_StartUnavailable(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "500", err: &StatusError{Code: 500}},
		{name: "503", err: &StatusError{Code: 503}},
		{name: "429", err: &StatusError{Code: 429}},
		{name: "408", err: &StatusError{Code: 408}},
		{name: "refused", err: &TransportError{Err: errors.New("connection refused")}},
		{name: "hang", err: &TransportError{Err: context.DeadlineExceeded}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ep := &fakeEndpoint{probeErr: tc.err}
			clock := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			g, err := NewGate(GateConfig{
				Endpoint:      ep,
				RetryInterval: 30 * time.Second,
				Now:           clock.Now,
			})
			if err != nil {
				t.Fatalf("NewGate unexpected error: %v", err)
			}

			if err := g.Start(context.Background()); err != nil {
				t.Fatalf("Start unexpected error for unavailable: %v", err)
			}
			if got := g.State(); got != StateNotReady {
				t.Errorf("state = %q, want %q", got, StateNotReady)
			}

			_, err = g.Complete(context.Background(), "sys", "usr")
			if !errors.Is(err, ErrNotReady) {
				t.Fatalf("Complete err = %v, want ErrNotReady", err)
			}
			if ep.completeCalls.Load() != 0 {
				t.Errorf("complete calls = %d, want 0", ep.completeCalls.Load())
			}
		})
	}
}

func TestGate_Negative_StartRejected(t *testing.T) {
	codes := []int{400, 401, 404, 302}
	for _, code := range codes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			ep := &fakeEndpoint{probeErr: &StatusError{Code: code}}
			g, err := NewGate(GateConfig{Endpoint: ep})
			if err != nil {
				t.Fatalf("NewGate unexpected error: %v", err)
			}

			err = g.Start(context.Background())
			if err == nil {
				t.Fatal("Start expected error, got nil")
			}
			var statusErr *StatusError
			if !errors.As(err, &statusErr) || statusErr.Code != code {
				t.Errorf("Start error %v does not wrap StatusError with code %d", err, code)
			}
			if got := g.State(); got != StateNotReady {
				t.Errorf("state = %q, want %q", got, StateNotReady)
			}
		})
	}
}

func TestGate_Negative_StartCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ep := &fakeEndpoint{probeErr: context.Canceled}
	g, err := NewGate(GateConfig{Endpoint: ep})
	if err != nil {
		t.Fatalf("NewGate unexpected error: %v", err)
	}

	err = g.Start(ctx)
	if err == nil {
		t.Fatal("Start with cancelled context expected error, got nil")
	}
	if got := g.State(); got != StateNotReady {
		t.Errorf("state = %q, want %q", got, StateNotReady)
	}
}

func TestGate_Boundary_RetryInterval(t *testing.T) {
	clock := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	interval := 30 * time.Second
	ep := &fakeEndpoint{probeErr: &StatusError{Code: 503}, completeRes: "ok"}

	g, err := NewGate(GateConfig{
		Endpoint:      ep,
		RetryInterval: interval,
		Now:           clock.Now,
	})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}

	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	clock.Advance(10 * time.Second)
	_, err = g.Complete(context.Background(), "s", "u")
	if !errors.Is(err, ErrNotReady) || ep.completeCalls.Load() != 0 {
		t.Fatalf("inside interval: err=%v, completeCalls=%d", err, ep.completeCalls.Load())
	}

	clock.Advance(20*time.Second - time.Nanosecond)
	_, err = g.Complete(context.Background(), "s", "u")
	if !errors.Is(err, ErrNotReady) || ep.completeCalls.Load() != 0 {
		t.Fatalf("1ns before interval: err=%v, completeCalls=%d", err, ep.completeCalls.Load())
	}

	clock.Advance(time.Nanosecond)
	res, err := g.Complete(context.Background(), "s", "u")
	if err != nil || res != "ok" || ep.completeCalls.Load() != 1 {
		t.Fatalf("at interval: err=%v, res=%q, completeCalls=%d", err, res, ep.completeCalls.Load())
	}
	if g.State() != StateReady {
		t.Errorf("state = %q, want ready", g.State())
	}
}

func TestGate_Positive_TrialOutcomes(t *testing.T) {
	clock := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ep := &fakeEndpoint{probeErr: &StatusError{Code: 503}}
	g, err := NewGate(GateConfig{Endpoint: ep, RetryInterval: 30 * time.Second, Now: clock.Now})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	clock.Advance(30 * time.Second)
	ep.completeErr = &StatusError{Code: 503}
	if _, err := g.Complete(context.Background(), "s", "u"); err == nil || g.State() != StateNotReady {
		t.Fatalf("trial 503: err=%v, state=%q", err, g.State())
	}

	clock.Advance(15 * time.Second)
	if _, err := g.Complete(context.Background(), "s", "u"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ErrNotReady, got %v", err)
	}

	clock.Advance(15 * time.Second)
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ep.completeErr = context.Canceled
	if _, err := g.Complete(cancelCtx, "s", "u"); err == nil {
		t.Fatal("expected error on cancelled trial, got nil")
	}

	ep.completeErr = nil
	ep.completeRes = "trial ok"
	res, err := g.Complete(context.Background(), "s", "u")
	if err != nil || res != "trial ok" || g.State() != StateReady {
		t.Fatalf("after cancelled trial: err=%v, res=%q, state=%q", err, res, g.State())
	}
}

func TestGate_Positive_ReadyTransitions(t *testing.T) {
	ep := &fakeEndpoint{probeErr: nil, completeRes: "ok"}
	g, err := NewGate(GateConfig{Endpoint: ep})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ep.completeErr = &StatusError{Code: 400}
	if _, err := g.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("expected error on 400, got nil")
	}
	if g.State() != StateReady {
		t.Fatalf("after 400, state = %q, want ready", g.State())
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ep.completeErr = context.Canceled
	if _, err := g.Complete(cancelCtx, "s", "u"); err == nil {
		t.Fatal("expected error on cancel, got nil")
	}
	if g.State() != StateReady {
		t.Fatalf("after cancel, state = %q, want ready", g.State())
	}

	ep.completeErr = &StatusError{Code: 503}
	if _, err := g.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("expected error on 503, got nil")
	}
	if g.State() != StateNotReady {
		t.Fatalf("after 503, state = %q, want not ready", g.State())
	}
}

func TestGate_Boundary_ConcurrentCompletesTrialDue(t *testing.T) {
	clock := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	startTrial := make(chan struct{})
	releaseTrial := make(chan struct{})

	ep := &fakeEndpoint{
		probeErr:    &StatusError{Code: 503},
		completeRes: "ok",
		onComplete: func() {
			close(startTrial)
			<-releaseTrial
		},
	}

	g, err := NewGate(GateConfig{Endpoint: ep, RetryInterval: 30 * time.Second, Now: clock.Now})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	clock.Advance(30 * time.Second)

	var wg sync.WaitGroup
	var err1 error
	var res1 string

	wg.Add(1)
	go func() {
		defer wg.Done()
		res1, err1 = g.Complete(context.Background(), "s", "u")
	}()

	<-startTrial
	res2, err2 := g.Complete(context.Background(), "s", "u")

	close(releaseTrial)
	wg.Wait()

	if ep.completeCalls.Load() != 1 {
		t.Errorf("complete calls = %d, want 1", ep.completeCalls.Load())
	}
	if err1 != nil || res1 != "ok" {
		t.Errorf("trial call unexpected result: res1=%q, err1=%v", res1, err1)
	}
	if !errors.Is(err2, ErrNotReady) || res2 != "" {
		t.Errorf("concurrent call unexpected result: res2=%q, err2=%v", res2, err2)
	}
}

type logCapture struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *logCapture) Write(p []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *logCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func TestGate_Positive_OneLogLinePerStateChange(t *testing.T) {
	var cap logCapture
	logger := slog.New(slog.NewTextHandler(&cap, nil))
	clock := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ep := &fakeEndpoint{probeErr: &StatusError{Code: 503}}

	g, err := NewGate(GateConfig{
		Endpoint:      ep,
		Logger:        logger,
		RetryInterval: 30 * time.Second,
		Now:           clock.Now,
	})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}

	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	for range 3 {
		if _, callErr := g.Complete(context.Background(), "secret-system", "secret-user-question"); !errors.Is(callErr, ErrNotReady) {
			t.Fatalf("expected ErrNotReady, got %v", callErr)
		}
	}

	clock.Advance(30 * time.Second)
	ep.completeErr = &StatusError{Code: 503}
	if _, callErr := g.Complete(context.Background(), "secret-system", "secret-user-question"); callErr == nil {
		t.Fatal("expected error on trial 503, got nil")
	}

	clock.Advance(30 * time.Second)
	ep.completeErr = nil
	ep.completeRes = "ok"
	if _, callErr := g.Complete(context.Background(), "secret-system", "secret-user-question"); callErr != nil {
		t.Fatalf("expected ok, got %v", callErr)
	}

	if _, callErr := g.Complete(context.Background(), "secret-system", "secret-user-question"); callErr != nil {
		t.Fatalf("expected ok, got %v", callErr)
	}

	ep.completeErr = &StatusError{Code: 503}
	if _, callErr := g.Complete(context.Background(), "secret-system", "secret-user-question"); callErr == nil {
		t.Fatal("expected 503 error, got nil")
	}

	out := cap.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 log lines, got %d:\n%s", len(lines), out)
	}

	if strings.Contains(out, "secret-system") || strings.Contains(out, "secret-user-question") {
		t.Errorf("log leaked prompt: %s", out)
	}
	if !strings.Contains(lines[0], "model not ready") || !strings.Contains(lines[0], "status") {
		t.Errorf("line 0 unexpected: %s", lines[0])
	}
	if !strings.Contains(lines[1], "model ready") {
		t.Errorf("line 1 unexpected: %s", lines[1])
	}
	if !strings.Contains(lines[2], "model not ready") {
		t.Errorf("line 2 unexpected: %s", lines[2])
	}
}

func TestGate_Positive_RaceFree(t *testing.T) {
	clock := newClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ep := &fakeEndpoint{completeRes: "ok"}
	g, err := NewGate(GateConfig{Endpoint: ep, RetryInterval: time.Millisecond, Now: clock.Now})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_ = g.State()
				if _, completeErr := g.Complete(context.Background(), "sys", "usr"); completeErr != nil && !errors.Is(completeErr, ErrNotReady) {
					t.Errorf("unexpected error: %v", completeErr)
				}
				clock.Advance(time.Millisecond)
			}
		}()
	}
	wg.Wait()
}
