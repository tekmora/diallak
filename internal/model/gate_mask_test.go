package model

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// newClockedMaskingGate is newMaskingGate with a fake clock.
func newClockedMaskingGate(t *testing.T, c *Client, clock *fakeClock) *Gate {
	t.Helper()
	g, err := NewGate(GateConfig{Endpoint: c, Masker: c, Logger: discardLogger(), Now: clock.Now})
	if err != nil {
		t.Fatalf("NewGate() unexpected error: %v", err)
	}
	return g
}

// The planted case of #69: a start whose planted address comes back unmasked stops.
func TestGateStart_Negative_UnmaskedProbeStopsTheStart(t *testing.T) {
	gw := &fakeGateway{unmasked: true}
	g := newMaskingGate(t, newMaskingClient(t, gw))
	err := g.Start(context.Background())
	if !errors.Is(err, ErrMaskProbeUnmasked) {
		t.Fatalf("Start() = %v, want ErrMaskProbeUnmasked", err)
	}
	if reqs := gw.masks(); len(reqs) != 1 || reqs[0].Text != MaskProbeText {
		t.Errorf("guardrail requests = %+v, want the fixed text", reqs)
	}
	if len(gw.chats()) != 0 {
		t.Errorf("chat requests = %q, want none", gw.chats())
	}
}

func TestGateStart_Negative_RefusedProbeStopsTheStart(t *testing.T) {
	gw := &fakeGateway{maskStatus: http.StatusNotFound, maskBody: `{}`}
	g := newMaskingGate(t, newMaskingClient(t, gw))
	if err := g.Start(context.Background()); err == nil {
		t.Fatal("Start() = nil, want an error for a refused masking probe")
	}
	if len(gw.chats()) != 0 {
		t.Errorf("chat requests = %q, want none", gw.chats())
	}
}

func TestGateStart_Boundary_UnavailableProbeLeavesNotReady(t *testing.T) {
	gw := &fakeGateway{maskStatus: http.StatusServiceUnavailable, maskBody: `{}`}
	g := newMaskingGate(t, newMaskingClient(t, gw))
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start() = %v, want nil and not ready", err)
	}
	if g.State() != StateNotReady || len(gw.chats()) != 0 {
		t.Errorf("state = %s, chats = %q; want not ready and no chat probe", g.State(), gw.chats())
	}
}

// The planted case of #69: a masking request that times out, is refused or returns no text calls
// no model. A timeout or a server error closes the gate; a refusal or no text leaves it ready.
func TestCompleteMasked_Negative_NoMaskedTextCallsNoModel(t *testing.T) {
	tests := []struct {
		name      string
		gw        *fakeGateway
		ctxLimit  time.Duration
		wantState string
	}{
		{"timeout", &fakeGateway{maskBlock: true}, 50 * time.Millisecond, StateNotReady},
		{"server error", &fakeGateway{maskStatus: http.StatusBadGateway, maskBody: `{}`}, 0, StateNotReady},
		{"refused", &fakeGateway{maskStatus: http.StatusBadRequest, maskBody: `{}`}, 0, StateReady},
		{"no text", &fakeGateway{maskBody: `{"response_text":""}`}, 0, StateReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newMaskingGate(t, newMaskingClient(t, &fakeGateway{}))
			if err := g.Start(context.Background()); err != nil {
				t.Fatalf("Start() unexpected error: %v", err)
			}
			// Swap in the failing gateway behind a gate that is ready.
			c := newMaskingClient(t, tt.gw)
			g.endpoint, g.masker = c, c
			trace, err := completeWithin(g, tt.ctxLimit)
			verifyNoModelCall(t, tt.gw, trace, err)
			if tt.ctxLimit == 0 && g.State() != tt.wantState {
				t.Errorf("state = %s, want %s", g.State(), tt.wantState)
			}
		})
	}
}

// verifyNoModelCall checks that a failed masking request reached no model and says so in the trace.
func verifyNoModelCall(t *testing.T, gw *fakeGateway, trace Trace, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("CompleteMasked() = nil error, want a masking error")
	}
	if len(gw.chats()) != 0 {
		t.Errorf("model called: %q", gw.chats())
	}
	if trace.Mask == nil || !trace.Mask.Failed || trace.Mask.Text != "" || !trace.Sent {
		t.Errorf("trace = %+v, want sent with a failed mask and no text", trace)
	}
}

// completeWithin sends the planted question through g, within limit when limit is set.
func completeWithin(g *Gate, limit time.Duration) (Trace, error) {
	ctx := context.Background()
	if limit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}
	_, trace, err := g.CompleteMasked(ctx, "System", plantedQuestion, plantedPrompt)
	return trace, err
}

func TestCompleteMasked_Negative_NoMasker(t *testing.T) {
	g, err := NewGate(GateConfig{Endpoint: &fakeEndpoint{}, Logger: discardLogger()})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.CompleteMasked(context.Background(), "s", "q", plantedPrompt); err == nil {
		t.Error("CompleteMasked() without a masker = nil error")
	}
}

// A trial never starts with a member's question: the fixed text goes to the guardrail route first.
func TestCompleteMasked_Boundary_TrialSendsTheProbeFirst(t *testing.T) {
	clock := newClock(time.Unix(1_700_000_000, 0))
	gw := &fakeGateway{maskStatus: http.StatusServiceUnavailable, maskBody: `{}`}
	c := newMaskingClient(t, gw)
	g := newClockedMaskingGate(t, c, clock)
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	if _, _, err := g.CompleteMasked(context.Background(), "s", plantedQuestion, plantedPrompt); !errors.Is(err, ErrNotReady) {
		t.Fatalf("CompleteMasked() before the trial = %v, want ErrNotReady", err)
	}

	clock.Advance(RetryInterval)
	_, trace, err := g.CompleteMasked(context.Background(), "s", plantedQuestion, plantedPrompt)
	if err == nil || trace.Sent {
		t.Fatalf("failed trial = %v, trace %+v; want an error and nothing of the member sent", err, trace)
	}
	for _, req := range gw.masks() {
		if req.Text != MaskProbeText {
			t.Errorf("guardrail received %q during a failed trial, want only the fixed text", req.Text)
		}
	}

	gw.mu.Lock()
	gw.maskStatus, gw.maskBody = 0, ""
	gw.mu.Unlock()
	clock.Advance(RetryInterval)
	if _, _, err := g.CompleteMasked(context.Background(), "s", plantedQuestion, plantedPrompt); err != nil {
		t.Fatalf("trial = %v, want success", err)
	}
	reqs := gw.masks()
	n := len(reqs)
	if n < 2 || reqs[n-2].Text != MaskProbeText || reqs[n-1].Text != plantedQuestion {
		t.Errorf("last guardrail requests = %+v, want the fixed text, then the question", reqs)
	}
	if g.State() != StateReady {
		t.Errorf("state = %s, want ready", g.State())
	}
}
