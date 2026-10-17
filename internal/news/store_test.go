package news

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var storeNow = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func openTestStore(t *testing.T) (*FileStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "news-state.json")
	s, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("OpenFileStore() error = %v", err)
	}
	return s, path
}

func item(id string, published time.Time) Item {
	return Item{ID: id, Title: "Title " + id, Link: "https://example.org/" + id, Published: published}
}

func ids(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// primed returns a store whose feed "f" has had its first read.
func primed(t *testing.T) (*FileStore, string) {
	t.Helper()
	s, path := openTestStore(t)
	if got := s.Record("f", Result{Items: []Item{item("old", storeNow)}}, storeNow); len(got) != 0 {
		t.Fatalf("first read returned %v, want nothing", ids(got))
	}
	return s, path
}

func TestRecord_Positive_FirstReadPostsNothingThenNewItemsArePosted(t *testing.T) {
	s, _ := primed(t)
	got := s.Record("f", Result{Items: []Item{item("new", storeNow), item("old", storeNow)}}, storeNow)
	if strings.Join(ids(got), ",") != "new" {
		t.Fatalf("second read returned %v, want [new]", ids(got))
	}
	if !s.Feed("f").Primed {
		t.Fatal("feed not marked as read")
	}
}

func TestRecord_Negative_SameItemTwiceIsPostedOnce(t *testing.T) {
	s, _ := primed(t)
	res := Result{Items: []Item{item("a", storeNow)}}
	if got := s.Record("f", res, storeNow); len(got) != 1 {
		t.Fatalf("first sight returned %v, want [a]", ids(got))
	}
	if got := s.Record("f", res, storeNow.Add(time.Minute)); len(got) != 0 {
		t.Fatalf("second sight returned %v, want nothing", ids(got))
	}
}

func TestRecord_Negative_ChangedTextKeepsTheKey(t *testing.T) {
	s, _ := primed(t)
	s.Record("f", Result{Items: []Item{item("a", storeNow)}}, storeNow)
	changed := item("a", storeNow)
	changed.Title, changed.Link = "A corrected title", "https://example.org/a-corrected"
	if got := s.Record("f", Result{Items: []Item{changed}}, storeNow); len(got) != 0 {
		t.Fatalf("changed item returned %v, want nothing", ids(got))
	}
}

func TestRecord_Positive_SameItemIDInAnotherFeedIsItsOwnItem(t *testing.T) {
	s, _ := primed(t)
	s.Record("g", Result{}, storeNow)
	if got := s.Record("g", Result{Items: []Item{item("old", storeNow)}}, storeNow); len(got) != 1 {
		t.Fatalf("feed g returned %v, want [old]", ids(got))
	}
}

func TestStore_Positive_RestartKeepsKeysAndFeedState(t *testing.T) {
	s, path := primed(t)
	v := Validators{ETag: `"v1"`, LastModified: "Sat, 17 Oct 2026 08:00:00 GMT"}
	s.Record("f", Result{Items: []Item{item("a", storeNow)}, Validators: v}, storeNow)
	s.SetNextRead("f", storeNow.Add(10*time.Minute))
	if err := s.Save(context.Background()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	again, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("OpenFileStore() after restart error = %v", err)
	}
	if got := again.Record("f", Result{Items: []Item{item("a", storeNow), item("old", storeNow)}, Validators: v}, storeNow); len(got) != 0 {
		t.Fatalf("read after restart returned %v, want nothing", ids(got))
	}
	want := FeedState{Validators: v, NextRead: storeNow.Add(10 * time.Minute), Primed: true}
	if got := again.Feed("f"); got != want {
		t.Fatalf("Feed() after restart = %+v, want %+v", got, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode = %v, %v; want 0600", info.Mode().Perm(), err)
	}
}

func TestRecord_Boundary_ItemAge(t *testing.T) {
	s, _ := primed(t)
	res := Result{Items: []Item{
		item("exactly-max-age", storeNow.Add(-MaxItemAge)),
		item("past-max-age", storeNow.Add(-MaxItemAge-time.Second)),
		item("undated", time.Time{}),
	}}
	got := s.Record("f", res, storeNow)
	if strings.Join(ids(got), ",") != "exactly-max-age,undated" {
		t.Fatalf("Record() = %v, want [exactly-max-age undated]", ids(got))
	}
	if again := s.Record("f", res, storeNow); len(again) != 0 {
		t.Fatalf("too old item was not marked as seen: %v", ids(again))
	}
}

func TestExpire_Boundary_KeyRetention(t *testing.T) {
	s, _ := primed(t)
	s.Record("f", Result{Items: []Item{item("a", time.Time{})}}, storeNow)

	s.Expire(storeNow.Add(KeyRetention))
	if got := s.Record("f", Result{Items: []Item{item("a", time.Time{})}}, storeNow.Add(KeyRetention)); len(got) != 0 {
		t.Fatalf("key dropped at exactly KeyRetention: %v", ids(got))
	}
	s.Expire(storeNow.Add(KeyRetention + time.Second))
	if got := s.Record("f", Result{Items: []Item{item("a", time.Time{})}}, storeNow.Add(KeyRetention+time.Second)); len(got) != 1 {
		t.Fatalf("key kept past KeyRetention: %v", ids(got))
	}
}

func TestRecord_Boundary_MaxKeysDropsTheOldest(t *testing.T) {
	s, _ := primed(t)
	for i := range MaxKeys {
		s.state.Seen["k"+strconv.Itoa(i)] = storeNow.Add(time.Duration(i) * time.Millisecond)
	}
	s.Record("f", Result{Items: []Item{item("newest", time.Time{})}}, storeNow.Add(time.Hour))
	if n := len(s.state.Seen); n != MaxKeys {
		t.Fatalf("store holds %d keys, want %d", n, MaxKeys)
	}
	if _, ok := s.state.Seen[ItemKey("f", "newest")]; !ok {
		t.Fatal("the newest key was dropped")
	}
	if _, ok := s.state.Seen[ItemKey("f", "old")]; ok {
		t.Fatal("the oldest key (the priming read) was kept")
	}
	if _, ok := s.state.Seen["k0"]; ok {
		t.Fatal("key k0 was kept, want the oldest two dropped")
	}
	if _, ok := s.state.Seen["k1"]; !ok {
		t.Fatal("key k1 was dropped, want only the oldest two dropped")
	}
}

func TestKeepFeeds_Positive_ForgetsUnlistedFeeds(t *testing.T) {
	s, _ := primed(t)
	s.SetNextRead("gone", storeNow)
	s.KeepFeeds([]string{"f"})
	if s.Feed("gone") != (FeedState{}) || !s.Feed("f").Primed {
		t.Fatalf("after KeepFeeds: gone = %+v, f = %+v", s.Feed("gone"), s.Feed("f"))
	}
}

func TestOpenFileStore_Negative_DamagedStateIsAnError(t *testing.T) {
	for name, content := range map[string]string{
		"not json":      "{",
		"unknown field": `{"version":1,"feeds":{},"seen":{},"extra":1}`,
		"other version": `{"version":2,"feeds":{},"seen":{}}`,
		"no version":    `{"feeds":{},"seen":{}}`,
		"oversized":     `{"version":1,"feeds":{},"seen":{}}` + strings.Repeat(" ", MaxStateBytes),
	} {
		path := filepath.Join(t.TempDir(), "news-state.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenFileStore(path); err == nil {
			t.Fatalf("OpenFileStore(%s) error = nil, want an error", name)
		}
	}
}

func TestOpenFileStore_Positive_MissingFileIsAnEmptyState(t *testing.T) {
	s, path := openTestStore(t)
	if s.Feed("f") != (FeedState{}) || len(s.state.Seen) != 0 {
		t.Fatalf("new store not empty: %+v", s.state)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("OpenFileStore created %s: %v", path, err)
	}
}

func TestSave_Negative_FailedReplaceKeepsTheOldStateAndNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "news-state.json")
	if err := os.Mkdir(path, 0o700); err != nil { // a directory in the way makes the rename fail
		t.Fatal(err)
	}
	s := &FileStore{path: path, state: stateFile{Version: stateVersion, Feeds: map[string]FeedState{}, Seen: map[string]time.Time{}}}
	if err := s.Save(context.Background()); err == nil {
		t.Fatal("Save() over a directory error = nil, want an error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("directory holds %v, want only the old entry", entries)
	}
}

func TestSave_Negative_EndedContextWritesNothing(t *testing.T) {
	s, path := primed(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Save(ctx); err == nil {
		t.Fatal("Save(ended context) error = nil, want an error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Save(ended context) wrote %s: %v", path, err)
	}
}
