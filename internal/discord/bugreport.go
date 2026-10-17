package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	// BugCommandName is the slash command a member opens a prefilled new issue with.
	BugCommandName = "bug"
	// BugCommandDescription is the description of the bug command in Discord's command list.
	BugCommandDescription = "Get a link that opens a filled-in bug report on GitHub"
	// OptionBugTitle is the option name for the bug title.
	OptionBugTitle = "title"
	// OptionBugSteps is the option name for the reproduction steps.
	OptionBugSteps = "steps"

	// MaxBugTitleRunes bounds the title: chosen, not measured.
	MaxBugTitleRunes = 200
	// MaxBugStepsRunes bounds the steps: chosen, not measured.
	MaxBugStepsRunes = 1500

	// TextBugIntro introduces the bug report link.
	TextBugIntro = "Open this link to file the report under your own GitHub account. Nothing is sent until you submit it there."
	// TextBugEmpty refuses a report without title or steps.
	TextBugEmpty = "A bug report needs a title and the steps. Run /bug again."
	// TextBugTooLong refuses a report whose title or steps exceed the length limits.
	TextBugTooLong = "The report is too long for a link. Shorten the title or the steps and run /bug again."

	// defaultBugCallTimeout is chosen, not measured.
	defaultBugCallTimeout = 10 * time.Second
	maxBugCallTimeout     = 1 * time.Minute
)

// ErrBugEmpty reports that a bug report title or steps are empty or invalid UTF-8.
var ErrBugEmpty = errors.New("bug report needs a non-empty title and steps")

// ErrBugTooLong reports that a bug report exceeds length limits.
var ErrBugTooLong = errors.New("bug report is too long for a link")

// BugReportLink builds the prefilled GitHub new-issue URL.
func BugReportLink(repository, title, steps string) (string, error) {
	title = strings.TrimSpace(title)
	steps = strings.TrimSpace(steps)
	if title == "" || steps == "" || !utf8.ValidString(title) || !utf8.ValidString(steps) {
		return "", ErrBugEmpty
	}
	if utf8.RuneCountInString(title) > MaxBugTitleRunes || utf8.RuneCountInString(steps) > MaxBugStepsRunes {
		return "", ErrBugTooLong
	}
	body := "### Steps\n\n" + steps + "\n\n_Filed through the community server's /bug command._"
	link := "https://github.com/" + repository + "/issues/new?" + url.Values{"title": {title}, "body": {body}}.Encode()
	if utf8.RuneCountInString(TextBugIntro+"\n"+link) > maxMessageRunes {
		return "", ErrBugTooLong
	}
	return link, nil
}

// BugInteraction is a bug command interaction as the handler needs it.
type BugInteraction interface {
	GuildID() uint64
	Title() string
	Steps() string
	Show(ctx context.Context, text string) error
}

// BugConfig configures a BugHandler.
type BugConfig struct {
	Repository  string
	GuildID     uint64
	Logger      *slog.Logger
	CallTimeout time.Duration
}

// BugHandler serves the bug command.
type BugHandler struct {
	repository  string
	guildID     uint64
	logger      *slog.Logger
	callTimeout time.Duration
}

// NewBugHandler validates configuration and returns a new BugHandler.
func NewBugHandler(cfg BugConfig) (*BugHandler, error) {
	if !serverdef.ValidRepository(cfg.Repository) {
		return nil, fmt.Errorf("invalid repository %q", cfg.Repository)
	}
	if cfg.GuildID == 0 {
		return nil, errors.New("guild ID must not be zero")
	}
	if cfg.CallTimeout < 0 || cfg.CallTimeout > maxBugCallTimeout {
		return nil, fmt.Errorf("call timeout %v out of range [0, %v]", cfg.CallTimeout, maxBugCallTimeout)
	}
	timeout := cfg.CallTimeout
	if timeout == 0 {
		timeout = defaultBugCallTimeout
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &BugHandler{
		repository:  cfg.Repository,
		guildID:     cfg.GuildID,
		logger:      logger,
		callTimeout: timeout,
	}, nil
}

// HandleCommand handles a /bug slash command interaction.
func (h *BugHandler) HandleCommand(ctx context.Context, in BugInteraction) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error("recovered from panic in HandleCommand", slog.String("panic_type", fmt.Sprintf("%T", r)))
		}
	}()

	var (
		text    string
		outcome string
	)
	if in.GuildID() == 0 || in.GuildID() != h.guildID {
		text = TextWrongGuild
		outcome = "wrong_guild"
	} else {
		link, err := BugReportLink(h.repository, in.Title(), in.Steps())
		switch {
		case errors.Is(err, ErrBugEmpty):
			text = TextBugEmpty
			outcome = "empty"
		case errors.Is(err, ErrBugTooLong):
			text = TextBugTooLong
			outcome = "too_long"
		case err == nil:
			text = TextBugIntro + "\n" + link
			outcome = "link"
		default:
			text = TextBugEmpty
			outcome = "empty"
		}
	}

	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	defer cancel()

	if err := in.Show(callCtx, text); err != nil {
		h.logger.Warn("bug report failed",
			slog.String("outcome", outcome),
			slog.String("error_class", classifyError(err)),
		)
		return
	}
	h.logger.Info("bug report answered", slog.String("outcome", outcome))
}
