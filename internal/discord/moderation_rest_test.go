package discord

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"
)

// newModerationRest returns the calls against a server that answers every request with status.
func newModerationRest(t *testing.T, status int) restModerationCalls {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := fmt.Fprint(w, `{"code":0,"message":"refused"}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := newRestClient(makeFakeToken(gatewayAppID), srv.URL)
	if err != nil {
		t.Fatalf("newRestClient: %v", err)
	}
	t.Cleanup(func() { client.Close(context.Background()) })
	return restModerationCalls{rest: client.Rest, guildID: snowflake.ID(modGuildID)}
}

func TestRestModerationCalls_Negative_AnswersAreTold(t *testing.T) {
	for _, c := range []struct {
		status        int
		refused, gone bool
	}{
		{http.StatusForbidden, true, false},
		{http.StatusNotFound, false, true},
		{http.StatusBadRequest, false, false},
	} {
		t.Run(fmt.Sprint(c.status), func(t *testing.T) {
			calls := newModerationRest(t, c.status)
			ctx := context.Background()
			for name, err := range map[string]error{
				"delete":  calls.DeleteMessage(ctx, modChannelID, modMessageID, "Spam"),
				"timeout": calls.TimeOut(ctx, modMemberID, modNow, "Spam"),
				"line":    calls.PostLine(ctx, modTrailID, "line"),
			} {
				if err == nil || errors.Is(err, ErrDiscordRefused) != c.refused || errors.Is(err, ErrDiscordGone) != c.gone {
					t.Errorf("%s: %v; want refused %v, gone %v", name, err, c.refused, c.gone)
				}
			}
		})
	}
}

func TestDiscordCallError_Boundary_NoErrorStaysNil(t *testing.T) {
	if err := discordCallError("delete message", nil); err != nil {
		t.Errorf("discordCallError(nil) = %v, want nil", err)
	}
	if err := discordCallError("delete message", context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrDiscordRefused) || errors.Is(err, ErrDiscordGone) {
		t.Errorf("discordCallError(deadline) = %v, want the deadline unmarked", err)
	}
}

// recordingRest returns a client against a server that answers each path with its body and
// records every request address.
func recordingRest(t *testing.T, bodies map[string]string) (*bot.Client, *httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		body, ok := bodies[r.URL.Path]
		if !ok {
			body = "{}"
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if _, err := fmt.Fprint(w, body); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := newRestClient(makeFakeToken(gatewayAppID), srv.URL)
	if err != nil {
		t.Fatalf("newRestClient: %v", err)
	}
	t.Cleanup(func() { client.Close(context.Background()) })
	return client, srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func TestRestModerationCalls_Positive_ListLinesPagesBeforeTheGivenLine(t *testing.T) {
	client, _, seen := recordingRest(t, map[string]string{fmt.Sprintf("/channels/%d/messages", modTrailID): "[]"})
	calls := restModerationCalls{rest: client.Rest, guildID: snowflake.ID(modGuildID)}
	if _, err := calls.ListLines(context.Background(), modTrailID, 777); err != nil {
		t.Fatalf("ListLines: %v", err)
	}
	got := seen()
	if len(got) != 1 || !strings.Contains(got[0], "before=777") || !strings.Contains(got[0], "limit=100") ||
		strings.Contains(got[0], "around") || strings.Contains(got[0], "after") {
		t.Errorf("requests %q, want one read before 777 with limit 100", got)
	}
}

func TestReadModerationServer_Negative_BotUserWithoutIDFailsTheRead(t *testing.T) {
	client, _, _ := recordingRest(t, map[string]string{
		"/applications/@me": fmt.Sprintf(`{"id":"%d","name":"diallak","bot":{"id":"0","username":"diallak"}}`, gatewayAppID),
	})
	mod, err := NewModerationHandler(ModerationConfig{Definition: modTestDefinition(), GuildID: modGuildID})
	if err != nil {
		t.Fatal(err)
	}
	if err := readModerationServer(context.Background(), client.Rest, modGuildID, mod, time.Second); err == nil ||
		!strings.Contains(err.Error(), "no bot user") {
		t.Errorf("readModerationServer = %v, want the missing bot user named", err)
	}
}

func TestSweepTrail_Positive_SweepsAgainAfterTheInterval(t *testing.T) {
	trailPath := fmt.Sprintf("/channels/%d/messages", modTrailID)
	client, _, seen := recordingRest(t, map[string]string{
		"/applications/@me": fmt.Sprintf(`{"id":"%d","name":"diallak","bot":{"id":"%d","username":"diallak"}}`,
			gatewayAppID, modBotUserID),
		fmt.Sprintf("/guilds/%d/roles", modGuildID): fmt.Sprintf(`[{"id":"%d","name":"Moderator","permissions":"0"}]`, modRoleID),
		fmt.Sprintf("/guilds/%d/channels", modGuildID): fmt.Sprintf(`[{"id":"%d","guild_id":"%d","type":0,"name":"mod-log"}]`,
			modTrailID, modGuildID),
		trailPath: "[]",
	})
	mod, err := NewModerationHandler(ModerationConfig{
		Definition: modTestDefinition(), GuildID: modGuildID, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sweepTrail(ctx, client.Rest, BotConfig{GuildID: modGuildID, Moderation: mod, Logger: slog.New(slog.DiscardHandler)},
			clientOptions{trailSweepEvery: time.Millisecond})
	}()
	deadline := time.After(5 * time.Second)
	for reads := 0; reads < 2; {
		select {
		case <-deadline:
			t.Fatalf("trail read %d times in 5s, want 2; requests %q", reads, seen())
		case <-time.After(5 * time.Millisecond):
		}
		reads = 0
		for _, r := range seen() {
			if strings.HasPrefix(r, "GET "+trailPath+"?") {
				reads++
			}
		}
	}
	cancel()
	<-done
}

func TestSweepTrail_Positive_RefreshFailsKeepsHeldServerAndSweeps(t *testing.T) {
	trailPath := fmt.Sprintf("/channels/%d/messages", modTrailID)
	client, _, seen := recordingRest(t, map[string]string{
		trailPath: "[]",
	})
	mod, err := NewModerationHandler(ModerationConfig{
		Definition: modTestDefinition(), GuildID: modGuildID, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	mod.SetServer(modTestRoles(), modTestChannels(), modBotUserID)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sweepTrail(ctx, client.Rest, BotConfig{GuildID: modGuildID, Moderation: mod, Logger: slog.New(slog.DiscardHandler)},
			clientOptions{trailSweepEvery: time.Millisecond})
	}()

	deadline := time.After(5 * time.Second)
	found := false
	for !found {
		select {
		case <-deadline:
			t.Fatalf("trail was not swept after refresh failure; requests %q", seen())
		case <-time.After(5 * time.Millisecond):
		}
		for _, r := range seen() {
			if strings.HasPrefix(r, "GET "+trailPath+"?") {
				found = true
				break
			}
		}
	}
	cancel()
	<-done
}

func TestSweepTrail_Negative_RefreshFailsWithoutServerSkipsSweep(t *testing.T) {
	trailPath := fmt.Sprintf("/channels/%d/messages", modTrailID)
	client, _, seen := recordingRest(t, map[string]string{
		trailPath: "[]",
	})
	mod, err := NewModerationHandler(ModerationConfig{
		Definition: modTestDefinition(), GuildID: modGuildID, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sweepTrail(ctx, client.Rest, BotConfig{GuildID: modGuildID, Moderation: mod, Logger: slog.New(slog.DiscardHandler)},
			clientOptions{trailSweepEvery: 5 * time.Millisecond})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	for _, r := range seen() {
		if strings.HasPrefix(r, "GET "+trailPath+"?") {
			t.Errorf("sweep ran on unresolved server: %v", r)
		}
	}
}

func TestSweepTrail_Boundary_LogsClassifiedErrorOnSweepAndReadFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client, _, _ := recordingRest(t, map[string]string{})
	mod, err := NewModerationHandler(ModerationConfig{
		Definition: modTestDefinition(), GuildID: modGuildID, Logger: logger,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sweepTrail(ctx, client.Rest, BotConfig{GuildID: modGuildID, Moderation: mod, Logger: logger},
			clientOptions{trailSweepEvery: time.Millisecond})
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	logOutput := buf.String()
	if !strings.Contains(logOutput, `"error_class"`) {
		t.Errorf("log output missing error_class: %s", logOutput)
	}
	if strings.Contains(logOutput, `"error":`) {
		t.Errorf("log output contains raw error: %s", logOutput)
	}
}
