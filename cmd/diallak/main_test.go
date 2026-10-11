package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/discord"
	"github.com/tekmora/diallak/internal/health"
	"github.com/tekmora/diallak/internal/serverplan"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write refused") }

func writeSecret(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write secret fixture %q: %v", name, err)
	}
	return path
}

func setupTestCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	manifest := `{
		"version": 1,
		"sources": [
			{
				"directory": "alpha",
				"repository": "tekmora/alpha",
				"revision": "abcdef123456",
				"paths": ["docs/*.md"]
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(dir, corpus.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	docsDir := filepath.Join(dir, "alpha", "docs")
	if err := os.MkdirAll(docsDir, 0o700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	content := "# Overview\nDocumentation text for questions.\n"
	if err := os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	return dir
}

func validEnv(t *testing.T) func(string) string {
	t.Helper()
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	values := map[string]string{
		config.EnvTokenFile: tokenPath,
		config.EnvGuildID:   "42",
		config.EnvCorpusDir: corpusPath,
	}
	return func(key string) string { return values[key] }
}

func noopDeps() dependencies {
	return dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord:       func(_ context.Context, _ discord.BotConfig) error { return nil },
	}
}

func TestRun_Positive_ReadyWithoutModel(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), nil, validEnv(t), &out, noopDeps())
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	got := out.String()
	want := "ready: 1 sources, 1 chunks, model not configured\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if strings.Contains(got, "distinctive-token-xyz") {
		t.Errorf("output leaks the token: %q", got)
	}
}

func TestRun_Positive_ReadyWithModel(t *testing.T) {
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	keyPath := writeSecret(t, "key", "distinctive-key-abc")
	corpusPath := setupTestCorpus(t)

	envMap := map[string]string{
		config.EnvTokenFile:    tokenPath,
		config.EnvGuildID:      "42",
		config.EnvCorpusDir:    corpusPath,
		config.EnvModelBaseURL: "http://127.0.0.1:8000",
		config.EnvModelAlias:   "router/test",
		config.EnvModelKeyFile: keyPath,
	}

	var out bytes.Buffer
	err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, noopDeps())
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	got := out.String()
	want := "ready: 1 sources, 1 chunks, model not ready\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if strings.Contains(got, "distinctive-token-xyz") {
		t.Errorf("output leaks the token: %q", got)
	}
	if strings.Contains(got, "distinctive-key-abc") {
		t.Errorf("output leaks the model key: %q", got)
	}
}

// wrongServerInteraction is a command from a server the bot does not serve. It records the
// replies it is given.
type wrongServerInteraction struct {
	replies []string
}

func (*wrongServerInteraction) GuildID() uint64             { return 7 }
func (*wrongServerInteraction) Question() string            { return "unused" }
func (*wrongServerInteraction) Defer(context.Context) error { return nil }

func (w *wrongServerInteraction) Reply(_ context.Context, text string) error {
	w.replies = append(w.replies, text)
	return nil
}

func TestRun_Positive_SupportChannelReachesTheReplies(t *testing.T) {
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:      tokenPath,
		config.EnvGuildID:        "42",
		config.EnvCorpusDir:      corpusPath,
		config.EnvSupportChannel: "#help-desk",
	}

	var capturedCfg discord.BotConfig
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord: func(_ context.Context, cfg discord.BotConfig) error {
			capturedCfg = cfg
			return nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if capturedCfg.Handler == nil {
		t.Fatalf("the Discord wiring got no handler")
	}

	// The handler is the one the process would serve with: a reply it sends must name the
	// configured place in its last line.
	in := &wrongServerInteraction{}
	capturedCfg.Handler.HandleAsk(context.Background(), in)
	if len(in.replies) != 1 {
		t.Fatalf("replies = %q, want exactly one", in.replies)
	}
	const wantLastLine = "Automated reply. A person can help in #help-desk."
	lines := strings.Split(in.replies[0], "\n")
	if lines[len(lines)-1] != wantLastLine {
		t.Errorf("last line = %q, want %q", lines[len(lines)-1], wantLastLine)
	}
}

func TestRun_Positive_RegisterCommands(t *testing.T) {
	var out bytes.Buffer
	var calledWithToken string
	var calledWithGuild uint64

	deps := dependencies{
		registerCommands: func(_ context.Context, token string, guildID uint64, _ int) error {
			calledWithToken = token
			calledWithGuild = guildID
			return nil
		},
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			return errors.New("runDiscord should not be called")
		},
	}

	args := []string{"-register-commands"}
	err := run(context.Background(), args, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}

	got := out.String()
	want := "commands registered for guild 42\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if calledWithGuild != 42 {
		t.Errorf("calledWithGuild = %d, want 42", calledWithGuild)
	}
	if calledWithToken != "distinctive-token-xyz" {
		t.Errorf("calledWithToken = %q, want distinctive-token-xyz", calledWithToken)
	}
	if strings.Contains(got, "distinctive-token-xyz") {
		t.Errorf("output leaks token: %q", got)
	}
}

func TestRun_Negative_ControlPathIsolation(t *testing.T) {
	var registerCalls int
	var runCalls int

	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
			registerCalls++
			return nil
		},
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			runCalls++
			return nil
		},
	}

	// Case 1: with -register-commands, register is called and run is NEVER called.
	var regOut bytes.Buffer
	if err := run(context.Background(), []string{"-register-commands"}, validEnv(t), &regOut, deps); err != nil {
		t.Fatalf("run with -register-commands failed: %v", err)
	}
	if registerCalls != 1 {
		t.Errorf("registerCalls with flag = %d, want 1", registerCalls)
	}
	if runCalls != 0 {
		t.Errorf("runCalls with register flag = %d, want 0", runCalls)
	}

	// Reset counters
	registerCalls = 0
	runCalls = 0

	// Case 2: without -register-commands, run is called and register is NEVER called.
	var runOut bytes.Buffer
	if err := run(context.Background(), []string{}, validEnv(t), &runOut, deps); err != nil {
		t.Fatalf("run without flag failed: %v", err)
	}
	if registerCalls != 0 {
		t.Errorf("registerCalls without flag = %d, want 0", registerCalls)
	}
	if runCalls != 1 {
		t.Errorf("runCalls without flag = %d, want 1", runCalls)
	}
}

func TestRun_Negative_ConfigurationErrors(t *testing.T) {
	distinctToken := "distinct-secret-token-111"
	distinctKey := "distinct-secret-key-222"
	tokenPath := writeSecret(t, "token", distinctToken)
	keyPath := writeSecret(t, "key", distinctKey)
	corpusPath := setupTestCorpus(t)

	cases := []struct {
		name    string
		env     map[string]string
		wantVar string
	}{
		{
			name: "guild missing",
			env: map[string]string{
				config.EnvTokenFile: tokenPath,
				config.EnvCorpusDir: corpusPath,
			},
			wantVar: config.EnvGuildID,
		},
		{
			name: "token missing",
			env: map[string]string{
				config.EnvGuildID:   "42",
				config.EnvCorpusDir: corpusPath,
			},
			wantVar: config.EnvTokenFile,
		},
		{
			name: "corpus missing",
			env: map[string]string{
				config.EnvGuildID:   "42",
				config.EnvTokenFile: tokenPath,
			},
			wantVar: config.EnvCorpusDir,
		},
		{
			name: "model base without alias",
			env: map[string]string{
				config.EnvGuildID:      "42",
				config.EnvTokenFile:    tokenPath,
				config.EnvCorpusDir:    corpusPath,
				config.EnvModelBaseURL: "http://127.0.0.1:8000",
			},
			wantVar: config.EnvModelBaseURL,
		},
		{
			name: "model alias without base",
			env: map[string]string{
				config.EnvGuildID:    "42",
				config.EnvTokenFile:  tokenPath,
				config.EnvCorpusDir:  corpusPath,
				config.EnvModelAlias: "router/test",
			},
			wantVar: config.EnvModelAlias,
		},
		{
			name: "model key without model",
			env: map[string]string{
				config.EnvGuildID:      "42",
				config.EnvTokenFile:    tokenPath,
				config.EnvCorpusDir:    corpusPath,
				config.EnvModelKeyFile: keyPath,
			},
			wantVar: config.EnvModelKeyFile,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(context.Background(), nil, func(k string) string { return tc.env[k] }, &out, noopDeps())
			if err == nil {
				t.Fatalf("run succeeded, want error naming %q", tc.wantVar)
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.wantVar) {
				t.Errorf("error %q does not name expected variable %q", errStr, tc.wantVar)
			}
			if strings.Contains(errStr, distinctToken) {
				t.Errorf("error leaks token: %q", errStr)
			}
			if strings.Contains(errStr, distinctKey) {
				t.Errorf("error leaks model key: %q", errStr)
			}
			if out.Len() != 0 {
				t.Errorf("run wrote output before configuration failure: %q", out.String())
			}
		})
	}
}

func TestRun_Negative_BuildFailure(t *testing.T) {
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	envMap := map[string]string{
		config.EnvTokenFile: tokenPath,
		config.EnvGuildID:   "42",
		config.EnvCorpusDir: filepath.Join(t.TempDir(), "nonexistent"),
	}

	var out bytes.Buffer
	err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, noopDeps())
	if err == nil || !strings.Contains(err.Error(), "build application") {
		t.Fatalf("run = %v, want build application error", err)
	}
	if out.Len() != 0 {
		t.Errorf("run wrote output before build failure: %q", out.String())
	}
}

func TestRun_Negative_RegisterCommandsError(t *testing.T) {
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
			return errors.New("network failure during registration")
		},
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			return nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-register-commands"}, validEnv(t), &out, deps)
	if err == nil || !strings.Contains(err.Error(), "register commands: network failure during registration") {
		t.Fatalf("run = %v, want register commands error", err)
	}
}

func TestRun_Negative_RunDiscordError(t *testing.T) {
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
			return nil
		},
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			return errors.New("gateway connection closed")
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), nil, validEnv(t), &out, deps)
	if err == nil || !strings.Contains(err.Error(), "run discord bot: gateway connection closed") {
		t.Fatalf("run = %v, want run discord bot error", err)
	}
}

func TestRun_Negative_InvalidFlag(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), []string{"-nonexistent-flag"}, validEnv(t), &out, noopDeps())
	if err == nil || !strings.Contains(err.Error(), "parse flags") {
		t.Fatalf("run = %v, want parse flags error", err)
	}
}

func TestRun_Boundary_UnwritableOutputFails(t *testing.T) {
	err := run(context.Background(), nil, validEnv(t), failingWriter{}, noopDeps())
	if err == nil || !strings.Contains(err.Error(), "write status") {
		t.Fatalf("run = %v, want a write error", err)
	}

	errReg := run(context.Background(), []string{"-register-commands"}, validEnv(t), failingWriter{}, noopDeps())
	if errReg == nil || !strings.Contains(errReg.Error(), "write status") {
		t.Fatalf("run with -register-commands = %v, want a write error", errReg)
	}
}

func TestReport_Positive_NoErrorExitsZero(t *testing.T) {
	var errOut bytes.Buffer
	if code := report(nil, &errOut); code != 0 || errOut.Len() != 0 {
		t.Fatalf("report(nil) = %d with output %q, want 0 and no output", code, errOut.String())
	}
}

func TestReport_Negative_ErrorIsPrintedAndExitsOne(t *testing.T) {
	var errOut bytes.Buffer
	if code := report(errors.New("boom"), &errOut); code != 1 {
		t.Fatalf("report(error) = %d, want 1", code)
	}
	if got := errOut.String(); got != "diallak: boom\n" {
		t.Errorf("report wrote %q, want the prefixed error", got)
	}
}

func TestReport_Boundary_UnwritableErrorOutputExitsTwo(t *testing.T) {
	if code := report(errors.New("boom"), failingWriter{}); code != 2 {
		t.Fatalf("report with an unwritable error output = %d, want 2", code)
	}
}

func TestRun_Positive_OptionalConfigurationThreaded(t *testing.T) {
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:          tokenPath,
		config.EnvGuildID:            "42",
		config.EnvCorpusDir:          corpusPath,
		config.EnvTopPassages:        "2",
		config.EnvMinScore:           "0.30",
		config.EnvQuestionMaxChars:   "250",
		config.EnvExcerptChars:       "150",
		config.EnvReplyMaxChars:      "800",
		config.EnvMaxConcurrentAsks:  "8",
		config.EnvAskTimeout:         "30s",
		config.EnvSourceLinkTemplate: "https://git.example.com/{repository}/blob/{revision}/{path}",
	}

	var capturedCfg discord.BotConfig
	var calledWithMaxQuestion int
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, maxQuestionChars int) error {
			calledWithMaxQuestion = maxQuestionChars
			return nil
		},
		runDiscord: func(_ context.Context, cfg discord.BotConfig) error {
			capturedCfg = cfg
			return nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if capturedCfg.Handler == nil {
		t.Fatalf("run got nil handler")
	}
	wantLimits := discord.Limits{MaxConcurrent: 8, AskTimeout: 30 * time.Second, QuestionMaxChars: 250}
	if got := capturedCfg.Handler.Limits(); got != wantLimits {
		t.Errorf("limits of the handler the process serves with = %+v, want %+v", got, wantLimits)
	}

	var regOut bytes.Buffer
	err = run(context.Background(), []string{"-register-commands"}, func(k string) string { return envMap[k] }, &regOut, deps)
	if err != nil {
		t.Fatalf("run -register-commands = %v, want nil", err)
	}
	if calledWithMaxQuestion != 250 {
		t.Errorf("calledWithMaxQuestion = %d, want 250", calledWithMaxQuestion)
	}
}

func TestRun_Positive_HealthEndpointServesAndReportsReady(t *testing.T) {
	ctx := context.Background()
	// Allocate free ephemeral port
	var lc net.ListenConfig
	probeLn, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := probeLn.Addr().String()
	if closeErr := probeLn.Close(); closeErr != nil {
		t.Fatalf("close probe listener: %v", closeErr)
	}

	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:  tokenPath,
		config.EnvGuildID:    "42",
		config.EnvCorpusDir:  corpusPath,
		config.EnvHealthAddr: addr,
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	var readyChecked bool

	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
			return errors.New("registerCommands should not be called")
		},
		runDiscord: func(discordCtx context.Context, cfg discord.BotConfig) error {
			if cfg.ReportReady == nil {
				return errors.New("cfg.ReportReady is nil")
			}
			// Probe /healthz while fake Discord run blocks
			healthReq, reqErr := http.NewRequestWithContext(discordCtx, http.MethodGet, "http://"+addr+"/healthz", nil)
			if reqErr != nil {
				return fmt.Errorf("new health request: %w", reqErr)
			}
			resp, getErr := httpClient.Do(healthReq)
			if getErr != nil {
				return fmt.Errorf("probe /healthz: %w", getErr)
			}
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return fmt.Errorf("read health body: %w", readErr)
			}
			if closeErr := resp.Body.Close(); closeErr != nil {
				return fmt.Errorf("close health body: %w", closeErr)
			}
			if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
				return fmt.Errorf("/healthz = %d %q, want 200 ok\\n", resp.StatusCode, string(body))
			}

			// Probe /readyz before ReportReady(true) -> 503
			readyReq1, reqErr := http.NewRequestWithContext(discordCtx, http.MethodGet, "http://"+addr+"/readyz", nil)
			if reqErr != nil {
				return fmt.Errorf("new ready request: %w", reqErr)
			}
			readyResp, readyErr := httpClient.Do(readyReq1)
			if readyErr != nil {
				return fmt.Errorf("probe /readyz before ready: %w", readyErr)
			}
			if closeErr := readyResp.Body.Close(); closeErr != nil {
				return fmt.Errorf("close ready body: %w", closeErr)
			}
			if readyResp.StatusCode != http.StatusServiceUnavailable {
				return fmt.Errorf("/readyz before ready = %d, want 503", readyResp.StatusCode)
			}

			// Call ReportReady(true)
			cfg.ReportReady(true)

			// Probe /readyz after ReportReady(true) -> 200
			readyReq2, reqErr := http.NewRequestWithContext(discordCtx, http.MethodGet, "http://"+addr+"/readyz", nil)
			if reqErr != nil {
				return fmt.Errorf("new ready request 2: %w", reqErr)
			}
			readyResp2, readyErr2 := httpClient.Do(readyReq2)
			if readyErr2 != nil {
				return fmt.Errorf("probe /readyz after ready: %w", readyErr2)
			}
			body2, readErr2 := io.ReadAll(readyResp2.Body)
			if readErr2 != nil {
				return fmt.Errorf("read ready body 2: %w", readErr2)
			}
			if closeErr := readyResp2.Body.Close(); closeErr != nil {
				return fmt.Errorf("close ready body 2: %w", closeErr)
			}
			if readyResp2.StatusCode != http.StatusOK || string(body2) != "ready\n" {
				return fmt.Errorf("/readyz after ready = %d %q, want 200 ready\\n", readyResp2.StatusCode, string(body2))
			}

			readyChecked = true
			return nil
		},
	}

	var out bytes.Buffer
	err = run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if !readyChecked {
		t.Fatal("health checks were not verified")
	}

	// Listener should be closed after run completes
	probeReq, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/healthz", nil)
	if reqErr != nil {
		t.Fatalf("new probe request: %v", reqErr)
	}
	probeResp, probeAfterErr := httpClient.Do(probeReq)
	if probeAfterErr == nil {
		if closeErr := probeResp.Body.Close(); closeErr != nil {
			t.Errorf("close probe response: %v", closeErr)
		}
		t.Fatal("health server still responding after run completed")
	}
}

func TestRun_Negative_HealthAddrAlreadyInUse(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() {
		if closeErr := ln.Close(); closeErr != nil {
			t.Errorf("close listener: %v", closeErr)
		}
	}()

	addr := ln.Addr().String()
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:  tokenPath,
		config.EnvGuildID:    "42",
		config.EnvCorpusDir:  corpusPath,
		config.EnvHealthAddr: addr,
	}

	var discordCalled bool
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			discordCalled = true
			return nil
		},
	}

	var out bytes.Buffer
	err = run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps)
	if err == nil {
		t.Fatal("run succeeded on address in use, want error")
	}
	if !strings.Contains(err.Error(), config.EnvHealthAddr) {
		t.Errorf("error %q does not name %s", err.Error(), config.EnvHealthAddr)
	}
	if discordCalled {
		t.Errorf("runDiscord was called although health address bind failed")
	}
}

func TestRun_Negative_HealthAddrUnsetNoListener(t *testing.T) {
	var discordCalled bool
	var capturedCfg discord.BotConfig

	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord: func(_ context.Context, cfg discord.BotConfig) error {
			discordCalled = true
			capturedCfg = cfg
			return nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), nil, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if !discordCalled {
		t.Fatal("runDiscord was not called")
	}
	if capturedCfg.ReportReady != nil {
		t.Error("capturedCfg.ReportReady is non-nil, want nil when health addr is unset")
	}
}

func TestRun_Positive_RegisterCommandsNeverStartsHealthListener(t *testing.T) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() {
		if closeErr := ln.Close(); closeErr != nil {
			t.Errorf("close listener: %v", closeErr)
		}
	}()

	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:  tokenPath,
		config.EnvGuildID:    "42",
		config.EnvCorpusDir:  corpusPath,
		config.EnvHealthAddr: ln.Addr().String(),
	}

	var registerCalled bool
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
			registerCalled = true
			return nil
		},
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			return errors.New("runDiscord should not be called")
		},
	}

	var out bytes.Buffer
	err = run(context.Background(), []string{"-register-commands"}, func(k string) string { return envMap[k] }, &out, deps)
	if err != nil {
		t.Fatalf("run -register-commands failed: %v", err)
	}
	if !registerCalled {
		t.Fatal("registerCommands was not called")
	}
}

// brokenListener is a listener whose Accept fails for good once fail is closed.
type brokenListener struct {
	fail chan struct{}
	addr net.Addr
}

func (b *brokenListener) Accept() (net.Conn, error) {
	<-b.fail
	return nil, errors.New("accept refused by the test")
}

func (*brokenListener) Close() error     { return nil }
func (b *brokenListener) Addr() net.Addr { return b.addr }

func TestRunDiscordWithHealth_Negative_FailingListenerStopsTheBotAndIsReported(t *testing.T) {
	ln := &brokenListener{fail: make(chan struct{}), addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}}
	state := &health.State{}
	hh := &healthHarness{state: state, server: health.NewServer(state, nil)}

	botStopped := make(chan struct{})
	runDiscord := func(ctx context.Context, _ discord.BotConfig) error {
		// The bot is running; now the health listener breaks. The bot must be told to stop.
		close(ln.fail)
		select {
		case <-ctx.Done():
			close(botStopped)
			return nil
		case <-time.After(5 * time.Second):
			return errors.New("the bot was not stopped")
		}
	}

	err := runDiscordWithHealth(context.Background(), hh, ln, discord.BotConfig{}, runDiscord)
	select {
	case <-botStopped:
	default:
		t.Fatalf("the bot kept running after the health listener failed (err = %v)", err)
	}
	if err == nil || !strings.Contains(err.Error(), "health server") || !strings.Contains(err.Error(), "accept refused by the test") {
		t.Fatalf("err = %v, want the health server's accept error", err)
	}
}

func TestRun_Negative_UnknownAliasStopsTheStart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:    tokenPath,
		config.EnvGuildID:      "42",
		config.EnvCorpusDir:    corpusPath,
		config.EnvModelBaseURL: server.URL,
		config.EnvModelAlias:   "test-alias",
	}

	var out bytes.Buffer
	var discordCalls int
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			discordCalls++
			return nil
		},
	}

	err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps)
	if err == nil {
		t.Fatal("run expected error, got nil")
	}
	if !strings.Contains(err.Error(), "model start probe") {
		t.Errorf("error %q should contain 'model start probe'", err.Error())
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("error %q should contain 'status 404'", err.Error())
	}
	want := "model start probe: model endpoint returned status 404; check " +
		config.EnvModelBaseURL + ", " + config.EnvModelAlias + " and " + config.EnvModelKeyFile
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if discordCalls != 0 {
		t.Errorf("discordCalls = %d, want 0", discordCalls)
	}
	if strings.Contains(out.String(), "ready:") {
		t.Errorf("output should not contain ready line, got: %q", out.String())
	}
}

func TestRun_Negative_RejectedProbeStopsTheStart(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusBadRequest} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
			corpusPath := setupTestCorpus(t)
			envMap := map[string]string{
				config.EnvTokenFile:    tokenPath,
				config.EnvGuildID:      "42",
				config.EnvCorpusDir:    corpusPath,
				config.EnvModelBaseURL: server.URL,
				config.EnvModelAlias:   "test-alias",
			}

			var out bytes.Buffer
			var discordCalls int
			deps := dependencies{
				registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
				runDiscord: func(_ context.Context, _ discord.BotConfig) error {
					discordCalls++
					return nil
				},
			}

			err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps)
			if err == nil {
				t.Fatal("run expected error, got nil")
			}
			if !strings.Contains(err.Error(), "model start probe") {
				t.Errorf("error %q should contain 'model start probe'", err.Error())
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("status %d", status)) {
				t.Errorf("error %q should contain 'status %d'", err.Error(), status)
			}
			if discordCalls != 0 {
				t.Errorf("discordCalls = %d, want 0", discordCalls)
			}
			if strings.Contains(out.String(), "ready:") {
				t.Errorf("output should not contain ready line, got: %q", out.String())
			}
		})
	}
}

func TestRun_Positive_ProbeUnavailableAllowsStart(t *testing.T) {
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)

	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{name: "503", baseURL: ""},
		{name: "closed_port", baseURL: "http://127.0.0.1:1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := tc.baseURL
			if baseURL == "" {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				defer server.Close()
				baseURL = server.URL
			}

			envMap := map[string]string{
				config.EnvTokenFile:    tokenPath,
				config.EnvGuildID:      "42",
				config.EnvCorpusDir:    corpusPath,
				config.EnvModelBaseURL: baseURL,
				config.EnvModelAlias:   "test-alias",
			}

			var out bytes.Buffer
			var discordCalls int
			deps := dependencies{
				registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
				runDiscord: func(_ context.Context, _ discord.BotConfig) error {
					discordCalls++
					return nil
				},
			}

			if err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps); err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if discordCalls != 1 {
				t.Errorf("discordCalls = %d, want 1", discordCalls)
			}
			if !strings.HasSuffix(out.String(), "model not ready\n") {
				t.Errorf("output %q should end with 'model not ready\\n'", out.String())
			}
		})
	}
}

func TestRun_Positive_ProbeReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:    tokenPath,
		config.EnvGuildID:      "42",
		config.EnvCorpusDir:    corpusPath,
		config.EnvModelBaseURL: server.URL,
		config.EnvModelAlias:   "test-alias",
	}

	var out bytes.Buffer
	var discordCalls int
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			discordCalls++
			return nil
		},
	}

	if err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if discordCalls != 1 {
		t.Errorf("discordCalls = %d, want 1", discordCalls)
	}
	if !strings.HasSuffix(out.String(), "model ready\n") {
		t.Errorf("output %q should end with 'model ready\\n'", out.String())
	}
}

func TestRun_Positive_NoModelVariables(t *testing.T) {
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile: tokenPath,
		config.EnvGuildID:   "42",
		config.EnvCorpusDir: corpusPath,
	}

	var out bytes.Buffer
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord:       func(_ context.Context, _ discord.BotConfig) error { return nil },
	}

	if err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.HasSuffix(out.String(), "model not configured\n") {
		t.Errorf("output %q should end with 'model not configured\\n'", out.String())
	}
}

func TestRun_Positive_RegisterCommandsSendsNoProbe(t *testing.T) {
	var probeCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		probeCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:    tokenPath,
		config.EnvGuildID:      "42",
		config.EnvCorpusDir:    corpusPath,
		config.EnvModelBaseURL: server.URL,
		config.EnvModelAlias:   "test-alias",
	}

	var out bytes.Buffer
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord:       func(_ context.Context, _ discord.BotConfig) error { return nil },
	}

	args := []string{"-register-commands"}
	if err := run(context.Background(), args, func(k string) string { return envMap[k] }, &out, deps); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if probeCalls != 0 {
		t.Errorf("probeCalls = %d, want 0", probeCalls)
	}
}

func fetchModelz(ctx context.Context, addr string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/modelz", nil)
	if err != nil {
		return "", fmt.Errorf("new /modelz request: %w", err)
	}
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get /modelz: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if closeErr := resp.Body.Close(); closeErr != nil {
		return "", fmt.Errorf("close /modelz body: %w", closeErr)
	}
	if readErr != nil {
		return "", fmt.Errorf("read /modelz body: %w", readErr)
	}
	return fmt.Sprintf("%d %s", resp.StatusCode, body), nil
}

func TestRun_Positive_HealthEndpointReportsTheModelState(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{name: "probe passes", status: http.StatusOK, want: "200 model: ready\n"},
		{name: "probe meets a server error", status: http.StatusServiceUnavailable, want: "200 model: not ready\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			var lc net.ListenConfig
			probeLn, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			addr := probeLn.Addr().String()
			if closeErr := probeLn.Close(); closeErr != nil {
				t.Fatalf("close probe listener: %v", closeErr)
			}

			envMap := map[string]string{
				config.EnvTokenFile:    writeSecret(t, "token", "distinctive-token-xyz"),
				config.EnvGuildID:      "42",
				config.EnvCorpusDir:    setupTestCorpus(t),
				config.EnvHealthAddr:   addr,
				config.EnvModelBaseURL: server.URL,
				config.EnvModelAlias:   "test-alias",
			}
			var got string
			deps := dependencies{
				registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
				runDiscord: func(ctx context.Context, _ discord.BotConfig) error {
					var fetchErr error
					got, fetchErr = fetchModelz(ctx, addr)
					return fetchErr
				},
			}

			var out bytes.Buffer
			if err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps); err != nil {
				t.Fatalf("run = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("/modelz = %q, want %q", got, tc.want)
			}
		})
	}
}

func loadFreshServerSnapshot(t *testing.T) *serverplan.Snapshot {
	t.Helper()
	data, err := os.ReadFile("../../internal/serverplan/testdata/fresh-server.json")
	if err != nil {
		t.Fatalf("read fresh-server.json: %v", err)
	}
	s, err := serverplan.LoadSnapshot(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("load fresh-server snapshot: %v", err)
	}
	return s
}

func TestRun_Positive_ServerPlan_ExitZeroAndOutputAndDigest(t *testing.T) {
	snap := loadFreshServerSnapshot(t)
	var readCalls int
	var calledWithToken string
	var calledWithGuild uint64

	deps := dependencies{
		readServer: func(_ context.Context, token string, guildID uint64) (*serverplan.Snapshot, error) {
			readCalls++
			calledWithToken = token
			calledWithGuild = guildID
			return snap, nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=plan"}, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run -server=plan failed: %v", err)
	}
	if readCalls != 1 {
		t.Errorf("readCalls = %d, want 1", readCalls)
	}
	if calledWithGuild != 42 {
		t.Errorf("calledWithGuild = %d, want 42", calledWithGuild)
	}
	if calledWithToken != "distinctive-token-xyz" {
		t.Errorf("calledWithToken = %q, want distinctive-token-xyz", calledWithToken)
	}

	got := out.String()
	if !strings.HasPrefix(got, "mode: ") {
		t.Errorf("output does not start with 'mode: ': %q", got)
	}
	if !strings.Contains(got, "changes:") {
		t.Errorf("output does not contain 'changes:': %q", got)
	}
	if !strings.Contains(got, "plan digest: sha256:") {
		t.Errorf("output does not contain 'plan digest: sha256:': %q", got)
	}
	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 0 {
		t.Errorf("report exit code = %d, want 0", code)
	}
}

func TestRun_Positive_ServerPlan_ExitZeroWithRefusals(t *testing.T) {
	snap := loadFreshServerSnapshot(t)
	snap.Caller.Permissions = []string{"VIEW_CHANNEL"}

	var readCalls int
	deps := dependencies{
		readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			readCalls++
			return snap, nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=plan"}, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run -server=plan failed with refusals: %v", err)
	}
	if readCalls != 1 {
		t.Errorf("readCalls = %d, want 1", readCalls)
	}

	got := out.String()
	if !strings.Contains(got, "refused:") {
		t.Errorf("output should contain 'refused:': %q", got)
	}
	if !strings.Contains(got, "plan digest: sha256:") {
		t.Errorf("output should contain 'plan digest: sha256:': %q", got)
	}
	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 0 {
		t.Errorf("report exit code = %d, want 0", code)
	}
}

func TestRun_Positive_ServerCompare_ExitZeroAndDifferences(t *testing.T) {
	snap := loadFreshServerSnapshot(t)
	var readCalls int
	deps := dependencies{
		readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			readCalls++
			return snap, nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=compare"}, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run -server=compare failed: %v", err)
	}
	if readCalls != 1 {
		t.Errorf("readCalls = %d, want 1", readCalls)
	}

	got := out.String()
	if !strings.HasPrefix(got, "mode: ") {
		t.Errorf("output does not start with 'mode: ': %q", got)
	}
	if !strings.Contains(got, "differences:") {
		t.Errorf("output does not contain 'differences:': %q", got)
	}
	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 0 {
		t.Errorf("report exit code = %d, want 0", code)
	}
}

func TestRun_Negative_ServerAndRegisterCommandsMutuallyExclusive(t *testing.T) {
	var readCalls int
	var registerCalls int
	deps := dependencies{
		readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			readCalls++
			return nil, nil
		},
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
			registerCalls++
			return nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=plan", "-register-commands"}, validEnv(t), &out, deps)
	if err == nil {
		t.Fatal("run succeeded with mutually exclusive flags, want error")
	}
	if readCalls != 0 {
		t.Errorf("readCalls = %d, want 0", readCalls)
	}
	if registerCalls != 0 {
		t.Errorf("registerCalls = %d, want 0", registerCalls)
	}

	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 1 {
		t.Errorf("report exit code = %d, want 1", code)
	}
}

func TestRun_Negative_ServerUnknownModeListsValues(t *testing.T) {
	var readCalls int
	deps := dependencies{
		readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			readCalls++
			return nil, nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=unknown"}, validEnv(t), &out, deps)
	if err == nil {
		t.Fatal("run succeeded with unknown server mode, want error")
	}
	if readCalls != 0 {
		t.Errorf("readCalls = %d, want 0", readCalls)
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "plan") || !strings.Contains(errStr, "compare") {
		t.Errorf("error %q should list both 'plan' and 'compare'", errStr)
	}

	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 1 {
		t.Errorf("report exit code = %d, want 1", code)
	}
}

func TestRun_Positive_DefaultDependenciesReadServer(t *testing.T) {
	deps := defaultDependencies()
	if deps.readServer == nil {
		t.Fatal("defaultDependencies().readServer is nil")
	}
	_, err := deps.readServer(context.Background(), "", 1001)
	if err == nil {
		t.Fatal("expected error with empty token, got nil")
	}
	if !strings.Contains(err.Error(), "bot token is required") {
		t.Errorf("error = %q, want 'bot token is required'", err.Error())
	}
}

func TestRun_Boundary_ServerTimeoutSixtySeconds(t *testing.T) {
	snap := loadFreshServerSnapshot(t)
	var capturedDeadline time.Time
	var hasDeadline bool

	deps := dependencies{
		readServer: func(ctx context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			capturedDeadline, hasDeadline = ctx.Deadline()
			return snap, nil
		},
	}

	start := time.Now()
	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=plan"}, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run -server=plan failed: %v", err)
	}
	if !hasDeadline {
		t.Fatal("ctx passed to readServer has no deadline")
	}
	remaining := time.Until(capturedDeadline)
	if remaining < 50*time.Second || remaining > 60*time.Second+500*time.Millisecond {
		t.Errorf("remaining deadline = %v (from start %v), want ~60s", remaining, start)
	}
}

func TestRun_Negative_ServerReadErrorWrapped(t *testing.T) {
	deps := dependencies{
		readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			return nil, errors.New("connection failed")
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=plan"}, validEnv(t), &out, deps)
	if err == nil {
		t.Fatal("run succeeded when readServer failed, want error")
	}
	if !strings.Contains(err.Error(), "read server:") {
		t.Errorf("error %q should wrap with 'read server:'", err.Error())
	}
}

func TestRun_Positive_ServerCommandIsolation(t *testing.T) {
	snap := loadFreshServerSnapshot(t)

	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() {
		if closeErr := ln.Close(); closeErr != nil {
			t.Errorf("close listener: %v", closeErr)
		}
	}()

	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:  tokenPath,
		config.EnvGuildID:    "42",
		config.EnvCorpusDir:  corpusPath,
		config.EnvHealthAddr: ln.Addr().String(),
	}

	var runCalled, registerCalled int
	deps := dependencies{
		readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			return snap, nil
		},
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
			registerCalled++
			return nil
		},
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			runCalled++
			return nil
		},
	}

	for _, mode := range []string{"plan", "compare"} {
		var out bytes.Buffer
		err := run(context.Background(), []string{"-server=" + mode}, func(k string) string { return envMap[k] }, &out, deps)
		if err != nil {
			t.Fatalf("run -server=%s failed: %v", mode, err)
		}
	}

	if runCalled != 0 {
		t.Errorf("runCalled = %d, want 0", runCalled)
	}
	if registerCalled != 0 {
		t.Errorf("registerCalled = %d, want 0", registerCalled)
	}
}
