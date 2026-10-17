package discord

import (
	"context"
	"encoding/json"
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
	"github.com/disgoorg/disgo/gateway"
)

// The tests in this file start the bot as Run does, with the areas command, and hand its client
// INTERACTION_CREATE frames as in run_gateway_test.go. The fake Discord serves the server's roles,
// the role calls on the member, the interaction callback, the edit of the original response and the
// direct message of a receipt.

// areasCall is one request the fake Discord received, with the audit log reason it carried.
type areasCall struct {
	method, path, reason string
	body                 areasBody
	raw                  string
}

// clientObserver is a componentsFake that looks at the client once the frames are delivered.
type clientObserver interface {
	observe(client *bot.Client)
}

// componentsFake is a fake Discord for runComponentsOverGateway: it serves the requests, signals
// every answer the test waits for and returns the requests it received.
type componentsFake interface {
	http.Handler
	answered() <-chan struct{}
	recorded() []areasCall
}

// areasBody is the body of an interaction callback or of the edit of a message.
type areasBody struct {
	Type int          `json:"type"`
	Data areasMessage `json:"data"`
	areasMessage
}

type areasMessage struct {
	Content    string `json:"content"`
	Flags      int    `json:"flags"`
	Components []struct {
		Components []struct {
			CustomID string `json:"custom_id"`
			Options  []struct {
				Default bool `json:"default"`
			} `json:"options"`
		} `json:"components"`
	} `json:"components"`
}

// selected counts the options of every menu of m that are selected.
func (m areasMessage) selected() int {
	n := 0
	for _, row := range m.Components {
		for _, menu := range row.Components {
			for _, o := range menu.Options {
				if o.Default {
					n++
				}
			}
		}
	}
	return n
}

type areasFake struct {
	t       *testing.T
	mu      sync.Mutex
	calls   []areasCall
	answers chan struct{}
	// directClosed refuses a direct message as Discord does when the member's direct messages
	// are closed.
	directClosed bool
}

func (f *areasFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}
	reason, err := url.QueryUnescape(r.Header.Get("X-Audit-Log-Reason"))
	if err != nil {
		f.t.Errorf("audit log reason %q does not decode: %v", r.Header.Get("X-Audit-Log-Reason"), err)
	}
	call := areasCall{method: r.Method, path: r.URL.Path, reason: reason, raw: string(raw)}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &call.body); err != nil {
			f.t.Errorf("%s %s: body does not decode: %v", r.Method, r.URL.Path, err)
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
func (f *areasFake) respond(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		return f.respondRead(w, r)
	}
	memberRoles := fmt.Sprintf("/guilds/%d/members/%d/roles/", areasGuildID, areasMemberID)
	var err error
	switch {
	case (r.Method == http.MethodPut || r.Method == http.MethodDelete) && strings.HasPrefix(r.URL.Path, memberRoles):
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost:
		err = f.respondPost(w, r)
	case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/messages/@original"):
		err = writeFakeMessage(w)
		f.answers <- struct{}{}
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

// respondPost answers the interaction callbacks and the direct message of a receipt.
func (f *areasFake) respondPost(w http.ResponseWriter, r *http.Request) error {
	switch {
	case strings.HasPrefix(r.URL.Path, "/interactions/"):
		w.WriteHeader(http.StatusNoContent)
		f.answers <- struct{}{}
		return nil
	case r.URL.Path == "/users/@me/channels":
		_, err := fmt.Fprintf(w, `{"id":"%d","type":1}`, receiptDirectChannelID)
		return err
	case r.URL.Path == fmt.Sprintf("/channels/%d/messages", receiptDirectChannelID):
		return f.respondDirect(w)
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		return nil
	}
}

// respondDirect answers a direct message, or refuses it with code 50007 when directClosed is set.
func (f *areasFake) respondDirect(w http.ResponseWriter) error {
	if f.directClosed {
		w.WriteHeader(http.StatusForbidden)
		_, err := fmt.Fprint(w, `{"code":50007,"message":"Cannot send messages to this user"}`)
		return err
	}
	return writeFakeMessage(w)
}

func writeFakeMessage(w http.ResponseWriter) error {
	_, err := fmt.Fprintf(w, `{"id":"1","channel_id":"2","type":0,"content":"","timestamp":"2026-10-17T00:00:00Z",`+
		`"author":{"id":"%d","username":"diallak"}}`, gatewayAppID)
	return err
}

// respondRead answers the reads: the application, the gateway and the server's roles.
func (f *areasFake) respondRead(w http.ResponseWriter, r *http.Request) error {
	switch r.URL.Path {
	case "/applications/@me":
		_, err := fmt.Fprintf(w, `{"id":"%d","name":"diallak"}`, gatewayAppID)
		return err
	case "/gateway", "/gateway/bot":
		_, err := fmt.Fprint(w, `{"url":"ws://127.0.0.1:0","shards":1}`)
		return err
	case fmt.Sprintf("/guilds/%d/roles", areasGuildID):
		raw, err := json.Marshal(areasTestRoles())
		if err != nil {
			return err
		}
		_, err = w.Write(raw)
		return err
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		return nil
	}
}

func (f *areasFake) answered() <-chan struct{} { return f.answers }

func (f *areasFake) recorded() []areasCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]areasCall(nil), f.calls...)
}

func areasMember(roles []uint64) map[string]any {
	ids := make([]string, len(roles))
	for i, r := range roles {
		ids[i] = fmt.Sprint(r)
	}
	return map[string]any{
		"user":  map[string]any{"id": fmt.Sprint(areasMemberID), "username": "member"},
		"roles": ids, "joined_at": "2026-01-01T00:00:00Z", "permissions": "0", "pending": false,
	}
}

func areasFrame(t *testing.T, seq int, id uint64, typ int, roles []uint64, data map[string]any, extra map[string]any) []byte {
	t.Helper()
	d := map[string]any{
		"id": fmt.Sprint(id), "application_id": fmt.Sprint(gatewayAppID), "type": typ,
		"token": fmt.Sprintf("interaction-token-%d", id), "version": 1,
		"guild_id": fmt.Sprint(areasGuildID), "channel_id": "740000000000000001",
		"locale": "en-US", "guild_locale": "en-US", "app_permissions": "0",
		"member": areasMember(roles), "data": data,
	}
	for k, v := range extra {
		d[k] = v
	}
	raw, err := json.Marshal(map[string]any{"op": 0, "s": seq, "t": "INTERACTION_CREATE", "d": d})
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return raw
}

// areasCommandFrame is the frame of /areas.
func areasCommandFrame(t *testing.T, seq int, id uint64, roles ...uint64) []byte {
	t.Helper()
	data := map[string]any{"id": "750000000000000002", "name": AreasCommandName, "type": 1}
	return areasFrame(t, seq, id, 2, roles, data, nil)
}

// areasChangeFrame is the frame of a change of the menu customID to values.
func areasChangeFrame(t *testing.T, seq int, id uint64, customID string, values []string, roles ...uint64) []byte {
	t.Helper()
	data := map[string]any{"custom_id": customID, "component_type": 3, "values": values}
	message := map[string]any{
		"id": "760000000000000001", "channel_id": "740000000000000001", "type": 0, "content": TextAreasIntro,
		"timestamp": "2026-10-17T00:00:00Z", "flags": 64,
		"author": map[string]any{"id": fmt.Sprint(gatewayAppID), "username": "diallak"},
	}
	return areasFrame(t, seq, id, 3, roles, data, map[string]any{"message": message})
}

// runAreasOverGateway starts the bot with the areas command, delivers the frames, waits for
// wantAnswers callbacks and edits and stops the bot.
func runAreasOverGateway(t *testing.T, wantAnswers int, frames ...[]byte) []areasCall {
	t.Helper()
	fake := &areasFake{t: t, answers: make(chan struct{}, 2*len(frames)+1)}
	areas, err := NewAreasHandler(AreasConfig{
		Definition: areasTestDefinition(), GuildID: areasGuildID, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("NewAreasHandler: %v", err)
	}
	return runComponentsOverGateway(t, fake, BotConfig{Areas: areas}, wantAnswers, frames...)
}

// runComponentsOverGateway starts the bot as cfg sets it against fake, delivers the frames, waits for
// wantAnswers callbacks and edits and stops the bot. It fills the token, the server and the logger,
// and builds a handler without receipts when cfg.Handler is nil.
func runComponentsOverGateway(t *testing.T, fake componentsFake, cfg BotConfig, wantAnswers int, frames ...[]byte) []areasCall {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	logger := slog.New(slog.DiscardHandler)
	if cfg.Handler == nil {
		handler, err := NewHandler(HandlerConfig{
			Asker: &fakeAsker{}, GuildID: areasGuildID, MaxConcurrent: 2,
			AskTimeout: 5 * time.Second, ReplyTimeout: 5 * time.Second, Logger: logger,
		})
		if err != nil {
			t.Fatalf("NewHandler: %v", err)
		}
		cfg.Handler = handler
	}
	cfg.Token, cfg.GuildID, cfg.Logger = makeFakeToken(gatewayAppID), areasGuildID, logger
	opts := clientOptions{
		restURL: srv.URL,
		openGateway: func(_ context.Context, client *bot.Client, _ string) error {
			if err := deliverFrames(client, frames); err != nil {
				return err
			}
			if o, ok := fake.(clientObserver); ok {
				o.observe(client)
			}
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runWithOptions(ctx, cfg, opts) }()

	for range wantAnswers {
		select {
		case <-fake.answered():
		case err := <-done:
			t.Fatalf("Run ended before the answers: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatalf("no answer within 10s; requests: %+v", fake.recorded())
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	return fake.recorded()
}

// deliverFrames hands each gateway frame to the event manager of client, in order.
func deliverFrames(client *bot.Client, frames [][]byte) error {
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
}

// areasInteractionCalls returns the answers of one interaction in order, with the role calls on
// the member when withRoleCalls is set.
func areasInteractionCalls(calls []areasCall, id uint64, withRoleCalls bool) []areasCall {
	token := fmt.Sprintf("interaction-token-%d", id)
	var out []areasCall
	for _, c := range calls {
		if strings.Contains(c.path, token) || (withRoleCalls && strings.Contains(c.path, "/members/")) {
			out = append(out, c)
		}
	}
	return out
}

// assertRolesReadBeforeAnyAnswer checks that the bot read the server's roles before it answered
// an interaction, so the first answer needs no call.
func assertRolesReadBeforeAnyAnswer(t *testing.T, calls []areasCall) {
	t.Helper()
	rolesPath := fmt.Sprintf("/guilds/%d/roles", areasGuildID)
	for i, c := range calls {
		if c.method == http.MethodGet && c.path == rolesPath {
			return
		}
		if strings.HasPrefix(c.path, "/interactions/") {
			t.Errorf("request %d answers an interaction before the roles were read", i)
			return
		}
	}
	t.Error("the roles were never read")
}

func requestLines(calls []areasCall) string {
	lines := make([]string, len(calls))
	for i, c := range calls {
		lines[i] = c.method + " " + c.path
	}
	return strings.Join(lines, "\n")
}

func TestRunAreas_Positive_CommandShowsMenusAndChangeGivesAndTakesRoles(t *testing.T) {
	const cmdID, changeID uint64 = 860000000000000001, 860000000000000002
	calls := runAreasOverGateway(t, 3,
		areasCommandFrame(t, 1, cmdID, roleHomeLab),
		areasChangeFrame(t, 2, changeID, "areas:interests", []string{"interest-media"}, roleHomeLab))

	assertRolesReadBeforeAnyAnswer(t, calls)
	cmd := areasInteractionCalls(calls, cmdID, false)
	if len(cmd) != 1 {
		t.Fatalf("command requests = %+v, want one answer", cmd)
	}
	if b := cmd[0].body; b.Type != 4 || b.Data.Flags != 64 || len(b.Data.Components) != 2 || b.Data.selected() != 1 {
		t.Errorf("command answer = %+v, want a message only the member sees with two menus, Home lab selected", b)
	}

	change := areasInteractionCalls(calls, changeID, true)
	want := strings.Join([]string{
		fmt.Sprintf("POST /interactions/%d/interaction-token-%d/callback", changeID, changeID),
		fmt.Sprintf("DELETE /guilds/%d/members/%d/roles/%d", areasGuildID, areasMemberID, roleHomeLab),
		fmt.Sprintf("PUT /guilds/%d/members/%d/roles/%d", areasGuildID, areasMemberID, roleMedia),
		fmt.Sprintf("PATCH /webhooks/%d/interaction-token-%d/messages/@original", gatewayAppID, changeID),
	}, "\n")
	if got := requestLines(change); got != want {
		t.Fatalf("change requests:\n%s\nwant:\n%s", got, want)
	}
	if change[0].body.Type != 6 {
		t.Errorf("change callback type = %d, want 6 (deferred update)", change[0].body.Type)
	}
	for _, c := range change[1:3] {
		if c.reason != areasAuditReason {
			t.Errorf("%s %s carried the audit reason %q, want %q", c.method, c.path, c.reason, areasAuditReason)
		}
	}
	if got := change[3].body.Content; got != "Given: Media.\nTaken: Home lab." {
		t.Errorf("edit content = %q", got)
	}
}

func TestRunAreas_Negative_RoleOutsideTheChoicesIsRefusedWithoutARoleCall(t *testing.T) {
	const id uint64 = 860000000000000003
	calls := runAreasOverGateway(t, 1,
		areasChangeFrame(t, 1, id, "areas:interests", []string{"staff"}))

	got := areasInteractionCalls(calls, id, true)
	if len(got) != 1 || got[0].body.Type != 7 || got[0].body.Data.Content != TextAreasRefused {
		t.Errorf("requests:\n%s\nwant one update of the message with the refusal and no role call", requestLines(got))
	}
}

func TestRunAreas_Boundary_MemberHoldingEveryOptionKeepsThemWithoutARoleCall(t *testing.T) {
	const id uint64 = 860000000000000004
	calls := runAreasOverGateway(t, 2,
		areasChangeFrame(t, 1, id, "areas:interests", []string{"interest-home-lab", "interest-media"},
			roleHomeLab, roleMedia, roleGerman))

	got := areasInteractionCalls(calls, id, true)
	if len(got) != 2 || got[0].body.Type != 6 || got[1].method != http.MethodPatch {
		t.Fatalf("requests:\n%s\nwant an acknowledgement and an edit, no role call", requestLines(got))
	}
	if got[1].body.Content != textAreasNoChange {
		t.Errorf("edit content = %q, want %q", got[1].body.Content, textAreasNoChange)
	}
	if n := got[1].body.selected(); n != 3 {
		t.Errorf("%d options selected in the edit, want all three held roles", n)
	}
}
