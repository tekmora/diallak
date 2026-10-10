package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeToken(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write token fixture: %v", err)
	}
	return path
}

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoad_Positive_ReadsTokenFileAndGuild(t *testing.T) {
	path := writeToken(t, "secret-token\n")
	cfg, err := Load(env(map[string]string{EnvTokenFile: path, EnvGuildID: "123456789012345678"}))
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if cfg.Token != "secret-token" {
		t.Errorf("Token = %q, want the trimmed file content", cfg.Token)
	}
	if cfg.GuildID != 123456789012345678 {
		t.Errorf("GuildID = %d, want 123456789012345678", cfg.GuildID)
	}
}

func TestLoad_Negative_RefusesInvalidInput(t *testing.T) {
	valid := writeToken(t, "secret-token")
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"guild missing", map[string]string{EnvTokenFile: valid}, EnvGuildID + " is not set"},
		{"guild not decimal", map[string]string{EnvTokenFile: valid, EnvGuildID: "0x10"}, "not a decimal snowflake"},
		{"guild negative", map[string]string{EnvTokenFile: valid, EnvGuildID: "-1"}, "not a decimal snowflake"},
		{"guild zero", map[string]string{EnvTokenFile: valid, EnvGuildID: "0"}, "must not be zero"},
		{"token variable missing", map[string]string{EnvGuildID: "1"}, EnvTokenFile + " is not set"},
		{"token file absent", map[string]string{EnvTokenFile: filepath.Join(t.TempDir(), "absent"), EnvGuildID: "1"}, "open token file"},
		{"token file blank", map[string]string{EnvTokenFile: writeToken(t, " \n\t"), EnvGuildID: "1"}, "token file is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(env(tc.env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want one naming %q", err, tc.want)
			}
			if cfg != (Config{}) {
				t.Errorf("Load returned %+v with an error, want the zero Config", cfg.GuildID)
			}
		})
	}
}

func TestLoad_Boundary_TokenSizeAndGuildRange(t *testing.T) {
	atLimit := writeToken(t, strings.Repeat("a", MaxTokenBytes))
	cfg, err := Load(env(map[string]string{EnvTokenFile: atLimit, EnvGuildID: "18446744073709551615"}))
	if err != nil {
		t.Fatalf("Load at the size limit = %v, want nil", err)
	}
	if len(cfg.Token) != MaxTokenBytes || cfg.GuildID != 18446744073709551615 {
		t.Errorf("Load at the limits = %d token bytes, guild %d", len(cfg.Token), cfg.GuildID)
	}

	overLimit := writeToken(t, strings.Repeat("a", MaxTokenBytes+1))
	if _, err := Load(env(map[string]string{EnvTokenFile: overLimit, EnvGuildID: "1"})); err == nil ||
		!strings.Contains(err.Error(), "larger than") {
		t.Errorf("Load over the size limit = %v, want an error naming the limit", err)
	}

	if _, err := Load(env(map[string]string{EnvTokenFile: atLimit, EnvGuildID: "18446744073709551616"})); err == nil {
		t.Error("Load with a guild id above uint64 = nil, want an error")
	}
}
