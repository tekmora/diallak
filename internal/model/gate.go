package model

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Endpoint specifies the model completion and probe operations needed by Gate.
type Endpoint interface {
	CompleteTraced(ctx context.Context, system, user string) (string, Trace, error)
	Probe(ctx context.Context) error
}

// Masker specifies the bot's own masking request of ADR-0013, step two.
type Masker interface {
	Mask(ctx context.Context, text string) (string, error)
	ProbeMask(ctx context.Context) error
}

// RetryInterval is the default duration to wait before a trial request when the endpoint is not ready.
// Source: chosen, not measured.
const RetryInterval = 30 * time.Second

const (
	// StateReady indicates the endpoint was probed or trialed successfully.
	StateReady = "ready"
	// StateNotReady indicates the endpoint is unavailable or has not yet succeeded.
	StateNotReady = "not ready"
)

// ErrNotReady is returned by Complete when the endpoint is not ready and no trial is due.
var ErrNotReady = errors.New("model not ready")

// ErrPolicyMissing is returned when the model endpoint does not name the configured policy, latching the gate.
var ErrPolicyMissing = errors.New("model policy missing")

// GateConfig configures a model Gate.
type GateConfig struct {
	Endpoint Endpoint
	// Masker is the bot's own masking request. Nil means none: the gate works as in step one.
	Masker        Masker
	Logger        *slog.Logger
	RetryInterval time.Duration
	Now           func() time.Time
}

// Gate holds and transitions model availability state behind a concurrency-safe barrier.
type Gate struct {
	endpoint      Endpoint
	masker        Masker
	logger        *slog.Logger
	retryInterval time.Duration
	now           func() time.Time

	mu            sync.Mutex
	ready         bool
	policyLatched bool
	nextTrial     time.Time
	trialInFlight bool
}

// NewGate validates cfg and constructs a new Gate.
func NewGate(cfg GateConfig) (*Gate, error) {
	if cfg.Endpoint == nil {
		return nil, errors.New("endpoint is required")
	}
	if cfg.RetryInterval < 0 {
		return nil, errors.New("retry interval must not be negative")
	}
	interval := cfg.RetryInterval
	if interval == 0 {
		interval = RetryInterval
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Gate{
		endpoint:      cfg.Endpoint,
		masker:        cfg.Masker,
		logger:        logger,
		retryInterval: interval,
		now:           nowFn,
	}, nil
}

// Start probes the endpoint and initializes model readiness state. With a Masker it first sends
// the fixed text to the guardrail route: an unmasked address or a refusal stops the start, a
// server error or a timeout leaves the model not ready without a chat probe.
func (g *Gate) Start(ctx context.Context) error {
	err := g.probe(ctx)
	if err == nil {
		g.mu.Lock()
		wasReady := g.ready
		g.ready = true
		g.mu.Unlock()
		if !wasReady {
			g.logger.Info("model ready")
		}
		return nil
	}

	if ctx.Err() == nil && Classify(err) == ClassUnavailable {
		g.mu.Lock()
		g.ready = false
		g.nextTrial = g.now().Add(g.retryInterval)
		g.mu.Unlock()
		g.logNotReady(err)
		return nil
	}

	return fmt.Errorf("model start probe: %w", err)
}

// probe sends the masking probe when there is a Masker, then the chat probe.
func (g *Gate) probe(ctx context.Context) error {
	if g.masker != nil {
		if err := g.masker.ProbeMask(ctx); err != nil {
			return fmt.Errorf("masking probe: %w", err)
		}
	}
	return g.endpoint.Probe(ctx)
}

// Complete executes a model completion or gates it if the endpoint is not ready.
func (g *Gate) Complete(ctx context.Context, system, user string) (string, error) {
	res, _, err := g.CompleteTraced(ctx, system, user)
	return res, err
}

// CompleteTraced is Complete, and it returns the trace of the request. A completion the gate
// holds back returns the zero Trace: nothing was sent.
func (g *Gate) CompleteTraced(ctx context.Context, system, user string) (string, Trace, error) {
	return g.run(ctx, func(callCtx context.Context) (string, Trace, error) {
		return g.endpoint.CompleteTraced(callCtx, system, user)
	})
}

// CompleteMasked sends question to the guardrail route first and the completion built by prompt
// from the masked text after it (ADR-0013, step two). Nothing falls back to an unmasked request:
// when no masked text comes back, the model is not called. The trace holds the masked text and
// its counts.
func (g *Gate) CompleteMasked(ctx context.Context, system, question string, prompt func(masked string) string) (string, Trace, error) {
	if g.masker == nil {
		return "", Trace{}, errors.New("no masking configured")
	}
	return g.run(ctx, func(callCtx context.Context) (string, Trace, error) {
		masked, err := g.masker.Mask(callCtx, question)
		if err != nil {
			return "", Trace{Sent: true, Mask: &MaskTrace{Failed: true}}, fmt.Errorf("mask question: %w", err)
		}
		mask := NewMaskTrace(question, masked)
		res, trace, err := g.endpoint.CompleteTraced(callCtx, system, prompt(masked))
		trace.Mask = &mask
		return res, trace, err
	})
}

// call is one request through the gate.
type call func(ctx context.Context) (string, Trace, error)

// run sends c when the gate is ready or a trial is due, and moves the state by its outcome.
func (g *Gate) run(ctx context.Context, c call) (string, Trace, error) {
	g.mu.Lock()
	if g.policyLatched {
		g.mu.Unlock()
		return "", Trace{}, ErrPolicyMissing
	}
	if g.ready {
		g.mu.Unlock()
		return g.completeReady(ctx, c)
	}
	return g.completeNotReady(ctx, c)
}

func (g *Gate) completeReady(ctx context.Context, c call) (string, Trace, error) {
	res, trace, err := c(ctx)
	if err == nil {
		return res, trace, nil
	}
	// A reply without the policy is a fact about the endpoint, whatever
	// became of the caller's context while its body was discarded.
	if Classify(err) == ClassPolicy {
		g.mu.Lock()
		defer g.mu.Unlock()
		return "", trace, g.latchLocked(err)
	}
	if ctx.Err() != nil {
		return "", trace, err
	}
	if Classify(err) == ClassUnavailable {
		g.mu.Lock()
		wasReady := g.ready
		g.ready = false
		g.nextTrial = g.now().Add(g.retryInterval)
		g.mu.Unlock()
		if wasReady {
			g.logNotReady(err)
		}
	}
	return "", trace, err
}

func (g *Gate) completeNotReady(ctx context.Context, c call) (string, Trace, error) {
	if g.policyLatched {
		g.mu.Unlock()
		return "", Trace{}, ErrPolicyMissing
	}
	now := g.now()
	if g.trialInFlight || now.Before(g.nextTrial) {
		g.mu.Unlock()
		return "", Trace{}, ErrNotReady
	}
	g.trialInFlight = true
	g.mu.Unlock()

	res, trace, err := g.trial(ctx, c)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.trialInFlight = false

	if err == nil {
		// A trial that began before the latch closed does not open the gate again.
		if !g.ready && !g.policyLatched {
			g.ready = true
			g.logger.Info("model ready")
		}
		return res, trace, nil
	}

	if Classify(err) == ClassPolicy {
		return "", trace, g.latchLocked(err)
	}

	if ctx.Err() != nil {
		return "", trace, err
	}

	g.nextTrial = g.now().Add(g.retryInterval)
	return "", trace, err
}

// trial sends c. With a Masker it sends the fixed text to the guardrail route first, so a trial
// never starts with a member's question; a probe that fails sends nothing of the member.
func (g *Gate) trial(ctx context.Context, c call) (string, Trace, error) {
	if g.masker != nil {
		if err := g.masker.ProbeMask(ctx); err != nil {
			return "", Trace{}, fmt.Errorf("masking probe: %w", err)
		}
	}
	return c(ctx)
}

// State reports the current readiness state ("ready" or "not ready").
func (g *Gate) State() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ready {
		return StateReady
	}
	return StateNotReady
}

// latchLocked closes the gate for good after a reply without the policy and
// returns the error for the call that met it. The caller holds g.mu.
func (g *Gate) latchLocked(err error) error {
	wasLatched := g.policyLatched
	g.policyLatched = true
	g.ready = false
	if !wasLatched {
		g.logPolicyMissing(err)
	}
	return fmt.Errorf("%w: %w", ErrPolicyMissing, err)
}

// Reason reports why the gate is not ready. It returns "" when ready or when not ready for an absence,
// and "policy" when latched.
func (g *Gate) Reason() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.policyLatched {
		return "policy"
	}
	return ""
}

func (g *Gate) logPolicyMissing(err error) {
	appliedCount := 0
	var policyErr *PolicyError
	if errors.As(err, &policyErr) {
		appliedCount = len(policyErr.Applied)
	}
	g.logger.Warn("model not ready",
		slog.String("error_class", "policy"),
		slog.Int("applied", appliedCount),
	)
}

func (g *Gate) logNotReady(err error) {
	attrs := errorClassAttrs(err)
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	g.logger.Warn("model not ready", args...)
}

func errorClassAttrs(err error) []slog.Attr {
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return []slog.Attr{
			slog.String("error_class", "status"),
			slog.Int("code", statusErr.Code),
		}
	}
	if isTimeout(err) {
		return []slog.Attr{
			slog.String("error_class", "timeout"),
		}
	}
	return []slog.Attr{
		slog.String("error_class", "transport"),
	}
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}
