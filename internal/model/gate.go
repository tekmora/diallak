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
	Complete(ctx context.Context, system, user string) (string, error)
	Probe(ctx context.Context) error
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
	Endpoint      Endpoint
	Logger        *slog.Logger
	RetryInterval time.Duration
	Now           func() time.Time
}

// Gate holds and transitions model availability state behind a concurrency-safe barrier.
type Gate struct {
	endpoint      Endpoint
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
		logger:        logger,
		retryInterval: interval,
		now:           nowFn,
	}, nil
}

// Start probes the endpoint and initializes model readiness state.
func (g *Gate) Start(ctx context.Context) error {
	err := g.endpoint.Probe(ctx)
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

// Complete executes a model completion or gates it if the endpoint is not ready.
func (g *Gate) Complete(ctx context.Context, system, user string) (string, error) {
	g.mu.Lock()
	if g.policyLatched {
		g.mu.Unlock()
		return "", ErrPolicyMissing
	}
	if g.ready {
		g.mu.Unlock()
		return g.completeReady(ctx, system, user)
	}
	return g.completeNotReady(ctx, system, user)
}

func (g *Gate) completeReady(ctx context.Context, system, user string) (string, error) {
	res, err := g.endpoint.Complete(ctx, system, user)
	if err == nil {
		return res, nil
	}
	if ctx.Err() != nil {
		return "", err
	}
	if Classify(err) == ClassPolicy {
		g.mu.Lock()
		wasLatched := g.policyLatched
		g.policyLatched = true
		g.ready = false
		g.mu.Unlock()
		if !wasLatched {
			g.logPolicyMissing(err)
		}
		return "", fmt.Errorf("%w: %w", ErrPolicyMissing, err)
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
	return "", err
}

func (g *Gate) completeNotReady(ctx context.Context, system, user string) (string, error) {
	if g.policyLatched {
		g.mu.Unlock()
		return "", ErrPolicyMissing
	}
	now := g.now()
	if g.trialInFlight || now.Before(g.nextTrial) {
		g.mu.Unlock()
		return "", ErrNotReady
	}
	g.trialInFlight = true
	g.mu.Unlock()

	res, err := g.endpoint.Complete(ctx, system, user)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.trialInFlight = false

	if err == nil {
		wasReady := g.ready
		g.ready = true
		if !wasReady {
			g.logger.Info("model ready")
		}
		return res, nil
	}

	if ctx.Err() != nil {
		return "", err
	}

	if Classify(err) == ClassPolicy {
		wasLatched := g.policyLatched
		g.policyLatched = true
		g.ready = false
		if !wasLatched {
			g.logPolicyMissing(err)
		}
		return "", fmt.Errorf("%w: %w", ErrPolicyMissing, err)
	}

	g.nextTrial = g.now().Add(g.retryInterval)
	return "", err
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
