package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/rest"

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
	if strings.Contains(out, "13579") || strings.Contains(out, answer.Refusal("")) {
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
	wantFailure := answer.AppendFooter(TextFailure, "")
	if len(got) != 1 || got[0] != wantFailure {
		t.Fatalf("delivered replies = %q, want exactly the failure text with footer %q", got, wantFailure)
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
	if err := RegisterCommands(context.Background(), "", 7, 0); err == nil || !strings.Contains(err.Error(), "token is required") {
		t.Fatalf("RegisterCommands without token err = %v", err)
	}
	if err := RegisterCommands(context.Background(), "not-a-real-token", 0, 0); err == nil || !strings.Contains(err.Error(), "guild ID must not be zero") {
		t.Fatalf("RegisterCommands with guild 0 err = %v", err)
	}
}

// stubRest is a REST client whose application lookup returns a fixed answer.
type stubRest struct {
	rest.Rest
	app *discord.Application
	err error
}

func (s stubRest) GetCurrentApplication(...rest.RequestOpt) (*discord.Application, error) {
	return s.app, s.err
}

func TestLookupApplicationID_Negative_ErrorTextNeverHoldsTheToken(t *testing.T) {
	token := makeFakeToken(111111111111111111)
	// A transport error that prints its request headers would quote the token like this.
	echo := stubRest{err: errors.New("request with Authorization: Bot " + token + " failed")}
	_, err := lookupApplicationID(context.Background(), echo, token, time.Second)
	if err == nil {
		t.Fatalf("lookupApplicationID succeeded, want an error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("lookup error holds the token: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("lookup error does not show the redaction: %v", err)
	}
}

func TestLookupApplicationID_Negative_AnswerWithoutApplicationIsAnError(t *testing.T) {
	// A body of "null" with status 200 decodes to no application and no error.
	id, err := lookupApplicationID(context.Background(), stubRest{}, "token", time.Second)
	if !errors.Is(err, errEmptyApplicationID) {
		t.Fatalf("error = %v, want %v", err, errEmptyApplicationID)
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0", id)
	}
}

func TestLookupApplicationID_Negative_EndedCallerContextSendsNothing(t *testing.T) {
	var requests int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		writeTestResponse(w, http.StatusOK, `{"id":"222222222222222222"}`)
	}))
	t.Cleanup(srv.Close)

	token := makeFakeToken(111111111111111111)
	restClient := rest.New(rest.NewClient(token, rest.WithURL(srv.URL)))
	t.Cleanup(func() { restClient.Close(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lookupApplicationID(ctx, restClient, token, time.Second); err == nil {
		t.Fatalf("lookupApplicationID succeeded under an ended context")
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 0 {
		t.Fatalf("%d requests reached the platform under an ended context", requests)
	}
}

func TestRegisterCommands_Negative_SlowLookupHitsItsOwnDeadline(t *testing.T) {
	release := make(chan struct{})
	var commandRequests int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			mu.Lock()
			commandRequests++
			mu.Unlock()
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	// The caller sets no deadline, so only the lookup's own deadline can end the call.
	opts := clientOptions{restURL: srv.URL, lookupTimeout: 50 * time.Millisecond}
	start := time.Now()
	err := registerCommandsWithOptions(context.Background(), makeFakeToken(111111111111111111), 7, opts)
	if err == nil {
		t.Fatalf("registration succeeded against a platform that never answers")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the lookup took %v; its deadline did not apply", elapsed)
	}
	mu.Lock()
	defer mu.Unlock()
	if commandRequests != 0 {
		t.Fatalf("%d command list requests were made after a failed lookup", commandRequests)
	}
}

func TestHandleAsk_Positive_AnswerRepliesNameTheConfiguredPlace(t *testing.T) {
	const wantLastLine = "Automated reply. A person can help in #help-desk."
	kinds := []answer.Answer{
		{Kind: answer.KindAnswer, Text: "Use the flag [1].", Sources: []answer.Source{{N: 1, Title: "Guide", URL: "https://example.org/guide", Revision: "abc"}}},
		{Kind: answer.KindExcerpts, Sources: []answer.Source{{N: 1, Title: "Guide", URL: "https://example.org/guide", Revision: "abc", Excerpt: "Use the flag."}}},
		{Kind: answer.KindRefusal},
	}
	for _, ans := range kinds {
		t.Run(string(ans.Kind), func(t *testing.T) {
			h, err := NewHandler(HandlerConfig{
				Asker: &fakeAsker{askFunc: func(context.Context, string) (answer.Answer, error) {
					return ans, nil
				}},
				GuildID:       42,
				RenderOptions: answer.RenderOptions{Place: "#help-desk"},
			})
			if err != nil {
				t.Fatalf("NewHandler = %v", err)
			}
			in := &ctxInteraction{guildID: 42, question: "how do I use the flag"}
			h.HandleAsk(context.Background(), in)

			got := in.delivered()
			if len(got) != 1 {
				t.Fatalf("delivered replies = %q, want exactly one", got)
			}
			lines := strings.Split(got[0], "\n")
			if lines[len(lines)-1] != wantLastLine {
				t.Fatalf("last line = %q, want %q", lines[len(lines)-1], wantLastLine)
			}
			if ans.Kind == answer.KindRefusal && !strings.Contains(got[0], "Please ask in #help-desk so") {
				t.Fatalf("refusal does not name the configured place: %q", got[0])
			}
		})
	}
}

// assertCommandOptionMaxLength parses registered command payload and verifies the max_length property.
func assertCommandOptionMaxLength(t *testing.T, body []byte, want int) {
	t.Helper()
	var payload []struct {
		Options []struct {
			MaxLength int `json:"max_length"`
		} `json:"options"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("payload %q does not parse: %v", body, err)
	}
	if len(payload) != 2 || len(payload[0].Options) != 1 {
		t.Fatalf("payload %q is not the ask command with one option and the areas command", body)
	}
	if got := payload[0].Options[0].MaxLength; got != want {
		t.Fatalf("max_length = %d, want %d", got, want)
	}
}

// runRegisterCommandsPayloadTest tests that command registration options carry the expected max question length.
func runRegisterCommandsPayloadTest(t *testing.T, appID, guildID uint64, setting, want int) {
	t.Helper()
	var mu sync.Mutex
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/applications/@me" {
			writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d"}`, appID))
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		body = raw
		mu.Unlock()
		writeTestResponse(w, http.StatusOK, "[]")
	}))
	t.Cleanup(srv.Close)

	opts := clientOptions{restURL: srv.URL, maxQuestionChars: setting}
	if err := registerCommandsWithOptions(context.Background(), makeFakeToken(appID), guildID, opts); err != nil {
		t.Fatalf("registerCommandsWithOptions = %v", err)
	}

	mu.Lock()
	captured := body
	mu.Unlock()
	assertCommandOptionMaxLength(t, captured, want)
}

func TestRegisterCommands_Positive_PayloadCarriesTheQuestionLimit(t *testing.T) {
	const (
		appID   uint64 = 555555555555555555
		guildID uint64 = 666666666666666666
	)
	cases := []struct {
		name    string
		setting int
		want    int
	}{
		{"not set: the built-in 500", 0, 500},
		{"250", 250, 250},
		{"1", 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runRegisterCommandsPayloadTest(t, appID, guildID, tc.setting, tc.want)
		})
	}
}

func TestRegisterCommands_Negative_QuestionLimitOutOfRangeRefusedBeforeAnyClient(t *testing.T) {
	// The token is not a credential and no address is given: a call that got past the check
	// would try the real platform, so the error must come first and must name the limit.
	for _, limit := range []int{-1, answer.MaxQuestionRunes + 1} {
		err := RegisterCommands(context.Background(), "not-a-real-token", 7, limit)
		if err == nil || !strings.Contains(err.Error(), "question limit") {
			t.Fatalf("RegisterCommands with limit %d = %v, want a question limit error", limit, err)
		}
	}
}

func TestNewHandler_Boundary_LimitsInEffect(t *testing.T) {
	asker := &fakeAsker{}
	h, err := NewHandler(HandlerConfig{Asker: asker, GuildID: 42})
	if err != nil {
		t.Fatalf("NewHandler = %v", err)
	}
	if got, want := h.Limits(), (Limits{MaxConcurrent: 4, AskTimeout: 60 * time.Second, QuestionMaxChars: 500}); got != want {
		t.Fatalf("built-in limits = %+v, want %+v", got, want)
	}

	h, err = NewHandler(HandlerConfig{Asker: asker, GuildID: 42, MaxConcurrent: 8, AskTimeout: 30 * time.Second, QuestionMaxChars: 250})
	if err != nil {
		t.Fatalf("NewHandler = %v", err)
	}
	if got, want := h.Limits(), (Limits{MaxConcurrent: 8, AskTimeout: 30 * time.Second, QuestionMaxChars: 250}); got != want {
		t.Fatalf("configured limits = %+v, want %+v", got, want)
	}

	for _, limit := range []int{-1, answer.MaxQuestionRunes + 1} {
		if _, err := NewHandler(HandlerConfig{Asker: asker, GuildID: 42, QuestionMaxChars: limit}); err == nil {
			t.Fatalf("NewHandler accepted the question limit %d", limit)
		}
	}
}
