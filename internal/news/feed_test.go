package news

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return data
}

// releases.atom is this repository's own release feed as GitHub served it on 2026-10-17.
func TestParse_Positive_RecordedReleaseFeed(t *testing.T) {
	items, err := Parse(readTestdata(t, "releases.atom"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(items) != 10 {
		t.Fatalf("Parse() = %d items, want 10", len(items))
	}
	want := Item{
		ID:        "tag:github.com,2008:Repository/1413111065/v0.4.2",
		Title:     "v0.4.2",
		Link:      "https://github.com/tekmora/diallak/releases/tag/v0.4.2",
		Published: time.Date(2026, 10, 12, 15, 20, 19, 0, time.UTC),
	}
	if items[0] != want {
		t.Fatalf("items[0] = %+v, want %+v", items[0], want)
	}
}

func TestParse_Positive_RSSKeepsGuidDateAndPlainTitle(t *testing.T) {
	items, err := Parse(readTestdata(t, "sample.rss"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Parse() = %+v, want the two items with an https address and a title", items)
	}
	first := Item{
		ID:        "example-1",
		Title:     "First release is out",
		Link:      "https://example.org/posts/1",
		Published: time.Date(2026, 10, 16, 6, 30, 0, 0, time.UTC),
	}
	if items[0] != first {
		t.Fatalf("items[0] = %+v, want %+v", items[0], first)
	}
	second := Item{ID: "https://example.org/posts/2", Title: "No guid, no date", Link: "https://example.org/posts/2"}
	if items[1] != second {
		t.Fatalf("items[1] = %+v, want %+v (address as id, zero date)", items[1], second)
	}
}

func TestParse_Negative_BrokenFeedIsAnError(t *testing.T) {
	if items, err := Parse(readTestdata(t, "broken.atom")); err == nil {
		t.Fatalf("Parse(truncated feed) = %+v, want an error", items)
	}
}

func TestParse_Negative_DocumentThatIsNoFeed(t *testing.T) {
	_, err := Parse(readTestdata(t, "notfeed.html"))
	if !errors.Is(err, ErrNotFeed) {
		t.Fatalf("Parse(html) error = %v, want ErrNotFeed", err)
	}
}

func TestParse_Negative_AtomRootWithoutNamespaceIsNoFeed(t *testing.T) {
	_, err := Parse([]byte(`<feed><entry><id>x</id><title>t</title><link href="https://example.org/"/></entry></feed>`))
	if !errors.Is(err, ErrNotFeed) {
		t.Fatalf("Parse(feed without Atom namespace) error = %v, want ErrNotFeed", err)
	}
}

func TestParse_Negative_OversizedInputRefused(t *testing.T) {
	if _, err := Parse(make([]byte, MaxFeedBytes+1)); err == nil {
		t.Fatal("Parse(MaxFeedBytes+1 bytes) error = nil, want an error")
	}
}

func rssWithItems(n int) []byte {
	var b strings.Builder
	b.WriteString(`<rss version="2.0"><channel>`)
	for i := range n {
		fmt.Fprintf(&b, `<item><title>Item %d</title><link>https://example.org/%d</link></item>`, i, i)
	}
	b.WriteString(`</channel></rss>`)
	return []byte(b.String())
}

func TestParse_Boundary_ItemsCutAtMaxItems(t *testing.T) {
	for _, n := range []int{MaxItems - 1, MaxItems, MaxItems + 1, 2 * MaxItems} {
		items, err := Parse(rssWithItems(n))
		if err != nil {
			t.Fatalf("Parse(%d items) error = %v", n, err)
		}
		if want := min(n, MaxItems); len(items) != want {
			t.Fatalf("Parse(%d items) = %d items, want %d", n, len(items), want)
		}
		if items[0].Title != "Item 0" {
			t.Fatalf("Parse(%d items) first = %q, want the feed's order", n, items[0].Title)
		}
	}
}

func TestParse_Negative_AtomEntryWithOnlySelfLinkLeftOut(t *testing.T) {
	doc := `<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>a</id><title>t</title>` +
		`<link rel="self" href="https://example.org/self"/></entry></feed>`
	items, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("Parse() = %+v, want no item without an alternate link", items)
	}
}

func TestParse_Negative_AddressWithUserInformationLeftOut(t *testing.T) {
	doc := `<rss version="2.0"><channel><item><title>t</title><link>https://user:pw@example.org/</link></item></channel></rss>`
	items, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("Parse() = %+v, want the item left out", items)
	}
}

func TestPlainText_Negative_MarkupIsRemovedNotPassedOn(t *testing.T) {
	got := PlainText("<script>x</script> &lt;img src=x&gt; <a href='https://e'>link</a>\x07\n\tend", 200)
	if want := "x <img src=x> link end"; got != want {
		t.Fatalf("PlainText() = %q, want %q", got, want)
	}
}

func TestPlainText_Negative_HalfTagRemovesTheRest(t *testing.T) {
	if got := PlainText("safe <b onclick=", 200); got != "safe" {
		t.Fatalf("PlainText() = %q, want %q", got, "safe")
	}
}

func TestPlainText_Boundary_CutAtMaxRunes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{strings.Repeat("ä", MaxTitleRunes-1), MaxTitleRunes - 1},
		{strings.Repeat("ä", MaxTitleRunes), MaxTitleRunes},
		{strings.Repeat("ä", MaxTitleRunes+1), MaxTitleRunes},
	} {
		got := PlainText(tc.in, MaxTitleRunes)
		if n := utf8.RuneCountInString(got); n != tc.want || !utf8.ValidString(got) {
			t.Fatalf("PlainText(%d runes) = %d runes (valid %v), want %d", utf8.RuneCountInString(tc.in), n, utf8.ValidString(got), tc.want)
		}
	}
}

func TestPlainText_Boundary_NoTrailingSpaceAtTheCut(t *testing.T) {
	got := PlainText(strings.Repeat("a", 4)+"   b", 5)
	if got != "aaaa" {
		t.Fatalf("PlainText() = %q, want %q", got, "aaaa")
	}
}
