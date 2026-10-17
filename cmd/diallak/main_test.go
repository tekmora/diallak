package main

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"github.com/tekmora/diallak/server"
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
		config.EnvModelPolicy:  "example-policy",
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

func (w *wrongServerInteraction) Reply(_ context.Context, text, _ string) error {
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

// verifyRunConfigError runs run() with the test environment and verifies failure output and secret leaks.
func verifyRunConfigError(t *testing.T, env map[string]string, wantVar, distinctToken, distinctKey string) {
	t.Helper()
	var out bytes.Buffer
	err := run(context.Background(), nil, func(k string) string { return env[k] }, &out, noopDeps())
	if err == nil {
		t.Fatalf("run succeeded, want error naming %q", wantVar)
	}
	errStr := err.Error()
	if !strings.Contains(errStr, wantVar) {
		t.Errorf("error %q does not name expected variable %q", errStr, wantVar)
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
}

type runConfigErrorCase struct {
	name    string
	env     map[string]string
	wantVar string
}

// buildBaseRunConfigCases returns config error cases for missing base settings.
func buildBaseRunConfigCases(tokenPath, corpusPath string) []runConfigErrorCase {
	return []runConfigErrorCase{
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
	}
}

// buildModelRunConfigCases returns config error cases for invalid model settings.
func buildModelRunConfigCases(tokenPath, keyPath, corpusPath string) []runConfigErrorCase {
	return []runConfigErrorCase{
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
		{
			name: "model base and alias without policy",
			env: map[string]string{
				config.EnvGuildID:      "42",
				config.EnvTokenFile:    tokenPath,
				config.EnvCorpusDir:    corpusPath,
				config.EnvModelBaseURL: "http://127.0.0.1:8000",
				config.EnvModelAlias:   "router/test",
			},
			wantVar: config.EnvModelPolicy,
		},
		{
			name: "model policy without model",
			env: map[string]string{
				config.EnvGuildID:     "42",
				config.EnvTokenFile:   tokenPath,
				config.EnvCorpusDir:   corpusPath,
				config.EnvModelPolicy: "example-policy",
			},
			wantVar: config.EnvModelPolicy,
		},
	}
}

func TestRun_Negative_ConfigurationErrors(t *testing.T) {
	distinctToken := "distinct-secret-token-111"
	distinctKey := "distinct-secret-key-222"
	tokenPath := writeSecret(t, "token", distinctToken)
	keyPath := writeSecret(t, "key", distinctKey)
	corpusPath := setupTestCorpus(t)

	cases := append(buildBaseRunConfigCases(tokenPath, corpusPath),
		buildModelRunConfigCases(tokenPath, keyPath, corpusPath)...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verifyRunConfigError(t, tc.env, tc.wantVar, distinctToken, distinctKey)
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

// probeHealthz requests /healthz and validates HTTP 200 and "ok\n" body.
func probeHealthz(ctx context.Context, client *http.Client, addr string) error {
	healthReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/healthz", nil)
	if reqErr != nil {
		return fmt.Errorf("new health request: %w", reqErr)
	}
	resp, getErr := client.Do(healthReq)
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
	return nil
}

// probeReadyzBeforeReady requests /readyz and validates HTTP 503 before ready is reported.
func probeReadyzBeforeReady(ctx context.Context, client *http.Client, addr string) error {
	readyReq1, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/readyz", nil)
	if reqErr != nil {
		return fmt.Errorf("new ready request: %w", reqErr)
	}
	readyResp, readyErr := client.Do(readyReq1)
	if readyErr != nil {
		return fmt.Errorf("probe /readyz before ready: %w", readyErr)
	}
	if closeErr := readyResp.Body.Close(); closeErr != nil {
		return fmt.Errorf("close ready body: %w", closeErr)
	}
	if readyResp.StatusCode != http.StatusServiceUnavailable {
		return fmt.Errorf("/readyz before ready = %d, want 503", readyResp.StatusCode)
	}
	return nil
}

// probeReadyzAfterReady requests /readyz and validates HTTP 200 and "ready\n" body.
func probeReadyzAfterReady(ctx context.Context, client *http.Client, addr string) error {
	readyReq2, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/readyz", nil)
	if reqErr != nil {
		return fmt.Errorf("new ready request 2: %w", reqErr)
	}
	readyResp2, readyErr2 := client.Do(readyReq2)
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
	return nil
}

// verifyHealthEndpointClosed checks that the health server stops responding after run completes.
func verifyHealthEndpointClosed(t *testing.T, client *http.Client, addr string) {
	t.Helper()
	probeReq, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+addr+"/healthz", nil)
	if reqErr != nil {
		t.Fatalf("new probe request: %v", reqErr)
	}
	probeResp, probeAfterErr := client.Do(probeReq)
	if probeAfterErr == nil {
		if closeErr := probeResp.Body.Close(); closeErr != nil {
			t.Errorf("close probe response: %v", closeErr)
		}
		t.Fatal("health server still responding after run completed")
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
			if err := probeHealthz(discordCtx, httpClient, addr); err != nil {
				return err
			}
			if err := probeReadyzBeforeReady(discordCtx, httpClient, addr); err != nil {
				return err
			}
			cfg.ReportReady(true)
			if err := probeReadyzAfterReady(discordCtx, httpClient, addr); err != nil {
				return err
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

	verifyHealthEndpointClosed(t, httpClient, addr)
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
		config.EnvModelPolicy:  "example-policy",
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
		config.EnvModelBaseURL + ", " + config.EnvModelAlias + ", " + config.EnvModelKeyFile + ", " + config.EnvModelPolicy +
		" and " + config.EnvModelGuardrail
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

// verifyRejectedProbeStopsStart checks that non-200/503 HTTP probe responses abort startup before runDiscord.
func verifyRejectedProbeStopsStart(t *testing.T, status int) {
	t.Helper()
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
		config.EnvModelPolicy:  "example-policy",
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
}

func TestRun_Negative_RejectedProbeStopsTheStart(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusBadRequest} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			verifyRejectedProbeStopsStart(t, status)
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
				config.EnvModelPolicy:  "example-policy",
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
		w.Header().Set("x-litellm-applied-policies", "example-policy")
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
		config.EnvModelPolicy:  "example-policy",
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
		config.EnvModelPolicy:  "example-policy",
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

func TestRun_Negative_ModelVariablesWithoutPolicyStopsStart(t *testing.T) {
	tokenPath := writeSecret(t, "token", "distinctive-token-xyz")
	corpusPath := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:    tokenPath,
		config.EnvGuildID:      "42",
		config.EnvCorpusDir:    corpusPath,
		config.EnvModelBaseURL: "http://127.0.0.1:8000",
		config.EnvModelAlias:   "test-alias",
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, noopDeps())
	code := report(err, &errOut)
	if code != 1 {
		t.Fatalf("report() code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), config.EnvModelPolicy) {
		t.Errorf("error output %q does not name %s", errOut.String(), config.EnvModelPolicy)
	}
}

func TestRun_Negative_ProbeWithoutPolicyHeaderStopsStart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
		}
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
		config.EnvModelPolicy:  "example-policy",
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, noopDeps())
	code := report(err, &errOut)
	if code != 1 {
		t.Fatalf("report() code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "model start probe") {
		t.Errorf("error output %q should mention model start probe", errOut.String())
	}
}

func TestRun_Positive_ProbeWithPolicyHeaderStarts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
		}
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
		config.EnvModelPolicy:  "example-policy",
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	var discordCalls int
	deps := dependencies{
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error { return nil },
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			discordCalls++
			return nil
		},
	}
	err := run(context.Background(), nil, func(k string) string { return envMap[k] }, &out, deps)
	code := report(err, &errOut)
	if code != 0 {
		t.Fatalf("report() code = %d, want 0; err = %s", code, errOut.String())
	}
	if discordCalls != 1 {
		t.Errorf("discordCalls = %d, want 1", discordCalls)
	}
	if !strings.HasSuffix(out.String(), "model ready\n") {
		t.Errorf("output %q should end with 'model ready\\n'", out.String())
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

// verifyHealthEndpointReportsModelState checks /modelz response status and body for a given probe status.
func verifyHealthEndpointReportsModelState(t *testing.T, status int, want string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status == http.StatusOK {
			w.Header().Set("x-litellm-applied-policies", "example-policy")
		}
		w.WriteHeader(status)
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
		config.EnvModelPolicy:  "example-policy",
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
	if got != want {
		t.Errorf("/modelz = %q, want %q", got, want)
	}
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
			verifyHealthEndpointReportsModelState(t, tc.status, tc.want)
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
	wantDefLine := fmt.Sprintf("definition: %s\n", server.DefinitionDigest())
	if !strings.Contains(got, wantDefLine) {
		t.Errorf("output does not contain definition line %q: %q", wantDefLine, got)
	}
	idx := strings.LastIndex(got, "plan digest: sha256:")
	if idx == -1 {
		t.Fatalf("output does not contain 'plan digest: sha256:': %q", got)
	}
	textAbove := got[:idx]
	if !strings.HasSuffix(textAbove, wantDefLine) {
		t.Errorf("text above digest does not end with definition line: %q", textAbove)
	}
	expectedDigest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(textAbove)))
	planDigestLine := got[idx:]
	wantPlanDigestLine := fmt.Sprintf("plan digest: %s\n", expectedDigest)
	if planDigestLine != wantPlanDigestLine {
		t.Errorf("plan digest line = %q, want %q", planDigestLine, wantPlanDigestLine)
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

func TestRun_Positive_ServerModes_UnnamedBitOnManagedRoleIsReported(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{mode: "compare", want: "  unknown-permission-bits server.roles[Server Booster]; bits 47\n"},
		{mode: "plan", want: "plan digest: sha256:"},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			snap := loadFreshServerSnapshot(t)
			snap.Roles = append([]serverplan.Role{{
				Name:        "Server Booster",
				Managed:     true,
				Permissions: []string{"VIEW_CHANNEL"},
				UnknownBits: []int{47},
			}}, snap.Roles...)
			deps := dependencies{
				readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
					return snap, nil
				},
			}

			var out bytes.Buffer
			err := run(context.Background(), []string{"-server=" + tt.mode}, validEnv(t), &out, deps)
			if err != nil {
				t.Fatalf("run -server=%s: %v", tt.mode, err)
			}
			if got := out.String(); !strings.Contains(got, tt.want) {
				t.Errorf("output does not contain %q:\n%s", tt.want, got)
			}
			var errOut bytes.Buffer
			if code := report(err, &errOut); code != 0 {
				t.Errorf("report exit code = %d, want 0", code)
			}
		})
	}
}

func TestRun_Negative_ServerAndRegisterCommandsMutuallyExclusive(t *testing.T) {
	modes := []string{
		"plan",
		"compare",
		"apply=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			var calls int
			deps := dependencies{
				readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
					calls++
					return nil, nil
				},
				registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
					calls++
					return nil
				},
				applyServer: func(_ context.Context, _ string, _ uint64, _ discord.ApplyInput, _ io.Writer) error {
					calls++
					return nil
				},
			}

			var out bytes.Buffer
			err := run(context.Background(), []string{"-server=" + mode, "-register-commands"}, validEnv(t), &out, deps)
			const want = "-server and -register-commands cannot be used together"
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v, want %q", err, want)
			}
			if calls != 0 {
				t.Errorf("calls to the platform = %d, want 0", calls)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want none", out.String())
			}

			var errOut bytes.Buffer
			if code := report(err, &errOut); code != 1 {
				t.Errorf("report exit code = %d, want 1", code)
			}
		})
	}
}

// verifyServerUnknownModeError checks that an unknown server mode flag is rejected with usage error.
func verifyServerUnknownModeError(t *testing.T, mode string) {
	t.Helper()
	var readCalls int
	deps := dependencies{
		readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			readCalls++
			return nil, nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=" + mode}, validEnv(t), &out, deps)
	if err == nil {
		t.Fatal("run succeeded with an unknown server mode, want error")
	}
	if readCalls != 0 {
		t.Errorf("readCalls = %d, want 0", readCalls)
	}
	want := fmt.Sprintf("invalid -server mode %q: must be plan, compare or apply=<plan digest>", mode)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want none", out.String())
	}

	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 1 {
		t.Errorf("report exit code = %d, want 1", code)
	}
}

func TestRun_Negative_ServerUnknownModeListsValues(t *testing.T) {
	for _, mode := range []string{"unknown", "apply", "PLAN", "plan ", "compare,plan"} {
		t.Run(mode, func(t *testing.T) {
			verifyServerUnknownModeError(t, mode)
		})
	}
}

func TestRun_Boundary_ServerCompareNamesTheMode(t *testing.T) {
	cases := []struct {
		name     string
		botPerms []string
		want     string
	}{
		{"rights of the definition only", []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "MANAGE_GUILD"}, "mode: own\n"},
		{"one right more", []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "MANAGE_GUILD", "MANAGE_CHANNELS"}, "mode: raised\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := loadFreshServerSnapshot(t)
			for i := range snap.Roles {
				if snap.Roles[i].Name == snap.Caller.Role {
					snap.Roles[i].Permissions = tc.botPerms
				}
			}
			deps := dependencies{
				readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
					return snap, nil
				},
			}

			var out bytes.Buffer
			if err := run(context.Background(), []string{"-server=compare"}, validEnv(t), &out, deps); err != nil {
				t.Fatalf("run -server=compare failed: %v", err)
			}
			if !strings.HasPrefix(out.String(), tc.want) {
				t.Errorf("output starts %q, want %q", out.String(), tc.want)
			}
		})
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

// verifyServerApplyInvalidDigestError checks that an invalid digest in -server=apply is rejected with usage error.
func verifyServerApplyInvalidDigestError(t *testing.T, mode string) {
	t.Helper()
	var applyCalls int
	deps := dependencies{
		applyServer: func(_ context.Context, _ string, _ uint64, _ discord.ApplyInput, _ io.Writer) error {
			applyCalls++
			return nil
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=" + mode}, validEnv(t), &out, deps)
	if err == nil {
		t.Fatalf("run succeeded with invalid apply mode %q, want error", mode)
	}
	if applyCalls != 0 {
		t.Errorf("applyCalls = %d, want 0", applyCalls)
	}
	want := fmt.Sprintf("invalid -server mode %q: apply needs the digest that -server=plan printed, as apply=sha256:<64 hex digits>", mode)
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want none", out.String())
	}

	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 1 {
		t.Errorf("report exit code = %d, want 1", code)
	}
}

func TestRun_Negative_ServerApplyInvalidDigest(t *testing.T) {
	cases := []string{
		"apply=",
		"apply=sha256:",
		"apply=sha256:1234",
		"apply=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdeG",
		"apply=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0",
		"apply=md5:0123456789abcdef0123456789abcdef",
		"apply=sha256:0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
		// the characters next to the two accepted ranges
		"apply=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdeg",
		"apply=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde`",
		"apply=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde/",
		"apply=sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde:",
	}
	for _, mode := range cases {
		t.Run(mode, func(t *testing.T) {
			verifyServerApplyInvalidDigestError(t, mode)
		})
	}
}

func TestRun_Positive_ServerApply_ExitZeroAndOutput(t *testing.T) {
	const validDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var calledWithToken string
	var calledWithGuild uint64
	var capturedInput discord.ApplyInput

	deps := dependencies{
		applyServer: func(_ context.Context, token string, guildID uint64, in discord.ApplyInput, out io.Writer) error {
			calledWithToken = token
			calledWithGuild = guildID
			capturedInput = in
			_, err := io.WriteString(out, "changes: 1\n  1. create-role roles[mod]: done\napplied: 1 of 1\n")
			return err
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=apply=" + validDigest}, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run -server=apply failed: %v", err)
	}

	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 0 {
		t.Errorf("report code = %d, want 0", code)
	}
	if calledWithToken != "distinctive-token-xyz" {
		t.Errorf("calledWithToken = %q, want distinctive-token-xyz", calledWithToken)
	}
	if calledWithGuild != 42 {
		t.Errorf("calledWithGuild = %d, want 42", calledWithGuild)
	}
	if capturedInput.Definition == nil {
		t.Error("capturedInput.Definition is nil")
	}
	if capturedInput.DefinitionDigest != server.DefinitionDigest() {
		t.Errorf("DefinitionDigest = %q, want %q", capturedInput.DefinitionDigest, server.DefinitionDigest())
	}
	if capturedInput.PlanDigest != validDigest {
		t.Errorf("PlanDigest = %q, want %q", capturedInput.PlanDigest, validDigest)
	}
	wantOut := "changes: 1\n  1. create-role roles[mod]: done\napplied: 1 of 1\n"
	if out.String() != wantOut {
		t.Errorf("output = %q, want %q", out.String(), wantOut)
	}
}

func TestRun_Negative_ServerApply_FailureReturnsExitOne(t *testing.T) {
	const validDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	deps := dependencies{
		applyServer: func(_ context.Context, _ string, _ uint64, _ discord.ApplyInput, out io.Writer) error {
			_, err := io.WriteString(out, "changes: 1\n  1. create-role roles[mod]: failed\napplied: 0 of 1\n")
			if err != nil {
				return err
			}
			return errors.New("step 1 create-role roles[mod]: create role: 50035: Invalid Form Body")
		},
	}

	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=apply=" + validDigest}, validEnv(t), &out, deps)
	if err == nil {
		t.Fatal("run succeeded when applyServer failed, want error")
	}
	wantSubstr := "apply: step 1 create-role roles[mod]: create role: 50035: Invalid Form Body"
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error = %q, want containing %q", err.Error(), wantSubstr)
	}

	var errOut bytes.Buffer
	if code := report(err, &errOut); code != 1 {
		t.Errorf("report exit code = %d, want 1", code)
	}
	wantErrOut := "diallak: " + wantSubstr + "\n"
	if errOut.String() != wantErrOut {
		t.Errorf("errOut = %q, want %q", errOut.String(), wantErrOut)
	}
}

func TestRun_Boundary_ServerApplyTimeoutFifteenMinutes(t *testing.T) {
	const validDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var capturedDeadline time.Time
	var hasDeadline bool

	deps := dependencies{
		applyServer: func(ctx context.Context, _ string, _ uint64, _ discord.ApplyInput, _ io.Writer) error {
			capturedDeadline, hasDeadline = ctx.Deadline()
			return nil
		},
	}

	start := time.Now()
	var out bytes.Buffer
	err := run(context.Background(), []string{"-server=apply=" + validDigest}, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run -server=apply failed: %v", err)
	}
	if !hasDeadline {
		t.Fatal("ctx passed to applyServer has no deadline")
	}
	remaining := time.Until(capturedDeadline)
	if remaining < 14*time.Minute+50*time.Second || remaining > 15*time.Minute+500*time.Millisecond {
		t.Errorf("remaining deadline = %v (from start %v), want ~15m", remaining, start)
	}
}

func TestRun_Positive_DefaultDependenciesApplyServer(t *testing.T) {
	deps := defaultDependencies()
	if deps.applyServer == nil {
		t.Fatal("defaultDependencies().applyServer is nil")
	}
	err := deps.applyServer(context.Background(), "", 1001, discord.ApplyInput{}, io.Discard)
	if err == nil {
		t.Fatal("expected error with empty token, got nil")
	}
	if !strings.Contains(err.Error(), "bot token is required") {
		t.Errorf("error = %q, want 'bot token is required'", err.Error())
	}
}

func TestRun_Positive_ServerApplyCommandIsolation(t *testing.T) {
	const validDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
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

	var runCalled, registerCalled, readCalled, applyCalled int
	deps := dependencies{
		readServer: func(_ context.Context, _ string, _ uint64) (*serverplan.Snapshot, error) {
			readCalled++
			return nil, nil
		},
		registerCommands: func(_ context.Context, _ string, _ uint64, _ int) error {
			registerCalled++
			return nil
		},
		runDiscord: func(_ context.Context, _ discord.BotConfig) error {
			runCalled++
			return nil
		},
		applyServer: func(_ context.Context, _ string, _ uint64, _ discord.ApplyInput, _ io.Writer) error {
			applyCalled++
			return nil
		},
	}

	var out bytes.Buffer
	err = run(context.Background(), []string{"-server=apply=" + validDigest}, func(k string) string { return envMap[k] }, &out, deps)
	if err != nil {
		t.Fatalf("run -server=apply failed: %v", err)
	}

	if applyCalled != 1 {
		t.Errorf("applyCalled = %d, want 1", applyCalled)
	}
	if readCalled != 0 {
		t.Errorf("readCalled = %d, want 0", readCalled)
	}
	if runCalled != 0 {
		t.Errorf("runCalled = %d, want 0", runCalled)
	}
	if registerCalled != 0 {
		t.Errorf("registerCalled = %d, want 0", registerCalled)
	}
}
