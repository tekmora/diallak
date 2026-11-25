package config

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Issue #180: the news function is off unless its state file is set, and its token is optional.
func TestLoad_Positive_NewsSettings(t *testing.T) {
	cfg, err := Load(env(validBaseEnv(t)))
	if err != nil || cfg.NewsStateFile != "" || cfg.NewsGitHubTokenFile != "" || cfg.HasNews() {
		t.Fatalf("Load without news = %+v, %v; want news off", cfg, err)
	}
	base := validBaseEnv(t)
	state := filepath.Join(t.TempDir(), "news.json")
	base[EnvNewsStateFile] = state
	cfg, err = Load(env(base))
	if err != nil || cfg.NewsStateFile != state || cfg.NewsGitHubTokenFile != "" || !cfg.HasNews() {
		t.Fatalf("Load with the state file = %+v, %v; want news on without a token", cfg, err)
	}
	tokenFile := writeSecret(t, "github", "advisory-fixture-token\n")
	base[EnvNewsGitHubTokenFile] = tokenFile
	cfg, err = Load(env(base))
	if err != nil || cfg.NewsGitHubTokenFile != tokenFile {
		t.Fatalf("Load with the token file = %v, %v; want the token file path", cfg, err)
	}
}

func TestLoad_Positive_NewsTokenFileVisibleInConfig(t *testing.T) {
	base := validBaseEnv(t)
	state := filepath.Join(t.TempDir(), "news.json")
	base[EnvNewsStateFile] = state
	const token = "advisory-fixture-token"
	tokenFile := writeSecret(t, "github", token+"\n")
	base[EnvNewsGitHubTokenFile] = tokenFile
	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	file := strconv.Quote(tokenFile)
	if s := cfg.String(); !strings.Contains(s, "NewsStateFile:"+strconv.Quote(state)) ||
		strings.Contains(s, token) || !strings.Contains(s, "NewsGitHubTokenFile:"+file) {
		t.Error("expected String() to show both paths and omit the token value")
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("dump", slog.Any("cfg", cfg))
	if strings.Contains(buf.String(), token) || !strings.Contains(buf.String(), `"NewsGitHubTokenFile":`+file) {
		t.Error("expected LogValue() to show the token file path and omit the token value")
	}
}

func TestLoad_Negative_NewsSettingsRefused(t *testing.T) {
	cases := []struct {
		name, state, token, want string
	}{
		{"relative state file", "news.json", "", EnvNewsStateFile},
		{"token without state file", "", "token", EnvNewsGitHubTokenFile},
		{"token file missing", "/var/lib/diallak/news.json", "missing", EnvNewsGitHubTokenFile + ": open GitHub token file"},
		{"token file empty", "/var/lib/diallak/news.json", "empty", EnvNewsGitHubTokenFile + ": GitHub token file is empty"},
	}
	for _, tc := range cases {
		base := validBaseEnv(t)
		base[EnvNewsStateFile] = tc.state
		switch tc.token {
		case "token":
			base[EnvNewsGitHubTokenFile] = writeSecret(t, "github", "secret")
		case "empty":
			base[EnvNewsGitHubTokenFile] = writeSecret(t, "github", " \n")
		case "missing":
			base[EnvNewsGitHubTokenFile] = filepath.Join(t.TempDir(), "missing")
		default:
			base[EnvNewsGitHubTokenFile] = tc.token
		}
		if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Load = %v, want an error containing %q", tc.name, err, tc.want)
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
	if _, err := Load(env(base)); err == nil ||
		!strings.Contains(err.Error(), EnvNewsGitHubTokenFile+": GitHub token file is larger than 4096 bytes") {
		t.Fatalf("Load with a token of %d bytes: %v, want a token size error", MaxTokenBytes+1, err)
	}
}
