package news

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/ghapp"
)

const (
	// MaxDiscussionRepos caps the repositories whose discussions are read in one round.
	// Source: chosen, not measured.
	MaxDiscussionRepos = 10
	// DiscussionInterval is how often discussions are read. ADR-0004 open decision settled with #99,
	// chosen not measured; same as advisories.
	DiscussionInterval = 15 * time.Minute
)

// DiscussionReader reads discussion items from forge repositories.
type DiscussionReader interface {
	ReadDiscussions(ctx context.Context) ([]Item, error)
}

// GitHubDiscussions reads new discussions from visible repositories via GitHub's GraphQL API.
type GitHubDiscussions struct {
	App    *ghapp.App
	Logger *slog.Logger
}

type installationRepo struct {
	FullName       string `json:"full_name"`
	Private        bool   `json:"private"`
	HasDiscussions bool   `json:"has_discussions"`
}

type installationReposResponse struct {
	Repositories []installationRepo `json:"repositories"`
}

type gqlDiscussionNode struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
	Category  struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"category"`
}

type gqlResponse struct {
	Data struct {
		Repository *struct {
			Discussions struct {
				Nodes []gqlDiscussionNode `json:"nodes"`
			} `json:"discussions"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// ReadDiscussions reads new discussions from visible public repositories that have discussions enabled.
func (g *GitHubDiscussions) ReadDiscussions(ctx context.Context) ([]Item, error) {
	if g.App == nil {
		return nil, errors.New("news: github app is nil")
	}
	repos, err := g.fetchRepos(ctx)
	if err != nil {
		return nil, err
	}
	var items []Item
	for i := 0; i < len(repos) && i < MaxDiscussionRepos; i++ {
		rItems, err := g.queryRepo(ctx, repos[i].FullName)
		if err != nil {
			return nil, err
		}
		items = append(items, rItems...)
	}
	slices.SortFunc(items, func(a, b Item) int {
		if c := b.Published.Compare(a.Published); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(items) > MaxItems {
		items = items[:MaxItems]
	}
	return items, nil
}

func (g *GitHubDiscussions) fetchRepos(ctx context.Context) ([]installationRepo, error) {
	var resp installationReposResponse
	if err := g.App.Do(ctx, "GET", "/installation/repositories?per_page=100", nil, &resp); err != nil {
		return nil, fmt.Errorf("news: fetch visible repos: %w", err)
	}
	var kept []installationRepo
	for _, r := range resp.Repositories {
		if !r.Private && r.HasDiscussions {
			kept = append(kept, r)
		}
	}
	slices.SortFunc(kept, func(a, b installationRepo) int {
		return strings.Compare(a.FullName, b.FullName)
	})
	if len(kept) > MaxDiscussionRepos {
		g.logger().Warn("news: discussion repos capped", slog.Int("total", len(kept)), slog.Int("kept", MaxDiscussionRepos))
		kept = kept[:MaxDiscussionRepos]
	}
	return kept, nil
}

func (g *GitHubDiscussions) queryRepo(ctx context.Context, fullName string) ([]Item, error) {
	owner, name, ok := strings.Cut(fullName, "/")
	if !ok {
		return nil, fmt.Errorf("news: invalid repo name %q", fullName)
	}
	reqBody := struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}{
		Query:     `query($o:String!,$n:String!){ repository(owner:$o,name:$n){ discussions(first:20, orderBy:{field:CREATED_AT,direction:DESC}){ nodes{ id title url createdAt category{ slug name } } } } }`,
		Variables: map[string]string{"o": owner, "n": name},
	}
	var resp gqlResponse
	if err := g.App.Do(ctx, "POST", "/graphql", reqBody, &resp); err != nil {
		return nil, fmt.Errorf("news: query repo discussions %s: %w", fullName, err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("news: graphql error for %s: %s", fullName, resp.Errors[0].Message)
	}
	if resp.Data.Repository == nil {
		return nil, nil
	}
	return g.parseNodes(fullName, resp.Data.Repository.Discussions.Nodes), nil
}

func (g *GitHubDiscussions) parseNodes(fullName string, nodes []gqlDiscussionNode) []Item {
	var items []Item
	for i := 0; i < len(nodes) && i < 20; i++ {
		node := nodes[i]
		if node.Category.Slug != "q-a" && node.Category.Slug != "ideas" {
			continue
		}
		if !validDiscussionURL(fullName, node.URL) {
			continue
		}
		title := PlainText(node.Title, MaxTitleRunes)
		if title == "" {
			continue
		}
		catName := PlainText(node.Category.Name, MaxTitleRunes)
		items = append(items, Item{
			ID:        node.ID,
			Title:     title,
			Link:      node.URL,
			Published: parseTime(node.CreatedAt, time.RFC3339, time.RFC3339Nano),
			Category:  catName,
		})
	}
	return items
}

func validDiscussionURL(repo, rawURL string) bool {
	prefix := "https://github.com/" + repo + "/discussions/"
	rest, ok := strings.CutPrefix(rawURL, prefix)
	if !ok || rest == "" {
		return false
	}
	for i := 0; i < len(rest) && i < 20; i++ {
		if rest[i] < '0' || rest[i] > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(rest, 10, 64)
	return err == nil && n > 0
}

func (g *GitHubDiscussions) logger() *slog.Logger {
	if g.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return g.Logger
}

// DiscussionMessage formats the message for a new discussion post.
// It returns false when the message would exceed MaxDigestRunes.
func DiscussionMessage(repo string, it Item) (string, bool) {
	var b strings.Builder
	fmt.Fprintf(&b, "New discussion in **%s** (%s): [%s](<%s>)",
		escapeMarkdown(repo), escapeMarkdown(it.Category), escapeMarkdown(it.Title), safeLink(it.Link))
	if !it.Published.IsZero() {
		fmt.Fprintf(&b, ", <t:%d>", it.Published.Unix())
	}
	s := b.String()
	return s, utf8.RuneCountInString(s) <= MaxDigestRunes
}

func discussionRepo(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	repo, _, _ := strings.Cut(path, "/discussions/")
	return repo
}
