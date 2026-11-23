package main

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

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
	if err := checkAdvisoryBudget(cfg, len(def.News.Repositories)); err != nil {
		return nil, err
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

// checkAdvisoryBudget stops the start when count own repositories need more unauthenticated
// advisory reads than fit GitHub's budget per address and no read-only token is set (ADR-0032).
func checkAdvisoryBudget(cfg config.Config, count int) error {
	if cfg.NewsGitHubToken != "" || count <= news.MaxRepositoriesWithoutToken {
		return nil
	}
	reads := count * int(time.Hour/news.AdvisoryInterval)
	return fmt.Errorf("news: %d own repositories need %d advisory reads an hour, and without a token "+
		"at most %d repositories fit GitHub's %d unauthenticated requests an hour per address; "+
		"set %s to the file of the read-only GitHub token",
		count, reads, news.MaxRepositoriesWithoutToken, news.UnauthenticatedHourlyRequests,
		config.EnvNewsGitHubTokenFile)
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
