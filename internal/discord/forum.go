package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/ruleanswer"
)

// ExcerptAsker tries fixed rules and otherwise answers with excerpts, without a model endpoint.
type ExcerptAsker interface {
	Excerpts(ctx context.Context, question string) (answer.Answer, error)
	Rule(q ruleanswer.Query) (answer.Answer, bool)
}

// ForumPost describes a thread the platform reports as created. It holds no title and no text.
type ForumPost struct {
	ThreadID     uint64
	ParentID     uint64
	PublicThread bool
	// NewlyCreated is false for a thread the platform reports again, for example after a reconnect.
	NewlyCreated bool
	TagIDs       []uint64
	TagNames     []string
}

// ForumMessage is the first message of a forum post.
type ForumMessage struct {
	Content string
	// FromApplication is true when a bot or an application wrote the message.
	FromApplication bool
}

// ForumCalls are the platform calls the forum answer makes.
type ForumCalls interface {
	// FirstMessage reads the first message of the post.
	FirstMessage(ctx context.Context, threadID uint64) (ForumMessage, error)
	// Reply posts text in the post.
	Reply(ctx context.Context, threadID uint64, text string) error
}

// ForumConfig configures the first answer in the support forum (ADR-0009, ADR-0020).
type ForumConfig struct {
	Asker ExcerptAsker
	// ChannelID is the support forum. Posts in other channels are not read.
	ChannelID uint64
	// OptInTagID is the tag a member sets to ask for the automated answer. Posts without it are not
	// read.
	OptInTagID uint64
	// MaxConcurrent is the number of posts handled at the same time; posts over it get no reply.
	// Zero means DefaultMaxConcurrent.
	MaxConcurrent int
	// Timeout is the time one post may take in total. Zero means DefaultAskTimeout.
	Timeout time.Duration
	// QuestionMaxChars is the length title and text are cut to, in runes. Zero means
	// answer.MaxQuestionRunes.
	QuestionMaxChars int
	Logger           *slog.Logger
	TicketsChannel   string
}

// ForumHandler answers opted-in support posts with fixed rules or excerpts.
type ForumHandler struct {
	asker            ExcerptAsker
	channelID        uint64
	optInTagID       uint64
	timeout          time.Duration
	questionMaxChars int
	logger           *slog.Logger
	ticketsChannel   string
	slots            slots
}

// NewForumHandler validates cfg and returns a handler.
func NewForumHandler(cfg ForumConfig) (*ForumHandler, error) {
	if cfg.Asker == nil {
		return nil, errors.New("forum: asker is required")
	}
	if cfg.ChannelID == 0 {
		return nil, errors.New("forum: channel ID must not be zero")
	}
	if cfg.OptInTagID == 0 {
		return nil, errors.New("forum: the opt-in tag ID must not be zero")
	}
	maxConcurrent, err := validateMaxConcurrent(cfg.MaxConcurrent)
	if err != nil {
		return nil, fmt.Errorf("forum: %w", err)
	}
	timeout, err := validateAskTimeout(cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("forum: %w", err)
	}
	questionMaxChars, err := validateQuestionMaxChars(cfg.QuestionMaxChars)
	if err != nil {
		return nil, fmt.Errorf("forum: %w", err)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &ForumHandler{
		asker: cfg.Asker, channelID: cfg.ChannelID, optInTagID: cfg.OptInTagID,
		timeout: timeout, questionMaxChars: questionMaxChars, logger: logger,
		ticketsChannel: cfg.TicketsChannel,
		slots:          slots{max: maxConcurrent},
	}, nil
}

// ChannelID returns the support forum the handler answers in.
func (h *ForumHandler) ChannelID() uint64 { return h.channelID }

// Wait blocks until every post in hand is done or ctx is cancelled.
func (h *ForumHandler) Wait(ctx context.Context) error { return h.slots.wait(ctx) }

// Dispatch handles one created thread in its own goroutine. A thread that is not a new, public,
// opted-in post of the forum is not read; a post over the concurrency bound is dropped.
func (h *ForumHandler) Dispatch(ctx context.Context, post ForumPost, title string, calls ForumCalls) {
	if reason := h.skipReason(post); reason != "" {
		h.logger.Debug("forum thread skipped", slog.String("reason", reason))
		return
	}
	if !h.slots.acquire() {
		h.logger.Warn("forum post dropped", slog.String("outcome", "busy"))
		return
	}
	h.logger.Debug("forum post accepted")
	go func() {
		defer h.slots.release()
		defer func() {
			if r := recover(); r != nil {
				// The panic value can quote what was being processed, so only its type is logged.
				h.logger.Error("recovered from panic in the forum answer", slog.String("panic_type", fmt.Sprintf("%T", r)))
			}
		}()
		postCtx, cancel := context.WithTimeout(ctx, h.timeout)
		defer cancel()
		h.answerPost(postCtx, post, title, calls)
	}()
}

func (h *ForumHandler) skipReason(post ForumPost) string {
	switch {
	case post.ParentID != h.channelID:
		return "other_channel"
	case !post.PublicThread:
		return "not_public_thread"
	case !post.NewlyCreated:
		return "not_new"
	case !slices.Contains(post.TagIDs, h.optInTagID):
		return "not_opted_in"
	default:
		return ""
	}
}

func (h *ForumHandler) answerPost(ctx context.Context, post ForumPost, title string, calls ForumCalls) {
	threadID := post.ThreadID
	start := time.Now()
	msg, err := calls.FirstMessage(ctx, threadID)
	if err != nil {
		h.logFailure("forum read failed", err, start)
		return
	}
	if msg.FromApplication {
		h.logOutcome("application_author", "", start)
		return
	}
	text := strings.TrimSpace(strings.ToValidUTF8(msg.Content, ""))
	if text == "" {
		h.logOutcome("no_reply", "empty_message", start)
		return
	}
	question := h.question(title, text)
	ans, matched := h.asker.Rule(ruleanswer.Query{Text: question, Forum: true, Tags: post.TagNames})
	reason := ""
	if post.TagNames == nil {
		reason = "tags_unresolved"
	}
	if !matched {
		ans, err = h.asker.Excerpts(ctx, question)
		if err != nil {
			h.logFailure("forum answer failed", err, start)
			return
		}
	}
	reply := answer.RenderForum(ans, 0, h.ticketsChannel)
	if reply == "" {
		h.logOutcome(string(ans.Kind), reasonOr(reason, string(ans.Reason)), start)
		return
	}
	if err := calls.Reply(ctx, threadID, reply); err != nil {
		h.logFailure("forum reply failed", err, start)
		return
	}
	h.logOutcome(string(ans.Kind), reasonOr(reason, string(ans.Reason)), start)
}

func reasonOr(reason, fallback string) string {
	if reason != "" {
		return reason
	}
	return fallback
}

// question joins title and text and cuts the result to the question limit (ADR-0009 case 14).
func (h *ForumHandler) question(title, text string) string {
	q := strings.TrimSpace(strings.ToValidUTF8(title, "")) + "\n\n" + text
	runes := []rune(q)
	if len(runes) > h.questionMaxChars {
		runes = runes[:h.questionMaxChars]
	}
	return strings.TrimSpace(string(runes))
}

func (h *ForumHandler) logOutcome(outcome, reason string, start time.Time) {
	h.logger.Info("forum post handled",
		slog.String("outcome", outcome),
		slog.String("reason", reason),
		slog.Duration("duration", time.Since(start)),
	)
}

func (h *ForumHandler) logFailure(msg string, err error, start time.Time) {
	h.logger.Warn(msg,
		slog.String("outcome", "failure"),
		slog.String("error_class", classifyError(err)),
		slog.Duration("duration", time.Since(start)),
	)
}
