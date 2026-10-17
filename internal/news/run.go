package news

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/tekmora/diallak/internal/ghapp"
)

const (
	// FeedInterval is how often a feed other than an own repository's is read. ADR-0019 names no
	// interval for them. Source: chosen, not measured.
	FeedInterval = time.Hour
	// DigestAt is the time of day, in UTC, of the daily digests (ADR-0019, "Open points": 08:00
	// UTC proposed).
	DigestAt = 8 * time.Hour
	// MinWait is the shortest wait between two rounds, so a source whose next read lies in the
	// past cannot make the loop spin. Source: chosen, not measured.
	MinWait = time.Second
	// PlacesRetry is the wait after a round that could not read the server's places.
	PlacesRetry = time.Minute
	// MaxRounds bounds the rounds of one Run (HISS-02): a round every second still runs for more
	// than three years. Source: chosen, not measured.
	MaxRounds = 100_000_000
)

// SourceKind says what a source is read as and where its items go.
type SourceKind int

const (
	// SourceFeed is an Atom or RSS feed whose items wait for the digest of an area's channel.
	SourceFeed SourceKind = iota
	// SourceReleases is an own repository's release feed, announced as it happens.
	SourceReleases
	// SourceAdvisories is an own repository's list of published security advisories, announced
	// as it happens.
	SourceAdvisories
	// SourceDiscussions is a repository's new discussions, posted as they happen.
	SourceDiscussions
)

// Source is one address the news function reads.
type Source struct {
	// ID names the source in the store.
	ID      string
	Address string
	Kind    SourceKind
	// Interval is the time between two reads.
	Interval time.Duration
	// Repo is the own repository ("owner/name") of a release or advisory source.
	Repo string
	// Channel is the server definition's id of the channel that gets a feed's digest.
	Channel string
}

// OwnSources returns the release and advisory sources of the own repositories repos.
func OwnSources(repos []string) []Source {
	out := make([]Source, 0, 2*len(repos))
	for _, r := range repos {
		out = append(out,
			Source{ID: "releases:" + r, Address: ReleasesAddress(r), Kind: SourceReleases, Interval: ReleaseInterval, Repo: r},
			Source{ID: "advisories:" + r, Address: AdvisoriesAddress(r), Kind: SourceAdvisories, Interval: AdvisoryInterval, Repo: r})
	}
	return out
}

// Places is where a round posts on the server, in Discord ids.
type Places struct {
	// Announcement is the announcement channel and the role its posts mention.
	Announcement Place
	// Channels maps the server definition's channel id to the Discord id of a channel that exists
	// on the server. A channel that is missing is not open.
	Channels map[string]string
	// Discussions is the Discord id of the channel where discussion posts go.
	Discussions string
}

// Runner reads the sources when they are due, announces own releases and advisories, holds the
// other feeds' items for the digests, and posts the digests once a day.
type Runner struct {
	Store       Store
	Reader      *Reader
	Discussions DiscussionReader
	Announcer   Announcer
	Poster      Poster
	Sources     []Source
	// ReadPlaces reads the server's places; it is called only in a round that reads a source.
	ReadPlaces func(ctx context.Context) (Places, error)
	Logger     *slog.Logger
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Run starts with every source whose next read is due and runs rounds until ctx ends, then
// returns ctx's error.
func (r *Runner) Run(ctx context.Context) error {
	r.Store.KeepFeeds(r.sourceIDs())
	nextDigest := NextDigest(r.now(), DigestAt)
	timer := time.NewTimer(0)
	defer timer.Stop()
	for range MaxRounds {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		var wake time.Time
		wake, nextDigest = r.Round(ctx, nextDigest)
		timer.Reset(max(wake.Sub(r.now()), MinWait))
	}
	return errors.New("news: round limit reached")
}

// Round reads every due source, posts the digests when nextDigest has come, saves the store, and
// returns when the next round is due and the digest time after this round.
func (r *Runner) Round(ctx context.Context, nextDigest time.Time) (wake, digest time.Time) {
	now := r.now()
	due := r.due(now)
	if len(due) > 0 {
		if places, err := r.ReadPlaces(ctx); err != nil {
			r.logger().Warn("news: server places not read", slog.Any("error", err))
			wake = now.Add(PlacesRetry)
		} else {
			for _, src := range due {
				r.read(ctx, src, places, now)
			}
		}
	}
	if !now.Before(nextDigest) {
		reps, err := PostDigests(ctx, r.Store, r.Poster, nextDigest, now)
		if err != nil {
			r.logger().Warn("news: digest not posted", slog.Any("error", err))
		}
		r.logger().Info("news: digests", slog.Int("channels", len(reps)))
		nextDigest = NextDigest(now, DigestAt)
	}
	r.Store.Expire(now)
	if err := r.Store.Save(ctx); err != nil {
		r.logger().Warn("news: state not saved", slog.Any("error", err))
	}
	return r.nextWake(wake, nextDigest), nextDigest
}

// read reads src once, keeps its result and sends its new items to their place. A failed read is
// tried again after the source's interval or the server's back-off, whichever is longer.
func (r *Runner) read(ctx context.Context, src Source, places Places, now time.Time) {
	if src.Kind == SourceDiscussions {
		r.readDiscussions(ctx, src, places, now)
		return
	}
	prev := r.Store.Feed(src.ID).Validators
	var res Result
	var err error
	if src.Kind == SourceAdvisories {
		res, err = r.Reader.FetchAdvisories(ctx, src.Address, prev)
	} else {
		res, err = r.Reader.Fetch(ctx, src.Address, prev)
	}
	if err != nil {
		wait := src.Interval
		var se *StatusError
		if errors.As(err, &se) {
			wait = max(wait, se.RetryAfter)
		}
		r.Store.SetNextRead(src.ID, now.Add(wait))
		r.logger().Warn("news: read failed", slog.String("source", src.ID), slog.Any("error", err),
			slog.Duration("next_read_in", wait))
		return
	}
	r.Store.SetNextRead(src.ID, now.Add(src.Interval))
	if res.NotModified {
		return
	}
	items := r.Store.Record(src.ID, res, now)
	switch src.Kind {
	case SourceReleases, SourceAdvisories:
		kind := KindRelease
		if src.Kind == SourceAdvisories {
			kind = KindAdvisory
		}
		rep, err := Announce(ctx, r.Store, r.Announcer, places.Announcement, src.ID, src.Repo, kind, items)
		if err != nil {
			r.logger().Warn("news: announcement failed", slog.String("source", src.ID), slog.Any("error", err))
		}
		r.logger().Info("news: announced", slog.String("source", src.ID), slog.Int("posted", rep.Posted),
			slog.Int("crossposted", rep.Crossposted), slog.Int("failed", rep.Failed))
	default:
		ch, open := places.Channels[src.Channel]
		n := HoldForPlace(r.Store, ch, open, items, now)
		r.logger().Info("news: held", slog.String("source", src.ID), slog.Int("held", n),
			slog.Int("dropped", len(items)-n))
	}
}

func (r *Runner) readDiscussions(ctx context.Context, src Source, places Places, now time.Time) {
	if r.Discussions == nil {
		r.logger().Warn("news: discussions reader is nil", slog.String("source", src.ID))
		return
	}
	items, err := r.Discussions.ReadDiscussions(ctx)
	if err != nil {
		wait := src.Interval
		var se *StatusError
		var gse *ghapp.StatusError
		if errors.As(err, &se) {
			wait = max(wait, se.RetryAfter)
		} else if errors.As(err, &gse) {
			wait = max(wait, gse.RetryAfter)
		}
		r.Store.SetNextRead(src.ID, now.Add(wait))
		r.logger().Warn("news: read discussions failed", slog.String("source", src.ID), slog.Any("error", err),
			slog.Duration("next_read_in", wait))
		return
	}
	r.Store.SetNextRead(src.ID, now.Add(src.Interval))
	fresh := r.Store.Record(src.ID, Result{Items: items}, now)
	if len(fresh) == 0 {
		return
	}
	r.postDiscussions(ctx, src.ID, places.Discussions, fresh)
}

func (r *Runner) postDiscussions(ctx context.Context, feedID, channelID string, items []Item) {
	if !discordID(channelID) {
		for i := range items {
			r.Store.Forget(feedID, items[i].ID)
		}
		r.logger().Warn("news: discussions place is not a Discord id", slog.String("channel", channelID))
		return
	}
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		repo := discussionRepo(it.Link)
		msg, ok := DiscussionMessage(repo, it)
		if !ok {
			r.logger().Warn("news: discussion post too long", slog.String("link", it.Link))
			continue
		}
		postCtx, cancel := context.WithTimeout(ctx, PostTimeout)
		err := r.Poster.PostDigest(postCtx, channelID, msg)
		cancel()
		if err != nil {
			r.Store.Forget(feedID, it.ID)
			r.logger().Warn("news: discussion post failed", slog.String("link", it.Link), slog.Any("error", err))
		}
	}
}

func (r *Runner) due(now time.Time) []Source {
	var due []Source
	for _, src := range r.Sources {
		if !now.Before(r.Store.Feed(src.ID).NextRead) {
			due = append(due, src)
		}
	}
	return due
}

// nextWake is the earliest of retry (when set), the sources' next reads and the digest time.
func (r *Runner) nextWake(retry, nextDigest time.Time) time.Time {
	wake := nextDigest
	if !retry.IsZero() && retry.Before(wake) {
		wake = retry
	}
	if !retry.IsZero() {
		// The sources were not read; their next reads lie in the past until the retry.
		return wake
	}
	for _, src := range r.Sources {
		if at := r.Store.Feed(src.ID).NextRead; at.Before(wake) {
			wake = at
		}
	}
	return wake
}

func (r *Runner) sourceIDs() []string {
	ids := make([]string, 0, len(r.Sources))
	for _, src := range r.Sources {
		ids = append(ids, src.ID)
	}
	return ids
}

func (r *Runner) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}
	return r.Now()
}

func (r *Runner) logger() *slog.Logger {
	if r.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return r.Logger
}
