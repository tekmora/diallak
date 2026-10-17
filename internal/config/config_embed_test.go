package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestLoad_Negative_HostedEmbedBaseURL tests that hosted embedding URLs are refused while on-premises URLs pass.
func TestLoad_Negative_HostedEmbedBaseURL(t *testing.T) {
	plantedHosted := []struct {
		name string
		url  string
	}{
		{"openai", "https://api.openai.com/v1"},
		{"public domain", "https://gateway.example.org/v1"},
		{"public ip", "http://8.8.8.8/v1"},
		{"trailing dot", "https://embeddings.example.com."},
	}

	for _, tc := range plantedHosted {
		t.Run("planted_"+tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvEmbedBaseURL] = tc.url
			base[EnvEmbedAlias] = "bge-m3"

			_, err := Load(env(base))
			if err == nil {
				t.Fatalf("Load with hosted base URL %q succeeded, want error", tc.url)
			}
			if !strings.Contains(err.Error(), EnvEmbedBaseURL) {
				t.Errorf("error %q does not name %s", err.Error(), EnvEmbedBaseURL)
			}
			if !strings.Contains(err.Error(), "ADR-0022") {
				t.Errorf("error %q does not mention ADR-0022", err.Error())
			}
		})
	}

	positives := []struct {
		name string
		url  string
	}{
		{"ipv4 loopback", "http://127.0.0.1:11434/v1"},
		{"cluster svc", "http://litellm.cordana.svc:4000/v1"},
		{"private rfc1918", "http://10.0.0.5/v1"},
		{"private ipv6", "http://[fd00::1]/v1"},
		{"single label", "http://gateway/v1"},
	}

	for _, tc := range positives {
		t.Run("positive_"+tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvEmbedBaseURL] = tc.url
			base[EnvEmbedAlias] = "bge-m3"

			cfg, err := Load(env(base))
			if err != nil {
				t.Fatalf("Load with on-premises URL %q failed: %v", tc.url, err)
			}
			if !cfg.HasEmbed() {
				t.Errorf("HasEmbed = false, want true")
			}
			if cfg.EmbedBaseURL != tc.url {
				t.Errorf("EmbedBaseURL = %q, want %q", cfg.EmbedBaseURL, tc.url)
			}
		})
	}

	boundaries := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"suffix trick localhost.example.com", "http://localhost.example.com/v1", true},
		{"suffix trick evil.svc.example.com", "http://evil.svc.example.com/v1", true},
		{"uppercase localhost", "HTTP://LOCALHOST/v1", false},
	}

	for _, tc := range boundaries {
		t.Run("boundary_"+tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvEmbedBaseURL] = tc.url
			base[EnvEmbedAlias] = "bge-m3"

			_, err := Load(env(base))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Load with URL %q err = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

func TestLoad_Positive_EmbedConfig(t *testing.T) {
	keyPath := writeSecret(t, "embed-key", "secret-embed-token\n")
	base := validBaseEnv(t)
	base[EnvEmbedBaseURL] = "http://127.0.0.1:11434/v1"
	base[EnvEmbedAlias] = "text-embedding-3-small"
	base[EnvEmbedKeyFile] = keyPath
	base[EnvEmbedTimeout] = "15s"
	base[EnvEmbedMinSimilarity] = "0.75"

	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.HasEmbed() {
		t.Fatal("HasEmbed = false, want true")
	}
	if cfg.EmbedBaseURL != "http://127.0.0.1:11434/v1" {
		t.Errorf("EmbedBaseURL = %q", cfg.EmbedBaseURL)
	}
	if cfg.EmbedAlias != "text-embedding-3-small" {
		t.Errorf("EmbedAlias = %q", cfg.EmbedAlias)
	}
	if cfg.EmbedKey != "secret-embed-token" {
		t.Errorf("EmbedKey = %q, want secret-embed-token", cfg.EmbedKey)
	}
	if cfg.EmbedTimeout != 15*time.Second {
		t.Errorf("EmbedTimeout = %v, want 15s", cfg.EmbedTimeout)
	}
	if cfg.EmbedMinSimilarity != 0.75 {
		t.Errorf("EmbedMinSimilarity = %g, want 0.75", cfg.EmbedMinSimilarity)
	}
}

func TestLoad_Negative_EmbedConfigPairsAndLimits(t *testing.T) {
	// Base without alias
	base := validBaseEnv(t)
	base[EnvEmbedBaseURL] = "http://127.0.0.1:11434/v1"
	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), EnvEmbedAlias) {
		t.Errorf("Load without alias err = %v, want error naming %s", err, EnvEmbedAlias)
	}

	// Alias without base
	base = validBaseEnv(t)
	base[EnvEmbedAlias] = "bge-m3"
	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), EnvEmbedBaseURL) {
		t.Errorf("Load without base URL err = %v, want error naming %s", err, EnvEmbedBaseURL)
	}

	// Key without embed
	base = validBaseEnv(t)
	base[EnvEmbedKeyFile] = "/tmp/key"
	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), EnvEmbedKeyFile) {
		t.Errorf("Load with key only err = %v, want error naming %s", err, EnvEmbedKeyFile)
	}

	// Invalid timeout
	base = validBaseEnv(t)
	base[EnvEmbedBaseURL] = "http://127.0.0.1:11434/v1"
	base[EnvEmbedAlias] = "bge-m3"
	base[EnvEmbedTimeout] = "120s" // Max is 60s
	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), EnvEmbedTimeout) {
		t.Errorf("Load with timeout 120s err = %v, want error naming %s", err, EnvEmbedTimeout)
	}

	// Invalid min similarity
	base[EnvEmbedTimeout] = "10s"
	base[EnvEmbedMinSimilarity] = "1.5"
	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), EnvEmbedMinSimilarity) {
		t.Errorf("Load with min similarity 1.5 err = %v, want error naming %s", err, EnvEmbedMinSimilarity)
	}
}

func TestConfig_Positive_EmbedRedaction(t *testing.T) {
	secretKey := "super-secret-embed-key"
	cfg := Config{
		Token:        "token",
		GuildID:      123,
		CorpusDir:    "/var/data/corpus",
		EmbedBaseURL: "http://127.0.0.1:11434/v1",
		EmbedAlias:   "bge-m3",
		EmbedKey:     secretKey,
	}

	str := cfg.String()
	if strings.Contains(str, secretKey) {
		t.Errorf("String() leaked EmbedKey: %s", str)
	}
	if !strings.Contains(str, `EmbedKey:"[redacted]"`) {
		t.Errorf("String() did not redact EmbedKey: %s", str)
	}

	gostr := cfg.GoString()
	if strings.Contains(gostr, secretKey) {
		t.Errorf("GoString() leaked EmbedKey: %s", gostr)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("test", slog.Any("cfg", cfg))
	logOutput := buf.String()
	if strings.Contains(logOutput, secretKey) {
		t.Errorf("LogValue() leaked EmbedKey: %s", logOutput)
	}
	if !strings.Contains(logOutput, `"EmbedKey":"[redacted]"`) {
		t.Errorf("LogValue() did not redact EmbedKey: %s", logOutput)
	}
}
