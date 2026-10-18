package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
)

// The tests in this file start the bot as Run does, with the supporter function, and hand its
// client member frames as the gateway delivers them. The fake Discord serves the application, the
// server's roles and the member list, and takes the role changes. Each role change and each page
// of the member list counts as one answer.

const supMemberC uint64 = 840000000000000003

// supporterFake is the REST side of Discord for the supporter function.
type supporterFake struct {
	t       *testing.T
	roles   []byte
	members [][]map[string]any
	mu      sync.Mutex
	calls   []areasCall
	answers chan struct{}
	intents gateway.Intents
	cached  bool
}

func newSupporterFake(t *testing.T, pages ...[]map[string]any) *supporterFake {
	t.Helper()
	roles, err := json.Marshal(supServerRoles(t))
	if err != nil {
		t.Fatalf("marshal roles: %v", err)
	}
	return &supporterFake{t: t, roles: roles, members: pages, answers: make(chan struct{}, 16)}
}

func (f *supporterFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		f.t.Errorf("read body: %v", err)
	}
	reason, err := url.QueryUnescape(r.Header.Get("X-Audit-Log-Reason"))
	if err != nil {
		f.t.Errorf("audit log reason %q does not decode: %v", r.Header.Get("X-Audit-Log-Reason"), err)
	}
	f.mu.Lock()
	f.calls = append(f.calls, areasCall{method: r.Method, path: r.URL.Path, reason: reason, raw: r.URL.RawQuery})
	f.mu.Unlock()
	if err := f.respond(w, r); err != nil {
		f.t.Errorf("write response: %v", err)
	}
}

func (f *supporterFake) respond(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	memberRoles := fmt.Sprintf("/guilds/%d/members/", areasGuildID)
	switch {
	case r.Method == http.MethodGet:
		return f.respondRead(w, r)
	case (r.Method == http.MethodPut || r.Method == http.MethodDelete) && strings.HasPrefix(r.URL.Path, memberRoles):
		w.WriteHeader(http.StatusNoContent)
		f.answers <- struct{}{}
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
	return nil
}

// respondRead answers the reads: the application, the gateway, the roles and the member list.
func (f *supporterFake) respondRead(w http.ResponseWriter, r *http.Request) error {
	var err error
	switch r.URL.Path {
	case "/applications/@me":
		_, err = fmt.Fprintf(w, `{"id":"%d","name":"diallak"}`, gatewayAppID)
	case "/gateway", "/gateway/bot":
		_, err = fmt.Fprint(w, `{"url":"ws://127.0.0.1:0","shards":1}`)
	case fmt.Sprintf("/guilds/%d/roles", areasGuildID):
		_, err = w.Write(f.roles)
	case fmt.Sprintf("/guilds/%d/members", areasGuildID):
		err = f.respondMembers(w)
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

// respondMembers serves the next page of the member list.
func (f *supporterFake) respondMembers(w http.ResponseWriter) error {
	f.mu.Lock()
	page := []map[string]any{}
	if len(f.members) > 0 {
		page, f.members = f.members[0], f.members[1:]
	}
	f.mu.Unlock()
	raw, err := json.Marshal(page)
	if err != nil {
		return err
	}
	_, err = w.Write(raw)
	f.answers <- struct{}{}
	return err
}

func (f *supporterFake) answered() <-chan struct{} { return f.answers }

func (f *supporterFake) recorded() []areasCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]areasCall(nil), f.calls...)
}

func (f *supporterFake) observe(client *bot.Client) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.intents = client.Gateway.Intents()
	_, f.cached = client.Caches.Member(snowflake.ID(areasGuildID), snowflake.ID(supMemberC))
}

// supMember is a member object as the platform sends it.
func supMember(id uint64, roles ...uint64) map[string]any {
	ids := make([]string, len(roles))
	for i, r := range roles {
		ids[i] = fmt.Sprint(r)
	}
	return map[string]any{
		"user":  map[string]any{"id": fmt.Sprint(id), "username": "member"},
		"roles": ids, "joined_at": "2026-01-01T00:00:00Z", "guild_id": fmt.Sprint(areasGuildID),
	}
}

func supMemberFrame(t *testing.T, seq int, event string, id uint64, roles ...uint64) []byte {
	t.Helper()
	return forumFrame(t, seq, event, supMember(id, roles...))
}

// roleChanges returns the role changes as "METHOD member role".
func roleChanges(calls []areasCall) []string {
	var out []string
	prefix := fmt.Sprintf("/guilds/%d/members/", areasGuildID)
	for _, c := range calls {
		if (c.method == http.MethodPut || c.method == http.MethodDelete) && strings.HasPrefix(c.path, prefix) {
			out = append(out, c.method+" "+strings.ReplaceAll(strings.TrimPrefix(c.path, prefix), "/roles/", " "))
		}
	}
	return out
}

func memberListCalls(calls []areasCall) []areasCall {
	var out []areasCall
	for _, c := range calls {
		if c.method == http.MethodGet && c.path == fmt.Sprintf("/guilds/%d/members", areasGuildID) {
			out = append(out, c)
		}
	}
	return out
}

// ADR-0027: switched on, the session asks for the members intent and no other, member events give
// Supporter, the start lists the members once and the member cache stays empty. ADR-0024: a member
// whose grace period ends loses Supporter at the next check, with the fixed audit reason.
func TestRunSupporter_Positive_EventsSweepAndGracePeriod(t *testing.T) {
	// The clock reads the start for the first grace period and its end at every check.
	var reads int
	var mu sync.Mutex
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		reads++
		if reads == 1 {
			return supStart
		}
		return supStart.Add(DefaultSupporterGrace)
	}
	h, err := NewSupporterHandler(SupporterConfig{Definition: supDefinition(t), GuildID: areasGuildID, Now: now})
	if err != nil {
		t.Fatalf("NewSupporterHandler: %v", err)
	}
	fake := newSupporterFake(t, []map[string]any{
		supMember(supMemberA, supRoleSupporter, supRoleHonorary),
		supMember(supMemberB, supRolePatreon),
	})
	frames := [][]byte{supMemberFrame(t, 1, "GUILD_MEMBER_UPDATE", supMemberC, supRoleKoFi)}
	// Answers: C given, the member page, B given, A taken.
	calls := runSupporterOverGateway(t, fake, h, 4, frames...)

	if fake.intents != gateway.IntentGuildMembers {
		t.Errorf("gateway intents = %d, want only the members intent %d", fake.intents, gateway.IntentGuildMembers)
	}
	if fake.cached {
		t.Error("the member is in the client cache, want no member held")
	}
	got := strings.Join(roleChanges(calls), ", ")
	for _, want := range []string{
		fmt.Sprintf("PUT %d %d", supMemberC, supRoleSupporter),
		fmt.Sprintf("PUT %d %d", supMemberB, supRoleSupporter),
		fmt.Sprintf("DELETE %d %d", supMemberA, supRoleSupporter),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("role changes = %s, want %s", got, want)
		}
	}
	if n := len(roleChanges(calls)); n != 3 {
		t.Errorf("role changes = %s, want exactly three", got)
	}
	for _, c := range calls {
		if c.method != http.MethodGet && c.reason != supporterAuditReason {
			t.Errorf("%s %s reason = %q, want %q", c.method, c.path, c.reason, supporterAuditReason)
		}
	}
}

// Boundary: a full page of the member list is followed by the next one, a short page ends the
// sweep.
func TestRunSupporter_Boundary_FullPageReadsTheNextOne(t *testing.T) {
	full := make([]map[string]any, memberPageSize)
	for i := range full {
		full[i] = supMember(supMemberA + uint64(i))
	}
	h, err := NewSupporterHandler(SupporterConfig{Definition: supDefinition(t), GuildID: areasGuildID})
	if err != nil {
		t.Fatalf("NewSupporterHandler: %v", err)
	}
	fake := newSupporterFake(t, full, []map[string]any{supMember(supMemberC, supRoleKoFi)})
	calls := runSupporterOverGateway(t, fake, h, 3)
	pages := memberListCalls(calls)
	if len(pages) != 2 {
		t.Fatalf("member list requests = %d, want 2", len(pages))
	}
	wantAfter := fmt.Sprintf("after=%d", supMemberA+memberPageSize-1)
	if !strings.Contains(pages[0].raw, fmt.Sprintf("limit=%d", memberPageSize)) || !strings.Contains(pages[1].raw, wantAfter) {
		t.Errorf("member list queries = %q, %q; want limit %d and then %s", pages[0].raw, pages[1].raw, memberPageSize, wantAfter)
	}
}

// ADR-0027: switched off, the session asks for no intent, lists no member and handles no member
// event.
func TestRunSupporter_Negative_OffAsksNoIntentAndChangesNothing(t *testing.T) {
	fake := newSupporterFake(t)
	frames := [][]byte{
		supMemberFrame(t, 1, "GUILD_MEMBER_ADD", supMemberC, supRoleKoFi),
		supMemberFrame(t, 2, "GUILD_MEMBER_UPDATE", supMemberC, supRolePatreon),
	}
	observed := make(chan struct{}, 1)
	calls := runComponentsOverGateway(t, &observedFake{supporterFake: fake, done: observed}, BotConfig{}, 1, frames...)
	if fake.intents != gateway.IntentsNone {
		t.Errorf("gateway intents = %d, want none", fake.intents)
	}
	if got := roleChanges(calls); len(got) != 0 {
		t.Errorf("role changes = %v, want none", got)
	}
	if got := memberListCalls(calls); len(got) != 0 {
		t.Errorf("member list requests = %d, want none", len(got))
	}
}

// Member events of another server are ignored.
func TestRunSupporter_Negative_OtherServerIsIgnored(t *testing.T) {
	h, err := NewSupporterHandler(SupporterConfig{Definition: supDefinition(t), GuildID: areasGuildID})
	if err != nil {
		t.Fatalf("NewSupporterHandler: %v", err)
	}
	other := supMember(supMemberC, supRoleKoFi)
	other["guild_id"] = fmt.Sprint(areasGuildID + 1)
	fake := newSupporterFake(t, []map[string]any{supMember(supMemberB, supRolePatreon)})
	// Answers: the member page and B given; the event of the other server gives nothing.
	calls := runSupporterOverGateway(t, fake, h, 2, forumFrame(t, 1, "GUILD_MEMBER_ADD", other))
	if got := roleChanges(calls); len(got) != 1 || !strings.HasPrefix(got[0], fmt.Sprintf("PUT %d ", supMemberB)) {
		t.Errorf("role changes = %v, want only B given", got)
	}
}

// observedFake signals the start as one answer once the frames are delivered, for a run that
// makes no call to wait for.
type observedFake struct {
	*supporterFake
	done chan struct{}
}

func (f *observedFake) answered() <-chan struct{} { return f.done }

func (f *observedFake) observe(client *bot.Client) {
	f.supporterFake.observe(client)
	f.done <- struct{}{}
}

// runSupporterOverGateway runs the bot with the supporter function h and a check every few
// milliseconds.
func runSupporterOverGateway(t *testing.T, fake *supporterFake, h *SupporterHandler, wantAnswers int, frames ...[]byte) []areasCall {
	t.Helper()
	return runComponentsOverGatewayWith(t, fake, BotConfig{Supporter: h},
		clientOptions{supporterEvery: 5 * time.Millisecond}, wantAnswers, frames...)
}
