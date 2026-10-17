package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/news"
)

func newsEnv(t *testing.T, state string) func(string) string {
	t.Helper()
	values := map[string]string{
		config.EnvTokenFile:     writeSecret(t, "token", "distinctive-token-xyz"),
		config.EnvGuildID:       "42",
		config.EnvCorpusDir:     setupTestCorpus(t),
		config.EnvNewsStateFile: state,
	}
	return func(key string) string { return values[key] }
}

// Issue #180: without the state file the news function is off and reads nothing.
func TestRun_Negative_NewsOffByDefault(t *testing.T) {
	cfg, err := captureBotConfig(t, newsEnv(t, ""))
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if cfg.News != nil {
		t.Fatalf("BotConfig.News = %+v, want nil", cfg.News)
	}
}

// With the state file the runner reads the committed definition's own repositories and keeps its
// state in that file.
func TestRun_Positive_NewsOnWithTheStateFile(t *testing.T) {
	state := filepath.Join(t.TempDir(), "news.json")
	cfg, err := captureBotConfig(t, newsEnv(t, state))
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if cfg.News == nil || cfg.News.Runner == nil || cfg.News.Runner.Store == nil || cfg.News.Runner.Reader == nil {
		t.Fatalf("BotConfig.News = %+v, want a runner with store and reader", cfg.News)
	}
	want := news.OwnSources([]string{"tekmora/diallak", "tekmora/epitrop"})
	if len(cfg.News.Runner.Sources) != len(want) || cfg.News.Runner.Sources[0] != want[0] {
		t.Errorf("sources = %+v, want %+v", cfg.News.Runner.Sources, want)
	}
	if cfg.News.Names.Announcement != "announcements" || cfg.News.Names.Role != "Notify: releases" {
		t.Errorf("names = %+v", cfg.News.Names)
	}
	if err := cfg.News.Runner.Store.Save(t.Context()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(state); err != nil {
		t.Errorf("state file not written: %v", err)
	}
}

// A damaged state file stops the start: an empty state would post everything again.
func TestRun_Boundary_NewsDamagedStateStopsStart(t *testing.T) {
	state := filepath.Join(t.TempDir(), "news.json")
	if err := os.WriteFile(state, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := captureBotConfig(t, newsEnv(t, state)); err == nil {
		t.Fatal("run with a damaged state file succeeded")
	}
}
