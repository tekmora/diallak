// Package discord provides the Discord adapter and interaction handler for Diallak.
package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/answer"
)

const (
	// CommandName is the slash command name for inquiries.
	CommandName = "ask"
	// CommandDescription is the approved slash command description explaining that the assistant is automated.
	CommandDescription = "Ask the automated assistant a question about the project documentation"
	// OptionQuestion is the required string option containing the inquiry.
	OptionQuestion = "question"

	// TextBusy is returned when the handler has reached maximum concurrent asks.
	TextBusy = "The bot is busy answering other questions right now. Please try again in a moment."
	// TextWrongGuild is returned when an interaction originates outside the configured guild.
	TextWrongGuild = "This command is only available in the configured community server."
	// TextFailure is returned when an error occurs during answer processing.
	TextFailure = "An error occurred while answering your question. Please try again later."

	// DefaultMaxConcurrent is the default maximum concurrent asking inquiries.
	// Source: chosen, not measured.
	DefaultMaxConcurrent = 4
	// MaxAllowedConcurrent is the upper bound on concurrent asking inquiries.
	MaxAllowedConcurrent = 64
	// DefaultAskTimeout is the default processing timeout for inquiries.
	// Source: chosen, not measured.
	DefaultAskTimeout = 60 * time.Second
	// MaxAllowedAskTimeout is the maximum allowed ask timeout.
	MaxAllowedAskTimeout = 10 * time.Minute
	// defaultReplyTimeout is the timeout for interaction defer and reply calls.
	// Source: chosen, not measured.
	defaultReplyTimeout    = 10 * time.Second
	maxAllowedReplyTimeout = 1 * time.Minute
)

// Limits are the limits a Handler works under, after its defaults are applied.
type Limits struct {
	// MaxConcurrent is the number of questions answered at the same time.
	MaxConcurrent int
	// AskTimeout is the time one question may take in total.
	AskTimeout time.Duration
	// QuestionMaxChars is the longest question accepted, in runes.
	QuestionMaxChars int
}

// Limits reports the limits in effect.
func (h *Handler) Limits() Limits {
	return Limits{
		MaxConcurrent:    h.maxConcurrent,
		AskTimeout:       h.askTimeout,
		QuestionMaxChars: h.questionMaxChars,
	}
}

// validateQuestionMaxChars returns the question limit in effect: zero means
// answer.MaxQuestionRunes, and a value outside 0 to answer.MaxQuestionRunes is an error.
func validateQuestionMaxChars(n int) (int, error) {
	if n < 0 || n > answer.MaxQuestionRunes {
		return 0, fmt.Errorf("question limit %d out of range [0, %d]", n, answer.MaxQuestionRunes)
	}
	if n == 0 {
		return answer.MaxQuestionRunes, nil
	}
	return n, nil
}

// TextBadQuestion returns the notice returned when the inquiry is empty, too long, or invalid UTF-8.
// If maxChars <= 0, answer.MaxQuestionRunes is used.
func TextBadQuestion(maxChars int) string {
	if maxChars <= 0 {
		maxChars = answer.MaxQuestionRunes
	}
	return fmt.Sprintf("Questions must be valid text between 1 and %d characters.", maxChars)
}

// Asker executes an inquiry against the documentation answer pipeline.
type Asker interface {
	Ask(ctx context.Context, question string) (answer.Answer, error)
}

// Interaction represents an incoming slash command interaction.
type Interaction interface {
	GuildID() uint64
	Question() string
	Defer(ctx context.Context) error
	Reply(ctx context.Context, text string) error
}

// HandlerConfig configures the interaction handler dependencies and limits.
type HandlerConfig struct {
	Asker         Asker
	GuildID       uint64
	MaxConcurrent int
	AskTimeout    time.Duration
	ReplyTimeout  time.Duration
	Logger        *slog.Logger
	// QuestionMaxChars is the longest question the handler accepts, in runes. Zero means
	// answer.MaxQuestionRunes.
	QuestionMaxChars int
	// RenderOptions holds the support place and the reply length limit. They apply to every
	// reply the handler sends. The zero value means the defaults of the answer package.
	RenderOptions answer.RenderOptions
}

// Handler coordinates slash command interactions, concurrency limits, and replies.
type Handler struct {
	asker            Asker
	guildID          uint64
	maxConcurrent    int
	askTimeout       time.Duration
	replyTimeout     time.Duration
	logger           *slog.Logger
	questionMaxChars int
	renderOptions    answer.RenderOptions

	mu          sync.Mutex
	inFlight    int
	zeroWaiters []chan struct{}
}

// NewHandler constructs and validates a new Handler instance.
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if cfg.Asker == nil {
		return nil, errors.New("asker is required")
	}
	if cfg.GuildID == 0 {
		return nil, errors.New("guild ID must not be zero")
	}
	maxConcurrent, err := validateMaxConcurrent(cfg.MaxConcurrent)
	if err != nil {
		return nil, err
	}
	askTimeout, err := validateAskTimeout(cfg.AskTimeout)
	if err != nil {
		return nil, err
	}
	replyTimeout, err := validateReplyTimeout(cfg.ReplyTimeout)
	if err != nil {
		return nil, err
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	questionMaxChars, err := validateQuestionMaxChars(cfg.QuestionMaxChars)
	if err != nil {
		return nil, err
	}

	return &Handler{
		asker:            cfg.Asker,
		guildID:          cfg.GuildID,
		maxConcurrent:    maxConcurrent,
		askTimeout:       askTimeout,
		replyTimeout:     replyTimeout,
		logger:           logger,
		questionMaxChars: questionMaxChars,
		renderOptions:    cfg.RenderOptions,
	}, nil
}

func validateMaxConcurrent(v int) (int, error) {
	if v < 0 || v > MaxAllowedConcurrent {
		return 0, fmt.Errorf("max concurrent %d out of range [0, %d]", v, MaxAllowedConcurrent)
	}
	if v == 0 {
		return DefaultMaxConcurrent, nil
	}
	return v, nil
}

func validateAskTimeout(d time.Duration) (time.Duration, error) {
	if d < 0 || d > MaxAllowedAskTimeout {
		return 0, fmt.Errorf("ask timeout %v out of range [0, %v]", d, MaxAllowedAskTimeout)
	}
	if d == 0 {
		return DefaultAskTimeout, nil
	}
	return d, nil
}

func validateReplyTimeout(d time.Duration) (time.Duration, error) {
	if d < 0 || d > maxAllowedReplyTimeout {
		return 0, fmt.Errorf("reply timeout %v out of range [0, %v]", d, maxAllowedReplyTimeout)
	}
	if d == 0 {
		return defaultReplyTimeout, nil
	}
	return d, nil
}

// InFlight returns the number of asks currently holding a processing slot.
func (h *Handler) InFlight() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.inFlight
}

// Wait blocks until all in-flight asks complete or ctx is cancelled.
func (h *Handler) Wait(ctx context.Context) error {
	h.mu.Lock()
	if h.inFlight == 0 {
		h.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	h.zeroWaiters = append(h.zeroWaiters, ch)
	h.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		h.mu.Lock()
		h.removeZeroWaiter(ch)
		h.mu.Unlock()
		return ctx.Err()
	}
}

func (h *Handler) removeZeroWaiter(ch chan struct{}) {
	for i, w := range h.zeroWaiters {
		if w == ch {
			h.zeroWaiters = append(h.zeroWaiters[:i], h.zeroWaiters[i+1:]...)
			break
		}
	}
}

func (h *Handler) acquireSlot() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.inFlight >= h.maxConcurrent {
		return false
	}
	h.inFlight++
	return true
}

func (h *Handler) releaseSlot() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.inFlight--
	if h.inFlight == 0 {
		for _, ch := range h.zeroWaiters {
			close(ch)
		}
		h.zeroWaiters = nil
	}
}

// HandleAsk processes an /ask command interaction safely without panicking.
func (h *Handler) HandleAsk(ctx context.Context, in Interaction) {
	defer func() {
		if r := recover(); r != nil {
			// The panic value can quote what was being processed, so only its type is logged.
			h.logger.Error("recovered from panic in HandleAsk", slog.String("panic_type", fmt.Sprintf("%T", r)))
		}
	}()

	if in.GuildID() == 0 || in.GuildID() != h.guildID {
		h.handleDirectReply(ctx, in, TextWrongGuild)
		return
	}

	if !h.acquireSlot() {
		h.handleDirectReply(ctx, in, TextBusy)
		return
	}
	defer h.releaseSlot()

	if !h.performDefer(ctx, in) {
		return
	}

	q := strings.TrimSpace(in.Question())
	if !h.validateQuestion(in.Question(), q) {
		h.sendFixedReply(ctx, in, TextBadQuestion(h.questionMaxChars))
		return
	}

	h.executeAskAndReply(ctx, in, q)
}

func (h *Handler) validateQuestion(raw, trimmed string) bool {
	if !utf8.ValidString(raw) {
		return false
	}
	if trimmed == "" {
		return false
	}
	return utf8.RuneCountInString(trimmed) <= h.questionMaxChars
}

func (h *Handler) handleDirectReply(ctx context.Context, in Interaction, text string) {
	if !h.performDefer(ctx, in) {
		return
	}
	h.sendFixedReply(ctx, in, text)
}

func (h *Handler) sendFixedReply(ctx context.Context, in Interaction, text string) {
	h.sendReply(ctx, in, answer.AppendFooter(text, h.renderOptions.Place))
}

func (h *Handler) performDefer(ctx context.Context, in Interaction) bool {
	deferCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.replyTimeout)
	defer cancel()

	if err := in.Defer(deferCtx); err != nil {
		h.logger.Warn("defer failed", slog.String("error_class", classifyError(err)))
		return false
	}
	return true
}

func (h *Handler) executeAskAndReply(ctx context.Context, in Interaction, q string) {
	start := time.Now()
	askCtx, cancel := context.WithTimeout(ctx, h.askTimeout)
	ans, err := h.asker.Ask(askCtx, q)
	cancel()

	if err != nil {
		h.logger.Warn("ask failed",
			slog.String("outcome", "failure"),
			slog.String("error_class", classifyError(err)),
			slog.Duration("duration", time.Since(start)),
		)
		h.sendFixedReply(ctx, in, TextFailure)
		return
	}

	h.logger.Info("ask answered",
		slog.String("outcome", string(ans.Kind)),
		slog.String("reason", string(ans.Reason)),
		slog.Duration("duration", time.Since(start)),
	)
	h.sendReply(ctx, in, answer.Render(ans, h.renderOptions))
}

func (h *Handler) sendReply(ctx context.Context, in Interaction, text string) {
	replyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.replyTimeout)
	defer cancel()

	if err := in.Reply(replyCtx, text); err != nil {
		h.logger.Warn("reply failed", slog.String("error_class", classifyError(err)))
	}
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}
