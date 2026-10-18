package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/answer"
)

// The tests in this file start the bot as Run does, with the forum answer, and hand its client
// thread, message and guild frames as the gateway delivers them. The fake Discord serves the
// application, the first message of a post and the reply in it. The start, every read and every
// reply count as one answer each.

// forumFake is the REST side of Discord for the forum answer.
type forumFake struct {
	t       *testing.T
	flags   discord.ApplicationFlags
	mu      sync.Mutex
	calls   []areasCall
	answers chan struct{}
	intents gateway.Intents
	cached  bool
	// authorBot makes the first message of the post one that a bot wrote.
	authorBot bool
}

func newForumFake(t *testing.T, flags discord.ApplicationFlags) *forumFake {
	return &forumFake{t: t, flags: flags, answers: make(chan struct{}, 8)}
}

func (f *forumFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("read body: %v", err)
	}
	f.mu.Lock()
	f.calls = append(f.calls, areasCall{method: r.Method, path: r.URL.Path, raw: string(raw)})
	f.mu.Unlock()
	if err := f.respond(w, r); err != nil {
		f.t.Errorf("write response: %v", err)
	}
}

// respond answers one request as Discord would, or records it as unexpected.
func (f *forumFake) respond(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	thread := fmt.Sprintf("/channels/%d/messages", forumThreadID)
	var err error
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/applications/@me":
		_, err = fmt.Fprintf(w, `{"id":"%d","name":"diallak","flags":%d}`, gatewayAppID, f.flags)
	case r.Method == http.MethodGet && (r.URL.Path == "/gateway" || r.URL.Path == "/gateway/bot"):
		_, err = fmt.Fprint(w, `{"url":"ws://127.0.0.1:0","shards":1}`)
	case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("%s/%d", thread, forumThreadID):
		_, err = fmt.Fprintf(w, `{"id":"%d","channel_id":"%d","type":0,"content":"It fails on Linux.",`+
			`"timestamp":"2026-10-17T00:00:00Z","author":{"id":"%d","username":"member","bot":%t}}`,
			forumThreadID, forumThreadID, gatewayMemberID, f.authorBot)
		f.answers <- struct{}{}
	case r.Method == http.MethodPost && r.URL.Path == thread:
		err = writeFakeMessage(w)
		f.answers <- struct{}{}
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

func (f *forumFake) answered() <-chan struct{} { return f.answers }

func (f *forumFake) recorded() []areasCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]areasCall(nil), f.calls...)
}

// observe records the intents and the cache once the frames are delivered, and counts as one
// answer, so a test that expects no reply still waits for the start.
func (f *forumFake) observe(client *bot.Client) {
	f.mu.Lock()
	f.intents = client.Gateway.Intents()
	_, f.cached = client.Caches.GuildThread(snowflake.ID(forumThreadID))
	f.mu.Unlock()
	f.answers <- struct{}{}
}

// channelCalls returns the requests on channels: the reads of a post and the replies.
func channelCalls(calls []areasCall) []areasCall {
	var out []areasCall
	for _, c := range calls {
		if strings.HasPrefix(c.path, "/channels/") {
			out = append(out, c)
		}
	}
	return out
}

// forumThread is the channel object of the test post with the given tags.
func forumThread(tags ...uint64) map[string]any {
	ids := make([]string, len(tags))
	for i, tag := range tags {
		ids[i] = fmt.Sprint(tag)
	}
	return map[string]any{
		"id": fmt.Sprint(forumThreadID), "type": int(discord.ChannelTypeGuildPublicThread),
		"guild_id": fmt.Sprint(areasGuildID), "parent_id": fmt.Sprint(forumChannelID),
		"owner_id": fmt.Sprint(gatewayMemberID), "name": "Install problem",
		"applied_tags": ids, "message_count": 0, "member_count": 1,
		"thread_metadata": map[string]any{"archived": false, "auto_archive_duration": 1440,
			"archive_timestamp": "2026-10-17T00:00:00Z", "locked": false},
	}
}

// forumFrame is a dispatch frame of event t with data d.
func forumFrame(t *testing.T, seq int, event string, d map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"op": 0, "s": seq, "t": event, "d": d})
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return raw
}

func threadCreateFrame(t *testing.T, seq int, newlyCreated bool, tags ...uint64) []byte {
	t.Helper()
	d := forumThread(tags...)
	d["newly_created"] = newlyCreated
	return forumFrame(t, seq, "THREAD_CREATE", d)
}

func messageFrame(t *testing.T, seq int, event string) []byte {
	t.Helper()
	return forumFrame(t, seq, event, map[string]any{
		"id": "810000000000000001", "channel_id": fmt.Sprint(forumThreadID), "guild_id": fmt.Sprint(areasGuildID),
		"type": 0, "content": "A second message", "timestamp": "2026-10-17T00:00:00Z",
		"author": map[string]any{"id": fmt.Sprint(gatewayMemberID), "username": "member"},
	})
}

func newGatewayForum(t *testing.T, asker ExcerptAsker) *ForumHandler {
	t.Helper()
	return newLoggedGatewayForum(t, asker, slog.New(slog.DiscardHandler))
}

func newLoggedGatewayForum(t *testing.T, asker ExcerptAsker, logger *slog.Logger) *ForumHandler {
	t.Helper()
	h, err := NewForumHandler(ForumConfig{
		Asker: asker, ChannelID: forumChannelID, OptInTagID: forumTagID,
		Timeout: 5 * time.Second, Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewForumHandler: %v", err)
	}
	return h
}

// ADR-0009 cases 2 and 12: the configured forum asks for the one standard intent, a new opted-in
// post is read once and answered once, the reply mentions nobody, and the thread is not cached.
func TestRunForum_Positive_OptedInPostIsAnsweredOverREST(t *testing.T) {
	asker := &fakeExcerpts{fn: func(context.Context, string) (answer.Answer, error) {
		return answer.Answer{Kind: answer.KindExcerpts, Reason: answer.ReasonNoModel, Sources: []answer.Source{
			{N: 1, Title: "Install", URL: "https://example.org/install", Revision: "abc123", Excerpt: "Ask <@123> or @everyone."},
		}}, nil
	}}
	fake := newForumFake(t, discord.ApplicationFlagGatewayMessageContentLimited)
	calls := runComponentsOverGateway(t, fake, BotConfig{Forum: newGatewayForum(t, asker)}, 3,
		threadCreateFrame(t, 1, true, forumTagID))

	if fake.intents != gateway.IntentGuilds {
		t.Errorf("gateway intents = %d, want only the guilds intent %d", fake.intents, gateway.IntentGuilds)
	}
	if fake.cached {
		t.Error("the thread is in the client cache, want no cache of titles")
	}
	got := channelCalls(calls)
	if len(got) != 2 || got[0].method != http.MethodGet || got[1].method != http.MethodPost {
		t.Fatalf("channel requests = %+v, want one read and one reply", got)
	}
	assertForumReply(t, got[1].raw)
	if q := asker.asked(); len(q) != 1 || q[0] != "Install problem\n\nIt fails on Linux." {
		t.Errorf("questions = %q, want title and first message", q)
	}
}

// assertForumReply checks that the reply body raw has the excerpt label first, the forum footer
// last and allows no mention.
func assertForumReply(t *testing.T, raw string) {
	t.Helper()
	var body struct {
		Content         string `json:"content"`
		AllowedMentions *struct {
			Parse []string `json:"parse"`
			Users []string `json:"users"`
			Roles []string `json:"roles"`
		} `json:"allowed_mentions"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("reply body: %v", err)
	}
	if !strings.HasPrefix(body.Content, answer.LabelExcerpts) || !strings.HasSuffix(body.Content, answer.FooterForum) {
		t.Errorf("reply = %q, want the excerpt label first and the forum footer last", body.Content)
	}
	m := body.AllowedMentions
	if m == nil || m.Parse == nil || len(m.Parse)+len(m.Users)+len(m.Roles) != 0 {
		t.Errorf("allowed mentions = %s, want none allowed", raw)
	}
}

// ADR-0009 case 1: without the forum the session asks for no intent and no thread event is handled.
func TestRunForum_Negative_NotConfiguredHandlesNoThread(t *testing.T) {
	fake := newForumFake(t, discord.ApplicationFlagGatewayMessageContent)
	calls := runComponentsOverGateway(t, fake, BotConfig{}, 1, threadCreateFrame(t, 1, true, forumTagID))
	if fake.intents != gateway.IntentsNone {
		t.Errorf("gateway intents = %d, want none", fake.intents)
	}
	if got := channelCalls(calls); len(got) != 0 {
		t.Errorf("channel requests = %+v, want none", got)
	}
}

// ADR-0009 cases 4, 6, 16, 17 and 19: posts present at start, message events, a tag changed later, a
// thread without the mark of a new one and a post without the opt-in tag are never read.
func TestRunForum_Negative_OnlyNewOptedInPostsAreRead(t *testing.T) {
	guild := map[string]any{
		"id": fmt.Sprint(areasGuildID), "name": "server", "owner_id": fmt.Sprint(gatewayMemberID),
		"roles": []any{}, "emojis": []any{}, "features": []any{}, "channels": []any{},
		"members": []any{}, "voice_states": []any{}, "presences": []any{}, "stage_instances": []any{},
		"stickers": []any{}, "guild_scheduled_events": []any{}, "joined_at": "2026-01-01T00:00:00Z",
		"threads": []any{forumThread(forumTagID)},
	}
	frames := [][]byte{
		forumFrame(t, 1, "GUILD_CREATE", guild),
		messageFrame(t, 2, "MESSAGE_CREATE"),
		messageFrame(t, 3, "MESSAGE_UPDATE"),
		forumFrame(t, 4, "THREAD_UPDATE", forumThread(forumTagID)),
		threadCreateFrame(t, 5, false, forumTagID),
		threadCreateFrame(t, 6, true, forumTagID+1),
		// The last frame is a new opted-in post: the one post the forum accepts.
		threadCreateFrame(t, 7, true, forumTagID),
	}
	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	fake := newForumFake(t, discord.ApplicationFlagGatewayMessageContent)
	// The frames are handled one after the other before the start counts as done, and the forum
	// accepts or skips each one at once; the log counts both.
	runComponentsOverGateway(t, fake, BotConfig{Forum: newLoggedGatewayForum(t, &fakeExcerpts{}, logger)}, 1, frames...)
	out := logs.String()
	if n := strings.Count(out, `"msg":"forum post accepted"`); n != 1 {
		t.Errorf("accepted posts = %d, want only the last one; log: %s", n, out)
	}
	if n := strings.Count(out, `"msg":"forum thread skipped"`); n != 2 {
		t.Errorf("skipped threads = %d, want the old one and the one without the tag; log: %s", n, out)
	}
}

// ADR-0009 case 11: without the message content right the start stops and says so.
func TestRunForum_Negative_StartStopsWithoutTheRight(t *testing.T) {
	fake := newForumFake(t, discord.ApplicationFlagGatewayPresence)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	opened := false
	opts := clientOptions{restURL: srv.URL, openGateway: func(context.Context, *bot.Client, string) error {
		opened = true
		return nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	handler, err := NewHandler(HandlerConfig{Asker: &fakeAsker{}, GuildID: areasGuildID})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	cfg := BotConfig{
		Token: makeFakeToken(gatewayAppID), GuildID: areasGuildID, Handler: handler,
		Forum: newGatewayForum(t, &fakeExcerpts{}),
	}
	err = runWithOptions(ctx, cfg, opts)
	if !errors.Is(err, errNoMessageContentRight) || !strings.Contains(err.Error(), "message content right") {
		t.Errorf("Run error = %v, want the missing message content right", err)
	}
	if opened {
		t.Error("the gateway was opened, want the start stopped before")
	}
}

func TestHasMessageContentRight_Boundary_EitherFlag(t *testing.T) {
	cases := map[discord.ApplicationFlags]bool{
		0: false,
		discord.ApplicationFlagGatewayPresence | discord.ApplicationFlagGatewayGuildMembers: false,
		discord.ApplicationFlagGatewayMessageContent:                                        true,
		discord.ApplicationFlagGatewayMessageContentLimited:                                 true,
	}
	for flags, want := range cases {
		if got := hasMessageContentRight(flags); got != want {
			t.Errorf("hasMessageContentRight(%d) = %v, want %v", flags, got, want)
		}
	}
}

func TestGatewayIntents_Positive_ForumAddsOnlyTheGuildsIntent(t *testing.T) {
	intents := gatewayIntents(true, false)
	if intents != gateway.IntentGuilds {
		t.Fatalf("gateway intents = %d, want %d", intents, gateway.IntentGuilds)
	}
	if intents.Has(gateway.IntentMessageContent) || intents.Has(gateway.IntentGuildMessages) {
		t.Fatalf("gateway intents %d include a message intent", intents)
	}
}

// ADR-0027: the supporter function adds the members intent and no other privileged one.
func TestGatewayIntents_Positive_SupporterAddsOnlyTheMembersIntent(t *testing.T) {
	cases := map[[2]bool]gateway.Intents{
		{false, true}: gateway.IntentGuildMembers,
		{true, true}:  gateway.IntentGuilds | gateway.IntentGuildMembers,
	}
	for in, want := range cases {
		got := gatewayIntents(in[0], in[1])
		if got != want {
			t.Errorf("gatewayIntents(%v, %v) = %d, want %d", in[0], in[1], got, want)
		}
		if got.Has(gateway.IntentGuildPresences) || got.Has(gateway.IntentMessageContent) {
			t.Errorf("gatewayIntents(%v, %v) = %d includes presences or message content", in[0], in[1], got)
		}
	}
}

// ADR-0027: without the supporter function the members intent is never requested.
func TestGatewayIntents_Negative_NoMembersIntentWithoutSupporter(t *testing.T) {
	for _, forum := range []bool{false, true} {
		if gatewayIntents(forum, false).Has(gateway.IntentGuildMembers) {
			t.Errorf("gatewayIntents(%v, false) requests the members intent", forum)
		}
	}
}

// ADR-0009 case 7: the first message of a post reads as one by an application when a bot wrote it.
func TestRestForumCalls_Positive_BotAuthorIsAnApplication(t *testing.T) {
	for _, bot := range []bool{false, true} {
		fake := newForumFake(t, 0)
		fake.authorBot = bot
		srv := httptest.NewServer(fake)
		client, err := disgo.New(makeFakeToken(gatewayAppID), botRestURL(srv.URL))
		if err != nil {
			t.Fatalf("disgo.New: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		msg, err := restForumCalls{rest: client.Rest}.FirstMessage(ctx, forumThreadID)
		cancel()
		srv.Close()
		if err != nil {
			t.Fatalf("FirstMessage: %v", err)
		}
		if msg.FromApplication != bot || msg.Content != "It fails on Linux." {
			t.Errorf("bot author %v: FirstMessage = %+v", bot, msg)
		}
	}
}

func botRestURL(url string) bot.ConfigOpt {
	return bot.WithRestClientConfigOpts(rest.WithURL(url))
}

// A thread event that arrives once shutdown has begun is not handed to the forum.
func TestHandleThreadCreate_Negative_IgnoredDuringShutdown(t *testing.T) {
	fake := newForumFake(t, discord.ApplicationFlagGatewayMessageContent)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	client, err := disgo.New(makeFakeToken(gatewayAppID), botRestURL(srv.URL))
	if err != nil {
		t.Fatalf("disgo.New: %v", err)
	}
	var ev gateway.EventThreadCreate
	raw, err := json.Marshal(map[string]any{"newly_created": true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	thread, err := json.Marshal(forumThread(forumTagID))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(append(thread[:len(thread)-1], append([]byte(","), raw[1:]...)...), &ev); err != nil {
		t.Fatalf("unmarshal thread: %v", err)
	}
	forum := newGatewayForum(t, &fakeExcerpts{})
	d := &dispatcher{cfg: BotConfig{Forum: forum}}
	d.handleThreadCreate(context.Background(), &events.ThreadCreate{
		GenericThread: &events.GenericThread{
			GenericEvent: events.NewGenericEvent(client, 1, 0), ThreadID: ev.ID(), Thread: ev.GuildThread,
		},
		NewlyCreated: ev.NewlyCreated,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := forum.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got := channelCalls(fake.recorded()); len(got) != 0 {
		t.Errorf("channel requests = %+v, want none during shutdown", got)
	}
}

// The shutdown waits for a forum post in hand, up to its timeout.
func TestShutdownClient_Boundary_WaitsForForumPosts(t *testing.T) {
	handler, err := NewHandler(HandlerConfig{Asker: &fakeAsker{}, GuildID: areasGuildID})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	forum := newGatewayForum(t, &fakeExcerpts{})
	cfg := BotConfig{Handler: handler, Forum: forum}
	var wg sync.WaitGroup
	newClient := func() *bot.Client {
		client, err := disgo.New(makeFakeToken(gatewayAppID))
		if err != nil {
			t.Fatalf("disgo.New: %v", err)
		}
		return client
	}
	if !forum.slots.acquire() {
		t.Fatal("no free slot")
	}
	if err := shutdownClient(cfg, newClient(), &wg, 50*time.Millisecond); err == nil {
		t.Error("shutdown with a post in hand: error = nil, want the timeout")
	}
	forum.slots.release()
	if err := shutdownClient(cfg, newClient(), &wg, time.Second); err != nil {
		t.Errorf("shutdown with no post in hand: %v", err)
	}
}

// syncBuffer is a log sink that goroutines may write to while a test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
