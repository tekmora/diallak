package news

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var digestDay = time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)

// digestAt is the digest time of digestDay used by the tests: 08:00 UTC.
var digestAt = digestDay.Add(8 * time.Hour)

type post struct{ channel, content string }

// fakePoster records the digests it is given and fails for the channels in fail.
type fakePoster struct {
	posts []post
	fail  map[string]bool
}

func (p *fakePoster) PostDigest(ctx context.Context, channel, content string) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("post without a deadline")
	}
	if p.fail[channel] {
		return errors.New("refused")
	}
	p.posts = append(p.posts, post{channel, content})
	return nil
}

func numbered(n int) []Item {
	items := make([]Item, 0, n)
	for i := range n {
		items = append(items, item("i"+strconv.Itoa(i), digestDay))
	}
	return items
}

// An empty day posts nothing, for a channel that never held an item and for one whose items all
// waited too long.
func TestPostDigests_Negative_EmptyDayPostsNothing(t *testing.T) {
	s, _ := openTestStore(t)
	p := &fakePoster{}
	reports, err := PostDigests(context.Background(), s, p, digestDay, digestAt)
	if err != nil || len(reports) != 0 || len(p.posts) != 0 {
		t.Fatalf("empty store: reports %v, posts %v, err %v; want none", reports, p.posts, err)
	}
	s.Hold("c1", numbered(2), digestAt.Add(-MaxHeldAge-time.Second))
	reports, err = PostDigests(context.Background(), s, p, digestDay, digestAt)
	if err != nil || len(p.posts) != 0 {
		t.Fatalf("expired items: posts %v, err %v; want none", p.posts, err)
	}
	if len(reports) != 1 || reports[0].Expired != 2 || reports[0].Posted {
		t.Fatalf("expired items: reports %+v, want one unposted report with 2 expired", reports)
	}
	if got := s.HeldChannels(); len(got) != 0 {
		t.Fatalf("channels still holding items: %v", got)
	}
}

// Two channels get one digest each, with their own items only, and forget them once posted.
func TestPostDigests_Positive_TwoChannelsOneDigestEach(t *testing.T) {
	s, _ := openTestStore(t)
	s.Hold("c1", []Item{item("a", digestDay)}, digestAt.Add(-time.Hour))
	s.Hold("c2", []Item{item("b", digestDay), item("c", digestDay)}, digestAt.Add(-time.Hour))
	p := &fakePoster{}
	if _, err := PostDigests(context.Background(), s, p, digestDay, digestAt); err != nil {
		t.Fatalf("PostDigests() error = %v", err)
	}
	if len(p.posts) != 2 || p.posts[0].channel != "c1" || p.posts[1].channel != "c2" {
		t.Fatalf("posts %v, want one for c1 and one for c2", p.posts)
	}
	if !strings.Contains(p.posts[0].content, "Title a") || strings.Contains(p.posts[0].content, "Title b") {
		t.Errorf("c1 digest %q, want item a only", p.posts[0].content)
	}
	for _, want := range []string{"Title b", "Title c", "<t:" + strconv.FormatInt(digestDay.Unix(), 10) + ":D>"} {
		if !strings.Contains(p.posts[1].content, want) {
			t.Errorf("c2 digest %q lacks %q", p.posts[1].content, want)
		}
	}
	if got := s.HeldChannels(); len(got) != 0 {
		t.Fatalf("channels still holding items after posting: %v", got)
	}
}

// A failed post keeps its channel's items for the next try and does not stop the other channel.
func TestPostDigests_Negative_FailedPostKeepsItems(t *testing.T) {
	s, _ := openTestStore(t)
	s.Hold("c1", []Item{item("a", digestDay)}, digestAt)
	s.Hold("c2", []Item{item("b", digestDay)}, digestAt)
	p := &fakePoster{fail: map[string]bool{"c1": true}}
	if _, err := PostDigests(context.Background(), s, p, digestDay, digestAt); err == nil {
		t.Fatal("PostDigests() error = nil, want the failed post")
	}
	if len(p.posts) != 1 || p.posts[0].channel != "c2" {
		t.Fatalf("posts %v, want the one for c2", p.posts)
	}
	if got := s.Held("c1"); len(got) != 1 {
		t.Fatalf("c1 holds %v, want its item kept", got)
	}
}

// A day above the bound shows the newest DigestItems items with titles, links the rest, and stays
// within Discord's limit.
func TestBuildDigest_Boundary_DayAboveTheBound(t *testing.T) {
	s, _ := openTestStore(t)
	s.Hold("c", numbered(DigestItems+5), digestAt)
	d, ok := BuildDigest(digestDay, s.Held("c"))
	if !ok {
		t.Fatal("BuildDigest() = false for a day with items")
	}
	if d.Shown != DigestItems || d.Linked != 5 || d.Dropped != 0 {
		t.Fatalf("shown %d, linked %d, dropped %d; want %d, 5, 0", d.Shown, d.Linked, d.Dropped, DigestItems)
	}
	newest := "Title i" + strconv.Itoa(DigestItems+4)
	if !strings.Contains(d.Content, newest) || strings.Index(d.Content, newest) > strings.Index(d.Content, "Title i5") {
		t.Errorf("digest %q does not show the newest item first", d.Content)
	}
	if strings.Contains(d.Content, "Title i4") || !strings.Contains(d.Content, "More: <https://example.org/i4>") {
		t.Errorf("digest %q does not link the oldest items without titles", d.Content)
	}
}

// Items past what one message holds are dropped and counted; the digest is exactly at the limit
// or below it.
func TestBuildDigest_Boundary_MessageLimit(t *testing.T) {
	long := strings.Repeat("x", 600)
	held := make([]HeldItem, 0, 30)
	for i := range 30 {
		held = append(held, HeldItem{Title: "T" + strconv.Itoa(i), Link: "https://example.org/" + long + strconv.Itoa(i)})
	}
	d, _ := BuildDigest(digestDay, held)
	if n := utf8.RuneCountInString(d.Content); n > MaxDigestRunes {
		t.Fatalf("digest of %d runes, want at most %d", n, MaxDigestRunes)
	}
	if d.Dropped == 0 || d.Shown+d.Linked+d.Dropped != len(held) {
		t.Fatalf("shown %d, linked %d, dropped %d of %d; want some dropped and all counted", d.Shown, d.Linked, d.Dropped, len(held))
	}
}

// A title is shown as written: it cannot add formatting, a link or a mention, and an address
// cannot end its link early.
func TestBuildDigest_Negative_FeedTextIsEscaped(t *testing.T) {
	d, _ := BuildDigest(digestDay, []HeldItem{{Title: "**big** [x](https://evil.example) @everyone", Link: "https://example.org/a>b)c"}})
	for _, bad := range []string{"**big**", "[x](", " @everyone", "a>b", "b)c"} {
		if strings.Contains(d.Content, bad) {
			t.Errorf("digest %q contains %q unescaped", d.Content, bad)
		}
	}
}

// The held items and their order survive a restart.
func TestHold_Positive_HeldItemsSurviveARestart(t *testing.T) {
	s, path := openTestStore(t)
	s.Hold("c", []Item{item("a", digestDay), item("b", digestDay)}, digestAt)
	if err := s.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	again, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("OpenFileStore() error = %v", err)
	}
	got := again.Held("c")
	if len(got) != 2 || got[0].Title != "Title a" || got[1].Link != "https://example.org/b" || !got[0].Held.Equal(digestAt) {
		t.Fatalf("held after restart %+v, want a then b", got)
	}
}

// A channel holds at most MaxHeldPerChannel items; the oldest go first.
func TestHold_Boundary_OldestGoPastTheBound(t *testing.T) {
	s, _ := openTestStore(t)
	for range MaxHeldPerChannel/MaxItems + 1 {
		s.Hold("c", numbered(MaxItems), digestAt)
	}
	s.Hold("c", []Item{item("last", digestDay)}, digestAt)
	got := s.Held("c")
	if len(got) != MaxHeldPerChannel || got[len(got)-1].Title != "Title last" {
		t.Fatalf("held %d items ending in %q, want %d ending in the last", len(got), got[len(got)-1].Title, MaxHeldPerChannel)
	}
}

// The digest time is the next one after now, the same day before it and the next day at or after
// it.
func TestNextDigest_Boundary_SameDayOrNext(t *testing.T) {
	at := 8 * time.Hour
	for _, tc := range []struct {
		now, want time.Time
	}{
		{digestDay.Add(7*time.Hour + 59*time.Minute), digestAt},
		{digestAt, digestAt.AddDate(0, 0, 1)},
		{digestAt.Add(time.Second), digestAt.AddDate(0, 0, 1)},
	} {
		if got := NextDigest(tc.now, at); !got.Equal(tc.want) {
			t.Errorf("NextDigest(%v) = %v, want %v", tc.now, got, tc.want)
		}
	}
}
