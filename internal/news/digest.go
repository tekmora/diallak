package news

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DigestItems is how many items a digest shows with their titles; the rest are linked
	// (ADR-0019, "Open points": at most 10 items, the newest first).
	DigestItems = 10
	// MaxHeldAge is how long an item waits for its digest (ADR-0019, "What is kept": at most 24
	// hours). An item held longer is dropped.
	MaxHeldAge = 24 * time.Hour
	// MaxHeldPerChannel bounds the items one channel holds; past it the oldest go first. 20 feeds
	// of 50 new items a day fill it. Source: chosen, not measured.
	MaxHeldPerChannel = 1000
	// MaxDigestRunes is Discord's limit on a message's content.
	MaxDigestRunes = 2000
	// PostTimeout is the deadline of one digest post.
	PostTimeout = 10 * time.Second
)

// Poster posts one digest into a channel. The message mentions nobody (ADR-0019, point 6).
type Poster interface {
	PostDigest(ctx context.Context, channelID, content string) error
}

// Digest is one channel's digest of a day.
type Digest struct {
	// Content is the message, at most MaxDigestRunes runes.
	Content string
	// Shown counts the items shown with their titles, Linked the items shown as a bare address,
	// Dropped the items that fit neither.
	Shown, Linked, Dropped int
}

// DigestReport is what posting one channel's digest did.
type DigestReport struct {
	Channel string
	Digest
	// Expired counts the items dropped for waiting longer than MaxHeldAge.
	Expired int
	// Posted is false for a channel without items that day or whose post failed.
	Posted bool
}

// NextDigest returns the first digest time after now: the day's start in UTC plus at. at is cut
// into one day.
func NextDigest(now time.Time, at time.Duration) time.Time {
	at = ((at % (24 * time.Hour)) + 24*time.Hour) % (24 * time.Hour)
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Add(at)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// BuildDigest writes the digest of day from held, which is in the order the items were held. The
// newest DigestItems items are shown with their titles and the rest as bare addresses, as many as
// fit into MaxDigestRunes. It returns false for a day without items.
func BuildDigest(day time.Time, held []HeldItem) (Digest, bool) {
	if len(held) == 0 {
		return Digest{}, false
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	var b strings.Builder
	fmt.Fprintf(&b, "**News of <t:%d:D>**", start.Unix())
	d := Digest{}
	var rest []string
	for i := len(held) - 1; i >= 0; i-- {
		line := "\n- [" + escapeMarkdown(held[i].Title) + "](<" + safeLink(held[i].Link) + ">)"
		if d.Shown < DigestItems && fits(&b, line) {
			b.WriteString(line)
			d.Shown++
			continue
		}
		rest = append(rest, "<"+safeLink(held[i].Link)+">")
	}
	addRest(&b, &d, rest)
	d.Content = b.String()
	return d, true
}

// addRest appends the addresses in rest after "More:", as many as fit, and counts the others as
// dropped.
func addRest(b *strings.Builder, d *Digest, rest []string) {
	sep := "\nMore: "
	for _, link := range rest {
		if !fits(b, sep+link) {
			d.Dropped++
			continue
		}
		b.WriteString(sep + link)
		sep = " "
		d.Linked++
	}
}

func fits(b *strings.Builder, s string) bool {
	return utf8.RuneCountInString(b.String())+utf8.RuneCountInString(s) <= MaxDigestRunes
}

// PostDigests posts the digest of day for every channel that holds items, in channel order, and
// forgets the items of each channel whose post succeeded or that had nothing left to post. Items
// held longer than MaxHeldAge at now are dropped. A failed post keeps its items for the next try.
func PostDigests(ctx context.Context, s Store, p Poster, day, now time.Time) ([]DigestReport, error) {
	channels := s.HeldChannels()
	reports := make([]DigestReport, 0, len(channels))
	var errs []error
	for _, ch := range channels {
		rep, err := postChannel(ctx, s, p, ch, day, now)
		reports = append(reports, rep)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return reports, errors.Join(errs...)
}

func postChannel(ctx context.Context, s Store, p Poster, ch string, day, now time.Time) (DigestReport, error) {
	rep := DigestReport{Channel: ch}
	var fresh []HeldItem
	for _, it := range s.Held(ch) {
		if now.Sub(it.Held) > MaxHeldAge {
			rep.Expired++
			continue
		}
		fresh = append(fresh, it)
	}
	d, ok := BuildDigest(day, fresh)
	if !ok {
		s.ClearHeld(ch)
		return rep, nil
	}
	rep.Digest = d
	postCtx, cancel := context.WithTimeout(ctx, PostTimeout)
	defer cancel()
	if err := p.PostDigest(postCtx, ch, d.Content); err != nil {
		return rep, fmt.Errorf("post digest to channel %s: %w", ch, err)
	}
	rep.Posted = true
	s.ClearHeld(ch)
	return rep, nil
}

// markdownReplacer escapes the characters Discord reads as formatting, so a title is shown as
// written.
var markdownReplacer = strings.NewReplacer(
	`\`, `\\`, `*`, `\*`, `_`, `\_`, "`", "\\`", `~`, `\~`, `|`, `\|`,
	`[`, `\[`, `]`, `\]`, `(`, `\(`, `)`, `\)`, `<`, `\<`, `>`, `\>`,
	`#`, `\#`, `-`, `\-`, `@`, `\@`,
)

func escapeMarkdown(s string) string {
	return markdownReplacer.Replace(s)
}

// linkReplacer encodes the characters that would end a link's address early.
var linkReplacer = strings.NewReplacer(`<`, `%3C`, `>`, `%3E`, `)`, `%29`, ` `, `%20`)

func safeLink(link string) string {
	return linkReplacer.Replace(link)
}
