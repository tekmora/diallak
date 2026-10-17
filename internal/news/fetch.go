package news

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// ReadTimeout is the deadline of one feed read (ADR-0019, "What is kept").
	ReadTimeout = 10 * time.Second
	// MaxRedirects bounds the redirects one read follows. Source: chosen, not measured.
	MaxRedirects = 3
	// userAgent names the bot to the feed's server.
	userAgent = "diallak-news (+https://github.com/tekmora/diallak)"
	accept    = "application/atom+xml, application/rss+xml, application/xml;q=0.9, text/xml;q=0.8"
)

// StatusError reports a feed server that answered with a status other than 200.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("feed server returned status %d", e.Code)
}

// ErrTooLarge reports a feed larger than MaxFeedBytes.
var ErrTooLarge = fmt.Errorf("feed exceeds max %d bytes", MaxFeedBytes)

// Reader reads feeds over HTTPS.
type Reader struct {
	client *http.Client
}

// NewReader returns a Reader. If hc is nil, the Reader makes its own client: it follows at most
// MaxRedirects redirects, each to an https address, and gives up after ReadTimeout. A caller that
// passes hc decides that client's redirect policy; Fetch still sets its own deadline.
func NewReader(hc *http.Client) *Reader {
	if hc == nil {
		hc = &http.Client{CheckRedirect: checkRedirect, Timeout: ReadTimeout}
	}
	return &Reader{client: hc}
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

// Fetch reads the feed at address and returns its items. The read ends after ReadTimeout or when
// ctx ends, whichever comes first, and takes at most MaxFeedBytes.
func (r *Reader) Fetch(ctx context.Context, address string) ([]Item, error) {
	if err := ValidFeedAddress(address); err != nil {
		return nil, err
	}
	data, err := r.get(ctx, address)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func (r *Reader) get(ctx context.Context, address string) (_ []byte, err error) {
	reqCtx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close response body: %w", closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Code: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxFeedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if len(data) > MaxFeedBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}
