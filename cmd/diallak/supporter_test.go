package main

import (
	"strings"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/discord"
)

func supporterEnv(t *testing.T, vars map[string]string) func(string) string {
	t.Helper()
	values := map[string]string{
		config.EnvTokenFile: writeSecret(t, "token", "distinctive-token-xyz"),
		config.EnvGuildID:   "42",
		config.EnvCorpusDir: setupTestCorpus(t),
	}
	for k, v := range vars {
		values[k] = v
	}
	return func(key string) string { return values[key] }
}

// Switched on, the supporter function reaches the Discord wiring with the configured grace period.
func TestRun_Positive_SupporterRolesReachTheWiring(t *testing.T) {
	cfg, err := captureBotConfig(t, supporterEnv(t, map[string]string{
		config.EnvSupporterRoles: "on", config.EnvSupporterGrace: "48h",
	}))
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if cfg.Supporter == nil || cfg.Supporter.Grace() != 48*time.Hour {
		t.Fatalf("BotConfig.Supporter = %+v, want a handler with grace 48h", cfg.Supporter)
	}
	cfg, err = captureBotConfig(t, supporterEnv(t, map[string]string{config.EnvSupporterRoles: "on"}))
	if err != nil || cfg.Supporter == nil || cfg.Supporter.Grace() != discord.DefaultSupporterGrace {
		t.Fatalf("run without a grace = %v, %+v; want the default grace", err, cfg.Supporter)
	}
}

// ADR-0027: without the switch there is no supporter function, so the members intent is not asked.
func TestRun_Negative_SupporterRolesOffByDefault(t *testing.T) {
	cfg, err := captureBotConfig(t, supporterEnv(t, nil))
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if cfg.Supporter != nil {
		t.Fatalf("BotConfig.Supporter = %+v, want nil without %s", cfg.Supporter, config.EnvSupporterRoles)
	}
	_, err = captureBotConfig(t, supporterEnv(t, map[string]string{config.EnvSupporterGrace: "48h"}))
	if err == nil || !strings.Contains(err.Error(), config.EnvSupporterRoles) {
		t.Fatalf("run with a grace and no switch = %v, want an error naming %s", err, config.EnvSupporterRoles)
	}
}
