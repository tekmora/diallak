package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/discord"
)

type recordingForumCalls struct {
	mu      sync.Mutex
	replies []string
}

func (*recordingForumCalls) FirstMessage(context.Context, uint64) (discord.ForumMessage, error) {
	return discord.ForumMessage{Content: "Where is the documentation text for questions?"}, nil
}

func (c *recordingForumCalls) Reply(_ context.Context, _ uint64, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replies = append(c.replies, text)
	return nil
}

func forumEnv(t *testing.T, channel, tag string) func(string) string {
	t.Helper()
	values := map[string]string{
		config.EnvTokenFile:       writeSecret(t, "token", "distinctive-token-xyz"),
		config.EnvGuildID:         "42",
		config.EnvCorpusDir:       setupTestCorpus(t),
		config.EnvForumChannelID:  channel,
		config.EnvForumOptInTagID: tag,
	}
	return func(key string) string { return values[key] }
}

func captureBotConfig(t *testing.T, getenv func(string) string) (discord.BotConfig, error) {
	t.Helper()
	var captured discord.BotConfig
	deps := dependencies{
		registerCommands: func(context.Context, string, uint64, int) error { return nil },
		runDiscord: func(_ context.Context, cfg discord.BotConfig) error {
			captured = cfg
			return nil
		},
	}
	var out bytes.Buffer
	err := run(context.Background(), nil, getenv, &out, deps)
	return captured, err
}

// The configured forum reaches the Discord wiring and answers an opted-in post with excerpts from
// the corpus the process serves.
func TestRun_Positive_ForumAnswersFromTheCorpus(t *testing.T) {
	cfg, err := captureBotConfig(t, forumEnv(t, "77", "88"))
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if cfg.Forum == nil || cfg.Forum.ChannelID() != 77 {
		t.Fatalf("BotConfig.Forum = %+v, want a handler for channel 77", cfg.Forum)
	}
	calls := &recordingForumCalls{}
	post := discord.ForumPost{ThreadID: 5, ParentID: 77, PublicThread: true, NewlyCreated: true, TagIDs: []uint64{88}}
	cfg.Forum.Dispatch(context.Background(), post, "Documentation", calls)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := cfg.Forum.Wait(ctx); err != nil {
		t.Fatalf("Forum.Wait: %v", err)
	}
	calls.mu.Lock()
	defer calls.mu.Unlock()
	if len(calls.replies) != 1 || !strings.HasSuffix(calls.replies[0], answer.FooterForum) ||
		!strings.Contains(calls.replies[0], "Documentation text for questions.") {
		t.Errorf("replies = %q, want one excerpt reply from the corpus with the forum footer", calls.replies)
	}
}

// ADR-0009 case 1: without the forum settings the wiring holds no forum handler.
func TestRun_Negative_ForumUnsetHasNoHandler(t *testing.T) {
	cfg, err := captureBotConfig(t, validEnv(t))
	if err != nil {
		t.Fatalf("run = %v, want nil", err)
	}
	if cfg.Forum != nil {
		t.Errorf("BotConfig.Forum = %+v, want nil", cfg.Forum)
	}
}

// ADR-0009 case 18: the forum without the opt-in tag stops the start before Discord is reached.
func TestRun_Negative_ForumWithoutTagStopsTheStart(t *testing.T) {
	var discordCalled bool
	deps := dependencies{
		registerCommands: func(context.Context, string, uint64, int) error { return nil },
		runDiscord: func(context.Context, discord.BotConfig) error {
			discordCalled = true
			return nil
		},
	}
	var out bytes.Buffer
	err := run(context.Background(), nil, forumEnv(t, "77", ""), &out, deps)
	if err == nil || !strings.Contains(err.Error(), config.EnvForumOptInTagID) {
		t.Errorf("run = %v, want an error naming %s", err, config.EnvForumOptInTagID)
	}
	if discordCalled {
		t.Error("runDiscord was called although the opt-in tag is missing")
	}
}
