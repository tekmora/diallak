package discord

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
)

// The tests in this file start the bot as Run does, with the moderation commands, and hand its
// client INTERACTION_CREATE frames as areas_gateway_test.go does. The fake Discord serves the
// application with its bot user, the server's roles and channels, the trail, the acts and the
// interaction answers.

const (
	modGatewayMessageID uint64 = 852000000000000001
	modGatewayOldLineID uint64 = 853000000000000001
	modGatewayNewLineID uint64 = 853000000000000002
)

type moderationFake struct {
	t       *testing.T
	mu      sync.Mutex
	calls   []areasCall
	answers chan struct{}
	// refuse answers 403 to every act, as Discord does for the owner or an Administrator.
	refuse bool
	// sweepSignals signals the deletion of a trail line as an answer.
	sweepSignals bool
}

func newModerationFake(t *testing.T, frames int) *moderationFake {
	return &moderationFake{t: t, answers: make(chan struct{}, 2*frames+4)}
}

func (f *moderationFake) answered() <-chan struct{} { return f.answers }

func (f *moderationFake) recorded() []areasCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]areasCall(nil), f.calls...)
}

func (f *moderationFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}
	reason, err := url.QueryUnescape(r.Header.Get("X-Audit-Log-Reason"))
	if err != nil {
		f.t.Errorf("audit log reason does not decode: %v", err)
	}
	f.mu.Lock()
	f.calls = append(f.calls, areasCall{method: r.Method, path: r.URL.Path + "?" + r.URL.RawQuery, reason: reason, raw: string(raw)})
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		f.respondRead(w, r)
		return
	}
	f.respondWrite(w, r)
}

func (f *moderationFake) respondRead(w http.ResponseWriter, r *http.Request) {
	var body string
	switch r.URL.Path {
	case "/applications/@me":
		body = fmt.Sprintf(`{"id":"%d","name":"diallak","bot":{"id":"%d","username":"diallak"}}`, gatewayAppID, modBotUserID)
	case "/gateway", "/gateway/bot":
		body = `{"url":"ws://127.0.0.1:0","shards":1}`
	case fmt.Sprintf("/guilds/%d/roles", areasGuildID):
		body = fmt.Sprintf(`[{"id":"%d","name":"@everyone","permissions":"0"},{"id":"%d","name":"Moderator","permissions":"0"}]`,
			areasGuildID, modRoleID)
	case fmt.Sprintf("/guilds/%d/channels", areasGuildID):
		body = fmt.Sprintf(`[{"id":"%d","guild_id":"%d","type":0,"name":"general"},{"id":"%d","guild_id":"%d","type":0,"name":"mod-log"}]`,
			modChannelID, areasGuildID, modTrailID, areasGuildID)
	case fmt.Sprintf("/channels/%d/messages", modTrailID):
		body = fmt.Sprintf(`[%s,%s]`, trailLineJSON(modGatewayNewLineID, modNow.Add(-time.Hour)),
			trailLineJSON(modGatewayOldLineID, modNow.Add(-DefaultTrailRetention-time.Hour)))
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if _, err := fmt.Fprint(w, body); err != nil {
		f.t.Errorf("write response: %v", err)
	}
}

func trailLineJSON(id uint64, sent time.Time) string {
	return fmt.Sprintf(`{"id":"%d","channel_id":"%d","type":0,"content":"line","timestamp":%q,"author":{"id":"%d","username":"diallak"}}`,
		id, modTrailID, sent.Format(time.RFC3339), modBotUserID)
}

func (f *moderationFake) respondWrite(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	isAct := (r.Method == http.MethodDelete && path == fmt.Sprintf("/channels/%d/messages/%d", modChannelID, modGatewayMessageID)) ||
		(r.Method == http.MethodPatch && path == fmt.Sprintf("/guilds/%d/members/%d", areasGuildID, modMemberID))
	var err error
	switch {
	case isAct && f.refuse:
		w.WriteHeader(http.StatusForbidden)
		_, err = fmt.Fprint(w, `{"code":50013,"message":"Missing Permissions"}`)
	case isAct && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case isAct:
		_, err = fmt.Fprintf(w, `{"user":{"id":"%d","username":"member"},"roles":[],"joined_at":"2026-01-01T00:00:00Z"}`, modMemberID)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, fmt.Sprintf("/channels/%d/messages/", modTrailID)):
		w.WriteHeader(http.StatusNoContent)
		if f.sweepSignals {
			f.answers <- struct{}{}
		}
	case r.Method == http.MethodPost && path == fmt.Sprintf("/channels/%d/messages", modTrailID):
		err = writeFakeMessage(w)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/interactions/"):
		w.WriteHeader(http.StatusNoContent)
		f.answers <- struct{}{}
	case r.Method == http.MethodPatch && strings.HasSuffix(path, "/messages/@original"):
		err = writeFakeMessage(w)
		f.answers <- struct{}{}
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, path)
		w.WriteHeader(http.StatusNotFound)
	}
	if err != nil {
		f.t.Errorf("write response: %v", err)
	}
}

func runModerationOverGateway(t *testing.T, fake *moderationFake, wantAnswers int, frames ...[]byte) []areasCall {
	t.Helper()
	mod, err := NewModerationHandler(ModerationConfig{
		Definition: modTestDefinition(), GuildID: areasGuildID, Logger: slog.New(slog.DiscardHandler),
		Now: func() time.Time { return modNow },
	})
	if err != nil {
		t.Fatalf("NewModerationHandler: %v", err)
	}
	return runComponentsOverGateway(t, fake, BotConfig{Moderation: mod}, wantAnswers, frames...)
}

func removeMessageFrame(t *testing.T, seq int, id uint64, roles ...uint64) []byte {
	t.Helper()
	target := fmt.Sprint(modGatewayMessageID)
	data := map[string]any{
		"id": "750000000000000003", "name": RemoveMessageCommandName, "type": 3, "target_id": target,
		"resolved": map[string]any{"messages": map[string]any{target: map[string]any{
			"id": target, "channel_id": fmt.Sprint(modChannelID), "type": 0, "content": "spam",
			"timestamp": "2026-10-17T00:00:00Z", "author": map[string]any{"id": fmt.Sprint(modMemberID), "username": "member"},
		}}},
	}
	return areasFrame(t, seq, id, 2, roles, data, nil)
}

func removeFormFrame(t *testing.T, seq int, id uint64, customID, reason string, roles ...uint64) []byte {
	t.Helper()
	data := map[string]any{"custom_id": customID, "components": []any{map[string]any{
		"type": 18, "id": 1, "component": map[string]any{"type": 4, "id": 2, "custom_id": removeReasonInputID, "value": reason},
	}}}
	return areasFrame(t, seq, id, 5, roles, data, nil)
}

func timeoutFrame(t *testing.T, seq int, id uint64, minutes int, roles ...uint64) []byte {
	t.Helper()
	data := map[string]any{"id": "750000000000000004", "name": TimeoutCommandName, "type": 1, "options": []any{
		map[string]any{"name": OptionTimeoutMember, "type": 6, "value": fmt.Sprint(modMemberID)},
		map[string]any{"name": OptionTimeoutMinutes, "type": 4, "value": minutes},
		map[string]any{"name": OptionTimeoutReason, "type": 3, "value": "Insults"},
	}}
	return areasFrame(t, seq, id, 2, roles, data, nil)
}

func gatewayRemoveCustomID() string {
	return fmt.Sprintf("moderation-remove:%d:%d:%d", modChannelID, modGatewayMessageID, modMemberID)
}

// callsTo returns the requests of one interaction and the acts and lines, in order.
func callsTo(calls []areasCall, id uint64) []areasCall {
	token := fmt.Sprintf("interaction-token-%d", id)
	var out []areasCall
	for _, c := range calls {
		trailRead := strings.Contains(c.path, fmt.Sprintf("/channels/%d/", modTrailID)) && c.method != http.MethodPost
		if strings.Contains(c.path, token) || (c.method != http.MethodGet && !strings.Contains(c.path, "interaction-token") && !trailRead) {
			out = append(out, c)
		}
	}
	return out
}

func TestModerationGateway_Positive_RemoveMessageAsksForAReasonThenActs(t *testing.T) {
	fake := newModerationFake(t, 2)
	calls := runModerationOverGateway(t, fake, 3,
		removeMessageFrame(t, 1, 900000000000000001, modRoleID),
		removeFormFrame(t, 2, 900000000000000002, gatewayRemoveCustomID(), "Spam link", modRoleID))

	form := callsTo(calls, 900000000000000001)
	if len(form) == 0 || !strings.Contains(form[0].raw, `"type":9`) ||
		!strings.Contains(form[0].raw, gatewayRemoveCustomID()) || !strings.Contains(form[0].raw, `"max_length":512`) {
		t.Fatalf("first answer = %+v, want the reason form of the message", form)
	}
	got := callsTo(calls, 900000000000000002)
	if len(got) != 4 {
		t.Fatalf("requests = %+v, want defer, delete, line, edit", got)
	}
	if !strings.Contains(got[0].raw, `"type":5`) || !strings.Contains(got[0].raw, `"flags":64`) {
		t.Errorf("first request = %s, want an ephemeral deferred answer", got[0].raw)
	}
	if got[1].method != http.MethodDelete || got[1].reason != "Spam link" {
		t.Errorf("second request = %s %s with reason %q, want the delete with the reason", got[1].method, got[1].path, got[1].reason)
	}
	if !strings.Contains(got[2].raw, "removed message") || !strings.Contains(got[2].raw, `"allowed_mentions":{"parse":[]`) {
		t.Errorf("third request = %s, want the trail line mentioning no one", got[2].raw)
	}
	if !strings.Contains(got[3].raw, TextModerationRemoved) {
		t.Errorf("fourth request = %s, want %q", got[3].raw, TextModerationRemoved)
	}
}

func TestModerationGateway_Positive_TimeoutSetsTheEndAndWritesTheLine(t *testing.T) {
	fake := newModerationFake(t, 1)
	calls := runModerationOverGateway(t, fake, 2, timeoutFrame(t, 1, 900000000000000003, 90, modRoleID))

	got := callsTo(calls, 900000000000000003)
	if len(got) != 4 || got[1].method != http.MethodPatch {
		t.Fatalf("requests = %+v, want defer, member update, line, edit", got)
	}
	if !strings.Contains(got[1].raw, `"communication_disabled_until":"2026-10-17T13:30:00Z"`) || got[1].reason != "Insults" {
		t.Errorf("member update = %s with reason %q, want the end 90 minutes on and the reason", got[1].raw, got[1].reason)
	}
	if !strings.Contains(got[2].raw, "timed out until 2026-10-17T13:30:00Z") {
		t.Errorf("line = %s, want the end of the timeout", got[2].raw)
	}
}

func TestModerationGateway_Negative_RefusedActWritesNoLine(t *testing.T) {
	fake := newModerationFake(t, 1)
	fake.refuse = true
	calls := runModerationOverGateway(t, fake, 2, timeoutFrame(t, 1, 900000000000000004, 90, modRoleID))

	got := callsTo(calls, 900000000000000004)
	if len(got) != 3 || !strings.Contains(got[2].raw, "Discord refused this act") {
		t.Errorf("requests = %+v, want defer, member update, the refusal and no line", got)
	}
}

func TestModerationGateway_Negative_MemberWithoutTheRoleIsRefused(t *testing.T) {
	fake := newModerationFake(t, 1)
	calls := runModerationOverGateway(t, fake, 1, timeoutFrame(t, 1, 900000000000000005, 90))

	got := callsTo(calls, 900000000000000005)
	if len(got) != 1 || !strings.Contains(got[0].raw, TextModerationNotModerator) || !strings.Contains(got[0].raw, `"flags":64`) {
		t.Errorf("requests = %+v, want only the ephemeral refusal", got)
	}
}

func TestModerationGateway_Positive_StartSweepsTheTrail(t *testing.T) {
	fake := newModerationFake(t, 0)
	fake.sweepSignals = true
	calls := runModerationOverGateway(t, fake, 1)

	var deleted []string
	for _, c := range calls {
		if c.method == http.MethodDelete {
			deleted = append(deleted, c.path)
		}
		if c.method == http.MethodGet && strings.HasPrefix(c.path, fmt.Sprintf("/channels/%d/messages", modTrailID)) &&
			!strings.Contains(c.path, "limit=100") {
			t.Errorf("trail read %s, want pages of 100", c.path)
		}
	}
	want := fmt.Sprintf("/channels/%d/messages/%d?", modTrailID, modGatewayOldLineID)
	if len(deleted) != 1 || deleted[0] != want {
		t.Errorf("deleted %q, want only the old line %q", deleted, want)
	}
}

// The areas fake answers the application lookup without a bot user: the start fails before the
// gateway opens.
func TestModerationGateway_Negative_NoBotUserFailsTheStart(t *testing.T) {
	mod, err := NewModerationHandler(ModerationConfig{Definition: modTestDefinition(), GuildID: areasGuildID})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(HandlerConfig{
		Asker: &fakeAsker{}, GuildID: areasGuildID, MaxConcurrent: 2,
		AskTimeout: 5 * time.Second, ReplyTimeout: 5 * time.Second, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(&areasFake{t: t, answers: make(chan struct{}, 1)})
	defer srv.Close()
	opened := false
	// A deadline keeps a start that wrongly goes on from blocking until the test binary times out.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err = runWithOptions(ctx, BotConfig{
		Token: makeFakeToken(gatewayAppID), GuildID: areasGuildID, Handler: handler, Moderation: mod,
		Logger: slog.New(slog.DiscardHandler),
	}, clientOptions{restURL: srv.URL, openGateway: func(context.Context, *bot.Client, string) error {
		opened = true
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "no bot user") || opened {
		t.Errorf("Run = %v, gateway opened %v; want the start to fail on the missing bot user", err, opened)
	}
}
