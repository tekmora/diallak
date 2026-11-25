package news

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/config"
)

// A published advisory becomes an item with its GHSA id, its summary as plain text and its page;
// a withdrawn one and one without an https page are left out.
func TestParseAdvisories_Positive_PublishedAdvisoryIsAnItem(t *testing.T) {
	items, err := ParseAdvisories(readTestdata(t, "advisories.json"))
	if err != nil {
		t.Fatalf("ParseAdvisories() error = %v", err)
	}
	want := Item{
		ID:        "GHSA-aaaa-bbbb-cccc",
		Title:     "Token leak in the log",
		Link:      "https://github.com/tekmora/diallak/security/advisories/GHSA-aaaa-bbbb-cccc",
		Published: time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC),
	}
	if len(items) != 1 || items[0] != want {
		t.Fatalf("ParseAdvisories() = %+v, want [%+v]", items, want)
	}
}

func TestParseAdvisories_Negative_NotAList(t *testing.T) {
	for name, body := range map[string]string{
		"object": `{"message":"API rate limit exceeded"}`,
		"broken": `[{"ghsa_id":`,
		"empty":  ``,
	} {
		if _, err := ParseAdvisories([]byte(body)); err == nil {
			t.Errorf("%s: ParseAdvisories() error = nil, want one", name)
		}
	}
}

func TestParseAdvisories_Boundary_EmptyListAndItemCap(t *testing.T) {
	items, err := ParseAdvisories([]byte(`[]`))
	if err != nil || len(items) != 0 {
		t.Fatalf("ParseAdvisories([]) = %v, %v; want no items, no error", items, err)
	}
	var b strings.Builder
	b.WriteString("[")
	for i := range MaxItems + 1 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"ghsa_id":"GHSA-` + strings.Repeat("x", i%7+1) + `-` + strconv.Itoa(i) +
			`","html_url":"https://example.org/a` + strconv.Itoa(i) + `","summary":"s","published_at":"2026-10-16T09:00:00Z"}`)
	}
	b.WriteString("]")
	items, err = ParseAdvisories([]byte(b.String()))
	if err != nil || len(items) != MaxItems {
		t.Fatalf("ParseAdvisories(%d) = %d items, %v; want %d", MaxItems+1, len(items), err, MaxItems)
	}
}

// The advisory read asks for GitHub's JSON with the pinned API version and sends no token.
func TestFetchAdvisories_Positive_SendsAPIHeaders(t *testing.T) {
	body := readTestdata(t, "advisories.json")
	got := make(chan http.Header, 1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Clone()
		if _, err := w.Write(body); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	res, err := testReader(srv).FetchAdvisories(testContext(t), srv.URL+"/repos/o/r/security-advisories", Validators{})
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("FetchAdvisories() = %d items, %v; want 1, nil", len(res.Items), err)
	}
	h := <-got
	if h.Get("Accept") != advisoryAccept || h.Get("X-GitHub-Api-Version") != apiVersion ||
		h.Get("User-Agent") != userAgent || h.Get("Authorization") != "" {
		t.Fatalf("headers = %v, want Accept %q, version %q, no Authorization", h, advisoryAccept, apiVersion)
	}
}

// A rate-limited advisory read tells the caller to wait until the limit resets.
func TestFetchAdvisories_Negative_RateLimitWaitsForReset(t *testing.T) {
	r, srv := limitedServer(t)
	_, err := r.FetchAdvisories(testContext(t), srv.URL+"/repos/o/r/security-advisories", Validators{})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden || se.RetryAfter != 30*time.Minute {
		t.Fatalf("FetchAdvisories() error = %v, want 403 retry after 30m", err)
	}
}

// A plain http address is refused before any request.
func TestFetchAdvisories_Negative_AddressNotHTTPS(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	if _, err := NewReader(srv.Client()).FetchAdvisories(testContext(t), srv.URL, Validators{}); err == nil {
		t.Fatal("FetchAdvisories(http) error = nil, want one")
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("server saw %d requests, want none", n)
	}
}

func TestAdvisoryRequestHeaders_Negative_UnparseableAddress(t *testing.T) {
	calls := 0
	r := NewReader(nil)
	r.UseAPITokenSource(func() (string, error) {
		calls++
		return "fixture-token", nil
	})
	headers, err := r.advisoryRequestHeaders("http://[::1")
	if err == nil {
		t.Error("advisoryRequestHeaders() error = nil, want a parse error")
	}
	if headers != nil {
		t.Error("advisoryRequestHeaders() returned headers, want nil")
	}
	if calls != 0 {
		t.Errorf("token source calls = %d, want none", calls)
	}
}

func TestAddresses_Positive_ReleasesAndAdvisories(t *testing.T) {
	if got, want := ReleasesAddress("tekmora/diallak"), "https://github.com/tekmora/diallak/releases.atom"; got != want {
		t.Errorf("ReleasesAddress() = %q, want %q", got, want)
	}
	if got, want := AdvisoriesAddress("tekmora/diallak"),
		"https://api.github.com/repos/tekmora/diallak/security-advisories?state=published"; got != want {
		t.Errorf("AdvisoriesAddress() = %q, want %q", got, want)
	}
}

// limitedServer answers like GitHub at the end of the hourly budget, 30 minutes before the reset.
func limitedServer(t *testing.T) (*Reader, *httptest.Server) {
	t.Helper()
	now := time.Date(2026, 10, 17, 17, 0, 0, 0, time.UTC)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Ratelimit-Remaining", "0")
		w.Header().Set("X-Ratelimit-Reset", strconv.Itoa(int(now.Add(30*time.Minute).Unix())))
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	r := testReader(srv)
	r.now = func() time.Time { return now }
	return r, srv
}

// authServer records the Authorization header of every request and answers an empty advisory list.
func authServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Get("Authorization"))
		mu.Unlock()
		if _, err := w.Write([]byte("[]")); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(got)
	}
}

// tokenReader returns a Reader that takes srv for the API host.
func tokenReader(srv *httptest.Server, source func() (string, error)) *Reader {
	r := testReader(srv)
	r.apiHost = srv.Listener.Addr().String()
	r.UseAPITokenSource(source)
	return r
}

func writeAdvisoryToken(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("write fixture token file: %v", err)
	}
}

func TestFetchAdvisories_Positive_RotatedTokenSentOnNextRead(t *testing.T) {
	t.Run("rewrite", func(t *testing.T) { testRotatedAdvisoryToken(t, false) })
	t.Run("replace", func(t *testing.T) { testRotatedAdvisoryToken(t, true) })
}

func testRotatedAdvisoryToken(t *testing.T, replace bool) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	const initial = "initial-fixture-token"
	rotated := strings.Repeat("b", config.MaxTokenBytes)
	writeAdvisoryToken(t, path, initial+"\n")
	srv, headers := authServer(t)
	calls := 0
	r := tokenReader(srv, func() (string, error) {
		calls++
		return config.ReadNewsGitHubToken(path)
	})
	ctx := testContext(t)
	if _, err := r.FetchAdvisories(ctx, srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("first advisory read: %v", err)
	}
	if replace {
		replacement := filepath.Join(dir, "replacement")
		writeAdvisoryToken(t, replacement, rotated)
		if err := os.Rename(replacement, path); err != nil {
			t.Fatalf("replace fixture token file: %v", err)
		}
	} else {
		writeAdvisoryToken(t, path, rotated)
	}
	if _, err := r.FetchAdvisories(ctx, srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("advisory read after rotation: %v", err)
	}
	if !slices.Equal(headers(), []string{"Bearer " + initial, "Bearer " + rotated}) {
		t.Error("expected the original token and then the rotated token")
	}
	if calls != 2 {
		t.Errorf("token source calls = %d, want one per advisory read", calls)
	}
}

const oversizedAdvisoryTokenFixture = "oversized-fixture-token"

func TestFetchAdvisories_Negative_InvalidRotatedTokenFileSkipsRequest(t *testing.T) {
	oversized := oversizedAdvisoryTokenFixture +
		strings.Repeat("x", config.MaxTokenBytes+1-len(oversizedAdvisoryTokenFixture))
	for _, tc := range []struct {
		name, contents, detail string
	}{
		{"missing", "", "open GitHub token file"},
		{"empty", "", "GitHub token file is empty"},
		{"oversized", oversized, "larger than 4096 bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testInvalidAdvisoryTokenFile(t, tc.name == "missing", tc.contents, tc.detail)
		})
	}
}

func testInvalidAdvisoryTokenFile(t *testing.T, remove bool, contents, detail string) {
	const initial = "initial-fixture-token"
	path := filepath.Join(t.TempDir(), "token")
	writeAdvisoryToken(t, path, initial)
	srv, headers := authServer(t)
	r := tokenReader(srv, func() (string, error) { return config.ReadNewsGitHubToken(path) })
	ctx := testContext(t)
	if _, err := r.FetchAdvisories(ctx, srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("first advisory read: %v", err)
	}
	if remove {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove fixture token file: %v", err)
		}
	} else {
		writeAdvisoryToken(t, path, contents)
	}

	_, err := r.FetchAdvisories(ctx, srv.URL+"/a", Validators{})
	assertInvalidAdvisoryTokenError(t, err, remove, detail, initial)
	if got := len(headers()); got != 1 {
		t.Errorf("request count = %d, want only the first read", got)
	}
}

func assertInvalidAdvisoryTokenError(t *testing.T, err error, remove bool, detail, initial string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), config.EnvNewsGitHubTokenFile) ||
		!strings.Contains(err.Error(), detail) {
		t.Error("expected a read error naming the token file variable and the file failure")
	}
	if remove && !errors.Is(err, os.ErrNotExist) {
		t.Error("expected the missing-file error to preserve os.ErrNotExist")
	}
	if err != nil && (strings.Contains(err.Error(), initial) || strings.Contains(err.Error(), oversizedAdvisoryTokenFixture)) {
		t.Error("advisory read error exposed token contents")
	}
}

// Another host and ordinary feed reads do not consult the token source.
func TestFetchAdvisories_Negative_TokenKeptFromOtherReads(t *testing.T) {
	srv, headers := authServer(t)
	calls := 0
	source := func() (string, error) {
		calls++
		return "tok-1", nil
	}
	other := testReader(srv)
	other.UseAPITokenSource(source)
	if _, err := other.FetchAdvisories(testContext(t), srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("FetchAdvisories() other host error = %v", err)
	}
	// The feed read fails to parse the advisory list; only its request headers matter here.
	if _, err := tokenReader(srv, source).Fetch(testContext(t), srv.URL+"/feed", Validators{}); err == nil {
		t.Fatal("Fetch() of the advisory list parsed as a feed")
	}
	if _, err := tokenReader(srv, nil).FetchAdvisories(testContext(t), srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("FetchAdvisories() without a source: %v", err)
	}
	if calls != 0 {
		t.Errorf("token source calls = %d, want none", calls)
	}
	if !slices.Equal(headers(), []string{"", "", ""}) {
		t.Error("expected three requests without Authorization")
	}
}

// The token does not change the shared advisory headers of later reads.
func TestFetchAdvisories_Boundary_TokenLeavesSharedHeaders(t *testing.T) {
	srv, _ := authServer(t)
	r := tokenReader(srv, func() (string, error) { return "tok-1", nil })
	if _, err := r.FetchAdvisories(testContext(t), srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("FetchAdvisories() error = %v", err)
	}
	if _, ok := advisoryHeaders["Authorization"]; ok {
		t.Fatal("advisoryHeaders holds the token")
	}
}

// A new Reader takes GitHub's API host.
func TestNewReader_Boundary_APIHost(t *testing.T) {
	if got := NewReader(nil).apiHost; got != "api.github.com" {
		t.Fatalf("apiHost = %q, want api.github.com", got)
	}
}
