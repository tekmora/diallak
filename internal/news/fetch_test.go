package news

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testReader returns a Reader that trusts srv's certificate and keeps the production redirect rule.
func testReader(srv *httptest.Server) *Reader {
	client := srv.Client()
	client.CheckRedirect = checkRedirect
	return NewReader(client)
}

func serveBytes(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		if _, err := w.Write(body); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestFetch_Positive_ReadsFeedAndSendsHeaders(t *testing.T) {
	body := readTestdata(t, "releases.atom")
	headers := make(chan [2]string, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- [2]string{r.Header.Get("User-Agent"), r.Header.Get("Accept")}
		if _, err := w.Write(body); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	items, err := testReader(srv).Fetch(testContext(t), srv.URL+"/releases.atom")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(items) != 10 {
		t.Fatalf("Fetch() = %d items, want 10", len(items))
	}
	if got := <-headers; got != [2]string{userAgent, accept} {
		t.Fatalf("headers = %q, want %q, %q", got, userAgent, accept)
	}
}

func TestFetch_Negative_StatusOtherThanOK(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	_, err := testReader(srv).Fetch(testContext(t), srv.URL)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusNotFound {
		t.Fatalf("Fetch() error = %v, want StatusError 404", err)
	}
}

func TestFetch_Negative_AddressRefusedBeforeAnyRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)

	for _, address := range []string{srv.URL, "https://user:pw@example.org/feed", "/relative", "https://", "javascript:alert(1)"} {
		if _, err := NewReader(srv.Client()).Fetch(testContext(t), address); err == nil {
			t.Fatalf("Fetch(%q) error = nil, want refusal", address)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server saw %d requests, want 0", n)
	}
}

func TestFetch_Negative_RedirectToHTTPRefused(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("plain http server was reached")
	}))
	t.Cleanup(plain.Close)
	srv := httptest.NewTLSServer(http.RedirectHandler(plain.URL, http.StatusFound))
	t.Cleanup(srv.Close)

	if _, err := testReader(srv).Fetch(testContext(t), srv.URL); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("Fetch() error = %v, want a refused redirect", err)
	}
}

func TestFetch_Negative_RedirectLoopEnds(t *testing.T) {
	var srv *httptest.Server
	var hits atomic.Int32
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	if _, err := testReader(srv).Fetch(testContext(t), srv.URL); err == nil {
		t.Fatal("Fetch() error = nil, want redirect limit")
	}
	if n := hits.Load(); n != MaxRedirects+1 {
		t.Fatalf("server saw %d requests, want %d", n, MaxRedirects+1)
	}
}

func TestFetch_Negative_HangingServerEndsAtDeadline(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Answers after 2 s at the latest, so a Fetch that ignores its deadline fails instead of hanging.
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := testReader(srv).Fetch(ctx, srv.URL)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Fetch() error = %v, want context.DeadlineExceeded", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("Fetch() took %v, want it to end near the deadline", took)
	}
}

func TestFetch_Negative_OversizedBody(t *testing.T) {
	srv := serveBytes(t, bytes.Repeat([]byte(" "), MaxFeedBytes+1))

	if _, err := testReader(srv).Fetch(testContext(t), srv.URL); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Fetch() error = %v, want ErrTooLarge", err)
	}
}

func TestFetch_Boundary_BodyOfExactlyMaxFeedBytes(t *testing.T) {
	feed := rssWithItems(1)
	body := append(feed, bytes.Repeat([]byte(" "), MaxFeedBytes-len(feed))...)
	srv := serveBytes(t, body)

	items, err := testReader(srv).Fetch(testContext(t), srv.URL)
	if err != nil {
		t.Fatalf("Fetch(%d bytes) error = %v", len(body), err)
	}
	if len(items) != 1 {
		t.Fatalf("Fetch() = %d items, want 1", len(items))
	}
}

func TestNewReader_Boundary_DefaultClientHasDeadlineAndRedirectRule(t *testing.T) {
	r := NewReader(nil)
	if r.client.Timeout != ReadTimeout || r.client.CheckRedirect == nil {
		t.Fatalf("default client = timeout %v, redirect rule set %v; want %v, true", r.client.Timeout, r.client.CheckRedirect != nil, ReadTimeout)
	}
}
