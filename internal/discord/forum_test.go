package discord

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/answer"
)

const (
	forumChannelID uint64 = 780000000000000001
	forumTagID     uint64 = 790000000000000001
	forumThreadID  uint64 = 800000000000000001
)

// fakeExcerpts answers every question with one excerpt, or with what fn returns.
type fakeExcerpts struct {
	mu        sync.Mutex
	questions []string
	fn        func(ctx context.Context, q string) (answer.Answer, error)
}

func (f *fakeExcerpts) Excerpts(ctx context.Context, q string) (answer.Answer, error) {
	f.mu.Lock()
	f.questions = append(f.questions, q)
	fn := f.fn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, q)
	}
	return answer.Answer{Kind: answer.KindExcerpts, Reason: answer.ReasonNoModel, Sources: []answer.Source{
		{N: 1, Title: "Install", URL: "https://example.org/install", Revision: "abc123", Excerpt: "Run the installer."},
	}}, nil
}

func (f *fakeExcerpts) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.questions...)
}

// fakeForumCalls is the platform side of the forum answer.
type fakeForumCalls struct {
	mu      sync.Mutex
	message ForumMessage
	readErr error
	block   chan struct{}
	reads   []uint64
	replies []string
}

func (f *fakeForumCalls) FirstMessage(ctx context.Context, threadID uint64) (ForumMessage, error) {
	f.mu.Lock()
	f.reads = append(f.reads, threadID)
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ForumMessage{}, ctx.Err()
		}
	}
	return f.message, f.readErr
}

func (f *fakeForumCalls) Reply(_ context.Context, threadID uint64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if threadID != forumThreadID {
		return errors.New("reply in the wrong thread")
	}
	f.replies = append(f.replies, text)
	return nil
}

func (f *fakeForumCalls) counts() (int, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reads), append([]string(nil), f.replies...)
}

func newTestForum(t *testing.T, asker ExcerptAsker, logger *slog.Logger, maxConcurrent int) *ForumHandler {
	t.Helper()
	h, err := NewForumHandler(ForumConfig{
		Asker: asker, ChannelID: forumChannelID, OptInTagID: forumTagID,
		MaxConcurrent: maxConcurrent, Timeout: 5 * time.Second, Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewForumHandler: %v", err)
	}
	return h
}

// optedInPost is a new public post in the forum that carries the opt-in tag.
func optedInPost() ForumPost {
	return ForumPost{
		ThreadID: forumThreadID, ParentID: forumChannelID, PublicThread: true, NewlyCreated: true,
		TagIDs: []uint64{forumTagID + 1, forumTagID},
	}
}

// handleOne hands one post to the handler and waits until it is done.
func handleOne(t *testing.T, h *ForumHandler, post ForumPost, title string, calls ForumCalls) {
	t.Helper()
	h.Dispatch(context.Background(), post, title, calls)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestForum_Positive_OptedInPostGetsOneReply(t *testing.T) {
	asker := &fakeExcerpts{}
	calls := &fakeForumCalls{message: ForumMessage{Content: "It fails on Linux."}}
	handleOne(t, newTestForum(t, asker, nil, 2), optedInPost(), "Install problem", calls)

	reads, replies := calls.counts()
	if reads != 1 || len(replies) != 1 {
		t.Fatalf("reads %d, replies %d; want one of each", reads, len(replies))
	}
	if got := asker.asked(); len(got) != 1 || got[0] != "Install problem\n\nIt fails on Linux." {
		t.Errorf("questions = %q, want the title and the first message", got)
	}
	// ADR-0009 case 13.
	lines := strings.Split(replies[0], "\n")
	if lines[0] != answer.LabelExcerpts || lines[len(lines)-1] != answer.FooterForum {
		t.Errorf("reply = %q, want the excerpt label first and the forum footer last", replies[0])
	}
}

// ADR-0009 cases 3 and 5, ADR-0020 cases 17 and 19: a thread that is not a new opted-in post in the
// forum is neither read nor answered, and its title is not used.
func TestForum_Negative_OtherThreadsAreNotRead(t *testing.T) {
	cases := map[string]func(p *ForumPost){
		"another channel":     func(p *ForumPost) { p.ParentID = forumChannelID + 1 },
		"not a public thread": func(p *ForumPost) { p.PublicThread = false },
		"not newly created":   func(p *ForumPost) { p.NewlyCreated = false },
		"no opt-in tag":       func(p *ForumPost) { p.TagIDs = []uint64{forumTagID + 1} },
		"no tag at all":       func(p *ForumPost) { p.TagIDs = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			asker := &fakeExcerpts{}
			calls := &fakeForumCalls{message: ForumMessage{Content: "text"}}
			post := optedInPost()
			change(&post)
			handleOne(t, newTestForum(t, asker, nil, 2), post, "title", calls)
			if reads, replies := calls.counts(); reads != 0 || len(replies) != 0 || len(asker.asked()) != 0 {
				t.Errorf("reads %d, replies %d, questions %d; want none", reads, len(replies), len(asker.asked()))
			}
		})
	}
}

// ADR-0009 case 7.
func TestForum_Negative_PostOfAnApplicationGetsNoReply(t *testing.T) {
	asker := &fakeExcerpts{}
	calls := &fakeForumCalls{message: ForumMessage{Content: "text", FromApplication: true}}
	handleOne(t, newTestForum(t, asker, nil, 2), optedInPost(), "title", calls)
	if _, replies := calls.counts(); len(replies) != 0 || len(asker.asked()) != 0 {
		t.Errorf("replies %d, questions %d; want none", len(replies), len(asker.asked()))
	}
}

// ADR-0009 case 9.
func TestForum_Negative_NoPassageGetsNoReply(t *testing.T) {
	asker := &fakeExcerpts{fn: func(context.Context, string) (answer.Answer, error) {
		return answer.Answer{Kind: answer.KindRefusal, Reason: answer.ReasonNoSource}, nil
	}}
	calls := &fakeForumCalls{message: ForumMessage{Content: "text"}}
	handleOne(t, newTestForum(t, asker, nil, 2), optedInPost(), "title", calls)
	if _, replies := calls.counts(); len(replies) != 0 {
		t.Errorf("replies %d, want none", len(replies))
	}
}

// ADR-0009 case 10: a failed, timed-out or empty read gets no reply; the outcome is logged with its
// reason and the handler keeps serving.
func TestForum_Negative_FailedReadIsLoggedAndNotAnswered(t *testing.T) {
	cases := map[string]struct {
		calls  *fakeForumCalls
		reason string
	}{
		"error":   {&fakeForumCalls{readErr: errors.New("500")}, `"error_class":"error"`},
		"timeout": {&fakeForumCalls{block: make(chan struct{})}, `"error_class":"timeout"`},
		"empty":   {&fakeForumCalls{message: ForumMessage{Content: "  "}}, `"reason":"empty_message"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			h, err := NewForumHandler(ForumConfig{
				Asker: &fakeExcerpts{}, ChannelID: forumChannelID, OptInTagID: forumTagID,
				Timeout: 50 * time.Millisecond, Logger: logger,
			})
			if err != nil {
				t.Fatalf("NewForumHandler: %v", err)
			}
			handleOne(t, h, optedInPost(), "title", c.calls)
			if _, replies := c.calls.counts(); len(replies) != 0 {
				t.Errorf("replies %d, want none", len(replies))
			}
			if !strings.Contains(buf.String(), c.reason) {
				t.Errorf("log %q does not hold %s", buf.String(), c.reason)
			}
			// The handler keeps running: the next post is answered.
			next := &fakeForumCalls{message: ForumMessage{Content: "text"}}
			handleOne(t, h, optedInPost(), "title", next)
			if _, replies := next.counts(); len(replies) != 1 {
				t.Errorf("next post: replies %d, want 1", len(replies))
			}
		})
	}
}

// ADR-0009 case 8: the log holds no title, no text, no post id.
func TestForum_Negative_LogHoldsNoContent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	text := "Mail jane.doe@example.org or call +49 30 1234567, Jane Doe"
	calls := &fakeForumCalls{message: ForumMessage{Content: text}}
	h := newTestForum(t, &fakeExcerpts{}, logger, 2)
	handleOne(t, h, optedInPost(), "Jane Doe needs help", calls)
	other := optedInPost()
	other.TagIDs = nil
	handleOne(t, h, other, "Jane Doe needs help", calls)

	if _, replies := calls.counts(); len(replies) != 1 {
		t.Fatalf("replies %d, want 1", len(replies))
	}
	for _, s := range []string{"jane.doe@example.org", "1234567", "Jane Doe", "800000000000000001"} {
		if strings.Contains(buf.String(), s) {
			t.Errorf("log holds %q: %s", s, buf.String())
		}
	}
	if !strings.Contains(buf.String(), `"outcome":"excerpts"`) {
		t.Errorf("log %q holds no outcome of the answered post", buf.String())
	}
}

// ADR-0009 case 14: a longer text is cut, and the question is built from the part that is left.
func TestForum_Boundary_LongTextIsCut(t *testing.T) {
	asker := &fakeExcerpts{}
	calls := &fakeForumCalls{message: ForumMessage{Content: strings.Repeat("ä", 50)}}
	h, err := NewForumHandler(ForumConfig{
		Asker: asker, ChannelID: forumChannelID, OptInTagID: forumTagID, QuestionMaxChars: 20,
	})
	if err != nil {
		t.Fatalf("NewForumHandler: %v", err)
	}
	handleOne(t, h, optedInPost(), "Title", calls)
	got := asker.asked()
	if len(got) != 1 || utf8.RuneCountInString(got[0]) != 20 || got[0] != "Title\n\n"+strings.Repeat("ä", 13) {
		t.Errorf("questions = %q, want the first 20 runes of title and text", got)
	}
	if _, replies := calls.counts(); len(replies) != 1 {
		t.Errorf("replies %d, want 1", len(replies))
	}
}

// ADR-0009 case 15: posts over the bound get no reply, and nothing waits for a slot.
func TestForum_Boundary_PostsOverTheBoundAreDropped(t *testing.T) {
	block := make(chan struct{})
	busy := &fakeForumCalls{message: ForumMessage{Content: "text"}, block: block}
	h := newTestForum(t, &fakeExcerpts{}, nil, 2)
	h.Dispatch(context.Background(), optedInPost(), "one", busy)
	h.Dispatch(context.Background(), optedInPost(), "two", busy)
	waitForReads(t, busy, 2)

	over := &fakeForumCalls{message: ForumMessage{Content: "text"}}
	h.Dispatch(context.Background(), optedInPost(), "three", over)
	close(block)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if reads, replies := over.counts(); reads != 0 || len(replies) != 0 {
		t.Errorf("post over the bound: reads %d, replies %d; want none", reads, len(replies))
	}
	if _, replies := busy.counts(); len(replies) != 2 {
		t.Errorf("posts within the bound: replies %d, want 2", len(replies))
	}
}

func waitForReads(t *testing.T, calls *fakeForumCalls, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for range 500 {
		if reads, _ := calls.counts(); reads >= n {
			return
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fewer than %d reads within 5s", n)
}

// ADR-0009 case 18 as amended by ADR-0020: the forum does not run without its opt-in tag.
func TestNewForumHandler_Negative_RequiredSettings(t *testing.T) {
	cases := map[string]ForumConfig{
		"no asker":      {ChannelID: forumChannelID, OptInTagID: forumTagID},
		"no channel":    {Asker: &fakeExcerpts{}, OptInTagID: forumTagID},
		"no opt-in tag": {Asker: &fakeExcerpts{}, ChannelID: forumChannelID},
		"concurrency":   {Asker: &fakeExcerpts{}, ChannelID: forumChannelID, OptInTagID: forumTagID, MaxConcurrent: MaxAllowedConcurrent + 1},
		"question":      {Asker: &fakeExcerpts{}, ChannelID: forumChannelID, OptInTagID: forumTagID, QuestionMaxChars: -1},
		"timeout":       {Asker: &fakeExcerpts{}, ChannelID: forumChannelID, OptInTagID: forumTagID, Timeout: -1},
	}
	for name, cfg := range cases {
		if _, err := NewForumHandler(cfg); err == nil {
			t.Errorf("%s: NewForumHandler error = nil, want an error", name)
		}
	}
	_, err := NewForumHandler(cases["no opt-in tag"])
	if err == nil || !strings.Contains(err.Error(), "opt-in tag") {
		t.Errorf("no opt-in tag: error = %v, want one that names the opt-in tag", err)
	}
}
