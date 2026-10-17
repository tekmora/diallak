package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/news"
	"github.com/tekmora/diallak/internal/serverdef"
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

func testDiscussionsKeyPEM(t *testing.T) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PrivateKey(priv)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: der,
	}
	return string(pem.EncodeToMemory(block))
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

// Issue #99: when discussions are configured and server definition has discussions_channel,
// newNews wires the discussions reader and appends the discussions source.
func TestNewNews_Positive_DiscussionsConfigured(t *testing.T) {
	keyPEM := testDiscussionsKeyPEM(t)
	cfg := config.Config{
		NewsStateFile:          filepath.Join(t.TempDir(), "news.json"),
		DiscussionsAppClientID: "Iv23test123",
		DiscussionsAppKey:      keyPEM,
	}
	def := &serverdef.Definition{
		Roles: []serverdef.Role{
			{ID: "notify-releases", Name: "Notify: releases"},
		},
		Channels: []serverdef.Channel{
			{ID: "announcements", Name: "announcements", Type: "announcement"},
			{ID: "discussions-ch", Name: "discussions", Type: "text"},
		},
		News: &serverdef.News{
			Repositories:       []string{"tekmora/diallak"},
			DiscussionsChannel: "discussions-ch",
		},
	}
	newsCfg, err := newNews(cfg, def)
	if err != nil {
		t.Fatalf("newNews error = %v", err)
	}
	if newsCfg == nil || newsCfg.Runner == nil || newsCfg.Runner.Discussions == nil {
		t.Fatalf("expected runner with discussions reader, got %+v", newsCfg)
	}
	if newsCfg.Names.Discussions != "discussions" {
		t.Errorf("newsCfg.Names.Discussions = %q, want %q", newsCfg.Names.Discussions, "discussions")
	}
	var foundDisc bool
	for _, src := range newsCfg.Runner.Sources {
		if src.ID == "discussions" && src.Kind == news.SourceDiscussions && src.Interval == news.DiscussionInterval {
			foundDisc = true
			break
		}
	}
	if !foundDisc {
		t.Errorf("sources does not contain discussions source: %+v", newsCfg.Runner.Sources)
	}
}

// Issue #99: when discussions are configured but server definition has no discussions_channel,
// newNews returns an error.
func TestNewNews_Negative_DiscussionsWithoutChannel(t *testing.T) {
	keyPEM := testDiscussionsKeyPEM(t)
	cfg := config.Config{
		NewsStateFile:          filepath.Join(t.TempDir(), "news.json"),
		DiscussionsAppClientID: "Iv23test123",
		DiscussionsAppKey:      keyPEM,
	}
	def := &serverdef.Definition{
		Roles: []serverdef.Role{
			{ID: "notify-releases", Name: "Notify: releases"},
		},
		Channels: []serverdef.Channel{
			{ID: "announcements", Name: "announcements", Type: "announcement"},
		},
		News: &serverdef.News{
			Repositories: []string{"tekmora/diallak"},
		},
	}
	_, err := newNews(cfg, def)
	if err == nil {
		t.Fatal("expected error when discussions configured without discussions_channel, got nil")
	}
}

// Issue #99: with the app configured, the committed server definition wires the bridge into the
// lounge (ledger diallak-99-channel).
func TestRun_Positive_DiscussionsWiredIntoLounge(t *testing.T) {
	state := filepath.Join(t.TempDir(), "news.json")
	keyPath := writeSecret(t, "ghapp.pem", testDiscussionsKeyPEM(t))
	env := func(key string) string {
		switch key {
		case config.EnvTokenFile:
			return writeSecret(t, "token", "distinctive-token-xyz")
		case config.EnvGuildID:
			return "42"
		case config.EnvCorpusDir:
			return setupTestCorpus(t)
		case config.EnvNewsStateFile:
			return state
		case config.EnvDiscussionsAppClientID:
			return "Iv23test123"
		case config.EnvDiscussionsAppKeyFile:
			return keyPath
		default:
			return ""
		}
	}
	cfg, err := captureBotConfig(t, env)
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if cfg.News == nil || cfg.News.Names.Discussions != "lounge" {
		t.Fatalf("news = %+v, want the discussions place named lounge", cfg.News)
	}
	if !hasDiscussionsSource(cfg.News.Runner.Sources) || cfg.News.Runner.Discussions == nil {
		t.Errorf("sources = %+v, want one discussions source with a reader", cfg.News.Runner.Sources)
	}
}

func hasDiscussionsSource(sources []news.Source) bool {
	for _, src := range sources {
		if src.Kind == news.SourceDiscussions && src.Interval == news.DiscussionInterval {
			return true
		}
	}
	return false
}
