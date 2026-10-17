// Package news reads the feeds of the news function (ADR-0019) and turns their items into post
// candidates. It posts nothing: the path to Discord waits for the record to be accepted.
package news

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// MaxFeedBytes bounds what one read takes from a feed (ADR-0019, "What is kept").
	MaxFeedBytes = 1 << 20
	// MaxItems bounds the items taken from one feed; the rest is ignored.
	// Source: chosen, not measured.
	MaxItems = 200
	// MaxTitleRunes is where a title is cut (ADR-0019, "What is kept").
	MaxTitleRunes = 200
	// maxIDBytes bounds an item's id, which only feeds the key of the posted-items store.
	maxIDBytes = 1024
	// maxLinkBytes bounds an item's address.
	maxLinkBytes = 2048
)

// ErrNotFeed reports a document that is neither RSS 2.0 nor Atom.
var ErrNotFeed = errors.New("not an RSS or Atom feed")

// Item is one post candidate. Its text is plain: no markup, no control characters, cut to its
// bound. It is data and is escaped again by whatever posts it.
type Item struct {
	// ID is the feed's own id for the item (RSS guid, Atom id), or its address when the feed gives
	// none.
	ID string
	// Title is the item's title as plain text, at most MaxTitleRunes runes.
	Title string
	// Link is the item's absolute https address.
	Link string
	// Published is the item's date, zero when the feed gives none the reader understands.
	Published time.Time
}

type xmlFeed struct {
	XMLName xml.Name
	Items   []rssItem   `xml:"channel>item"`
	Entries []atomEntry `xml:"entry"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

const atomNamespace = "http://www.w3.org/2005/Atom"

// Parse reads an RSS 2.0 or Atom document and returns at most MaxItems items, in the feed's
// order. An item without an absolute https address or without a title is left out.
func Parse(data []byte) ([]Item, error) {
	if len(data) > MaxFeedBytes {
		return nil, fmt.Errorf("feed of %d bytes exceeds max %d", len(data), MaxFeedBytes)
	}
	var doc xmlFeed
	dec := xml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode feed: %w", err)
	}
	switch {
	case doc.XMLName.Local == "rss" && doc.XMLName.Space == "":
		return fromRSS(doc.Items), nil
	case doc.XMLName.Local == "feed" && doc.XMLName.Space == atomNamespace:
		return fromAtom(doc.Entries), nil
	default:
		return nil, fmt.Errorf("root element %q: %w", doc.XMLName.Local, ErrNotFeed)
	}
}

func fromRSS(items []rssItem) []Item {
	out := make([]Item, 0, min(len(items), MaxItems))
	for i := 0; i < len(items) && len(out) < MaxItems; i++ {
		raw := items[i]
		id := strings.TrimSpace(raw.GUID)
		if id == "" {
			id = strings.TrimSpace(raw.Link)
		}
		if it, ok := newItem(id, raw.Title, raw.Link, parseTime(raw.PubDate, time.RFC1123Z, time.RFC1123)); ok {
			out = append(out, it)
		}
	}
	return out
}

func fromAtom(entries []atomEntry) []Item {
	out := make([]Item, 0, min(len(entries), MaxItems))
	for i := 0; i < len(entries) && len(out) < MaxItems; i++ {
		raw := entries[i]
		date := raw.Published
		if strings.TrimSpace(date) == "" {
			date = raw.Updated
		}
		if it, ok := newItem(raw.ID, raw.Title, alternateLink(raw.Links), parseTime(date, time.RFC3339)); ok {
			out = append(out, it)
		}
	}
	return out
}

// alternateLink returns the entry's link with rel "alternate", or with no rel, which Atom reads
// as "alternate" (RFC 4287, section 4.2.7.2).
func alternateLink(links []atomLink) string {
	for i := range links {
		if links[i].Rel == "" || links[i].Rel == "alternate" {
			return links[i].Href
		}
	}
	return ""
}

func newItem(id, title, link string, published time.Time) (Item, bool) {
	link = strings.TrimSpace(link)
	if !validLink(link) {
		return Item{}, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = link
	}
	if len(id) > maxIDBytes || !utf8.ValidString(id) {
		return Item{}, false
	}
	text := PlainText(title, MaxTitleRunes)
	if text == "" {
		return Item{}, false
	}
	return Item{ID: id, Title: text, Link: link, Published: published}, true
}

// validLink accepts an absolute https address with a host and without user information.
func validLink(link string) bool {
	if link == "" || len(link) > maxLinkBytes {
		return false
	}
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.Host != "" && u.User == nil
}

func parseTime(value string, layouts ...string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// PlainText turns feed text into plain text of at most maxRunes runes: tags are removed, entities
// decoded, control characters dropped and runs of white space joined into one space.
func PlainText(s string, maxRunes int) string {
	s = html.UnescapeString(stripTags(s))
	var b strings.Builder
	b.Grow(min(len(s), maxRunes*utf8.UTFMax))
	n := 0
	space := false
	for _, r := range s {
		if n >= maxRunes {
			break
		}
		switch {
		case unicode.IsSpace(r):
			space = n > 0
			continue
		case unicode.IsControl(r) || r == utf8.RuneError:
			continue
		}
		if space {
			b.WriteByte(' ')
			n++
			space = false
			if n >= maxRunes {
				break
			}
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimRight(b.String(), " ")
}

// stripTags removes everything between "<" and the next ">". A "<" without a closing ">" removes
// the rest of the text, so no half tag survives.
func stripTags(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '<':
			inTag = true
		case c == '>' && inTag:
			inTag = false
			b.WriteByte(' ')
		case !inTag:
			b.WriteByte(c)
		}
	}
	return b.String()
}
