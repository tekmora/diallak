package news

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	testRole    = "112233445566778899"
	testChannel = "998877665544332211"
	testRepo    = "tekmora/diallak"
)

var testPlace = Place{Channel: testChannel, Role: testRole}

type announcement struct{ channel, role, content string }

// fakeAnnouncer records posts and crossposts. It fails the posts whose content holds a word of
// failPost and every crosspost when failCrosspost is set.
type fakeAnnouncer struct {
	posts         []announcement
	crossposts    []string
	failPost      []string
	failCrosspost bool
}

func (a *fakeAnnouncer) PostAnnouncement(ctx context.Context, channel, role, content string) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		return "", errors.New("post without a deadline")
	}
	for _, w := range a.failPost {
		if strings.Contains(content, w) {
			return "", errors.New("refused")
		}
	}
	a.posts = append(a.posts, announcement{channel, role, content})
	return "m" + strconv.Itoa(len(a.posts)), nil
}

func (a *fakeAnnouncer) Crosspost(ctx context.Context, channel, message string) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("crosspost without a deadline")
	}
	if a.failCrosspost {
		return errors.New("refused")
	}
	a.crossposts = append(a.crossposts, channel+"/"+message)
	return nil
}

func release(tag string, published time.Time) Item {
	return Item{
		ID:        "tag:github.com,2008:Repository/1/" + tag,
		Title:     tag,
		Link:      "https://github.com/" + testRepo + "/releases/tag/" + tag,
		Published: published,
	}
}

// primedStore returns a store whose feed f has been read once, so the next new item is posted.
func primedStore(t *testing.T) *FileStore {
	t.Helper()
	s, _ := openTestStore(t)
	s.Record("f", Result{Items: []Item{release("v0.0.1", storeNow.Add(-time.Hour))}}, storeNow)
	return s
}

func announce(t *testing.T, s Store, a Announcer, kind Kind, items []Item) AnnounceReport {
	t.Helper()
	rep, err := Announce(context.Background(), s, a, testPlace, "f", testRepo, kind, items)
	if err != nil {
		t.Fatalf("Announce() error = %v", err)
	}
	return rep
}

// A release is posted at once into the announcement channel, mentions the release role, links the
// forge's page without copying its text, and is published to the servers that follow the channel.
func TestAnnounce_Positive_ReleasePostedAndCrossposted(t *testing.T) {
	s := primedStore(t)
	a := &fakeAnnouncer{}
	it := release("v0.4.2", storeNow.Add(-time.Minute))
	rep := announce(t, s, a, KindRelease, s.Record("f", Result{Items: []Item{it}}, storeNow))

	want := "<@&" + testRole + "> New release of **tekmora/diallak**: [v0.4.2](<" + it.Link + ">), <t:" +
		strconv.FormatInt(it.Published.Unix(), 10) + ">"
	if len(a.posts) != 1 || a.posts[0] != (announcement{testChannel, testRole, want}) {
		t.Fatalf("posts = %+v, want one %q", a.posts, want)
	}
	if len(a.crossposts) != 1 || a.crossposts[0] != testChannel+"/m1" {
		t.Fatalf("crossposts = %v, want [%s/m1]", a.crossposts, testChannel)
	}
	if rep != (AnnounceReport{Posted: 1, Crossposted: 1}) {
		t.Fatalf("report = %+v, want 1 posted, 1 crossposted", rep)
	}
}

// A pre-release is a release of its own and is posted like one.
// A pre-release is announced to everybody, marked as an early build (issue #98; ADR-0014: early
// builds are announced in a channel every member reads).
func TestAnnounce_Positive_PreReleaseIsAnEarlyBuild(t *testing.T) {
	s := primedStore(t)
	a := &fakeAnnouncer{}
	it := release("v0.5.0-rc.1", time.Time{})
	announce(t, s, a, KindRelease, s.Record("f", Result{Items: []Item{it}}, storeNow))
	want := "<@&" + testRole + "> New early build of **tekmora/diallak**: [v0.5.0\\-rc.1](<" + it.Link + ">)"
	if len(a.posts) != 1 || a.posts[0].content != want {
		t.Fatalf("posts = %+v, want one %q", a.posts, want)
	}
}

// The release that follows an early build is a release again.
func TestAnnounce_Positive_ReleaseAfterEarlyBuild(t *testing.T) {
	s := primedStore(t)
	a := &fakeAnnouncer{}
	announce(t, s, a, KindRelease, s.Record("f", Result{Items: []Item{release("v0.5.0-rc.1", time.Time{})}}, storeNow))
	final := release("v0.5.0", time.Time{})
	announce(t, s, a, KindRelease, s.Record("f", Result{Items: []Item{final, release("v0.5.0-rc.1", time.Time{})}}, storeNow))
	if len(a.posts) != 2 {
		t.Fatalf("posts = %+v, want the early build and then the release", a.posts)
	}
	if !strings.Contains(a.posts[0].content, "New early build of") || !strings.Contains(a.posts[1].content, "New release of") ||
		!strings.Contains(a.posts[1].content, "[v0.5.0]") {
		t.Fatalf("posts = %q, want an early build, then the release v0.5.0", []string{a.posts[0].content, a.posts[1].content})
	}
}

// Only a SemVer pre-release suffix makes an early build: a hyphen in build metadata does not, and
// a link that names no tag is a release.
func TestEarlyBuild_Boundary_TagShapes(t *testing.T) {
	base := "https://github.com/" + testRepo + "/releases/tag/"
	for link, want := range map[string]bool{
		base + "v1.0.0":                false,
		base + "v1.0.0-rc.1":           true,
		base + "1.0.0-alpha":           true,
		base + "v1.0.0%2Bbuild-5":      false,
		base + "v1.0.0+build-5":        false,
		base + "v1.0.0-beta%2Bexp.sha": true,
		base + "v1.0.0-":               false,
		base:                           false,
		"https://github.com/" + testRepo + "/releases": false,
		"https://example.org/v1.0.0-rc.1":              false,
	} {
		if got := earlyBuild(Item{Link: link}); got != want {
			t.Errorf("earlyBuild(%q) = %v, want %v", link, got, want)
		}
	}
}

// An advisory is never an early build, whatever its link holds.
func TestAnnouncement_Negative_AdvisoryIsNoEarlyBuild(t *testing.T) {
	it := Item{Title: "x", Link: "https://github.com/" + testRepo + "/releases/tag/v1.0.0-rc.1"}
	got, _ := Announcement(KindAdvisory, testRepo, testRole, it)
	if !strings.Contains(got, "Security advisory for") {
		t.Fatalf("Announcement() = %q, want an advisory", got)
	}
}

func TestAnnounce_Positive_AdvisoryPosted(t *testing.T) {
	s := primedStore(t)
	a := &fakeAnnouncer{}
	it := Item{
		ID: "GHSA-aaaa-bbbb-cccc", Title: "Token leak in the log",
		Link: "https://github.com/tekmora/diallak/security/advisories/GHSA-aaaa-bbbb-cccc",
	}
	announce(t, s, a, KindAdvisory, s.Record("f", Result{Items: []Item{it}}, storeNow))
	want := "<@&" + testRole + "> Security advisory for **tekmora/diallak**: [Token leak in the log](<" + it.Link + ">)"
	if len(a.posts) != 1 || a.posts[0].content != want {
		t.Fatalf("posts = %+v, want one %q", a.posts, want)
	}
}

// The same release seen by two reads is posted once.
func TestAnnounce_Negative_SameReleaseSeenTwicePostedOnce(t *testing.T) {
	s := primedStore(t)
	a := &fakeAnnouncer{}
	it := release("v0.4.2", storeNow)
	for range 2 {
		announce(t, s, a, KindRelease, s.Record("f", Result{Items: []Item{it}}, storeNow))
	}
	if len(a.posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(a.posts))
	}
}

// A post that fails is forgotten by the store, so the next read posts it; the others are posted.
func TestAnnounce_Negative_FailedPostRetriedByNextRead(t *testing.T) {
	s := primedStore(t)
	a := &fakeAnnouncer{failPost: []string{"v0.4.1"}}
	items := []Item{release("v0.4.2", storeNow), release("v0.4.1", storeNow)}
	rep, err := Announce(context.Background(), s, a, testPlace, "f", testRepo, KindRelease, s.Record("f", Result{Items: items}, storeNow))
	if err == nil || rep.Posted != 1 || rep.Failed != 1 {
		t.Fatalf("Announce() = %+v, %v; want 1 posted, 1 failed, an error", rep, err)
	}
	a.failPost = nil
	announce(t, s, a, KindRelease, s.Record("f", Result{Items: items}, storeNow))
	if len(a.posts) != 2 || !strings.Contains(a.posts[1].content, "v0.4.1") {
		t.Fatalf("posts = %+v, want v0.4.1 posted by the second read", a.posts)
	}
}

// A failed crosspost is told, but the post stands and is not repeated.
func TestAnnounce_Negative_FailedCrosspostKeepsPost(t *testing.T) {
	s := primedStore(t)
	a := &fakeAnnouncer{failCrosspost: true}
	it := release("v0.4.2", storeNow)
	rep, err := Announce(context.Background(), s, a, testPlace, "f", testRepo, KindRelease, s.Record("f", Result{Items: []Item{it}}, storeNow))
	if err == nil || rep != (AnnounceReport{Posted: 1}) {
		t.Fatalf("Announce() = %+v, %v; want 1 posted, 0 crossposted, an error", rep, err)
	}
	announce(t, s, a, KindRelease, s.Record("f", Result{Items: []Item{it}}, storeNow))
	if len(a.posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(a.posts))
	}
}

// Without a channel or with a role that is not a Discord id nothing is posted, and the items are
// forgotten so that a fixed definition posts them.
func TestAnnounce_Negative_BadPlacePostsNothing(t *testing.T) {
	for name, place := range map[string]Place{
		"no channel":        {Role: testRole},
		"no role":           {Channel: testChannel},
		"role not an id":    {Channel: testChannel, Role: "everyone"},
		"channel not an id": {Channel: "announcements", Role: testRole},
	} {
		t.Run(name, func(t *testing.T) {
			s := primedStore(t)
			a := &fakeAnnouncer{}
			it := release("v0.4.2", storeNow)
			if _, err := Announce(context.Background(), s, a, place, "f", testRepo, KindRelease,
				s.Record("f", Result{Items: []Item{it}}, storeNow)); err == nil {
				t.Fatal("Announce() error = nil, want one")
			}
			if len(a.posts) != 0 {
				t.Fatalf("posts = %+v, want none", a.posts)
			}
			if got := s.Record("f", Result{Items: []Item{it}}, storeNow); len(got) != 1 {
				t.Fatalf("Record() after a bad place = %v, want the item again", ids(got))
			}
		})
	}
}

// Several new items are posted oldest first, one post each.
func TestAnnounce_Boundary_OldestFirstOnePostEach(t *testing.T) {
	s := primedStore(t)
	a := &fakeAnnouncer{}
	items := []Item{release("v0.4.2", storeNow), release("v0.4.1", storeNow), release("v0.4.0", storeNow)}
	rep := announce(t, s, a, KindRelease, s.Record("f", Result{Items: items}, storeNow))
	if rep.Posted != 3 || len(a.posts) != 3 {
		t.Fatalf("posted %d, want 3", rep.Posted)
	}
	for i, tag := range []string{"v0.4.0", "v0.4.1", "v0.4.2"} {
		if !strings.Contains(a.posts[i].content, tag) {
			t.Errorf("post %d = %q, want %s", i, a.posts[i].content, tag)
		}
	}
}

func TestAnnounce_Boundary_NoItemsNoCalls(t *testing.T) {
	a := &fakeAnnouncer{}
	rep := announce(t, primedStore(t), a, KindRelease, nil)
	if rep != (AnnounceReport{}) || len(a.posts)+len(a.crossposts) != 0 {
		t.Fatalf("Announce(nil) = %+v, %d calls; want nothing", rep, len(a.posts)+len(a.crossposts))
	}
}

// A post longer than Discord allows is not sent and not retried.
func TestAnnouncement_Boundary_TooLongIsRefused(t *testing.T) {
	it := release("v1", storeNow)
	if _, ok := Announcement(KindRelease, testRepo, testRole, it); !ok {
		t.Fatal("Announcement(short) ok = false, want true")
	}
	it.Link = "https://example.org/" + strings.Repeat("a", MaxDigestRunes)
	if _, ok := Announcement(KindRelease, testRepo, testRole, it); ok {
		t.Fatal("Announcement(long) ok = true, want false")
	}
	s := primedStore(t)
	a := &fakeAnnouncer{}
	rep := announce(t, s, a, KindRelease, s.Record("f", Result{Items: []Item{it}}, storeNow))
	if rep != (AnnounceReport{TooLong: 1}) || len(a.posts) != 0 {
		t.Fatalf("report = %+v, posts %d; want 1 too long, none posted", rep, len(a.posts))
	}
	if got := s.Record("f", Result{Items: []Item{it}}, storeNow); len(got) != 0 {
		t.Fatalf("Record() after too long = %v, want nothing", ids(got))
	}
}

// Feed text cannot mention anybody: its markup is escaped, the only mention is the role's.
func TestAnnouncement_Negative_FeedTextMentionsNobody(t *testing.T) {
	it := release("v1", time.Time{})
	it.Title = "@everyone <@&1> [x](https://evil.example)"
	got, ok := Announcement(KindRelease, testRepo, testRole, it)
	if !ok || strings.Count(got, "<@") != 1 || !strings.Contains(got, `\@everyone`) || strings.Contains(got, "<t:") {
		t.Fatalf("Announcement() = %q, want one mention, escaped title, no time", got)
	}
}
