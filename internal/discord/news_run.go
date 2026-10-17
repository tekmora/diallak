package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/news"
	"github.com/tekmora/diallak/internal/serverdef"
)

// NewsConfig runs the news function in the bot (ADR-0019, issue #180).
type NewsConfig struct {
	// Runner holds the store, the reader and the sources; Run sets its announcer, its poster and
	// its reading of the server's places.
	Runner *news.Runner
	// Names are the server names of the places the runner posts in.
	Names serverdef.NewsNames
}

// wire makes the runner post through restClient and find its places on the server guildID by
// name before every round that reads a source, so a renamed or new channel needs no restart.
func (nc *NewsConfig) wire(restClient rest.Rest, guildID uint64, timeout time.Duration) {
	poster := NewNewsPoster(restClient)
	nc.Runner.Announcer = poster
	nc.Runner.Poster = poster
	nc.Runner.ReadPlaces = func(ctx context.Context) (news.Places, error) {
		return readNewsPlaces(ctx, restClient, guildID, nc.Names, timeout)
	}
}

// readNewsPlaces reads the server's roles and channels and finds the news places by name. The
// announcement channel and the release role must each be held once; a feed channel that is missing
// or held twice is not open, and its feed's items are dropped.
func readNewsPlaces(ctx context.Context, restClient rest.Rest, guildID uint64, names serverdef.NewsNames, timeout time.Duration) (news.Places, error) {
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	roles, err := restClient.GetRoles(snowflake.ID(guildID), rest.WithCtx(readCtx))
	if err != nil {
		return news.Places{}, callError("get roles", err, "")
	}
	channels, err := restClient.GetGuildChannels(snowflake.ID(guildID), rest.WithCtx(readCtx))
	if err != nil {
		return news.Places{}, callError("get guild channels", err, "")
	}
	idx := newServerIndex(guildID, 0)
	for _, r := range roles {
		idx.addRole(r.Name, r.ID)
	}
	for _, ch := range channels {
		idx.addChannel(ch.Name(), ch.ID())
	}
	announcement, err := idx.lookupChannel(names.Announcement)
	if err != nil {
		return news.Places{}, fmt.Errorf("announcement channel: %w", err)
	}
	role, err := idx.lookupRole(names.Role)
	if err != nil {
		return news.Places{}, fmt.Errorf("release role: %w", err)
	}
	places := news.Places{
		Announcement: news.Place{Channel: announcement.String(), Role: role.String()},
		Channels:     make(map[string]string, len(names.Channels)),
	}
	for id, name := range names.Channels {
		if ch, err := idx.lookupChannel(name); err == nil {
			places.Channels[id] = ch.String()
		}
	}
	return places, nil
}

// runNews runs the news function until ctx ends.
func runNews(ctx context.Context, restClient rest.Rest, cfg BotConfig, opts clientOptions) {
	cfg.News.wire(restClient, cfg.GuildID, opts.callTimeout())
	if cfg.News.Runner.Logger == nil {
		cfg.News.Runner.Logger = cfg.Logger
	}
	if err := cfg.News.Runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		cfg.Logger.Warn("news stopped", slog.Any("error", err))
	}
}
