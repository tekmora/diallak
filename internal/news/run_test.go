package news

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// runNow is the start of the runner tests: 09:00 UTC, an hour past the day's digest time.
var runNow = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

// feedServer serves an Atom feed whose entries the test changes, and counts the reads.
type feedServer struct {
	mu      sync.Mutex
	entries []string
	reads   atomic.Int32
	srv     *httptest.Server
}

func newFeedServer(t *testing.T, entries ...string) *feedServer {
	t.Helper()
	fs := &feedServer{entries: entries}
	fs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fs.reads.Add(1)
		fs.mu.Lock()
		defer fs.mu.Unlock()
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom"><title>t</title>`)
		for _, e := range fs.entries {
			fmt.Fprintf(&b, `<entry><id>tag:github.com,2008:Repository/1/%s</id><updated>%s</updated>`+
				`<link rel="alternate" href="https://github.com/%s/releases/tag/%s"/><title>%s</title></entry>`,
				e, runNow.Format(time.RFC3339), testRepo, e, e)
		}
		b.WriteString(`</feed>`)
		if _, err := w.Write([]byte(b.String())); err != nil {
			t.Errorf("write feed: %v", err)
		}
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

// publish puts entry first in the feed, as a forge lists its newest release first.
func (fs *feedServer) publish(entry string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.entries = append([]string{entry}, fs.entries...)
}

type testRun struct {
	r      *Runner
	a      *fakeAnnouncer
	p      *fakePoster
	now    time.Time
	places atomic.Int32
}

func newTestRun(t *testing.T, srv *httptest.Server, sources ...Source) *testRun {
	t.Helper()
	s, _ := openTestStore(t)
	tr := &testRun{a: &fakeAnnouncer{}, p: &fakePoster{}, now: runNow}
	tr.r = &Runner{
		Store: s, Reader: testReader(srv), Announcer: tr.a, Poster: tr.p, Sources: sources,
		ReadPlaces: func(context.Context) (Places, error) {
			tr.places.Add(1)
			return Places{Announcement: testPlace, Channels: map[string]string{"ai": testChannel}}, nil
		},
		Now: func() time.Time { return tr.now },
	}
	return tr
}

// round runs one round at the test's clock with the digest a day away, and returns the next wake.
func (tr *testRun) round(t *testing.T) time.Time {
	t.Helper()
	wake, _ := tr.r.Round(testContext(t), tr.now.Add(24*time.Hour))
	return wake
}

func releaseSource(srv *httptest.Server) Source {
	return Source{ID: "releases:" + testRepo, Address: srv.URL + "/releases.atom", Kind: SourceReleases,
		Interval: ReleaseInterval, Repo: testRepo}
}

// A new release reaches the announcement channel once: the first read only marks what is there,
// the read after the release posts it, and a later read of the same feed posts nothing.
func TestRunner_Positive_ReleasePostedOnce(t *testing.T) {
	fs := newFeedServer(t, "v0.4.2")
	tr := newTestRun(t, fs.srv, releaseSource(fs.srv))

	if wake := tr.round(t); !wake.Equal(runNow.Add(ReleaseInterval)) {
		t.Fatalf("first round wakes at %v, want %v", wake, runNow.Add(ReleaseInterval))
	}
	fs.publish("v0.5.0")
	tr.now = tr.now.Add(ReleaseInterval)
	tr.round(t)
	tr.now = tr.now.Add(ReleaseInterval)
	tr.round(t)

	if fs.reads.Load() != 3 {
		t.Fatalf("reads = %d, want 3", fs.reads.Load())
	}
	if len(tr.a.posts) != 1 || !strings.Contains(tr.a.posts[0].content, "[v0.5.0]") ||
		tr.a.posts[0].channel != testChannel || tr.a.posts[0].role != testRole {
		t.Fatalf("posts = %+v, want v0.5.0 once in the announcement channel", tr.a.posts)
	}
	if len(tr.a.crossposts) != 1 {
		t.Fatalf("crossposts = %v, want 1", tr.a.crossposts)
	}
}

// A source is not read before its next read is due, and a round with nothing due reads neither the
// source nor the server's places.
func TestRunner_Boundary_NotDueNotRead(t *testing.T) {
	fs := newFeedServer(t, "v0.4.2")
	tr := newTestRun(t, fs.srv, releaseSource(fs.srv))
	tr.round(t)
	tr.now = tr.now.Add(ReleaseInterval - time.Second)
	if wake := tr.round(t); !wake.Equal(runNow.Add(ReleaseInterval)) {
		t.Fatalf("wake = %v, want %v", wake, runNow.Add(ReleaseInterval))
	}
	if fs.reads.Load() != 1 || tr.places.Load() != 1 {
		t.Fatalf("reads = %d, place reads = %d; want 1, 1", fs.reads.Load(), tr.places.Load())
	}
	tr.now = runNow.Add(ReleaseInterval)
	tr.round(t)
	if fs.reads.Load() != 2 {
		t.Fatalf("reads at the due time = %d, want 2", fs.reads.Load())
	}
}

// A rate-limited read waits for the server's reset, past the source's own interval.
func TestRunner_Negative_RateLimitedReadWaitsForReset(t *testing.T) {
	reader, srv := limitedServer(t)
	src := Source{ID: "advisories:" + testRepo, Address: srv.URL + "/repos/o/r/security-advisories",
		Kind: SourceAdvisories, Interval: AdvisoryInterval, Repo: testRepo}
	tr := newTestRun(t, srv, src)
	tr.r.Reader = reader
	tr.now = time.Date(2026, 10, 17, 17, 0, 0, 0, time.UTC)

	tr.round(t)
	if got := tr.r.Store.Feed(src.ID).NextRead; !got.Equal(tr.now.Add(30 * time.Minute)) {
		t.Fatalf("next read = %v, want the reset %v", got, tr.now.Add(30*time.Minute))
	}
}

// A failed read without a back-off is tried again after the source's interval.
func TestRunner_Negative_FailedReadTriedAfterInterval(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	tr := newTestRun(t, srv, releaseSource(srv))
	tr.round(t)
	if got := tr.r.Store.Feed("releases:" + testRepo).NextRead; !got.Equal(runNow.Add(ReleaseInterval)) {
		t.Fatalf("next read = %v, want %v", got, runNow.Add(ReleaseInterval))
	}
}

// A round that cannot read the server's places reads no source and tries again after PlacesRetry.
func TestRunner_Negative_PlacesNotReadReadsNothing(t *testing.T) {
	fs := newFeedServer(t, "v0.4.2")
	tr := newTestRun(t, fs.srv, releaseSource(fs.srv))
	tr.r.ReadPlaces = func(context.Context) (Places, error) { return Places{}, errors.New("discord down") }
	if wake := tr.round(t); !wake.Equal(runNow.Add(PlacesRetry)) {
		t.Fatalf("wake = %v, want %v", wake, runNow.Add(PlacesRetry))
	}
	if fs.reads.Load() != 0 {
		t.Fatalf("reads = %d, want 0", fs.reads.Load())
	}
}

// A feed's new items wait for the digest of its area's channel, which is posted at the digest
// time; a feed whose channel is not on the server holds nothing.
func TestRunner_Positive_FeedHeldThenDigestPosted(t *testing.T) {
	fs := newFeedServer(t, "a1")
	open := Source{ID: "ai-feed", Address: fs.srv.URL + "/feed.atom", Kind: SourceFeed, Interval: FeedInterval, Channel: "ai"}
	closed := Source{ID: "video-feed", Address: fs.srv.URL + "/video.atom", Kind: SourceFeed, Interval: FeedInterval, Channel: "video"}
	tr := newTestRun(t, fs.srv, open, closed)
	tr.round(t)
	fs.publish("a2")
	tr.now = tr.now.Add(FeedInterval)
	tr.round(t)

	if len(tr.p.posts) != 0 || len(tr.a.posts) != 0 {
		t.Fatalf("posted before the digest time: %+v %+v", tr.p.posts, tr.a.posts)
	}
	if got := tr.r.Store.HeldChannels(); len(got) != 1 || got[0] != testChannel {
		t.Fatalf("held channels = %v, want only %s", got, testChannel)
	}
	digest := tr.now.Add(time.Minute)
	tr.now = digest
	_, next := tr.r.Round(testContext(t), digest)
	if len(tr.p.posts) != 1 || tr.p.posts[0].channel != testChannel || !strings.Contains(tr.p.posts[0].content, "a2") {
		t.Fatalf("digests = %+v, want one holding a2", tr.p.posts)
	}
	if want := NextDigest(digest, DigestAt); !next.Equal(want) {
		t.Fatalf("next digest = %v, want %v", next, want)
	}
}

// The wake time is the earliest of the next reads and the digest time.
func TestRunner_Boundary_WakesForTheDigest(t *testing.T) {
	fs := newFeedServer(t, "v0.4.2")
	tr := newTestRun(t, fs.srv, releaseSource(fs.srv))
	digest := runNow.Add(time.Minute)
	if wake, _ := tr.r.Round(testContext(t), digest); !wake.Equal(digest) {
		t.Fatalf("wake = %v, want the digest %v", wake, digest)
	}
}

// The state is saved after every round, so a restart neither posts again nor reads early.
func TestRunner_Positive_StateSavedEachRound(t *testing.T) {
	fs := newFeedServer(t, "v0.4.2")
	s, path := openTestStore(t)
	tr := newTestRun(t, fs.srv, releaseSource(fs.srv))
	tr.r.Store = s
	tr.round(t)
	again, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("OpenFileStore() error = %v", err)
	}
	if st := again.Feed("releases:" + testRepo); !st.Primed || !st.NextRead.Equal(runNow.Add(ReleaseInterval)) {
		t.Fatalf("saved state = %+v, want primed with the next read", st)
	}
}

// Run ends with the context's error and forgets the state of sources no longer listed.
func TestRunner_Boundary_RunEndsWithContext(t *testing.T) {
	fs := newFeedServer(t, "v0.4.2")
	tr := newTestRun(t, fs.srv, releaseSource(fs.srv))
	tr.r.Store.SetNextRead("gone", runNow)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- tr.r.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for fs.reads.Load() == 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if st := tr.r.Store.Feed("gone"); !st.NextRead.IsZero() {
		t.Fatalf("unlisted source kept %+v", st)
	}
}

// Own repositories give one release and one advisory source each, at their intervals.
func TestOwnSources_Positive_ReleasesAndAdvisories(t *testing.T) {
	got := OwnSources([]string{"tekmora/diallak", "tekmora/epitrop"})
	if len(got) != 4 {
		t.Fatalf("OwnSources() = %d sources, want 4", len(got))
	}
	want := Source{ID: "advisories:tekmora/epitrop", Address: AdvisoriesAddress("tekmora/epitrop"),
		Kind: SourceAdvisories, Interval: AdvisoryInterval, Repo: "tekmora/epitrop"}
	if got[3] != want {
		t.Fatalf("OwnSources()[3] = %+v, want %+v", got[3], want)
	}
	if got[0].Kind != SourceReleases || got[0].Address != ReleasesAddress("tekmora/diallak") ||
		got[0].Interval != ReleaseInterval || got[0].ID != "releases:tekmora/diallak" {
		t.Fatalf("OwnSources()[0] = %+v", got[0])
	}
	if len(OwnSources(nil)) != 0 {
		t.Fatal("OwnSources(nil) is not empty")
	}
}
