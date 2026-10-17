package main

import (
	"testing"

	"github.com/tekmora/diallak/internal/config"
)

// Issue #122: with the shipped definition naming bug_reports, the bot wires a non-nil Bug handler.
func TestRun_Positive_BugHandlerWiredFromDefinition(t *testing.T) {
	env := func(key string) string {
		switch key {
		case config.EnvTokenFile:
			return writeSecret(t, "token", "distinctive-token-xyz")
		case config.EnvGuildID:
			return "42"
		case config.EnvCorpusDir:
			return setupTestCorpus(t)
		default:
			return ""
		}
	}
	cfg, err := captureBotConfig(t, env)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if cfg.Bug == nil {
		t.Fatal("BotConfig.Bug = nil, want non-nil bug handler")
	}
}
