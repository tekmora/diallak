package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"

	"github.com/tekmora/diallak/internal/answer"
)

func debugLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestNormalizeBotConfig_Boundary_DefaultsForLoggerAndTimeout(t *testing.T) {
	h, err := NewHandler(HandlerConfig{Asker: &fakeAsker{}, GuildID: 7})
	if err != nil {
		t.Fatalf("NewHandler() failed: %v", err)
	}
	cfg, err := normalizeBotConfig(BotConfig{Token: "t", GuildID: 7, Handler: h})
	if err != nil {
		t.Fatalf("normalizeBotConfig() failed: %v", err)
	}
	if cfg.Logger == nil {
		t.Fatalf("Logger is nil; the event listener would dereference it")
	}
	cfg.Logger.Debug("usable")
	if cfg.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, defaultShutdownTimeout)
	}
}

func TestSlashCommandData_Negative_OtherCommandTypesAreNotSlash(t *testing.T) {
	// The library's own accessor panics here; the shared application can send these.
	for _, data := range []discord.ApplicationCommandInteractionData{
		discord.UserCommandInteractionData{},
		discord.MessageCommandInteractionData{},
	} {
		if _, ok := slashCommandData(discord.ApplicationCommandInteraction{Data: data}); ok {
			t.Fatalf("slashCommandData(%T) reported a slash command", data)
		}
	}
	adapter := &interactionAdapter{interaction: discord.ApplicationCommandInteraction{Data: discord.UserCommandInteractionData{}}}
	if q := adapter.Question(); q != "" {
		t.Fatalf("Question() of a user command = %q, want empty", q)
	}
}

func TestSlashCommandData_Positive_ReadsNameAndQuestion(t *testing.T) {
	raw := `{"id":"1","name":"ask","type":1,"options":[{"name":"question","type":3,"value":"How do I install it?"}]}`
	var slash discord.SlashCommandInteractionData
	if err := json.Unmarshal([]byte(raw), &slash); err != nil {
		t.Fatalf("unmarshal slash command data: %v", err)
	}
	data, ok := slashCommandData(discord.ApplicationCommandInteraction{Data: slash})
	if !ok || data.CommandName() != CommandName {
		t.Fatalf("slashCommandData = name %q ok %v, want %q true", data.CommandName(), ok, CommandName)
	}
	if got := extractQuestion(data); got != "How do I install it?" {
		t.Fatalf("extractQuestion = %q", got)
	}
}

func TestLibraryLogger_Negative_BodiesNeverReachTheLog(t *testing.T) {
	var buf bytes.Buffer
	lib := libraryLogger(debugLogger(&buf))

	// The library logs request and response bodies at debug level.
	lib.Debug("new request", slog.String("body", "question-text-4711"))
	lib.Info("info line", slog.String("body", "answer-text-0815"))
	lib.With("k", "v").Debug("derived", slog.String("body", "question-text-4711"))
	lib.WithGroup("g").Info("grouped", slog.String("body", "answer-text-0815"))
	if buf.Len() != 0 {
		t.Fatalf("library log below the floor reached the log: %s", buf.String())
	}
	if lib.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatalf("library logger reports debug as enabled")
	}

	lib.Warn("gateway closed")
	if !strings.Contains(buf.String(), "gateway closed") {
		t.Fatalf("library warning did not reach the log: %s", buf.String())
	}
}

func TestHandleAsk_Negative_PanicValueIsNotLogged(t *testing.T) {
	const question = "Private question 24680?"
	var buf bytes.Buffer
	asker := &fakeAsker{askFunc: func(_ context.Context, q string) (answer.Answer, error) {
		panic(fmt.Sprintf("failed while processing %q", q))
	}}
	h, err := NewHandler(HandlerConfig{Asker: asker, GuildID: 7, Logger: debugLogger(&buf)})
	if err != nil {
		t.Fatalf("NewHandler() failed: %v", err)
	}

	h.HandleAsk(context.Background(), &fakeInteraction{guildID: 7, question: question})

	if !strings.Contains(buf.String(), "recovered from panic") {
		t.Fatalf("panic was not logged: %s", buf.String())
	}
	if strings.Contains(buf.String(), "24680") {
		t.Fatalf("log carries the panic text with the question: %s", buf.String())
	}
	if h.InFlight() != 0 {
		t.Fatalf("InFlight() = %d after a panic, want 0", h.InFlight())
	}
}

func TestHandleAsk_Positive_SuccessLogsOutcomeWithoutContent(t *testing.T) {
	const question = "Private question 13579?"
	var buf bytes.Buffer
	asker := &fakeAsker{askFunc: func(context.Context, string) (answer.Answer, error) {
		return answer.Answer{Kind: answer.KindRefusal, Reason: answer.ReasonNoSource}, nil
	}}
	h, err := NewHandler(HandlerConfig{Asker: asker, GuildID: 7, Logger: debugLogger(&buf)})
	if err != nil {
		t.Fatalf("NewHandler() failed: %v", err)
	}

	h.HandleAsk(context.Background(), &fakeInteraction{guildID: 7, question: question})

	out := buf.String()
	if !strings.Contains(out, "outcome=refusal") || !strings.Contains(out, "reason=no_source") || !strings.Contains(out, "duration=") {
		t.Fatalf("success line misses outcome, reason or duration: %s", out)
	}
	if strings.Contains(out, "13579") || strings.Contains(out, answer.LabelRefusal) {
		t.Fatalf("success line carries question or reply text: %s", out)
	}
}

func TestGatewayIntents_Negative_NoIntentAndNoPrivilegedOne(t *testing.T) {
	intents := gatewayIntents()
	if intents != gateway.IntentsNone {
		t.Fatalf("gateway intents = %d, want none", intents)
	}
	if intents.Has(gateway.IntentGuildMembers) || intents.Has(gateway.IntentGuildPresences) || intents.Has(gateway.IntentMessageContent) {
		t.Fatalf("gateway intents %d include a privileged intent", intents)
	}
}

// ctxInteraction delivers a reply only while the context it is given is alive, as a real
// connection does.
type ctxInteraction struct {
	guildID  uint64
	question string

	mu      sync.Mutex
	replies []string
}

func (c *ctxInteraction) GuildID() uint64  { return c.guildID }
func (c *ctxInteraction) Question() string { return c.question }

func (c *ctxInteraction) Defer(ctx context.Context) error { return ctx.Err() }

func (c *ctxInteraction) Reply(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replies = append(c.replies, text)
	return nil
}

func (c *ctxInteraction) delivered() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.replies...)
}

func TestHandleAsk_Boundary_ReplyIsSentAfterTheCallerContextEnded(t *testing.T) {
	// A shutdown cancels the caller context while a member still waits. The member gets a reply.
	asker := &fakeAsker{askFunc: func(ctx context.Context, _ string) (answer.Answer, error) {
		return answer.Answer{}, ctx.Err()
	}}
	h, err := NewHandler(HandlerConfig{Asker: asker, GuildID: 7})
	if err != nil {
		t.Fatalf("NewHandler() failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := &ctxInteraction{guildID: 7, question: "Question?"}

	h.HandleAsk(ctx, in)

	got := in.delivered()
	if len(got) != 1 || got[0] != TextFailure {
		t.Fatalf("delivered replies = %q, want exactly the failure text", got)
	}
}

func TestFloorHandler_Negative_HandleDropsRecordsBelowTheFloor(t *testing.T) {
	var buf bytes.Buffer
	handler := floorHandler{inner: debugLogger(&buf).Handler()}

	// A caller that skips Enabled must still not get a body into the log.
	record := slog.NewRecord(time.Now(), slog.LevelDebug, "new response", 0)
	record.AddAttrs(slog.String("body", "answer-text-0815"))
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() failed: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("record below the floor reached the log: %s", buf.String())
	}

	warn := slog.NewRecord(time.Now(), slog.LevelWarn, "gateway closed", 0)
	if err := handler.Handle(context.Background(), warn); err != nil {
		t.Fatalf("Handle() failed: %v", err)
	}
	if !strings.Contains(buf.String(), "gateway closed") {
		t.Fatalf("record at the floor did not reach the log: %s", buf.String())
	}
}

func TestRegisterCommands_Negative_RefusesBeforeAnyClientExists(t *testing.T) {
	// Both refusals name their cause; a later failure of the library would hide a missing check.
	if err := RegisterCommands(context.Background(), "", 7); err == nil || !strings.Contains(err.Error(), "token is required") {
		t.Fatalf("RegisterCommands without token err = %v", err)
	}
	if err := RegisterCommands(context.Background(), "not-a-real-token", 0); err == nil || !strings.Contains(err.Error(), "guild ID must not be zero") {
		t.Fatalf("RegisterCommands with guild 0 err = %v", err)
	}
}
