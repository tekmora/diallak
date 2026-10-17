package discord

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/tekmora/diallak/internal/news"
)

// newsGatewayFake answers the start of the bot and signals each read of the server's roles, the
// first step of a news round.
type newsGatewayFake struct {
	t       *testing.T
	mu      sync.Mutex
	calls   []areasCall
	answers chan struct{}
}

func (f *newsGatewayFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, areasCall{method: r.Method, path: r.URL.Path})
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	var err error
	switch r.URL.Path {
	case "/applications/@me":
		_, err = fmt.Fprintf(w, `{"id":"%d","name":"diallak"}`, gatewayAppID)
	case "/gateway", "/gateway/bot":
		_, err = fmt.Fprint(w, `{"url":"ws://127.0.0.1:0","shards":1}`)
	case fmt.Sprintf("/guilds/%d/roles", areasGuildID):
		_, err = fmt.Fprint(w, `[]`)
		select {
		case f.answers <- struct{}{}:
		default:
		}
	case fmt.Sprintf("/guilds/%d/channels", areasGuildID):
		_, err = fmt.Fprint(w, `[]`)
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
	if err != nil {
		f.t.Errorf("write response: %v", err)
	}
}

func (f *newsGatewayFake) answered() <-chan struct{} { return f.answers }

func (f *newsGatewayFake) recorded() []areasCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]areasCall(nil), f.calls...)
}

// Issue #180: the bot starts the news runner with the gateway, and its first round looks for the
// places on the server before it reads a source.
func TestNewsGateway_Positive_StartRunsTheNews(t *testing.T) {
	s, err := news.OpenFileStore(t.TempDir() + "/news.json")
	if err != nil {
		t.Fatal(err)
	}
	runner := &news.Runner{Store: s, Reader: news.NewReader(nil), Sources: news.OwnSources([]string{"tekmora/diallak"})}
	fake := &newsGatewayFake{t: t, answers: make(chan struct{}, 1)}
	calls := runComponentsOverGateway(t, fake, BotConfig{News: &NewsConfig{Runner: runner, Names: newsNames()}}, 1)
	for _, c := range calls {
		if c.method != http.MethodGet {
			t.Errorf("request %s %s; a round without places posts nothing", c.method, c.path)
		}
	}
}
