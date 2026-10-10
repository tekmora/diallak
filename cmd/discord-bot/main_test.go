package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekmora/discord-bot/internal/config"
	"github.com/tekmora/discord-bot/internal/corpus"
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

func TestRun_Positive_ReadyWithoutModel(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), validEnv(t), &out)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	got := out.String()
	want := "ready: 1 sources, 1 chunks, model not configured; this build opens no gateway connection\n"
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
	err := run(context.Background(), func(k string) string { return envMap[k] }, &out)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	got := out.String()
	want := "ready: 1 sources, 1 chunks, model configured; this build opens no gateway connection\n"
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
			err := run(context.Background(), func(k string) string { return tc.env[k] }, &out)
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
	err := run(context.Background(), func(k string) string { return envMap[k] }, &out)
	if err == nil || !strings.Contains(err.Error(), "build application") {
		t.Fatalf("run = %v, want build application error", err)
	}
	if out.Len() != 0 {
		t.Errorf("run wrote output before build failure: %q", out.String())
	}
}

func TestRun_Boundary_UnwritableOutputFails(t *testing.T) {
	err := run(context.Background(), validEnv(t), failingWriter{})
	if err == nil || !strings.Contains(err.Error(), "write status") {
		t.Fatalf("run = %v, want a write error", err)
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
	if got := errOut.String(); got != "discord-bot: boom\n" {
		t.Errorf("report wrote %q, want the prefixed error", got)
	}
}

func TestReport_Boundary_UnwritableErrorOutputExitsTwo(t *testing.T) {
	if code := report(errors.New("boom"), failingWriter{}); code != 2 {
		t.Fatalf("report with an unwritable error output = %d, want 2", code)
	}
}
