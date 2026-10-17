package config

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #180: the news function is off unless its state file is set, and its token is optional.
func TestLoad_Positive_NewsSettings(t *testing.T) {
	cfg, err := Load(env(validBaseEnv(t)))
	if err != nil || cfg.NewsStateFile != "" || cfg.NewsGitHubToken != "" || cfg.HasNews() {
		t.Fatalf("Load without news = %+v, %v; want news off", cfg, err)
	}
	base := validBaseEnv(t)
	state := filepath.Join(t.TempDir(), "news.json")
	base[EnvNewsStateFile] = state
	cfg, err = Load(env(base))
	if err != nil || cfg.NewsStateFile != state || cfg.NewsGitHubToken != "" || !cfg.HasNews() {
		t.Fatalf("Load with the state file = %+v, %v; want news on without a token", cfg, err)
	}
	base[EnvNewsGitHubTokenFile] = writeSecret(t, "github", "github_pat_secret\n")
	cfg, err = Load(env(base))
	if err != nil || cfg.NewsGitHubToken != "github_pat_secret" {
		t.Fatalf("Load with the token = %v, %v; want the token read", cfg, err)
	}
}

// The token never shows in the configuration's text or log value.
func TestLoad_Positive_NewsTokenRedacted(t *testing.T) {
	base := validBaseEnv(t)
	state := filepath.Join(t.TempDir(), "news.json")
	base[EnvNewsStateFile] = state
	base[EnvNewsGitHubTokenFile] = writeSecret(t, "github", "github_pat_secret\n")
	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if s := cfg.String(); !strings.Contains(s, "NewsStateFile:\""+state) || strings.Contains(s, "github_pat_secret") ||
		!strings.Contains(s, "NewsGitHubToken:\"[redacted]\"") {
		t.Errorf("String() = %s; want the state file and the token redacted", s)
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("dump", slog.Any("cfg", cfg))
	if strings.Contains(buf.String(), "github_pat_secret") || !strings.Contains(buf.String(), `"NewsGitHubToken":"[redacted]"`) {
		t.Errorf("LogValue() = %s; want the token redacted", buf.String())
	}
}

func TestLoad_Negative_NewsSettingsRefused(t *testing.T) {
	cases := []struct {
		name, state, token, wantVar string
	}{
		{"relative state file", "news.json", "", EnvNewsStateFile},
		{"token without state file", "", "token", EnvNewsGitHubTokenFile},
		{"token file missing", "/var/lib/diallak/news.json", "/nonexistent/github", EnvNewsGitHubTokenFile},
		{"token file empty", "/var/lib/diallak/news.json", "empty", EnvNewsGitHubTokenFile},
	}
	for _, tc := range cases {
		base := validBaseEnv(t)
		base[EnvNewsStateFile] = tc.state
		switch tc.token {
		case "token":
			base[EnvNewsGitHubTokenFile] = writeSecret(t, "github", "secret")
		case "empty":
			base[EnvNewsGitHubTokenFile] = writeSecret(t, "github", " \n")
		default:
			base[EnvNewsGitHubTokenFile] = tc.token
		}
		if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), tc.wantVar) {
			t.Errorf("%s: Load = %v, want an error naming %s", tc.name, err, tc.wantVar)
		}
	}
}

func TestLoad_Boundary_NewsTokenSize(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvNewsStateFile] = "/news.json"
	base[EnvNewsGitHubTokenFile] = writeSecret(t, "github", strings.Repeat("a", MaxTokenBytes))
	if _, err := Load(env(base)); err != nil {
		t.Fatalf("Load with a token of %d bytes: %v", MaxTokenBytes, err)
	}
	base[EnvNewsGitHubTokenFile] = writeSecret(t, "github", strings.Repeat("a", MaxTokenBytes+1))
	if _, err := Load(env(base)); err == nil {
		t.Fatalf("Load with a token of %d bytes succeeded", MaxTokenBytes+1)
	}
}
