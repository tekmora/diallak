package news

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/tekmora/diallak/internal/ghapp"
)

const (
	// ReadTimeout is the deadline of one feed read (ADR-0019, "What is kept").
	ReadTimeout = 10 * time.Second
	// MaxRedirects bounds the redirects one read follows. Source: chosen, not measured.
	MaxRedirects = 3
	// MinBackoff and MaxBackoff bound the wait after a feed's server asked to slow down; they are
	// the GitHub client's bounds, since both follow the same rule (ghapp.SlowDown).
	MinBackoff = ghapp.MinBackoff
	MaxBackoff = ghapp.MaxBackoff
	// maxValidatorBytes bounds a stored ETag or Last-Modified value; a longer one is not kept.
	maxValidatorBytes = 1024
	// userAgent names the bot to the feed's server.
	userAgent = "diallak-news (+https://github.com/tekmora/diallak)"
	accept    = "application/atom+xml, application/rss+xml, application/xml;q=0.9, text/xml;q=0.8"
)

// StatusError reports a feed server that answered with a status other than 200 or 304.
type StatusError struct {
	Code int
	// RetryAfter is how long the server asked the reader to wait, between MinBackoff and
	// MaxBackoff; zero when the answer did not ask to slow down.
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("feed server returned status %d, retry after %v", e.Code, e.RetryAfter)
	}
	return fmt.Sprintf("feed server returned status %d", e.Code)
}

// ErrTooLarge reports a feed larger than MaxFeedBytes.
var ErrTooLarge = fmt.Errorf("feed exceeds max %d bytes", MaxFeedBytes)

// Validators are what a feed's server sent to name the version it served (RFC 9110, sections 8.8.2
// and 8.8.3). A Reader sends them back, so an unchanged feed is answered with 304 and no body.
type Validators struct {
	ETag         string
	LastModified string
}

// Result is one read of a feed.
type Result struct {
	// Items is empty when NotModified is true.
	Items []Item
	// NotModified is true when the server answered 304: nothing changed since the read that
	// returned the Validators passed in.
	NotModified bool
	// Validators are the ones to keep for the next read. Empty fields make that read unconditional.
	Validators Validators
}

// Reader reads feeds over HTTPS.
type Reader struct {
	client *http.Client
	now    func() time.Time
	// apiToken is sent with advisory reads to apiHost only; empty sends none.
	apiToken string
	apiHost  string
}

// NewReader returns a Reader. If hc is nil, the Reader makes its own client: it follows at most
// MaxRedirects redirects, each to an https address, and gives up after ReadTimeout. A caller that
// passes hc decides that client's redirect policy; Fetch still sets its own deadline.
func NewReader(hc *http.Client) *Reader {
	if hc == nil {
		hc = &http.Client{CheckRedirect: checkRedirect, Timeout: ReadTimeout}
	}
	return &Reader{client: hc, now: time.Now, apiHost: apiHost}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > MaxRedirects {
		return fmt.Errorf("more than %d redirects", MaxRedirects)
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("redirect to scheme %q, want https", req.URL.Scheme)
	}
	return nil
}

// ValidFeedAddress accepts an absolute https address with a host and without user information.
func ValidFeedAddress(address string) error {
	u, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("parse feed address: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("feed address must be absolute https without user information")
	}
	return nil
}

// Fetch reads the feed at address. It sends prev back as If-None-Match and If-Modified-Since, so
// an unchanged feed ends the read with NotModified. The read ends after ReadTimeout or when ctx
// ends, whichever comes first, and takes at most MaxFeedBytes. A server that asks to slow down
// yields a *StatusError whose RetryAfter the caller waits before the next read.
func (r *Reader) Fetch(ctx context.Context, address string, prev Validators) (Result, error) {
	if err := ValidFeedAddress(address); err != nil {
		return Result{}, err
	}
	data, res, err := r.get(ctx, address, prev, feedHeaders)
	if err != nil || res.NotModified {
		return res, err
	}
	items, err := Parse(data)
	if err != nil {
		return Result{}, err
	}
	res.Items = items
	return res, nil
}

// feedHeaders are the request headers of a feed read.
var feedHeaders = http.Header{"Accept": {accept}}

// get reads address with the request headers h and prev as validators.
func (r *Reader) get(ctx context.Context, address string, prev Validators, h http.Header) (_ []byte, _ Result, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, address, nil)
	if err != nil {
		return nil, Result{}, fmt.Errorf("create request: %w", err)
	}
	setRequestHeaders(req.Header, prev, h)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, Result{}, fmt.Errorf("do request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close response body: %w", closeErr))
		}
	}()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		return nil, Result{NotModified: true, Validators: keptValidators(prev, resp.Header)}, nil
	default:
		return nil, Result{}, &StatusError{Code: resp.StatusCode, RetryAfter: ghapp.SlowDown(resp.StatusCode, resp.Header, r.now())}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxFeedBytes+1))
	if err != nil {
		return nil, Result{}, fmt.Errorf("read response body: %w", err)
	}
	if len(data) > MaxFeedBytes {
		return nil, Result{}, ErrTooLarge
	}
	return data, Result{Validators: keptValidators(Validators{}, resp.Header)}, nil
}

func setRequestHeaders(h http.Header, prev Validators, extra http.Header) {
	h.Set("User-Agent", userAgent)
	for k, v := range extra {
		h[k] = slices.Clone(v)
	}
	if v := validator(prev.ETag); v != "" {
		h.Set("If-None-Match", v)
	}
	if v := validator(prev.LastModified); v != "" {
		h.Set("If-Modified-Since", v)
	}
}

// keptValidators returns the validators of resp, each falling back to base when resp sent none.
// A 304 repeats the ETag but should leave out Last-Modified (RFC 9110, section 15.4.5).
func keptValidators(base Validators, h http.Header) Validators {
	out := Validators{ETag: validator(base.ETag), LastModified: validator(base.LastModified)}
	if v := validator(h.Get("ETag")); v != "" {
		out.ETag = v
	}
	if v := validator(h.Get("Last-Modified")); v != "" {
		out.LastModified = v
	}
	return out
}

// validator returns v if it is short and printable ASCII, else "", which makes the next read
// unconditional instead of sending a value the reader cannot vouch for.
func validator(v string) string {
	if len(v) > maxValidatorBytes {
		return ""
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7e {
			return ""
		}
	}
	return v
}
