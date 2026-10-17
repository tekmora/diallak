package news

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/ghapp"
)

const testDiscussionChannel = "601"

type fakeForge struct {
	mu           sync.Mutex
	repos        []installationRepo
	discussions  map[string][]gqlDiscussionNode
	graphqlCalls []string
}

func (f *fakeForge) addDiscussion(repo, id, title, url, createdAt, slug, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.discussions == nil {
		f.discussions = make(map[string][]gqlDiscussionNode)
	}
	node := gqlDiscussionNode{
		ID:        id,
		Title:     title,
		URL:       url,
		CreatedAt: createdAt,
	}
	node.Category.Slug = slug
	node.Category.Name = name
	f.discussions[repo] = append([]gqlDiscussionNode{node}, f.discussions[repo]...)
}

func (f *fakeForge) recordedCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.graphqlCalls...)
}

func writeForgeResponse(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func (f *fakeForge) handleGraphQL(t *testing.T, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Variables map[string]string `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("decode graphql request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fullName := req.Variables["o"] + "/" + req.Variables["n"]
	f.mu.Lock()
	f.graphqlCalls = append(f.graphqlCalls, fullName)
	nodes := f.discussions[fullName]
	f.mu.Unlock()

	nodesJSON, err := json.Marshal(nodes)
	if err != nil {
		t.Errorf("marshal nodes: %v", err)
		return
	}
	resp := fmt.Sprintf(`{"data":{"repository":{"discussions":{"nodes":%s}}}}`, string(nodesJSON))
	writeForgeResponse(t, w, http.StatusOK, resp)
}

func newFakeForgeServer(t *testing.T, forge *fakeForge) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations":
			writeForgeResponse(t, w, http.StatusOK, `[{"id":1}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/1/access_tokens":
			body := fmt.Sprintf(`{"token":"test-token","expires_at":%q}`, runNow.Add(time.Hour).Format(time.RFC3339))
			writeForgeResponse(t, w, http.StatusCreated, body)
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			forge.mu.Lock()
			data, err := json.Marshal(installationReposResponse{Repositories: forge.repos})
			forge.mu.Unlock()
			if err != nil {
				t.Errorf("marshal repos: %v", err)
				return
			}
			writeForgeResponse(t, w, http.StatusOK, string(data))
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			forge.handleGraphQL(t, w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setupDiscussionRun(t *testing.T, forge *fakeForge) (*testRun, *httptest.Server) {
	t.Helper()
	srv := newFakeForgeServer(t, forge)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	app := &ghapp.App{
		ClientID: "test-app",
		Key:      key,
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
		Now:      func() time.Time { return runNow },
	}
	discReader := &GitHubDiscussions{App: app}
	src := Source{ID: "discussions", Kind: SourceDiscussions, Interval: DiscussionInterval}
	tr := newTestRun(t, srv, src)
	tr.r.Discussions = discReader
	tr.r.ReadPlaces = func(context.Context) (Places, error) {
		tr.places.Add(1)
		return Places{
			Announcement: testPlace,
			Channels:     map[string]string{"ai": testChannel},
			Discussions:  testDiscussionChannel,
		}, nil
	}
	return tr, srv
}

func TestRunner_Positive_DiscussionPostedOnce(t *testing.T) {
	forge := &fakeForge{
		repos: []installationRepo{
			{FullName: "tekmora/diallak", Private: false, HasDiscussions: true},
		},
	}
	forge.addDiscussion("tekmora/diallak", "D1", "First Q",
		"https://github.com/tekmora/diallak/discussions/1", runNow.Format(time.RFC3339), "q-a", "Q&A")

	tr, _ := setupDiscussionRun(t, forge)

	// First round: primes the store, posts nothing.
	tr.round(t)
	if len(tr.p.posts) != 0 {
		t.Fatalf("posts on prime round = %d, want 0", len(tr.p.posts))
	}

	// Add a new discussion and run second round.
	forge.addDiscussion("tekmora/diallak", "D2", "Ideas for bots",
		"https://github.com/tekmora/diallak/discussions/2", runNow.Add(time.Minute).Format(time.RFC3339), "ideas", "Ideas")
	tr.now = tr.now.Add(DiscussionInterval)
	tr.round(t)

	if len(tr.p.posts) != 1 {
		t.Fatalf("posts after new discussion = %d, want 1", len(tr.p.posts))
	}
	post := tr.p.posts[0]
	if post.channel != testDiscussionChannel {
		t.Errorf("channel = %q, want %q", post.channel, testDiscussionChannel)
	}
	if strings.Contains(post.content, "<@") {
		t.Errorf("post contains mention: %s", post.content)
	}
	for _, want := range []string{"tekmora/diallak", "Ideas", "Ideas for bots", "https://github.com/tekmora/diallak/discussions/2"} {
		if !strings.Contains(post.content, want) {
			t.Errorf("post content missing %q: %s", want, post.content)
		}
	}

	// Third round with same data: already announced, no second post.
	tr.now = tr.now.Add(DiscussionInterval)
	tr.round(t)
	if len(tr.p.posts) != 1 {
		t.Fatalf("posts on third round = %d, want 1", len(tr.p.posts))
	}
}

func TestRunner_Positive_FailedPostRetried(t *testing.T) {
	forge := &fakeForge{
		repos: []installationRepo{
			{FullName: "tekmora/diallak", Private: false, HasDiscussions: true},
		},
	}
	tr, _ := setupDiscussionRun(t, forge)
	tr.round(t) // prime

	forge.addDiscussion("tekmora/diallak", "D1", "Retry me",
		"https://github.com/tekmora/diallak/discussions/1", runNow.Add(time.Minute).Format(time.RFC3339), "q-a", "Q&A")

	// Fail the post in this round.
	tr.p.fail = map[string]bool{testDiscussionChannel: true}
	tr.now = tr.now.Add(DiscussionInterval)
	tr.round(t)
	if len(tr.p.posts) != 0 {
		t.Fatalf("posts on failed round = %d, want 0", len(tr.p.posts))
	}

	// Unfail and run next round: should retry and succeed.
	tr.p.fail = nil
	tr.now = tr.now.Add(DiscussionInterval)
	tr.round(t)
	if len(tr.p.posts) != 1 {
		t.Fatalf("posts on retried round = %d, want 1", len(tr.p.posts))
	}
}

func TestRunner_Negative_NoGraphQLForPrivateOrNoDiscussionsRepo(t *testing.T) {
	forge := &fakeForge{
		repos: []installationRepo{
			{FullName: "tekmora/public-discussions", Private: false, HasDiscussions: true},
			{FullName: "tekmora/private-repo", Private: true, HasDiscussions: true},
			{FullName: "tekmora/no-discussions", Private: false, HasDiscussions: false},
		},
	}
	tr, _ := setupDiscussionRun(t, forge)
	tr.round(t)

	calls := forge.recordedCalls()
	if len(calls) != 1 || calls[0] != "tekmora/public-discussions" {
		t.Errorf("graphql calls = %v, want only tekmora/public-discussions", calls)
	}
}

func TestRunner_Negative_IgnoredCategoryNotPosted(t *testing.T) {
	forge := &fakeForge{
		repos: []installationRepo{
			{FullName: "tekmora/diallak", Private: false, HasDiscussions: true},
		},
	}
	tr, _ := setupDiscussionRun(t, forge)
	tr.round(t) // prime

	forge.addDiscussion("tekmora/diallak", "D1", "General Chat",
		"https://github.com/tekmora/diallak/discussions/1", runNow.Add(time.Minute).Format(time.RFC3339), "general", "General")
	forge.addDiscussion("tekmora/diallak", "D2", "Show & Tell",
		"https://github.com/tekmora/diallak/discussions/2", runNow.Add(2*time.Minute).Format(time.RFC3339), "show-and-tell", "Show and tell")

	tr.now = tr.now.Add(DiscussionInterval)
	tr.round(t)

	if len(tr.p.posts) != 0 {
		t.Errorf("posts = %+v, want 0 for non-whitelisted category slugs", tr.p.posts)
	}
}

func TestRunner_Negative_InvalidURLDropped(t *testing.T) {
	forge := &fakeForge{
		repos: []installationRepo{
			{FullName: "tekmora/diallak", Private: false, HasDiscussions: true},
		},
	}
	tr, _ := setupDiscussionRun(t, forge)
	tr.round(t) // prime

	forge.addDiscussion("tekmora/diallak", "D1", "Other domain",
		"https://evil.example.com/tekmora/diallak/discussions/1", runNow.Add(time.Minute).Format(time.RFC3339), "q-a", "Q&A")
	forge.addDiscussion("tekmora/diallak", "D2", "Issues not discussions",
		"https://github.com/tekmora/diallak/issues/2", runNow.Add(2*time.Minute).Format(time.RFC3339), "ideas", "Ideas")
	forge.addDiscussion("tekmora/diallak", "D3", "Not a number",
		"https://github.com/tekmora/diallak/discussions/abc", runNow.Add(3*time.Minute).Format(time.RFC3339), "q-a", "Q&A")

	tr.now = tr.now.Add(DiscussionInterval)
	tr.round(t)

	if len(tr.p.posts) != 0 {
		t.Errorf("posts = %+v, want 0 for invalid URLs", tr.p.posts)
	}
}

func TestRunner_Boundary_ElevenReposCappedAtTen(t *testing.T) {
	repos := make([]installationRepo, 11)
	for i := range 11 {
		repos[i] = installationRepo{
			FullName:       fmt.Sprintf("tekmora/repo-%02d", i),
			Private:        false,
			HasDiscussions: true,
		}
	}
	forge := &fakeForge{repos: repos}
	tr, _ := setupDiscussionRun(t, forge)
	tr.round(t)

	calls := forge.recordedCalls()
	if len(calls) != MaxDiscussionRepos {
		t.Errorf("graphql calls = %d, want %d", len(calls), MaxDiscussionRepos)
	}
	for i, c := range calls {
		want := fmt.Sprintf("tekmora/repo-%02d", i)
		if c != want {
			t.Errorf("call[%d] = %q, want %q", i, c, want)
		}
	}
}
