package main

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/discord"
	"github.com/tekmora/diallak/internal/news"
	"github.com/tekmora/diallak/internal/serverdef"
)

// newNews builds the news runner when its state file is configured (ADR-0019, issue #180): the own
// repositories' releases and advisories and the definition's feeds, each to its place.
func newNews(cfg config.Config, def *serverdef.Definition) (*discord.NewsConfig, error) {
	if !cfg.HasNews() {
		return nil, nil
	}
	if def.News == nil {
		return nil, errors.New("news: the definition has no news block")
	}
	names, ok := def.NewsNames()
	if !ok {
		return nil, errors.New("news: the definition lacks the announcement channel or the release role")
	}
	store, err := news.OpenFileStore(cfg.NewsStateFile)
	if err != nil {
		return nil, fmt.Errorf("news: open state: %w", err)
	}
	reader := news.NewReader(nil)
	reader.UseAPIToken(cfg.NewsGitHubToken)
	sources := news.OwnSources(def.News.Repositories)
	for _, f := range def.News.Feeds {
		ch, _ := def.FeedChannel(f.ID)
		sources = append(sources, news.Source{
			ID: f.ID, Address: f.URL, Kind: news.SourceFeed, Interval: news.FeedInterval, Channel: ch,
		})
	}
	return &discord.NewsConfig{
		Runner: &news.Runner{Store: store, Reader: reader, Sources: sources, Logger: slog.Default()},
		Names:  names,
	}, nil
}
