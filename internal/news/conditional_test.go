package news

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// conditionalServer serves one feed version and answers 304 the way RFC 9110, section 13.1.2 and
// 13.1.3, asks: If-None-Match decides when present, If-Modified-Since only without it.
type conditionalServer struct {
	t            *testing.T
	mu           sync.Mutex
	body         []byte
	etag         string
	lastModified string
	requests     []http.Header
}

func (s *conditionalServer) set(body []byte, etag, lastModified string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body, s.etag, s.lastModified = body, etag, lastModified
}

func (s *conditionalServer) request(i int) http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[i]
}

func (s *conditionalServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.Header.Clone())
	if s.etag != "" {
		w.Header().Set("ETag", s.etag)
	}
	inm, ims := r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")
	if (inm != "" && inm == s.etag) || (inm == "" && ims != "" && ims == s.lastModified) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if s.lastModified != "" {
		w.Header().Set("Last-Modified", s.lastModified)
	}
	if _, err := w.Write(s.body); err != nil {
		s.t.Errorf("write body: %v", err)
	}
}

func startConditional(t *testing.T, body []byte, etag, lastModified string) (*conditionalServer, *httptest.Server) {
	t.Helper()
	cs := &conditionalServer{t: t}
	cs.set(body, etag, lastModified)
	srv := httptest.NewTLSServer(cs)
	t.Cleanup(srv.Close)
	return cs, srv
}

const testLastModified = "Sat, 17 Oct 2026 08:00:00 GMT"

func TestFetch_Positive_UnchangedFeedEndsWithNotModified(t *testing.T) {
	cs, srv := startConditional(t, readTestdata(t, "releases.atom"), `W/"v1"`, testLastModified)
	r := testReader(srv)

	first, err := r.Fetch(testContext(t), srv.URL, Validators{})
	if err != nil || len(first.Items) != 10 {
		t.Fatalf("first Fetch() = %d items, %v; want 10, nil", len(first.Items), err)
	}
	if want := (Validators{ETag: `W/"v1"`, LastModified: testLastModified}); first.Validators != want {
		t.Fatalf("first validators = %+v, want %+v", first.Validators, want)
	}

	second, err := r.Fetch(testContext(t), srv.URL, first.Validators)
	if err != nil {
		t.Fatalf("second Fetch() error = %v", err)
	}
	if !second.NotModified || len(second.Items) != 0 || second.Validators != first.Validators {
		t.Fatalf("second Fetch() = %+v, want not modified, no items, the same validators", second)
	}
	h := cs.request(1)
	if h.Get("If-None-Match") != `W/"v1"` || h.Get("If-Modified-Since") != testLastModified {
		t.Fatalf("second request sent If-None-Match %q, If-Modified-Since %q", h.Get("If-None-Match"), h.Get("If-Modified-Since"))
	}
}

func TestFetch_Positive_ChangedFeedIsReadAgain(t *testing.T) {
	cs, srv := startConditional(t, rssWithItems(1), `"v1"`, testLastModified)
	r := testReader(srv)
	first, err := r.Fetch(testContext(t), srv.URL, Validators{})
	if err != nil {
		t.Fatalf("first Fetch() error = %v", err)
	}

	cs.set(rssWithItems(3), `"v2"`, "Sat, 17 Oct 2026 09:00:00 GMT")
	second, err := r.Fetch(testContext(t), srv.URL, first.Validators)
	if err != nil {
		t.Fatalf("second Fetch() error = %v", err)
	}
	if second.NotModified || len(second.Items) != 3 {
		t.Fatalf("second Fetch() = %d items, not modified %v; want 3, false", len(second.Items), second.NotModified)
	}
	if want := (Validators{ETag: `"v2"`, LastModified: "Sat, 17 Oct 2026 09:00:00 GMT"}); second.Validators != want {
		t.Fatalf("second validators = %+v, want %+v", second.Validators, want)
	}
}

func TestFetch_Negative_ServerWithoutValidatorsIsReadInFull(t *testing.T) {
	cs, srv := startConditional(t, rssWithItems(2), "", "")
	r := testReader(srv)
	for i := range 2 {
		res, err := r.Fetch(testContext(t), srv.URL, Validators{})
		if err != nil || res.NotModified || len(res.Items) != 2 || res.Validators != (Validators{}) {
			t.Fatalf("Fetch() #%d = %+v, %v; want 2 items, no validators", i, res, err)
		}
		h := cs.request(i)
		if h.Get("If-None-Match") != "" || h.Get("If-Modified-Since") != "" {
			t.Fatalf("request #%d sent a condition without validators: %v", i, h)
		}
	}
}

func TestFetch_Boundary_NotModifiedKeepsLastModifiedTheServerLeftOut(t *testing.T) {
	_, srv := startConditional(t, rssWithItems(1), `"v1"`, testLastModified)
	prev := Validators{ETag: `"v1"`, LastModified: testLastModified}

	res, err := testReader(srv).Fetch(testContext(t), srv.URL, prev)
	if err != nil || !res.NotModified {
		t.Fatalf("Fetch() = %+v, %v; want not modified", res, err)
	}
	if res.Validators != prev {
		t.Fatalf("validators = %+v, want %+v kept", res.Validators, prev)
	}
}

func TestFetch_Boundary_ValidatorLengthLimit(t *testing.T) {
	for _, tc := range []struct {
		n    int
		kept bool
	}{{maxValidatorBytes, true}, {maxValidatorBytes + 1, false}} {
		etag := `"` + strings.Repeat("a", tc.n-2) + `"`
		_, srv := startConditional(t, rssWithItems(1), etag, "")
		res, err := testReader(srv).Fetch(testContext(t), srv.URL, Validators{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := res.Validators.ETag == etag; got != tc.kept {
			t.Fatalf("ETag of %d bytes kept = %v, want %v", tc.n, got, tc.kept)
		}
	}
}

func TestValidator_Negative_ControlAndNonASCIIRefused(t *testing.T) {
	for _, v := range []string{"\"a\r\nX-Injected: 1\"", "\"a\x00\"", "\"ä\""} {
		if got := validator(v); got != "" {
			t.Fatalf("validator(%q) = %q, want empty", v, got)
		}
	}
	if got := validator(`W/"ok"`); got != `W/"ok"` {
		t.Fatalf("validator(W/\"ok\") = %q, want it kept", got)
	}
}

func TestFetch_Positive_TooManyRequestsCarriesRetryAfter(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	_, err := testReader(srv).Fetch(testContext(t), srv.URL, Validators{})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusTooManyRequests || se.RetryAfter != 2*time.Minute {
		t.Fatalf("Fetch() error = %v, want StatusError 429 with RetryAfter 2m", err)
	}
}
