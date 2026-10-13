package discord

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/tekmora/diallak/internal/answer"
)

func TestTextBadQuestion_Negative_NegativeLimitUsesDefault(t *testing.T) {
	got := TextBadQuestion(-1)
	want := fmt.Sprintf("Questions must be valid text between 1 and %d characters.", answer.MaxQuestionRunes)
	if got != want {
		t.Fatalf("TextBadQuestion(-1) = %q, want %q", got, want)
	}
}

func TestTextBadQuestion_Boundary_ZeroLimitUsesDefault(t *testing.T) {
	got := TextBadQuestion(0)
	want := fmt.Sprintf("Questions must be valid text between 1 and %d characters.", answer.MaxQuestionRunes)
	if got != want {
		t.Fatalf("TextBadQuestion(0) = %q, want %q", got, want)
	}
}

func TestRegisterCommands_Boundary_MaxQuestionRunesAllowed(t *testing.T) {
	const appID = 123456789012345678
	const guildID = 987654321098765432
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/applications/@me" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":"%d","name":"diallak"}`, appID)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	token := makeFakeToken(appID)
	opts := clientOptions{
		restURL:          srv.URL,
		maxQuestionChars: answer.MaxQuestionRunes,
	}
	if err := registerCommandsWithOptions(context.Background(), token, guildID, opts); err != nil {
		t.Fatalf("registerCommandsWithOptions with MaxQuestionRunes: %v", err)
	}
}

func TestEnabled_Positive_PassesWhenInnerEnabled(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := floorHandler{inner: inner}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("floorHandler.Enabled(warn) = false, want true")
	}
}

func TestEnabled_Boundary_ExactLibraryLogFloorLevel(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := floorHandler{inner: inner}
	if !h.Enabled(context.Background(), libraryLogFloor) {
		t.Fatal("floorHandler.Enabled(libraryLogFloor) = false, want true")
	}
}

func TestWithAttrs_Positive_FiltersDisallowedAttributes(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := floorHandler{inner: inner}
	h2 := h.WithAttrs([]slog.Attr{slog.String("secret_token_val", "leaked")})
	record := slog.NewRecord(time.Now(), slog.LevelWarn, "msg", 0)
	if err := h2.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if strings.Contains(buf.String(), "secret_token_val") {
		t.Fatalf("logged unallowed attribute: %s", buf.String())
	}
}

type attrCountingHandler struct {
	count int
}

// Enabled reports whether level is enabled.
func (c *attrCountingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle processes a record.
func (c *attrCountingHandler) Handle(_ context.Context, _ slog.Record) error { return nil }

// WithAttrs stores the count of received attributes.
func (c *attrCountingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &attrCountingHandler{count: len(attrs)}
}

// WithGroup returns the handler unchanged.
func (c *attrCountingHandler) WithGroup(_ string) slog.Handler { return c }

func TestWithAttrs_Boundary_CapsAtThirtyTwoAttributes(t *testing.T) {
	inner := &attrCountingHandler{}
	h := floorHandler{inner: inner}
	attrs := make([]slog.Attr, maxRecordAttrs+1)
	for i := range attrs {
		attrs[i] = slog.Int("try", i)
	}
	h2 := h.WithAttrs(attrs)
	fh, ok := h2.(floorHandler)
	if !ok {
		t.Fatalf("expected floorHandler, got %T", h2)
	}
	ch, ok := fh.inner.(*attrCountingHandler)
	if !ok {
		t.Fatalf("expected *attrCountingHandler, got %T", fh.inner)
	}
	if ch.count != maxRecordAttrs {
		t.Fatalf("attrs count = %d, want %d", ch.count, maxRecordAttrs)
	}
}

type groupTrackingHandler struct {
	groupCalled bool
}

// Enabled reports whether level is enabled.
func (g *groupTrackingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle processes a record.
func (g *groupTrackingHandler) Handle(_ context.Context, _ slog.Record) error { return nil }

// WithAttrs returns the handler unchanged.
func (g *groupTrackingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return g }

// WithGroup marks that inner WithGroup was called.
func (g *groupTrackingHandler) WithGroup(_ string) slog.Handler {
	return &groupTrackingHandler{groupCalled: true}
}

func TestWithGroup_Positive_ReturnsReceiverUnchanged(t *testing.T) {
	inner := &groupTrackingHandler{}
	h := floorHandler{inner: inner}
	h2 := h.WithGroup("telemetry")
	fh, ok := h2.(floorHandler)
	if !ok {
		t.Fatalf("expected floorHandler, got %T", h2)
	}
	gh, ok := fh.inner.(*groupTrackingHandler)
	if !ok {
		t.Fatalf("expected *groupTrackingHandler, got %T", fh.inner)
	}
	if gh.groupCalled {
		t.Fatal("floorHandler.WithGroup() called inner.WithGroup()")
	}
}

func TestWithGroup_Boundary_EmptyNameReturnsReceiverUnchanged(t *testing.T) {
	inner := &groupTrackingHandler{}
	h := floorHandler{inner: inner}
	h2 := h.WithGroup("")
	fh, ok := h2.(floorHandler)
	if !ok {
		t.Fatalf("expected floorHandler, got %T", h2)
	}
	gh, ok := fh.inner.(*groupTrackingHandler)
	if !ok {
		t.Fatalf("expected *groupTrackingHandler, got %T", fh.inner)
	}
	if gh.groupCalled {
		t.Fatal("floorHandler.WithGroup(\"\") called inner.WithGroup()")
	}
}

func TestDefer_Boundary_PassesContextOption(t *testing.T) {
	var passedOpts []rest.RequestOpt
	adapter := &interactionAdapter{
		respond: func(_ discord.InteractionResponseType, _ discord.InteractionResponseData, opts ...rest.RequestOpt) error {
			passedOpts = opts
			return nil
		},
	}
	if err := adapter.Defer(context.Background()); err != nil {
		t.Fatalf("Defer failed: %v", err)
	}
	if len(passedOpts) != 1 {
		t.Fatalf("passedOpts count = %d, want 1 (rest.WithCtx)", len(passedOpts))
	}
}
