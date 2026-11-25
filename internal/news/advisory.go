package news

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// ReleaseInterval is how often an own repository's releases are read (ADR-0019, point 1).
	ReleaseInterval = 10 * time.Minute
	// AdvisoryInterval is how often an own repository's security advisories are read (ADR-0019,
	// point 1).
	AdvisoryInterval = 15 * time.Minute
	// UnauthenticatedHourlyRequests is GitHub's REST budget an hour per address without a token:
	// the x-ratelimit-limit header read on 2026-11-23 (ADR-0032).
	UnauthenticatedHourlyRequests = 60
	// MaxRepositoriesWithoutToken keeps unauthenticated advisory reads within that budget
	// (ADR-0032). Four reads an hour per repository use 40 at this cap, leaving 20 for retries
	// and other clients behind the same address. Atom reads used none of the REST core budget
	// in the same measurement.
	MaxRepositoriesWithoutToken = 10
	// advisoryAccept is the media type of GitHub's REST answers.
	advisoryAccept = "application/vnd.github+json"
	// apiVersion pins GitHub's REST API version; the fields read here are unchanged in it
	// (github/docs, "Breaking changes", version 2026-03-10, read on 2026-10-17).
	apiVersion = "2026-03-10"
)

// apiHost is the only host an advisory read sends the API token to.
const apiHost = "api.github.com"

// advisoryHeaders are the request headers of an advisory read without a token (ADR-0019, point
// 1).
var advisoryHeaders = http.Header{"Accept": {advisoryAccept}, "X-Github-Api-Version": {apiVersion}}

// UseAPITokenSource sets the source of the current token for advisory reads to the API host.
// A nil source allows unauthenticated reads. A configured source returns a non-empty token or
// an error safe to log. Set the source before using the Reader.
func (r *Reader) UseAPITokenSource(source func() (string, error)) {
	r.apiTokenSource = source
}

// advisoryRequestHeaders returns the headers of an advisory read of address, with the token when
// address is on the API host.
func (r *Reader) advisoryRequestHeaders(address string) (http.Header, error) {
	if r.apiTokenSource == nil {
		return advisoryHeaders, nil
	}
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("parse advisory address: %w", err)
	}
	if u.Host != r.apiHost {
		return advisoryHeaders, nil
	}
	token, err := r.apiTokenSource()
	if err != nil {
		return nil, err
	}
	h := advisoryHeaders.Clone()
	h.Set("Authorization", "Bearer "+token)
	return h, nil
}

// ReleasesAddress returns the Atom feed of repo's releases; repo is "owner/name".
func ReleasesAddress(repo string) string {
	return "https://github.com/" + repo + "/releases.atom"
}

// AdvisoriesAddress returns the REST address of repo's published security advisories; repo is
// "owner/name".
func AdvisoriesAddress(repo string) string {
	return "https://api.github.com/repos/" + repo + "/security-advisories?state=published"
}

// FetchAdvisories reads the published security advisories at address, under the same rules as
// Fetch: validators sent back, ReadTimeout, MaxFeedBytes, and a *StatusError whose RetryAfter
// follows the server's rate-limit headers.
func (r *Reader) FetchAdvisories(ctx context.Context, address string, prev Validators) (Result, error) {
	if err := ValidFeedAddress(address); err != nil {
		return Result{}, err
	}
	headers, err := r.advisoryRequestHeaders(address)
	if err != nil {
		return Result{}, fmt.Errorf("load advisory API token: %w", err)
	}
	data, res, err := r.get(ctx, address, prev, headers)
	if err != nil || res.NotModified {
		return res, err
	}
	items, err := ParseAdvisories(data)
	if err != nil {
		return Result{}, err
	}
	res.Items = items
	return res, nil
}

type restAdvisory struct {
	GHSAID      string  `json:"ghsa_id"`
	HTMLURL     string  `json:"html_url"`
	Summary     string  `json:"summary"`
	PublishedAt string  `json:"published_at"`
	WithdrawnAt *string `json:"withdrawn_at"`
}

// ParseAdvisories reads GitHub's list of repository security advisories. Each published one
// becomes an item: its GHSA id, its summary as plain text, its page and its publication date. A
// withdrawn advisory, and one without an https page, is left out; at most MaxItems are kept.
func ParseAdvisories(data []byte) ([]Item, error) {
	if len(data) > MaxFeedBytes {
		return nil, fmt.Errorf("advisory list of %d bytes exceeds max %d", len(data), MaxFeedBytes)
	}
	var list []restAdvisory
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode advisory list: %w", err)
	}
	out := make([]Item, 0, min(len(list), MaxItems))
	for i := 0; i < len(list) && len(out) < MaxItems; i++ {
		a := list[i]
		if a.WithdrawnAt != nil && strings.TrimSpace(*a.WithdrawnAt) != "" {
			continue
		}
		if it, ok := newItem(a.GHSAID, a.Summary, a.HTMLURL, parseTime(a.PublishedAt, time.RFC3339)); ok {
			out = append(out, it)
		}
	}
	return out, nil
}
