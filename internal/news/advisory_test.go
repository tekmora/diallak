package news

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
func authServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var got []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization"))
		if _, err := w.Write([]byte("[]")); err != nil {
			t.Errorf("write body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

// tokenReader returns a Reader of srv that holds token and takes srv for the API host.
func tokenReader(srv *httptest.Server, token string) *Reader {
	r := testReader(srv)
	r.apiHost = srv.Listener.Addr().String()
	r.UseAPIToken(token)
	return r
}

// The token goes with an advisory read to the API host.
func TestFetchAdvisories_Positive_TokenSentToAPIHost(t *testing.T) {
	srv, got := authServer(t)
	if _, err := tokenReader(srv, "tok-1").FetchAdvisories(testContext(t), srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("FetchAdvisories() error = %v", err)
	}
	if len(*got) != 1 || (*got)[0] != "Bearer tok-1" {
		t.Fatalf("Authorization = %q, want the token", *got)
	}
}

// The token never goes to another host, nor with a feed read, and an empty token sends none.
func TestFetchAdvisories_Negative_TokenKeptFromOtherReads(t *testing.T) {
	srv, got := authServer(t)
	other := testReader(srv)
	other.UseAPIToken("tok-1")
	if _, err := other.FetchAdvisories(testContext(t), srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("FetchAdvisories() other host error = %v", err)
	}
	// The feed read fails to parse the advisory list; only its request headers matter here.
	if _, err := tokenReader(srv, "tok-1").Fetch(testContext(t), srv.URL+"/feed", Validators{}); err == nil {
		t.Fatal("Fetch() of the advisory list parsed as a feed")
	}
	if _, err := tokenReader(srv, "").FetchAdvisories(testContext(t), srv.URL+"/a", Validators{}); err != nil {
		t.Fatalf("FetchAdvisories() empty token error = %v", err)
	}
	if len(*got) != 3 || (*got)[0] != "" || (*got)[1] != "" || (*got)[2] != "" {
		t.Fatalf("Authorization = %q, want none", *got)
	}
}

// The token does not change the shared advisory headers of later reads.
func TestFetchAdvisories_Boundary_TokenLeavesSharedHeaders(t *testing.T) {
	srv, _ := authServer(t)
	if _, err := tokenReader(srv, "tok-1").FetchAdvisories(testContext(t), srv.URL+"/a", Validators{}); err != nil {
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
