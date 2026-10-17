package discord

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
)

type fakeBugInteraction struct {
	guildID uint64
	title   string
	steps   string
	shown   string
	showErr error
}

func (f *fakeBugInteraction) GuildID() uint64 { return f.guildID }
func (f *fakeBugInteraction) Title() string   { return f.title }
func (f *fakeBugInteraction) Steps() string   { return f.steps }
func (f *fakeBugInteraction) Show(_ context.Context, text string) error {
	f.shown = text
	return f.showErr
}

func TestBugReportLink_Positive_NormalReport(t *testing.T) {
	const (
		repo  = "tekmora/diallak"
		title = "Crash on /ask"
		steps = "1. run /ask\n2. see error"
	)
	link, err := BugReportLink(repo, title, steps)
	if err != nil {
		t.Fatalf("BugReportLink failed: %v", err)
	}
	prefix := "https://github.com/" + repo + "/issues/new?"
	if !strings.HasPrefix(link, prefix) {
		t.Fatalf("link %q does not start with %q", link, prefix)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link URL: %v", err)
	}
	q := u.Query()
	if got := q.Get("title"); got != title {
		t.Errorf("title = %q, want %q", got, title)
	}
	body := q.Get("body")
	if !strings.Contains(body, steps) {
		t.Errorf("body %q does not contain steps verbatim", body)
	}
	if !strings.Contains(body, "### Steps") {
		t.Errorf("body %q does not contain ### Steps", body)
	}
}

func TestBugReportLink_Negative_EmptyOrWhitespace(t *testing.T) {
	cases := []struct {
		name  string
		title string
		steps string
	}{
		{"empty title", "", "steps to reproduce"},
		{"whitespace title", "   \t\n  ", "steps to reproduce"},
		{"empty steps", "Bug title", ""},
		{"whitespace steps", "Bug title", "  \n  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BugReportLink("tekmora/diallak", tc.title, tc.steps)
			if !errors.Is(err, ErrBugEmpty) {
				t.Fatalf("BugReportLink(%q, %q) err = %v, want ErrBugEmpty", tc.title, tc.steps, err)
			}
		})
	}
}

func TestBugReportLink_Negative_InvalidUTF8(t *testing.T) {
	cases := []struct {
		name  string
		title string
		steps string
	}{
		{"invalid utf8 title", "crash\xffhere", "steps to reproduce"},
		{"invalid utf8 steps", "Valid title", "step 1\xffstep 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BugReportLink("tekmora/diallak", tc.title, tc.steps)
			if !errors.Is(err, ErrBugEmpty) {
				t.Fatalf("BugReportLink(%q, %q) err = %v, want ErrBugEmpty", tc.title, tc.steps, err)
			}
		})
	}
}

func TestBugReportLink_Boundary_TitleLength(t *testing.T) {
	repo := "tekmora/diallak"
	titleMax := strings.Repeat("t", MaxBugTitleRunes)
	link, err := BugReportLink(repo, titleMax, "repro steps")
	if err != nil {
		t.Fatalf("BugReportLink with MaxBugTitleRunes failed: %v", err)
	}
	if link == "" {
		t.Fatal("expected non-empty link")
	}

	titleTooLong := strings.Repeat("t", MaxBugTitleRunes+1)
	_, err = BugReportLink(repo, titleTooLong, "repro steps")
	if !errors.Is(err, ErrBugTooLong) {
		t.Fatalf("BugReportLink with MaxBugTitleRunes+1 err = %v, want ErrBugTooLong", err)
	}
}

func checkStepsBoundaryForChar(t *testing.T, char string) {
	t.Helper()
	repo := "tekmora/diallak"
	var maxN int
	for n := MaxBugStepsRunes; n >= 1; n-- {
		steps := strings.Repeat(char, n)
		link, err := BugReportLink(repo, "Title", steps)
		if err == nil {
			if utf8.RuneCountInString(TextBugIntro+"\n"+link) <= maxMessageRunes {
				maxN = n
				break
			}
		}
	}
	if maxN == 0 {
		t.Fatalf("no fitting steps found for char %q", char)
	}
	link, err := BugReportLink(repo, "Title", strings.Repeat(char, maxN))
	if err != nil {
		t.Fatalf("BugReportLink at maxN=%d failed: %v", maxN, err)
	}
	totalRunes := utf8.RuneCountInString(TextBugIntro + "\n" + link)
	if totalRunes > maxMessageRunes {
		t.Fatalf("intro+link has %d runes, want <= %d", totalRunes, maxMessageRunes)
	}
	_, err = BugReportLink(repo, "Title", strings.Repeat(char, maxN+1))
	if !errors.Is(err, ErrBugTooLong) {
		t.Fatalf("at maxN+1=%d err = %v, want ErrBugTooLong", maxN+1, err)
	}
}

func TestBugReportLink_Boundary_StepsLength(t *testing.T) {
	t.Run("ascii", func(t *testing.T) {
		checkStepsBoundaryForChar(t, "a")
	})
	t.Run("three-byte rune", func(t *testing.T) {
		checkStepsBoundaryForChar(t, "€")
	})
}

func TestBugHandler_Negative_WrongGuild(t *testing.T) {
	h, err := NewBugHandler(BugConfig{Repository: "tekmora/diallak", GuildID: 42})
	if err != nil {
		t.Fatalf("NewBugHandler: %v", err)
	}
	for _, guildID := range []uint64{0, 999} {
		in := &fakeBugInteraction{guildID: guildID, title: "Title", steps: "Steps"}
		h.HandleCommand(context.Background(), in)
		if in.shown != TextWrongGuild {
			t.Errorf("guild %d: shown = %q, want %q", guildID, in.shown, TextWrongGuild)
		}
	}
}

func TestBugHandler_Negative_EmptyShowsTextBugEmpty(t *testing.T) {
	h, err := NewBugHandler(BugConfig{Repository: "tekmora/diallak", GuildID: 42})
	if err != nil {
		t.Fatalf("NewBugHandler: %v", err)
	}
	in := &fakeBugInteraction{guildID: 42, title: "   ", steps: "steps"}
	h.HandleCommand(context.Background(), in)
	if in.shown != TextBugEmpty {
		t.Errorf("shown = %q, want %q", in.shown, TextBugEmpty)
	}
}

func TestBugHandler_Negative_TooLongShowsTextBugTooLong(t *testing.T) {
	h, err := NewBugHandler(BugConfig{Repository: "tekmora/diallak", GuildID: 42})
	if err != nil {
		t.Fatalf("NewBugHandler: %v", err)
	}
	in := &fakeBugInteraction{
		guildID: 42,
		title:   strings.Repeat("t", MaxBugTitleRunes+1),
		steps:   "steps",
	}
	h.HandleCommand(context.Background(), in)
	if in.shown != TextBugTooLong {
		t.Errorf("shown = %q, want %q", in.shown, TextBugTooLong)
	}
}

func TestBugHandler_Positive_OutcomeLink(t *testing.T) {
	h, err := NewBugHandler(BugConfig{Repository: "tekmora/diallak", GuildID: 42})
	if err != nil {
		t.Fatalf("NewBugHandler: %v", err)
	}
	in := &fakeBugInteraction{guildID: 42, title: "Bug title", steps: "Bug steps"}
	h.HandleCommand(context.Background(), in)
	prefix := TextBugIntro + "\nhttps://github.com/tekmora/diallak/issues/new?"
	if !strings.HasPrefix(in.shown, prefix) {
		t.Errorf("shown %q does not start with %q", in.shown, prefix)
	}
}

func TestBugHandler_Negative_LogCaptureNeverContainsInputOrLink(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h, err := NewBugHandler(BugConfig{Repository: "tekmora/diallak", GuildID: 42, Logger: logger})
	if err != nil {
		t.Fatalf("NewBugHandler: %v", err)
	}
	const (
		secretTitle = "SecretBugTitle7731"
		secretSteps = "ConfidentialRepro9928"
	)
	in := &fakeBugInteraction{guildID: 42, title: secretTitle, steps: secretSteps}
	h.HandleCommand(context.Background(), in)

	logOutput := buf.String()
	if strings.Contains(logOutput, secretTitle) {
		t.Errorf("logs contain secret title: %s", logOutput)
	}
	if strings.Contains(logOutput, secretSteps) {
		t.Errorf("logs contain secret steps: %s", logOutput)
	}
	if strings.Contains(logOutput, "issues/new") {
		t.Errorf("logs contain generated link: %s", logOutput)
	}
	if !strings.Contains(logOutput, "outcome=link") {
		t.Errorf("logs missing outcome=link: %s", logOutput)
	}
}

func TestNewBugHandler_Negative_InvalidRepositoryOrGuild(t *testing.T) {
	cases := []struct {
		name    string
		repo    string
		guildID uint64
	}{
		{"no slash", "tekmora", 42},
		{"two slashes", "a/b/c", 42},
		{"empty repo", "", 42},
		{"zero guild", "tekmora/diallak", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewBugHandler(BugConfig{Repository: tc.repo, GuildID: tc.guildID})
			if err == nil {
				t.Fatalf("NewBugHandler(%q, %d) expected error, got nil", tc.repo, tc.guildID)
			}
		})
	}
}

func TestBugAdapter_Positive_PayloadHasEphemeralAndNoMentions(t *testing.T) {
	const content = "Test bug link reply"
	payload := buildBugReplyPayload(content)
	if payload.Content != content {
		t.Errorf("Content = %q, want %q", payload.Content, content)
	}
	if payload.Flags != discord.MessageFlagEphemeral {
		t.Errorf("Flags = %v, want MessageFlagEphemeral", payload.Flags)
	}
	if payload.AllowedMentions == nil {
		t.Fatal("AllowedMentions is nil, want empty struct")
	}
	if len(payload.AllowedMentions.Parse) != 0 ||
		len(payload.AllowedMentions.Roles) != 0 ||
		len(payload.AllowedMentions.Users) != 0 {
		t.Errorf("AllowedMentions carries mentions: %+v", payload.AllowedMentions)
	}
}
