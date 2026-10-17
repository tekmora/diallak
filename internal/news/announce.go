package news

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind is what an announcement tells of.
type Kind int

const (
	// KindRelease is a release of an own repository, a pre-release included.
	KindRelease Kind = iota + 1
	// KindAdvisory is a published security advisory of an own repository.
	KindAdvisory
)

// Announcer posts announcements into the announcement channel (ADR-0019, points 4 and 6).
type Announcer interface {
	// PostAnnouncement posts content into channelID and returns the message's id. The message may
	// mention role roleID and nothing else.
	PostAnnouncement(ctx context.Context, channelID, roleID, content string) (messageID string, err error)
	// Crosspost publishes the bot's own message messageID of channelID to the servers that follow
	// the channel.
	Crosspost(ctx context.Context, channelID, messageID string) error
}

// Place is where announcements go: the announcement channel and the role they mention, both as
// Discord ids.
type Place struct {
	Channel, Role string
}

// AnnounceReport is what one Announce did.
type AnnounceReport struct {
	// Posted counts the items posted, Crossposted those also published to following servers.
	Posted, Crossposted int
	// Failed counts the posts that failed; their items are read again by the next read.
	Failed int
	// TooLong counts the items whose post would exceed MaxDigestRunes; they are not posted.
	TooLong int
}

// Announcement writes the post for item it of repo ("owner/name"): the role's mention, what
// happened, the item's title linked to the forge's page, and its date when the item has one. Feed
// text is escaped, so the role's mention is the only one. It returns false when the post would be
// longer than Discord allows.
func Announcement(kind Kind, repo, roleID string, it Item) (string, bool) {
	what := "New release of"
	if kind == KindAdvisory {
		what = "Security advisory for"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<@&%s> %s **%s**: [%s](<%s>)", roleID, what, escapeMarkdown(repo),
		escapeMarkdown(it.Title), safeLink(it.Link))
	if !it.Published.IsZero() {
		fmt.Fprintf(&b, ", <t:%d>", it.Published.Unix())
	}
	return b.String(), utf8.RuneCountInString(b.String()) <= MaxDigestRunes
}

// Announce posts each of items, which a Store's Record of feed feedID returned in feed order, at
// once and oldest first: one post per item into place, each then published to following servers.
// A post that fails is forgotten by s, so the next read of the feed posts it; a failed crosspost
// keeps its post. A place whose channel or role is not a Discord id posts nothing and forgets all
// items.
func Announce(ctx context.Context, s Store, a Announcer, place Place, feedID, repo string, kind Kind,
	items []Item) (AnnounceReport, error) {
	var rep AnnounceReport
	if len(items) == 0 {
		return rep, nil
	}
	if !discordID(place.Channel) || !discordID(place.Role) {
		for i := range items {
			s.Forget(feedID, items[i].ID)
		}
		return rep, fmt.Errorf("announcement place %+v: channel and role must be Discord ids", place)
	}
	var errs []error
	for i := len(items) - 1; i >= 0; i-- {
		if err := announceOne(ctx, s, a, place, feedID, repo, kind, items[i], &rep); err != nil {
			errs = append(errs, err)
		}
	}
	return rep, errors.Join(errs...)
}

func announceOne(ctx context.Context, s Store, a Announcer, place Place, feedID, repo string, kind Kind,
	it Item, rep *AnnounceReport) error {
	content, ok := Announcement(kind, repo, place.Role, it)
	if !ok {
		rep.TooLong++
		return nil
	}
	postCtx, cancel := context.WithTimeout(ctx, PostTimeout)
	defer cancel()
	id, err := a.PostAnnouncement(postCtx, place.Channel, place.Role, content)
	if err != nil {
		rep.Failed++
		s.Forget(feedID, it.ID)
		return fmt.Errorf("post announcement %s: %w", it.Link, err)
	}
	rep.Posted++
	crossCtx, crossCancel := context.WithTimeout(ctx, PostTimeout)
	defer crossCancel()
	if err := a.Crosspost(crossCtx, place.Channel, id); err != nil {
		return fmt.Errorf("crosspost announcement %s: %w", it.Link, err)
	}
	rep.Crossposted++
	return nil
}

// discordID accepts a Discord id: a snowflake written in decimal.
func discordID(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil
}
