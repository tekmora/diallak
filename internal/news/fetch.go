package news

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// ReadTimeout is the deadline of one feed read (ADR-0019, "What is kept").
	ReadTimeout = 10 * time.Second
	// MaxRedirects bounds the redirects one read follows. Source: chosen, not measured.
	MaxRedirects = 3
	// MinBackoff is the shortest wait after a server asked to slow down: GitHub asks for "at least
	// one minute" when it names no time (github/docs dae83f7d28f3, "Exceeding the rate limit").
	MinBackoff = time.Minute
	// MaxBackoff caps the wait a server can ask for. Source: chosen, not measured.
	MaxBackoff = 24 * time.Hour
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
}

// NewReader returns a Reader. If hc is nil, the Reader makes its own client: it follows at most
// MaxRedirects redirects, each to an https address, and gives up after ReadTimeout. A caller that
// passes hc decides that client's redirect policy; Fetch still sets its own deadline.
func NewReader(hc *http.Client) *Reader {
	if hc == nil {
		hc = &http.Client{CheckRedirect: checkRedirect, Timeout: ReadTimeout}
	}
	return &Reader{client: hc, now: time.Now}
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
	data, res, err := r.get(ctx, address, prev)
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

func (r *Reader) get(ctx context.Context, address string, prev Validators) (_ []byte, _ Result, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, address, nil)
	if err != nil {
		return nil, Result{}, fmt.Errorf("create request: %w", err)
	}
	setRequestHeaders(req.Header, prev)

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
		return nil, Result{}, &StatusError{Code: resp.StatusCode, RetryAfter: slowDown(resp.StatusCode, resp.Header, r.now())}
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

func setRequestHeaders(h http.Header, prev Validators) {
	h.Set("User-Agent", userAgent)
	h.Set("Accept", accept)
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

// slowDown returns how long to wait after an answer with status code and headers h, or zero when
// the answer does not ask to slow down. It follows GitHub's rule for 403 and 429 (github/docs
// dae83f7d28f3, "Exceeding the rate limit") and RFC 9110, section 10.2.3, for 503.
func slowDown(code int, h http.Header, now time.Time) time.Duration {
	if code != http.StatusTooManyRequests && code != http.StatusServiceUnavailable && code != http.StatusForbidden {
		return 0
	}
	if d, ok := retryAfter(h.Get("Retry-After"), now); ok {
		return clampBackoff(d)
	}
	if h.Get("X-Ratelimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(h.Get("X-Ratelimit-Reset"), 10, 64); err == nil {
			return clampBackoff(time.Unix(reset, 0).Sub(now))
		}
		return MinBackoff
	}
	if code == http.StatusForbidden {
		return 0
	}
	return MinBackoff
}

// retryAfter reads a Retry-After value: delay-seconds or an HTTP date (RFC 9110, section 10.2.3).
func retryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseUint(v, 10, 32); err == nil {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return t.Sub(now), true
	}
	return 0, false
}

func clampBackoff(d time.Duration) time.Duration {
	return min(max(d, MinBackoff), MaxBackoff)
}
