package discord

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/answer"
)

func TestBuildCommandCreate_Positive_Properties(t *testing.T) {
	cmd := buildCommandCreate(0)

	if cmd.CommandName() != CommandName {
		t.Errorf("command name = %q, want %q", cmd.CommandName(), CommandName)
	}
	if cmd.Type() != discord.ApplicationCommandTypeSlash {
		t.Errorf("command type = %v, want slash", cmd.Type())
	}

	slashCmd, ok := cmd.(discord.SlashCommandCreate)
	if !ok {
		t.Fatalf("command type is not SlashCommandCreate: %T", cmd)
	}
	if slashCmd.Description == "" {
		t.Errorf("command description is empty")
	}
	wantDesc := "Ask the automated assistant a question about the project documentation"
	if slashCmd.Description != wantDesc {
		t.Errorf("command description = %q, want %q", slashCmd.Description, wantDesc)
	}
	if len(slashCmd.Options) != 1 {
		t.Fatalf("options len = %d, want 1", len(slashCmd.Options))
	}

	opt, ok := slashCmd.Options[0].(discord.ApplicationCommandOptionString)
	if !ok {
		t.Fatalf("option 0 is not ApplicationCommandOptionString: %T", slashCmd.Options[0])
	}
	if opt.Name != OptionQuestion {
		t.Errorf("option name = %q, want %q", opt.Name, OptionQuestion)
	}
	if !opt.Required {
		t.Errorf("option required = false, want true")
	}
	if opt.MaxLength == nil || *opt.MaxLength != answer.MaxQuestionRunes {
		t.Errorf("option max length = %v, want %d", opt.MaxLength, answer.MaxQuestionRunes)
	}
	if opt.Description == "" {
		t.Errorf("option description is empty")
	}
}

func TestBuildCommandCreate_Positive_CustomMaxQuestionLength(t *testing.T) {
	cmd := buildCommandCreate(10)
	slashCmd, ok := cmd.(discord.SlashCommandCreate)
	if !ok {
		t.Fatalf("command type is not SlashCommandCreate: %T", cmd)
	}
	if len(slashCmd.Options) != 1 {
		t.Fatalf("options len = %d, want 1", len(slashCmd.Options))
	}
	opt, ok := slashCmd.Options[0].(discord.ApplicationCommandOptionString)
	if !ok {
		t.Fatalf("option 0 is not ApplicationCommandOptionString: %T", slashCmd.Options[0])
	}
	if opt.MaxLength == nil || *opt.MaxLength != 10 {
		t.Errorf("option max length = %v, want 10", opt.MaxLength)
	}
}

func TestBuildDeferPayload_Positive_EphemeralAndAllowedMentions(t *testing.T) {
	payload := buildDeferPayload()

	if !payload.Flags.Has(discord.MessageFlagEphemeral) {
		t.Errorf("defer payload missing MessageFlagEphemeral: %v", payload.Flags)
	}
	if payload.AllowedMentions == nil {
		t.Fatalf("defer payload allowed mentions is nil")
	}
	if len(payload.AllowedMentions.Parse) != 0 {
		t.Errorf("defer payload parse mentions non-empty: %v", payload.AllowedMentions.Parse)
	}
	if len(payload.AllowedMentions.Roles) != 0 {
		t.Errorf("defer payload role mentions non-empty: %v", payload.AllowedMentions.Roles)
	}
	if len(payload.AllowedMentions.Users) != 0 {
		t.Errorf("defer payload user mentions non-empty: %v", payload.AllowedMentions.Users)
	}
	if payload.AllowedMentions.RepliedUser {
		t.Errorf("defer payload replied_user is true, want false")
	}
}

func TestBuildReplyPayload_Positive_ContentAndAllowedMentions(t *testing.T) {
	text := "Here is the rendered answer text."
	payload := buildReplyPayload(text)

	if payload.Content == nil || *payload.Content != text {
		t.Errorf("reply content = %v, want %q", payload.Content, text)
	}
	if payload.AllowedMentions == nil {
		t.Fatalf("reply payload allowed mentions is nil")
	}
	if len(payload.AllowedMentions.Parse) != 0 {
		t.Errorf("reply payload parse mentions non-empty: %v", payload.AllowedMentions.Parse)
	}
	if len(payload.AllowedMentions.Roles) != 0 {
		t.Errorf("reply payload role mentions non-empty: %v", payload.AllowedMentions.Roles)
	}
	if len(payload.AllowedMentions.Users) != 0 {
		t.Errorf("reply payload user mentions non-empty: %v", payload.AllowedMentions.Users)
	}
	if payload.AllowedMentions.RepliedUser {
		t.Errorf("reply payload replied_user is true, want false")
	}
}

func verifyDeferPayload(p discord.MessageCreate) error {
	if !p.Flags.Has(discord.MessageFlagEphemeral) {
		return errors.New("missing ephemeral flag")
	}
	if p.AllowedMentions == nil {
		return errors.New("nil allowed mentions")
	}
	if len(p.AllowedMentions.Parse) != 0 || len(p.AllowedMentions.Roles) != 0 || len(p.AllowedMentions.Users) != 0 || p.AllowedMentions.RepliedUser {
		return errors.New("non-empty allowed mentions")
	}
	return nil
}

func verifyReplyPayload(p discord.MessageUpdate) error {
	if p.AllowedMentions == nil {
		return errors.New("nil allowed mentions")
	}
	if len(p.AllowedMentions.Parse) != 0 || len(p.AllowedMentions.Roles) != 0 || len(p.AllowedMentions.Users) != 0 || p.AllowedMentions.RepliedUser {
		return errors.New("non-empty allowed mentions")
	}
	return nil
}

func TestBuildPayloads_Negative_ControlAllowedMentionsAndEphemeralFlag(t *testing.T) {
	// Assert normal builder succeeds
	if err := verifyDeferPayload(buildDeferPayload()); err != nil {
		t.Fatalf("legitimate defer payload failed verification: %v", err)
	}
	if err := verifyReplyPayload(buildReplyPayload("ok")); err != nil {
		t.Fatalf("legitimate reply payload failed verification: %v", err)
	}

	// Negative control 1: Defer payload without ephemeral flag fails
	missingEphemeral := buildDeferPayload()
	missingEphemeral.Flags = discord.MessageFlagsNone
	if err := verifyDeferPayload(missingEphemeral); err == nil {
		t.Errorf("verifyDeferPayload accepted missing ephemeral flag")
	}

	// Negative control 2: Defer payload with everyone parse mention fails
	leakyDeferMentions := buildDeferPayload()
	leakyDeferMentions.AllowedMentions = &discord.AllowedMentions{
		Parse: []discord.AllowedMentionType{discord.AllowedMentionTypeEveryone},
	}
	if err := verifyDeferPayload(leakyDeferMentions); err == nil {
		t.Errorf("verifyDeferPayload accepted leaky mentions")
	}

	// Negative control 3: Reply payload with role mention fails
	leakyReplyMentions := buildReplyPayload("ok")
	leakyReplyMentions.AllowedMentions = &discord.AllowedMentions{
		Roles: []snowflake.ID{123},
	}
	if err := verifyReplyPayload(leakyReplyMentions); err == nil {
		t.Errorf("verifyReplyPayload accepted leaky mentions")
	}
}

func TestExtractGuildID_Positive_And_Boundary(t *testing.T) {
	if got := extractGuildID(nil); got != 0 {
		t.Errorf("extractGuildID(nil) = %d, want 0", got)
	}
	id := snowflake.ID(987654321)
	if got := extractGuildID(&id); got != 987654321 {
		t.Errorf("extractGuildID(&id) = %d, want 987654321", got)
	}
}

func TestSanitizeToken_Positive_And_Negative(t *testing.T) {
	const secret = "token-secret-xyz-987"

	if got := sanitizeToken(nil, secret); got != nil {
		t.Errorf("sanitizeToken(nil) = %v, want nil", got)
	}

	plainErr := errors.New("connection failed to remote server")
	if got := sanitizeToken(plainErr, secret); !errors.Is(got, plainErr) {
		t.Errorf("sanitizeToken(plainErr) = %v, want %v", got, plainErr)
	}

	leakyErr := errors.New("unauthorized request with token token-secret-xyz-987 to api")
	sanitized := sanitizeToken(leakyErr, secret)
	if strings.Contains(sanitized.Error(), secret) {
		t.Errorf("sanitized error still contains secret: %v", sanitized)
	}
	if !strings.Contains(sanitized.Error(), "[redacted]") {
		t.Errorf("sanitized error does not contain [redacted]: %v", sanitized)
	}
}

func TestNewHandler_Negative_ValidationErrors(t *testing.T) {
	asker := &fakeAsker{}

	cases := []struct {
		name string
		cfg  HandlerConfig
	}{
		{
			name: "nil asker",
			cfg: HandlerConfig{
				Asker:   nil,
				GuildID: 42,
			},
		},
		{
			name: "zero guild ID",
			cfg: HandlerConfig{
				Asker:   asker,
				GuildID: 0,
			},
		},
		{
			name: "negative max concurrent",
			cfg: HandlerConfig{
				Asker:         asker,
				GuildID:       42,
				MaxConcurrent: -1,
			},
		},
		{
			name: "max concurrent over 64",
			cfg: HandlerConfig{
				Asker:         asker,
				GuildID:       42,
				MaxConcurrent: 65,
			},
		},
		{
			name: "negative ask timeout",
			cfg: HandlerConfig{
				Asker:      asker,
				GuildID:    42,
				AskTimeout: -time.Second,
			},
		},
		{
			name: "ask timeout over 10m",
			cfg: HandlerConfig{
				Asker:      asker,
				GuildID:    42,
				AskTimeout: 11 * time.Minute,
			},
		},
		{
			name: "negative reply timeout",
			cfg: HandlerConfig{
				Asker:        asker,
				GuildID:      42,
				ReplyTimeout: -time.Second,
			},
		},
		{
			name: "reply timeout over 1m",
			cfg: HandlerConfig{
				Asker:        asker,
				GuildID:      42,
				ReplyTimeout: 2 * time.Minute,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewHandler(tc.cfg)
			if err == nil {
				t.Fatalf("NewHandler() = %v, want error", h)
			}
		})
	}
}

func TestRegisterCommands_Negative_ValidationErrors(t *testing.T) {
	if err := RegisterCommands(context.Background(), "", 42, 0); err == nil {
		t.Errorf("RegisterCommands with empty token succeeded, want error")
	}
	if err := RegisterCommands(context.Background(), "token", 0, 0); err == nil {
		t.Errorf("RegisterCommands with zero guild ID succeeded, want error")
	}
}

func TestRun_Negative_ValidationErrors(t *testing.T) {
	asker := &fakeAsker{}
	h, err := NewHandler(HandlerConfig{Asker: asker, GuildID: 42})
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}

	cases := []struct {
		name string
		cfg  BotConfig
	}{
		{
			name: "empty token",
			cfg: BotConfig{
				Token:   "",
				GuildID: 42,
				Handler: h,
			},
		},
		{
			name: "zero guild ID",
			cfg: BotConfig{
				Token:   "some-token",
				GuildID: 0,
				Handler: h,
			},
		},
		{
			name: "nil handler",
			cfg: BotConfig{
				Token:   "some-token",
				GuildID: 42,
				Handler: nil,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Run(context.Background(), tc.cfg); err == nil {
				t.Errorf("Run() with %s succeeded, want error", tc.name)
			}
		})
	}
}

// makeFakeToken creates a synthetic bot token whose first dot-separated segment encodes the ID.
// NOTE: This is a synthetic test token for unit testing and is not a real credential.
func makeFakeToken(id uint64) string {
	seg := base64.RawStdEncoding.EncodeToString([]byte(strconv.FormatUint(id, 10)))
	return seg + ".synthetic-test-token.not-a-real-credential"
}

type recordedRequest struct {
	method string
	path   string
	body   []byte
}

func assertRecordedCommandPath(t *testing.T, recorded []recordedRequest, idA, idB, guildID uint64) {
	t.Helper()
	var cmdReq *recordedRequest
	for i := range recorded {
		if recorded[i].method == http.MethodPut && strings.Contains(recorded[i].path, "/commands") {
			cmdReq = &recorded[i]
			break
		}
	}
	if cmdReq == nil {
		t.Fatalf("no command list PUT request was recorded")
	}

	wantPath := fmt.Sprintf("/applications/%d/guilds/%d/commands", idB, guildID)
	if cmdReq.path != wantPath {
		t.Errorf("request path = %q, want %q", cmdReq.path, wantPath)
	}
	if strings.Contains(cmdReq.path, strconv.FormatUint(idA, 10)) {
		t.Errorf("request path contains token-derived id %d: %q", idA, cmdReq.path)
	}

	assertAskCommandBody(t, cmdReq.body)
}

func assertAskCommandBody(t *testing.T, body []byte) {
	t.Helper()
	var cmds []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Options     []struct {
			Name string `json:"name"`
		} `json:"options"`
	}
	if err := json.Unmarshal(body, &cmds); err != nil {
		t.Fatalf("unmarshal command list body: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("commands len = %d, want 1", len(cmds))
	}
	if cmds[0].Name != CommandName {
		t.Errorf("command name = %q, want %q", cmds[0].Name, CommandName)
	}
	if len(cmds[0].Options) != 1 || cmds[0].Options[0].Name != OptionQuestion {
		t.Errorf("command options = %+v, want option %q", cmds[0].Options, OptionQuestion)
	}
}

func writeTestResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)
	}
}

func TestRegisterCommands_Positive_MismatchFixtureUsesPlatformID(t *testing.T) {
	const (
		idA     uint64 = 111111111111111111
		idB     uint64 = 222222222222222222
		guildID uint64 = 333333333333333333
	)

	// fakeToken encodes ID A in its first segment; the server reports application ID B.
	// NOTE: This synthetic token is used only for testing and is not a real credential.
	fakeToken := makeFakeToken(idA)

	var mu sync.Mutex
	var recorded []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		recorded = append(recorded, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
		mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications/@me":
			writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":"diallak-app"}`, idB))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/applications/"):
			writeTestResponse(w, http.StatusOK, "[]")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	if err := registerCommandsWithOptions(context.Background(), fakeToken, guildID, opts); err != nil {
		t.Fatalf("registerCommandsWithOptions failed: %v", err)
	}

	mu.Lock()
	copied := append([]recordedRequest(nil), recorded...)
	mu.Unlock()
	assertRecordedCommandPath(t, copied, idA, idB, guildID)
}

func TestRegisterCommands_NegativeControl_FailsIfTokenDerivedIDUsed(t *testing.T) {
	const (
		idA     uint64 = 444444444444444444
		idB     uint64 = 555555555555555555
		guildID uint64 = 666666666666666666
	)
	// fakeToken encodes ID A, server returns ID B.
	// NOTE: This synthetic token is used only for testing and is not a real credential.
	fakeToken := makeFakeToken(idA)

	var mu sync.Mutex
	var recorded []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		mu.Lock()
		recorded = append(recorded, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
		mu.Unlock()

		if r.Method == http.MethodGet && r.URL.Path == "/applications/@me" {
			writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":"diallak-app"}`, idB))
			return
		}
		writeTestResponse(w, http.StatusOK, "[]")
	}))
	defer srv.Close()

	if err := registerCommandsWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL}); err != nil {
		t.Fatalf("registerCommandsWithOptions failed: %v", err)
	}

	mu.Lock()
	copied := append([]recordedRequest(nil), recorded...)
	mu.Unlock()
	assertRecordedCommandPath(t, copied, idA, idB, guildID)
}

func TestRegisterCommands_Negative_LookupFailuresFailClosed(t *testing.T) {
	cleanupCh := make(chan struct{})
	t.Cleanup(func() { close(cleanupCh) })

	const (
		idA     uint64 = 111111111111111111
		guildID uint64 = 333333333333333333
	)
	// NOTE: synthetic token for unit testing, not a real credential.
	fakeToken := makeFakeToken(idA)

	cases := []struct {
		name    string
		handler http.HandlerFunc
		ctxFn   func() (context.Context, context.CancelFunc)
	}{
		{
			name: "status 401",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeTestResponse(w, http.StatusUnauthorized, `{"message":"401: Unauthorized","code":0}`)
			},
		},
		{
			name: "status 500",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeTestResponse(w, http.StatusInternalServerError, `{"message":"Internal Server Error","code":0}`)
			},
		},
		{
			name: "malformed JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeTestResponse(w, http.StatusOK, `{not valid json`)
			},
		},
		{
			name: "empty id",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeTestResponse(w, http.StatusOK, `{"name":"diallak-app","description":"no id field"}`)
			},
		},
		{
			name: "server slower than deadline",
			handler: func(_ http.ResponseWriter, r *http.Request) {
				select {
				case <-cleanupCh:
				case <-r.Context().Done():
				}
			},
			ctxFn: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 20*time.Millisecond)
			},
		},
		{
			name: "cancelled ctx",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeTestResponse(w, http.StatusOK, `{"id":"222222222222222222"}`)
			},
			ctxFn: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runLookupFailureCase(t, tc.handler, tc.ctxFn, fakeToken, guildID)
		})
	}
}

func runLookupFailureCase(
	t *testing.T,
	handler http.HandlerFunc,
	ctxFn func() (context.Context, context.CancelFunc),
	token string,
	guildID uint64,
) {
	t.Helper()
	var mu sync.Mutex
	var commandRequests int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if strings.Contains(r.URL.Path, "/commands") {
			commandRequests++
		}
		mu.Unlock()
		handler(w, r)
	}))
	defer srv.Close()

	ctx := context.Background()
	if ctxFn != nil {
		c, cancel := ctxFn()
		defer cancel()
		ctx = c
	}

	err := registerCommandsWithOptions(ctx, token, guildID, clientOptions{restURL: srv.URL})
	if err == nil {
		t.Fatalf("registerCommandsWithOptions succeeded, want error")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("error contains token: %v", err)
	}

	mu.Lock()
	cmds := commandRequests
	mu.Unlock()
	if cmds != 0 {
		t.Errorf("server recorded %d command requests, want 0", cmds)
	}
}

func TestPreGatewayStep_Positive_LookupOkReturnsIDAndOpensGateway(t *testing.T) {
	const (
		idA     uint64 = 111111111111111111
		idB     uint64 = 222222222222222222
		guildID uint64 = 333333333333333333
	)
	// NOTE: synthetic token for unit testing, not a real credential.
	fakeToken := makeFakeToken(idA)

	var logBuf bytes.Buffer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/applications/@me" {
			writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":"diallak-app"}`, idB))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client, err := disgo.New(fakeToken, bot.WithRestClientConfigOpts(rest.WithURL(srv.URL)))
	if err != nil {
		t.Fatalf("disgo.New failed: %v", err)
	}
	defer client.Close(context.Background())

	var gatewayOpened bool
	opts := clientOptions{
		restURL: srv.URL,
		openGateway: func(_ context.Context, _ *bot.Client, _ string) error {
			gatewayOpened = true
			return nil
		},
	}

	cfg := BotConfig{
		Token:   fakeToken,
		GuildID: guildID,
		Logger:  slog.New(slog.NewJSONHandler(&logBuf, nil)),
	}

	gotID, err := preGatewayStep(context.Background(), cfg, client, opts)
	if err != nil {
		t.Fatalf("preGatewayStep failed: %v", err)
	}
	if gotID != snowflake.ID(idB) {
		t.Errorf("preGatewayStep appID = %v, want %d", gotID, idB)
	}
	if !gatewayOpened {
		t.Errorf("gatewayOpened = false, want true")
	}

	logText := logBuf.String()
	if !strings.Contains(logText, `"application_id"`) || !strings.Contains(logText, fmt.Sprint(idB)) {
		t.Errorf("log output missing application_id: %s", logText)
	}
	if !strings.Contains(logText, `"matches_token_id":false`) {
		t.Errorf("log output missing matches_token_id false: %s", logText)
	}
	if strings.Contains(logText, fakeToken) {
		t.Errorf("log output contains token: %s", logText)
	}
}

func TestPreGatewayStep_Negative_LookupErrorGatewayNeverOpened(t *testing.T) {
	const (
		idA     uint64 = 111111111111111111
		guildID uint64 = 333333333333333333
	)
	// NOTE: synthetic token for unit testing, not a real credential.
	fakeToken := makeFakeToken(idA)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := disgo.New(fakeToken, bot.WithRestClientConfigOpts(rest.WithURL(srv.URL)))
	if err != nil {
		t.Fatalf("disgo.New failed: %v", err)
	}
	defer client.Close(context.Background())

	var gatewayOpened bool
	opts := clientOptions{
		restURL: srv.URL,
		openGateway: func(_ context.Context, _ *bot.Client, _ string) error {
			gatewayOpened = true
			return nil
		},
	}

	cfg := BotConfig{
		Token:   fakeToken,
		GuildID: guildID,
		Logger:  slog.New(slog.DiscardHandler),
	}

	_, err = preGatewayStep(context.Background(), cfg, client, opts)
	if err == nil {
		t.Fatalf("preGatewayStep succeeded, want error")
	}
	if gatewayOpened {
		t.Errorf("gatewayOpened = true on lookup error, want false")
	}
	if strings.Contains(err.Error(), fakeToken) {
		t.Errorf("error contains token: %v", err)
	}
}

func TestPreGatewayStep_Positive_MatchesTokenIDTrue(t *testing.T) {
	const (
		idA     uint64 = 111111111111111111
		guildID uint64 = 333333333333333333
	)
	// NOTE: synthetic token for unit testing, not a real credential.
	fakeToken := makeFakeToken(idA)

	var logBuf bytes.Buffer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/applications/@me" {
			writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":"diallak-app"}`, idA))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client, err := disgo.New(fakeToken, bot.WithRestClientConfigOpts(rest.WithURL(srv.URL)))
	if err != nil {
		t.Fatalf("disgo.New failed: %v", err)
	}
	defer client.Close(context.Background())

	opts := clientOptions{
		restURL: srv.URL,
		openGateway: func(_ context.Context, _ *bot.Client, _ string) error {
			return nil
		},
	}

	cfg := BotConfig{
		Token:   fakeToken,
		GuildID: guildID,
		Logger:  slog.New(slog.NewJSONHandler(&logBuf, nil)),
	}

	gotID, err := preGatewayStep(context.Background(), cfg, client, opts)
	if err != nil {
		t.Fatalf("preGatewayStep failed: %v", err)
	}
	if gotID != snowflake.ID(idA) {
		t.Errorf("preGatewayStep appID = %v, want %d", gotID, idA)
	}

	logText := logBuf.String()
	if !strings.Contains(logText, `"matches_token_id":true`) {
		t.Errorf("log output missing matches_token_id true: %s", logText)
	}
}

func TestRegisterCommands_Positive_RegistrationPayloadDescription(t *testing.T) {
	const (
		appID   uint64 = 555555555555555555
		guildID uint64 = 666666666666666666
	)
	fakeToken := makeFakeToken(appID)

	var mu sync.Mutex
	var recordedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications/@me":
			writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":"diallak-app"}`, appID))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, fmt.Sprintf("/applications/%d/guilds/%d/commands", appID, guildID)):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read error", http.StatusInternalServerError)
				return
			}
			mu.Lock()
			recordedBody = body
			mu.Unlock()
			writeTestResponse(w, http.StatusOK, "[]")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	if err := registerCommandsWithOptions(context.Background(), fakeToken, guildID, opts); err != nil {
		t.Fatalf("registerCommandsWithOptions failed: %v", err)
	}

	mu.Lock()
	body := append([]byte(nil), recordedBody...)
	mu.Unlock()

	if len(body) == 0 {
		t.Fatalf("no registration PUT request recorded")
	}

	var payload []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal registered commands: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("registered %d commands, want 1", len(payload))
	}
	if payload[0].Description != CommandDescription {
		t.Errorf("registered command description = %q, want %q", payload[0].Description, CommandDescription)
	}
}

func TestBuildCommandCreate_Boundary_DescriptionLengthWithinLimit(t *testing.T) {
	// The platform limits the description of a command to 100 characters.
	// Source: Discord API documentation, interactions/application-commands: "1-100 character
	// description" (github.com/discord/discord-api-docs at 37148620d232, read 2026-10-10).
	cmd := buildCommandCreate(0)
	slashCmd, ok := cmd.(discord.SlashCommandCreate)
	if !ok {
		t.Fatalf("command is not SlashCommandCreate: %T", cmd)
	}
	descLen := utf8.RuneCountInString(slashCmd.Description)
	if descLen < 1 || descLen > 100 {
		t.Errorf("command description rune count = %d, want in [1, 100]", descLen)
	}
	if slashCmd.Description != CommandDescription {
		t.Errorf("command description = %q, want CommandDescription %q", slashCmd.Description, CommandDescription)
	}
}
