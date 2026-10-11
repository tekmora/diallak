package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestLoad_Positive_AnswerPathDefaults(t *testing.T) {
	t.Run("unset variables remain zero values", func(t *testing.T) {
		cfg, err := Load(env(validBaseEnv(t)))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.TopPassages != 0 || cfg.MinScore != 0.0 || cfg.QuestionMaxChars != 0 {
			t.Errorf("unexpected passages=%d, minScore=%f, questionMax=%d",
				cfg.TopPassages, cfg.MinScore, cfg.QuestionMaxChars)
		}
		if cfg.ExcerptChars != 0 || cfg.ReplyMaxChars != 0 || cfg.MaxConcurrentAsks != 0 {
			t.Errorf("unexpected excerpt=%d, reply=%d, concurrent=%d",
				cfg.ExcerptChars, cfg.ReplyMaxChars, cfg.MaxConcurrentAsks)
		}
		if cfg.AskTimeout != 0 || cfg.ModelTimeout != 0 || cfg.SourceLinkTemplate != "" {
			t.Errorf("unexpected askTimeout=%v, modelTimeout=%v, linkTemplate=%q",
				cfg.AskTimeout, cfg.ModelTimeout, cfg.SourceLinkTemplate)
		}
	})

	t.Run("empty string variables remain zero values", func(t *testing.T) {
		base := validBaseEnv(t)
		emptyVars := []string{
			EnvTopPassages, EnvMinScore, EnvQuestionMaxChars,
			EnvExcerptChars, EnvReplyMaxChars, EnvMaxConcurrentAsks,
			EnvAskTimeout, EnvModelTimeout, EnvSourceLinkTemplate,
		}
		for _, v := range emptyVars {
			base[v] = ""
		}
		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load with empty optional variables = %v, want nil", err)
		}
		if cfg.TopPassages != 0 || cfg.MinScore != 0.0 || cfg.SourceLinkTemplate != "" {
			t.Errorf("empty variables did not result in zero values: %+v", cfg)
		}
	})
}

func TestLoad_Positive_AnswerPathConfigured(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvTopPassages] = "5"
	base[EnvMinScore] = "0.50"
	base[EnvQuestionMaxChars] = "250"
	base[EnvExcerptChars] = "200"
	base[EnvReplyMaxChars] = "1500"
	base[EnvMaxConcurrentAsks] = "10"
	base[EnvAskTimeout] = "30s"
	base[EnvModelTimeout] = "20s"
	base[EnvSourceLinkTemplate] = "https://gitlab.example.com/{repository}/-/blob/{revision}/{path}"

	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if cfg.TopPassages != 5 || cfg.MinScore != 0.50 || cfg.QuestionMaxChars != 250 {
		t.Errorf("topPassages=%d, minScore=%f, questionMax=%d", cfg.TopPassages, cfg.MinScore, cfg.QuestionMaxChars)
	}
	if cfg.ExcerptChars != 200 || cfg.ReplyMaxChars != 1500 || cfg.MaxConcurrentAsks != 10 {
		t.Errorf("excerpt=%d, reply=%d, concurrent=%d", cfg.ExcerptChars, cfg.ReplyMaxChars, cfg.MaxConcurrentAsks)
	}
	if cfg.AskTimeout != 30*time.Second || cfg.ModelTimeout != 20*time.Second {
		t.Errorf("askTimeout=%v, modelTimeout=%v", cfg.AskTimeout, cfg.ModelTimeout)
	}
	wantTmpl := "https://gitlab.example.com/{repository}/-/blob/{revision}/{path}"
	if cfg.SourceLinkTemplate != wantTmpl {
		t.Errorf("sourceLinkTemplate=%q, want %q", cfg.SourceLinkTemplate, wantTmpl)
	}
}

func TestLoad_Negative_IntegerVariables(t *testing.T) {
	cases := []struct {
		envVar string
		val    string
	}{
		{EnvTopPassages, "not-a-number"},
		{EnvTopPassages, "3.5"},
		{EnvTopPassages, "-1"},
		{EnvTopPassages, "0"},
		{EnvTopPassages, "11"},
		{EnvQuestionMaxChars, "foo"},
		{EnvQuestionMaxChars, "10.5"},
		{EnvQuestionMaxChars, "-1"},
		{EnvQuestionMaxChars, "0"},
		{EnvQuestionMaxChars, "501"},
		{EnvExcerptChars, "foo"},
		{EnvExcerptChars, "100.5"},
		{EnvExcerptChars, "-1"},
		{EnvExcerptChars, "49"},
		{EnvExcerptChars, "1001"},
		{EnvReplyMaxChars, "foo"},
		{EnvReplyMaxChars, "600.5"},
		{EnvReplyMaxChars, "-1"},
		{EnvReplyMaxChars, "499"},
		{EnvReplyMaxChars, "2001"},
		{EnvMaxConcurrentAsks, "foo"},
		{EnvMaxConcurrentAsks, "4.5"},
		{EnvMaxConcurrentAsks, "-1"},
		{EnvMaxConcurrentAsks, "0"},
		{EnvMaxConcurrentAsks, "65"},
	}

	for _, tc := range cases {
		t.Run(tc.envVar+"="+tc.val, func(t *testing.T) {
			base := validBaseEnv(t)
			base[tc.envVar] = tc.val
			_, err := Load(env(base))
			if err == nil {
				t.Fatalf("Load with %s=%q succeeded, want error", tc.envVar, tc.val)
			}
			if !strings.Contains(err.Error(), tc.envVar) {
				t.Errorf("error %q does not contain %s", err.Error(), tc.envVar)
			}
		})
	}
}

func TestLoad_Negative_MinScore(t *testing.T) {
	cases := []string{"not-a-number", "-0.1", "0", "0.0", "1.01", "NaN", "Inf", "-Inf"}
	for _, val := range cases {
		t.Run("val="+val, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvMinScore] = val
			_, err := Load(env(base))
			if err == nil {
				t.Fatalf("Load with %s=%q succeeded, want error", EnvMinScore, val)
			}
			if !strings.Contains(err.Error(), EnvMinScore) {
				t.Errorf("error %q does not contain %s", err.Error(), EnvMinScore)
			}
		})
	}
}

func TestLoad_Negative_Timeouts(t *testing.T) {
	cases := []struct {
		name      string
		envVar    string
		val       string
		extraVars map[string]string
		wantBoth  bool
	}{
		{"ask invalid duration", EnvAskTimeout, "not-a-duration", nil, false},
		{"ask negative", EnvAskTimeout, "-1s", nil, false},
		{"ask zero", EnvAskTimeout, "0s", nil, false},
		{"ask below min", EnvAskTimeout, "500ms", nil, false},
		{"ask above max", EnvAskTimeout, "10m1s", nil, false},
		{"model invalid duration", EnvModelTimeout, "not-a-duration", nil, false},
		{"model negative", EnvModelTimeout, "-1s", nil, false},
		{"model zero", EnvModelTimeout, "0s", nil, false},
		{"model below min", EnvModelTimeout, "500ms", nil, false},
		{"model above max", EnvModelTimeout, "5m1s", map[string]string{EnvAskTimeout: "6m"}, false},
		{"model equal default ask", EnvModelTimeout, "60s", nil, true},
		{"model above default ask", EnvModelTimeout, "70s", nil, true},
		{"model equal configured ask", EnvModelTimeout, "30s", map[string]string{EnvAskTimeout: "30s"}, true},
		{"model above configured ask", EnvModelTimeout, "40s", map[string]string{EnvAskTimeout: "30s"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[tc.envVar] = tc.val
			for k, v := range tc.extraVars {
				base[k] = v
			}
			_, err := Load(env(base))
			if err == nil {
				t.Fatalf("Load succeeded for %s, want error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.envVar) {
				t.Errorf("error %q does not contain %s", err.Error(), tc.envVar)
			}
			if tc.wantBoth {
				if !strings.Contains(err.Error(), EnvModelTimeout) || !strings.Contains(err.Error(), EnvAskTimeout) {
					t.Errorf("error %q does not contain both %s and %s", err.Error(), EnvModelTimeout, EnvAskTimeout)
				}
			}
		})
	}
}

func TestLoad_Negative_SourceLinkTemplate(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"missing repository", "https://github.com/foo/blob/{revision}/{path}"},
		{"missing revision", "https://github.com/{repository}/blob/main/{path}"},
		{"missing path", "https://github.com/{repository}/blob/{revision}/file"},
		{"duplicate placeholder", "https://github.com/{repository}/{repository}/blob/{revision}/{path}"},
		{"http scheme", "http://github.com/{repository}/blob/{revision}/{path}"},
		{"contains space", "https://github.com/ {repository}/blob/{revision}/{path}"},
		{"contains newline", "https://github.com/{repository}\n/blob/{revision}/{path}"},
		{"contains tab", "https://github.com/{repository}\t/blob/{revision}/{path}"},
		{"closing angle bracket", "https://github.com/{repository}>/blob/{revision}/{path}"},
		{"opening angle bracket", "https://github.com/<{repository}/blob/{revision}/{path}"},
		{"zero width space", "https://github.com/{repository}\u200b/blob/{revision}/{path}"},
		{"user name in the address", "https://user@github.com/{repository}/blob/{revision}/{path}"},
		{"no host", "https:///{repository}/blob/{revision}/{path}"},
		{"placeholder as host only", "https://{repository}{revision}{path}@"},
		{"placeholder is the host", "https://{repository}/blob/{revision}/{path}"},
		{"placeholder inside the host", "https://{repository}.example.com/{revision}/{path}"},
		{"placeholder as port", "https://example.com:{revision}/{repository}/{path}"},
		{"placeholder as user", "https://{path}@example.com/{repository}/{revision}"},
		{"exceeds 200 runes", "https://github.com/{repository}/blob/{revision}/{path}" + strings.Repeat("x", 151)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvSourceLinkTemplate] = tc.val
			_, err := Load(env(base))
			if err == nil {
				t.Fatalf("Load with template %s succeeded, want error", tc.name)
			}
			if !strings.Contains(err.Error(), EnvSourceLinkTemplate) {
				t.Errorf("error %q does not contain %s", err.Error(), EnvSourceLinkTemplate)
			}
		})
	}
}

func TestLoad_Boundary_AnswerPathBounds(t *testing.T) {
	t.Run("top passages bounds", func(t *testing.T) {
		checkBoundaryInt(t, EnvTopPassages, 1, 10, 0, 11)
	})
	t.Run("question max chars bounds", func(t *testing.T) {
		checkBoundaryInt(t, EnvQuestionMaxChars, 1, 500, 0, 501)
	})
	t.Run("excerpt chars bounds", func(t *testing.T) {
		checkBoundaryInt(t, EnvExcerptChars, 50, 1000, 49, 1001)
	})
	t.Run("reply max chars bounds", func(t *testing.T) {
		checkBoundaryInt(t, EnvReplyMaxChars, 500, 2000, 499, 2001)
	})
	t.Run("concurrent asks bounds", func(t *testing.T) {
		checkBoundaryInt(t, EnvMaxConcurrentAsks, 1, 64, 0, 65)
	})
	t.Run("min score bounds", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvMinScore] = "0.01"
		if _, err := Load(env(base)); err != nil {
			t.Errorf("Load with min score 0.01 = %v, want nil", err)
		}
		base[EnvMinScore] = "1.0"
		if _, err := Load(env(base)); err != nil {
			t.Errorf("Load with min score 1.0 = %v, want nil", err)
		}
		base[EnvMinScore] = "0.0"
		if _, err := Load(env(base)); err == nil {
			t.Errorf("Load with min score 0.0 succeeded, want error")
		}
		base[EnvMinScore] = "1.0001"
		if _, err := Load(env(base)); err == nil {
			t.Errorf("Load with min score 1.0001 succeeded, want error")
		}
	})
}

func checkBoundaryInt(t *testing.T, envVar string, minVal, maxVal, belowMin, aboveMax int) {
	t.Helper()
	base := validBaseEnv(t)
	base[envVar] = fmt.Sprintf("%d", minVal)
	if _, err := Load(env(base)); err != nil {
		t.Errorf("Load with %s=%d = %v, want nil", envVar, minVal, err)
	}
	base[envVar] = fmt.Sprintf("%d", maxVal)
	if _, err := Load(env(base)); err != nil {
		t.Errorf("Load with %s=%d = %v, want nil", envVar, maxVal, err)
	}
	base[envVar] = fmt.Sprintf("%d", belowMin)
	if _, err := Load(env(base)); err == nil {
		t.Errorf("Load with %s=%d succeeded, want error", envVar, belowMin)
	}
	base[envVar] = fmt.Sprintf("%d", aboveMax)
	if _, err := Load(env(base)); err == nil {
		t.Errorf("Load with %s=%d succeeded, want error", envVar, aboveMax)
	}
}

func TestLoad_Boundary_TimeoutBounds(t *testing.T) {
	t.Run("ask timeout bounds", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvAskTimeout] = "1s"
		if _, err := Load(env(base)); err != nil {
			t.Errorf("Load with ask timeout 1s = %v, want nil", err)
		}
		base[EnvAskTimeout] = "10m"
		if _, err := Load(env(base)); err != nil {
			t.Errorf("Load with ask timeout 10m = %v, want nil", err)
		}
		base[EnvAskTimeout] = "999ms"
		if _, err := Load(env(base)); err == nil {
			t.Errorf("Load with ask timeout 999ms succeeded, want error")
		}
		base[EnvAskTimeout] = "10m1s"
		if _, err := Load(env(base)); err == nil {
			t.Errorf("Load with ask timeout 10m1s succeeded, want error")
		}
	})

	t.Run("model timeout bounds", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvModelTimeout] = "1s"
		if _, err := Load(env(base)); err != nil {
			t.Errorf("Load with model timeout 1s = %v, want nil", err)
		}
		base[EnvModelTimeout] = "59s"
		if _, err := Load(env(base)); err != nil {
			t.Errorf("Load with model timeout 59s = %v, want nil", err)
		}
		base[EnvModelTimeout] = "60s"
		if _, err := Load(env(base)); err == nil {
			t.Errorf("Load with model timeout 60s (equal to default ask) succeeded, want error")
		}
		base[EnvAskTimeout] = "6m"
		base[EnvModelTimeout] = "5m"
		if _, err := Load(env(base)); err != nil {
			t.Errorf("Load with model timeout 5m = %v, want nil", err)
		}
		base[EnvModelTimeout] = "5m1s"
		if _, err := Load(env(base)); err == nil {
			t.Errorf("Load with model timeout 5m1s succeeded, want error")
		}
	})
}

func TestLoad_Boundary_SourceLinkTemplateLength(t *testing.T) {
	prefix := "https://github.com/{repository}/blob/{revision}/{path}"
	t.Run("exactly 200 runes accepted", func(t *testing.T) {
		pad := strings.Repeat("x", 200-len(prefix))
		base := validBaseEnv(t)
		base[EnvSourceLinkTemplate] = prefix + pad
		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load with 200 runes template = %v, want nil", err)
		}
		if len(cfg.SourceLinkTemplate) != 200 {
			t.Errorf("template length = %d, want 200", len(cfg.SourceLinkTemplate))
		}
	})

	t.Run("exactly 201 runes refused", func(t *testing.T) {
		pad := strings.Repeat("x", 201-len(prefix))
		base := validBaseEnv(t)
		base[EnvSourceLinkTemplate] = prefix + pad
		_, err := Load(env(base))
		if err == nil {
			t.Fatalf("Load with 201 runes template succeeded, want error")
		}
		if !strings.Contains(err.Error(), EnvSourceLinkTemplate) {
			t.Errorf("error %q does not contain %s", err.Error(), EnvSourceLinkTemplate)
		}
	})
}

func TestLoad_Positive_SourceLinkTemplatePlaceholderPlaces(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"placeholder in path", "https://example.com/{repository}/blob/{revision}/{path}"},
		{"placeholders in query and fragment", "https://example.com/x?repo={repository}&rev={revision}#{path}"},
		{"placeholder with port", "https://example.com:8443/{repository}/{revision}/{path}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvSourceLinkTemplate] = tc.val
			cfg, err := Load(env(base))
			if err != nil {
				t.Fatalf("Load with template %s = %v, want nil", tc.name, err)
			}
			if cfg.SourceLinkTemplate != tc.val {
				t.Errorf("cfg.SourceLinkTemplate = %q, want %q", cfg.SourceLinkTemplate, tc.val)
			}
		})
	}
}

func TestParseSourceLinkTemplate_Negative_ErrorNamesTheFailedCheck(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantSub string
	}{
		{"length", "http://example.com/" + strings.Repeat("x", 200), "exceeds"},
		{"scheme", "http://example.com/<>", "must start with 'https://'"},
		{"character", "https://example.com/<bad>", "angle bracket"},
		{"placeholder missing", "https://user@example.com/{revision}/{path}", "exactly once, found 0"},
		{"placeholder twice", "https://user@example.com/{repository}/{repository}/{revision}/{path}", "exactly once, found 2"},
		{"address", "https://user@example.com/{repository}/{revision}/{path}", "without a user name"},
		{"placeholder in host", "https://{repository}/{revision}/{path}", "before the path"},
		{"placeholder missing and one in the host", "https://{repository}/{revision}", "exactly once, found 0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSourceLinkTemplate(tc.input)
			if err == nil {
				t.Fatalf("parseSourceLinkTemplate(%q) succeeded, want error", tc.input)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("parseSourceLinkTemplate(%q) error = %q, want substring %q", tc.input, err.Error(), tc.wantSub)
			}
		})
	}
}

func TestConfig_Positive_StringAndLogValueContainAllFields(t *testing.T) {
	cfg := Config{
		Token:              "super-secret-token",
		GuildID:            42,
		ModelBaseURL:       "https://model.example.com",
		ModelAlias:         "router/test",
		ModelKey:           "super-secret-key",
		CorpusDir:          "/var/data/corpus",
		SupportChannel:     "#help",
		TopPassages:        3,
		MinScore:           0.20,
		QuestionMaxChars:   500,
		ExcerptChars:       400,
		ReplyMaxChars:      1900,
		MaxConcurrentAsks:  4,
		AskTimeout:         60 * time.Second,
		ModelTimeout:       45 * time.Second,
		SourceLinkTemplate: "https://github.com/{repository}/blob/{revision}/{path}",
	}

	str := cfg.String()
	for _, field := range []string{
		"TopPassages:3", "MinScore:0.2", "QuestionMaxChars:500",
		"ExcerptChars:400", "ReplyMaxChars:1900", "MaxConcurrentAsks:4",
		"AskTimeout:1m0s", "ModelTimeout:45s",
	} {
		if !strings.Contains(str, field) {
			t.Errorf("String() missing %s: %s", field, str)
		}
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("dump", slog.Any("cfg", cfg))
	logText := buf.String()
	for _, field := range []string{
		`"TopPassages":3`, `"QuestionMaxChars":500`,
		`"ExcerptChars":400`, `"ReplyMaxChars":1900`, `"MaxConcurrentAsks":4`,
	} {
		if !strings.Contains(logText, field) {
			t.Errorf("LogValue() JSON missing %s: %s", field, logText)
		}
	}
}

func TestLoad_Boundary_AskTimeoutLeavesRoomForTheModelTimeoutInEffect(t *testing.T) {
	withModel := func(extra map[string]string) map[string]string {
		base := validBaseEnv(t)
		base[EnvModelBaseURL] = "https://model.example.com"
		base[EnvModelAlias] = "router/test"
		for k, v := range extra {
			base[k] = v
		}
		return base
	}

	// A model is configured and its timeout is not set: the built-in 45 seconds are in effect.
	if _, err := Load(env(withModel(map[string]string{EnvAskTimeout: "46s"}))); err != nil {
		t.Errorf("ask timeout 46s with the built-in model timeout was refused: %v", err)
	}
	for _, ask := range []string{"45s", "30s"} {
		_, err := Load(env(withModel(map[string]string{EnvAskTimeout: ask})))
		if err == nil {
			t.Errorf("ask timeout %s with the built-in model timeout was accepted", ask)
			continue
		}
		if !strings.Contains(err.Error(), EnvModelTimeout) || !strings.Contains(err.Error(), EnvAskTimeout) {
			t.Errorf("error %q does not name both variables", err.Error())
		}
	}

	// Without a model the model timeout plays no part, so a short ask timeout is fine.
	base := validBaseEnv(t)
	base[EnvAskTimeout] = "30s"
	if _, err := Load(env(base)); err != nil {
		t.Errorf("ask timeout 30s without a model was refused: %v", err)
	}
}

func TestLoad_Positive_HealthAddr(t *testing.T) {
	t.Run("unset is empty", func(t *testing.T) {
		cfg, err := Load(env(validBaseEnv(t)))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.HealthAddr != "" {
			t.Errorf("HealthAddr = %q, want empty", cfg.HealthAddr)
		}
	})

	t.Run("valid IPv4 with port", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvHealthAddr] = "127.0.0.1:8080"
		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.HealthAddr != "127.0.0.1:8080" {
			t.Errorf("HealthAddr = %q, want 127.0.0.1:8080", cfg.HealthAddr)
		}
	})

	t.Run("empty host with port", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvHealthAddr] = ":8080"
		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.HealthAddr != ":8080" {
			t.Errorf("HealthAddr = %q, want :8080", cfg.HealthAddr)
		}
	})

	t.Run("IPv6 in brackets with port", func(t *testing.T) {
		base := validBaseEnv(t)
		base[EnvHealthAddr] = "[::1]:8081"
		cfg, err := Load(env(base))
		if err != nil {
			t.Fatalf("Load = %v, want nil", err)
		}
		if cfg.HealthAddr != "[::1]:8081" {
			t.Errorf("HealthAddr = %q, want [::1]:8081", cfg.HealthAddr)
		}
	})
}

func TestLoad_Negative_HealthAddrRefusesInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want string
	}{
		{"no port", "127.0.0.1", "host:port"},
		{"port 0", "127.0.0.1:0", "port"},
		{"port out of range", "127.0.0.1:65536", "port"},
		{"port negative", "127.0.0.1:-1", "port"},
		{"port with a sign", "127.0.0.1:+8080", "port"},
		{"port with a leading zero", "127.0.0.1:08080", "port"},
		{"port not a number", "127.0.0.1:abc", "port"},
		{"host not IP literal", "localhost:8080", "not an IP literal"},
		{"host domain", "example.com:8080", "not an IP literal"},
		{"leading space", " 127.0.0.1:8080", "not an IP literal"},
		{"space as host", " :8080", "not an IP literal"},
		{"trailing space", "127.0.0.1:8080 ", "port"},
		{"extra text path", "127.0.0.1:8080/healthz", "port"},
		{"extra text trailing", "127.0.0.1:8080 extra", "port"},
		{"ipv6 without brackets", "::1:8080", "host:port"},
		{"ipv4 in brackets", "[127.0.0.1]:8080", "brackets"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvHealthAddr] = tc.val
			cfg, err := Load(env(base))
			if err == nil {
				t.Fatalf("Load(%s=%q) succeeded, want error", EnvHealthAddr, tc.val)
			}
			if !strings.Contains(err.Error(), EnvHealthAddr) {
				t.Errorf("error %q does not name %s", err.Error(), EnvHealthAddr)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not say what is wrong (%q)", err.Error(), tc.want)
			}
			if cfg.HealthAddr != "" {
				t.Errorf("cfg.HealthAddr = %q, want empty on error", cfg.HealthAddr)
			}
		})
	}
}

func TestLoad_Boundary_HealthAddrPorts(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvHealthAddr] = "127.0.0.1:1"
	if cfg, err := Load(env(base)); err != nil || cfg.HealthAddr != "127.0.0.1:1" {
		t.Errorf("port 1 = %v (cfg.HealthAddr=%q), want success", err, cfg.HealthAddr)
	}

	base[EnvHealthAddr] = "127.0.0.1:65535"
	if cfg, err := Load(env(base)); err != nil || cfg.HealthAddr != "127.0.0.1:65535" {
		t.Errorf("port 65535 = %v (cfg.HealthAddr=%q), want success", err, cfg.HealthAddr)
	}

	base[EnvHealthAddr] = "127.0.0.1:0"
	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), EnvHealthAddr) {
		t.Errorf("port 0 err = %v, want error naming %s", err, EnvHealthAddr)
	}

	base[EnvHealthAddr] = "127.0.0.1:65536"
	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), EnvHealthAddr) {
		t.Errorf("port 65536 err = %v, want error naming %s", err, EnvHealthAddr)
	}
}
