package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekmora/discord-bot/internal/config"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write refused") }

func validEnv(t *testing.T) func(string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write token fixture: %v", err)
	}
	values := map[string]string{config.EnvTokenFile: path, config.EnvGuildID: "42"}
	return func(key string) string { return values[key] }
}

func TestRun_Positive_ReportsGuildAndNeverTheToken(t *testing.T) {
	var out bytes.Buffer
	if err := run(validEnv(t), &out); err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "guild 42") {
		t.Errorf("output %q does not name the guild", out.String())
	}
	if strings.Contains(out.String(), "secret-token") {
		t.Errorf("output %q leaks the token", out.String())
	}
}

func TestRun_Negative_MissingConfigurationFails(t *testing.T) {
	var out bytes.Buffer
	err := run(func(string) string { return "" }, &out)
	if err == nil || !strings.Contains(err.Error(), "load configuration") {
		t.Fatalf("run = %v, want a configuration error", err)
	}
	if out.Len() != 0 {
		t.Errorf("run wrote %q before failing, want no output", out.String())
	}
}

func TestRun_Boundary_UnwritableOutputFails(t *testing.T) {
	err := run(validEnv(t), failingWriter{})
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
