package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSecret(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write secret fixture %q: %v", name, err)
	}
	return path
}

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func validBaseEnv(t *testing.T) map[string]string {
	t.Helper()
	tokenPath := writeSecret(t, "token", "secret-token\n")
	return map[string]string{
		EnvTokenFile: tokenPath,
		EnvGuildID:   "123456789012345678",
		EnvCorpusDir: "/var/data/corpus",
	}
}

func TestLoad_Positive_ReadsConfiguration(t *testing.T) {
	t.Run("minimal without model", func(t *testing.T) {
		cfg, err := Load(env(validBaseEnv(t)))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.Token != "secret-token" {
			t.Errorf("Token = %q, want trimmed content", cfg.Token)
		}
		if cfg.GuildID != 123456789012345678 {
			t.Errorf("GuildID = %d, want 123456789012345678", cfg.GuildID)
		}
		if cfg.CorpusDir != "/var/data/corpus" {
			t.Errorf("CorpusDir = %q, want /var/data/corpus", cfg.CorpusDir)
		}
		if cfg.HasModel() {
			t.Errorf("HasModel = true, want false")
		}
		if cfg.ModelKey != "" {
			t.Errorf("ModelKey = %q, want empty", cfg.ModelKey)
		}
	})

	t.Run("with model without key", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvModelBaseURL] = "http://127.0.0.1:8000"
		base[EnvModelAlias] = "router/test"

		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if !cfg.HasModel() {
			t.Errorf("HasModel = false, want true")
		}
		if cfg.ModelBaseURL != "http://127.0.0.1:8000" {
			t.Errorf("ModelBaseURL = %q, want http://127.0.0.1:8000", cfg.ModelBaseURL)
		}
		if cfg.ModelAlias != "router/test" {
			t.Errorf("ModelAlias = %q, want router/test", cfg.ModelAlias)
		}
		if cfg.ModelKey != "" {
			t.Errorf("ModelKey = %q, want empty", cfg.ModelKey)
		}
	})

	t.Run("with model and key file", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvModelBaseURL] = "http://127.0.0.1:8000"
		base[EnvModelAlias] = "router/test"
		base[EnvModelKeyFile] = writeSecret(t, "key", "sk-endpoint-secret\n")

		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if !cfg.HasModel() {
			t.Errorf("HasModel = false, want true")
		}
		if cfg.ModelKey != "sk-endpoint-secret" {
			t.Errorf("ModelKey = %q, want sk-endpoint-secret", cfg.ModelKey)
		}
	})
}

func TestLoad_Negative_RefusesInvalidInput(t *testing.T) {
	validToken := writeSecret(t, "token", "secret-token")
	validKey := writeSecret(t, "key", "secret-key")

	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"guild missing", map[string]string{EnvTokenFile: validToken, EnvCorpusDir: "/corpus"}, EnvGuildID + " is not set"},
		{"guild not decimal", map[string]string{EnvTokenFile: validToken, EnvGuildID: "0x10", EnvCorpusDir: "/corpus"}, "not a decimal snowflake"},
		{"guild negative", map[string]string{EnvTokenFile: validToken, EnvGuildID: "-1", EnvCorpusDir: "/corpus"}, "not a decimal snowflake"},
		{"guild zero", map[string]string{EnvTokenFile: validToken, EnvGuildID: "0", EnvCorpusDir: "/corpus"}, "must not be zero"},
		{"token variable missing", map[string]string{EnvGuildID: "1", EnvCorpusDir: "/corpus"}, EnvTokenFile + " is not set"},
		{"token file absent", map[string]string{EnvTokenFile: filepath.Join(t.TempDir(), "absent"), EnvGuildID: "1", EnvCorpusDir: "/corpus"}, "open token file"},
		{"token file blank", map[string]string{EnvTokenFile: writeSecret(t, "blank", " \n\t"), EnvGuildID: "1", EnvCorpusDir: "/corpus"}, "token file is empty"},
		{"corpus variable missing", map[string]string{EnvTokenFile: validToken, EnvGuildID: "1"}, EnvCorpusDir + " is not set"},
		{"corpus path relative", map[string]string{EnvTokenFile: validToken, EnvGuildID: "1", EnvCorpusDir: "relative/path"}, "must be an absolute path"},
		{"model base set without alias", map[string]string{EnvTokenFile: validToken, EnvGuildID: "1", EnvCorpusDir: "/corpus", EnvModelBaseURL: "http://127.0.0.1"}, "must both be set or both be unset"},
		{"model alias set without base", map[string]string{EnvTokenFile: validToken, EnvGuildID: "1", EnvCorpusDir: "/corpus", EnvModelAlias: "router/test"}, "must both be set or both be unset"},
		{"model key set without model", map[string]string{EnvTokenFile: validToken, EnvGuildID: "1", EnvCorpusDir: "/corpus", EnvModelKeyFile: validKey}, "set but no model is configured"},
		{"model key file absent", map[string]string{EnvTokenFile: validToken, EnvGuildID: "1", EnvCorpusDir: "/corpus", EnvModelBaseURL: "http://127.0.0.1", EnvModelAlias: "test", EnvModelKeyFile: filepath.Join(t.TempDir(), "absent_key")}, "open model key file"},
		{"model key file blank", map[string]string{EnvTokenFile: validToken, EnvGuildID: "1", EnvCorpusDir: "/corpus", EnvModelBaseURL: "http://127.0.0.1", EnvModelAlias: "test", EnvModelKeyFile: writeSecret(t, "blank_key", " \n")}, "model key file is empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(env(tc.env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want one naming %q", err, tc.want)
			}
			if cfg != (Config{}) {
				t.Errorf("Load returned non-zero Config %+v with an error", cfg)
			}
		})
	}
}

func TestLoad_Boundary_SecretSizesAndGuildRange(t *testing.T) {
	atLimit := writeSecret(t, "token_limit", strings.Repeat("a", MaxTokenBytes))
	keyAtLimit := writeSecret(t, "key_limit", strings.Repeat("b", MaxTokenBytes))

	cfg, err := Load(env(map[string]string{
		EnvTokenFile:    atLimit,
		EnvGuildID:      "18446744073709551615",
		EnvCorpusDir:    "/var/data/corpus",
		EnvModelBaseURL: "http://127.0.0.1:8000",
		EnvModelAlias:   "router/test",
		EnvModelKeyFile: keyAtLimit,
	}))
	if err != nil {
		t.Fatalf("Load at size limits = %v, want nil", err)
	}
	if len(cfg.Token) != MaxTokenBytes || cfg.GuildID != 18446744073709551615 || len(cfg.ModelKey) != MaxTokenBytes {
		t.Errorf("Load at limits unexpected sizes: token %d, guild %d, key %d",
			len(cfg.Token), cfg.GuildID, len(cfg.ModelKey))
	}

	overLimit := writeSecret(t, "token_over", strings.Repeat("a", MaxTokenBytes+1))
	if _, err := Load(env(map[string]string{
		EnvTokenFile: overLimit,
		EnvGuildID:   "1",
		EnvCorpusDir: "/corpus",
	})); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("Load over token limit = %v, want larger than error", err)
	}

	keyOverLimit := writeSecret(t, "key_over", strings.Repeat("b", MaxTokenBytes+1))
	if _, err := Load(env(map[string]string{
		EnvTokenFile:    atLimit,
		EnvGuildID:      "1",
		EnvCorpusDir:    "/corpus",
		EnvModelBaseURL: "http://127.0.0.1:8000",
		EnvModelAlias:   "test",
		EnvModelKeyFile: keyOverLimit,
	})); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("Load over key limit = %v, want larger than error", err)
	}

	if _, err := Load(env(map[string]string{
		EnvTokenFile: atLimit,
		EnvGuildID:   "18446744073709551616",
		EnvCorpusDir: "/corpus",
	})); err == nil {
		t.Error("Load with guild id above uint64 = nil, want error")
	}
}

func TestConfig_Negative_RedactionHidesSecrets(t *testing.T) {
	cfg := Config{
		Token:        "super-secret-discord-token",
		GuildID:      42,
		ModelBaseURL: "https://model.example.com",
		ModelAlias:   "router/test",
		ModelKey:     "super-secret-model-key",
		CorpusDir:    "/var/data/corpus",
	}

	formats := []string{
		fmt.Sprintf("%v", cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", cfg),
		cfg.String(),
	}

	for _, text := range formats {
		if strings.Contains(text, "super-secret-discord-token") {
			t.Fatalf("formatted config %q contains token", text)
		}
		if strings.Contains(text, "super-secret-model-key") {
			t.Fatalf("formatted config %q contains model key", text)
		}
		if !strings.Contains(text, "[redacted]") {
			t.Fatalf("formatted config %q does not contain [redacted]", text)
		}
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("dump", slog.Any("cfg", cfg))
	logText := buf.String()

	if strings.Contains(logText, "super-secret-discord-token") {
		t.Fatalf("slog JSON %q contains token", logText)
	}
	if strings.Contains(logText, "super-secret-model-key") {
		t.Fatalf("slog JSON %q contains model key", logText)
	}
	if !strings.Contains(logText, "[redacted]") {
		t.Fatalf("slog JSON %q does not contain [redacted]", logText)
	}
}

func TestLoad_Positive_SupportChannel(t *testing.T) {
	t.Run("unset stays empty", func(t *testing.T) {
		cfg, err := Load(env(validBaseEnv(t)))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.SupportChannel != "" {
			t.Errorf("SupportChannel = %q, want it empty", cfg.SupportChannel)
		}
	})

	t.Run("empty string stays empty", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvSupportChannel] = ""
		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.SupportChannel != "" {
			t.Errorf("SupportChannel = %q, want it empty", cfg.SupportChannel)
		}
	})

	t.Run("configured value is trimmed", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvSupportChannel] = "  #help-desk  "
		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.SupportChannel != "#help-desk" {
			t.Errorf("SupportChannel = %q, want #help-desk", cfg.SupportChannel)
		}
		if !strings.Contains(cfg.String(), `SupportChannel:"#help-desk"`) {
			t.Errorf("String() = %q, missing SupportChannel", cfg.String())
		}
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		logger.Info("test", slog.Any("cfg", cfg))
		if !strings.Contains(buf.String(), `"SupportChannel":"#help-desk"`) {
			t.Errorf("LogValue() JSON = %q, missing SupportChannel", buf.String())
		}
	})
}

func TestLoad_Negative_SupportChannelRefused(t *testing.T) {
	refused := []struct {
		name string
		val  string
	}{
		{name: "over 64 runes", val: strings.Repeat("a", 65)},
		{name: "newline", val: "#help\ndesk"},
		{name: "tab", val: "#help\tdesk"},
		{name: "carriage return", val: "#help\rdesk"},
		{name: "bell control char", val: "#help\x07desk"},
		{name: "line separator", val: "#help\u2028desk"},
		{name: "paragraph separator", val: "#help\u2029desk"},
		{name: "right-to-left override", val: "#help\u202edesk"},
		{name: "zero width space", val: "#help\u200bdesk"},
		{name: "no-break space", val: "#help\u00a0desk"},
		{name: "contains at sign", val: "@here"},
		{name: "contains at sign inside", val: "#help@desk"},
		{name: "whitespace only spaces", val: "   "},
		{name: "whitespace only tabs and spaces", val: " \t "},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvSupportChannel] = tc.val
			_, err := Load(env(base))
			if err == nil {
				t.Fatalf("Load with %q succeeded, want error", tc.name)
			}
			msg := err.Error()
			if !strings.Contains(msg, EnvSupportChannel) {
				t.Errorf("error %q does not contain %s", msg, EnvSupportChannel)
			}
			if strings.TrimSpace(tc.val) != "" && strings.Contains(msg, tc.val) {
				t.Errorf("error %q leaks the refused value %q", msg, tc.val)
			}
		})
	}
}

func TestLoad_Boundary_SupportChannelLength(t *testing.T) {
	t.Run("exactly 64 runes accepted", func(t *testing.T) {
		val64 := strings.Repeat("c", 64)
		base := validBaseEnv(t)
		base[EnvSupportChannel] = val64
		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load with 64 runes = %v, want nil", err)
		}
		if cfg.SupportChannel != val64 {
			t.Errorf("SupportChannel len = %d, want 64", len(cfg.SupportChannel))
		}
	})

	t.Run("exactly 65 runes refused", func(t *testing.T) {
		val65 := strings.Repeat("c", 65)
		base := validBaseEnv(t)
		base[EnvSupportChannel] = val65
		_, err := Load(env(base))
		if err == nil {
			t.Fatalf("Load with 65 runes succeeded, want error")
		}
		if !strings.Contains(err.Error(), EnvSupportChannel) {
			t.Errorf("error %q does not contain %s", err.Error(), EnvSupportChannel)
		}
		if strings.Contains(err.Error(), val65) {
			t.Errorf("error %q leaks the refused value", err.Error())
		}
	})
}
