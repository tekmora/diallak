package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/gateway"

	"github.com/tekmora/diallak/internal/answer"
)

// The tests of HandleAsk use a fake interaction. The tests in this file start
// the bot as Run does and hand its client INTERACTION_CREATE frames as the
// gateway delivers them: decoded by the library's gateway.Message and passed to
// the client's event manager. From there the bot's own listener, the handler
// and the library's REST client answer, against a fake Discord. The websocket
// itself is the library's and is not opened.

const (
	gatewayAppID    uint64 = 710000000000000001
	gatewayGuildID  uint64 = 720000000000000001
	gatewayMemberID uint64 = 730000000000000001
)

// gatewayCall is one request the fake Discord received.
type gatewayCall struct {
	method, path string
	body         map[string]any
}

// gatewayFake is the REST side of Discord for a running bot: the application
// lookup, the interaction callback and the edit of the original response.
type gatewayFake struct {
	t       *testing.T
	mu      sync.Mutex
	calls   []gatewayCall
	replies chan struct{}
}

func (f *gatewayFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}
	call := gatewayCall{method: r.Method, path: r.URL.Path}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &call.body); err != nil {
			f.t.Errorf("%s %s: body is no JSON object: %v", r.Method, r.URL.Path, err)
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()

	if err := f.respond(w, r); err != nil {
		f.t.Errorf("write response: %v", err)
	}
}

// respond answers one request as Discord would, or records it as unexpected.
func (f *gatewayFake) respond(w http.ResponseWriter, r *http.Request) error {
	var err error
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/applications/@me":
		_, err = fmt.Fprintf(w, `{"id":"%d","name":"diallak"}`, gatewayAppID)
	case r.Method == http.MethodGet && (r.URL.Path == "/gateway" || r.URL.Path == "/gateway/bot"):
		_, err = fmt.Fprint(w, `{"url":"ws://127.0.0.1:0","shards":1}`)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/interactions/"):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/messages/@original"):
		_, err = fmt.Fprintf(w, `{"id":"1","channel_id":"2","type":0,"content":"","timestamp":"2026-10-13T00:00:00Z",`+
			`"author":{"id":"%d","username":"diallak"}}`, gatewayAppID)
		f.replies <- struct{}{}
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

func (f *gatewayFake) recorded() []gatewayCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]gatewayCall(nil), f.calls...)
}

// interactionFrame is the gateway frame of one /<command> interaction with the
// given question, sent in the given guild.
func interactionFrame(t *testing.T, seq int, id, guildID uint64, command, question string) []byte {
	t.Helper()
	frame := map[string]any{
		"op": 0, "s": seq, "t": "INTERACTION_CREATE",
		"d": map[string]any{
			"id": fmt.Sprint(id), "application_id": fmt.Sprint(gatewayAppID), "type": 2,
			"token": fmt.Sprintf("interaction-token-%d", id), "version": 1,
			"guild_id": fmt.Sprint(guildID), "channel_id": "740000000000000001",
			"locale": "en-US", "guild_locale": "en-US", "app_permissions": "0",
			"member": map[string]any{
				"user":  map[string]any{"id": fmt.Sprint(gatewayMemberID), "username": "member"},
				"roles": []string{}, "joined_at": "2026-01-01T00:00:00Z", "permissions": "0",
			},
			"data": map[string]any{
				"id": "750000000000000001", "name": command, "type": 1,
				"options": []map[string]any{{"name": OptionQuestion, "type": 3, "value": question}},
			},
		},
	}
	raw, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return raw
}

// runOverGateway starts the bot, delivers the frames once its gateway would be
// open, waits for wantReplies edits of an original response and stops the bot.
func runOverGateway(t *testing.T, asker *fakeAsker, wantReplies int, frames ...[]byte) []gatewayCall {
	t.Helper()
	fake := &gatewayFake{t: t, replies: make(chan struct{}, len(frames)+1)}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	logger := slog.New(slog.DiscardHandler)
	handler, err := NewHandler(HandlerConfig{
		Asker: asker, GuildID: gatewayGuildID, MaxConcurrent: 2,
		AskTimeout: 5 * time.Second, ReplyTimeout: 5 * time.Second, Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	opts := clientOptions{
		restURL: srv.URL,
		openGateway: func(_ context.Context, client *bot.Client, _ string) error {
			for _, raw := range frames {
				var msg gateway.Message
				if err := json.Unmarshal(raw, &msg); err != nil {
					return fmt.Errorf("decode frame: %w", err)
				}
				ev, ok := msg.D.(gateway.EventData)
				if !ok {
					return fmt.Errorf("frame %s holds %T, no event", msg.T, msg.D)
				}
				client.EventManager.HandleGatewayEvent(client.Gateway, msg.T, msg.S, ev)
			}
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	cfg := BotConfig{Token: makeFakeToken(gatewayAppID), GuildID: gatewayGuildID, Handler: handler, Logger: logger}
	go func() { done <- runWithOptions(ctx, cfg, opts) }()

	for range wantReplies {
		select {
		case <-fake.replies:
		case err := <-done:
			t.Fatalf("Run ended before the replies: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatalf("no reply within 10s; requests: %+v", fake.recorded())
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	return fake.recorded()
}

// interactionCalls returns the requests that answer the interaction with the
// given id: its callback and the edit of its original response.
func interactionCalls(calls []gatewayCall, id uint64) []gatewayCall {
	token := fmt.Sprintf("interaction-token-%d", id)
	var out []gatewayCall
	for _, c := range calls {
		if strings.Contains(c.path, token) {
			out = append(out, c)
		}
	}
	return out
}

// assertDeferredReply checks that the interaction was deferred and then answered
// with text that contains want.
func assertDeferredReply(t *testing.T, calls []gatewayCall, id uint64, want string) {
	t.Helper()
	got := interactionCalls(calls, id)
	if len(got) != 2 {
		t.Fatalf("interaction %d: %d requests, want a callback and an edit: %+v", id, len(got), got)
	}
	callback := fmt.Sprintf("/interactions/%d/interaction-token-%d/callback", id, id)
	if got[0].method != http.MethodPost || got[0].path != callback || got[0].body["type"] != float64(5) {
		t.Errorf("interaction %d: first request %s %s %v, want POST %s of type 5 (deferred)",
			id, got[0].method, got[0].path, got[0].body, callback)
	}
	edit := fmt.Sprintf("/webhooks/%d/interaction-token-%d/messages/@original", gatewayAppID, id)
	content, _ := got[1].body["content"].(string)
	if got[1].method != http.MethodPatch || got[1].path != edit || !strings.Contains(content, want) {
		t.Errorf("interaction %d: second request %s %s with content %q, want PATCH %s containing %q",
			id, got[1].method, got[1].path, content, edit, want)
	}
}

func TestRun_Positive_GatewayInteractionIsAnsweredOverREST(t *testing.T) {
	asker := &fakeAsker{}
	calls := runOverGateway(t, asker, 1,
		interactionFrame(t, 1, 760000000000000001, gatewayGuildID, CommandName, "How do I install the engine?"))

	if asker.Calls() != 1 {
		t.Errorf("asker called %d times, want 1", asker.Calls())
	}
	assertDeferredReply(t, calls, 760000000000000001, "Answer to: How do I install the engine?")
}

func TestRun_Negative_OtherCommandIgnoredAndOtherGuildRefused(t *testing.T) {
	asker := &fakeAsker{}
	calls := runOverGateway(t, asker, 1,
		interactionFrame(t, 1, 760000000000000002, gatewayGuildID, "other", "How do I install the engine?"),
		interactionFrame(t, 2, 760000000000000003, gatewayGuildID+1, CommandName, "How do I install the engine?"))

	if asker.Calls() != 0 {
		t.Errorf("asker called %d times, want 0", asker.Calls())
	}
	if got := interactionCalls(calls, 760000000000000002); len(got) != 0 {
		t.Errorf("another command got %d requests, want none: %+v", len(got), got)
	}
	assertDeferredReply(t, calls, 760000000000000003, TextWrongGuild)
}

func TestRun_Boundary_LongestQuestionReachesTheAskerUnchanged(t *testing.T) {
	question := strings.Repeat("ä", answer.MaxQuestionRunes-1) + "?"
	var got string
	asker := &fakeAsker{askFunc: func(_ context.Context, q string) (answer.Answer, error) {
		got = q
		return answer.Answer{Kind: answer.KindAnswer, Text: "Answer to the longest question"}, nil
	}}
	calls := runOverGateway(t, asker, 1,
		interactionFrame(t, 1, 760000000000000004, gatewayGuildID, CommandName, question))

	if got != question {
		t.Errorf("asker got %d runes, want the %d-rune question unchanged", len([]rune(got)), len([]rune(question)))
	}
	assertDeferredReply(t, calls, 760000000000000004, "Answer to the longest question")
}
