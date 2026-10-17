package main

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/discord"
	"github.com/tekmora/diallak/internal/ghapp"
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
	discSrc, discReader, err := buildDiscussions(cfg, def)
	if err != nil {
		return nil, err
	}
	if discReader != nil {
		sources = append(sources, discSrc)
	}
	return &discord.NewsConfig{
		Runner: &news.Runner{
			Store:       store,
			Reader:      reader,
			Discussions: discReader,
			Sources:     sources,
			Logger:      slog.Default(),
		},
		Names: names,
	}, nil
}

// buildDiscussions wires the GitHub Discussions reader and source when configured (issue #99).
func buildDiscussions(cfg config.Config, def *serverdef.Definition) (news.Source, news.DiscussionReader, error) {
	if !cfg.HasDiscussions() {
		return news.Source{}, nil, nil
	}
	if def.News.DiscussionsChannel == "" {
		return news.Source{}, nil, errors.New("news: discussions configured without serverdef discussions_channel")
	}
	key, err := ghapp.ParseKey([]byte(cfg.DiscussionsAppKey))
	if err != nil {
		return news.Source{}, nil, fmt.Errorf("news: parse discussions app key: %w", err)
	}
	app := &ghapp.App{
		ClientID: cfg.DiscussionsAppClientID,
		Key:      key,
	}
	reader := &news.GitHubDiscussions{
		App:    app,
		Logger: slog.Default(),
	}
	src := news.Source{
		ID:       "discussions",
		Kind:     news.SourceDiscussions,
		Interval: news.DiscussionInterval,
	}
	return src, reader, nil
}
