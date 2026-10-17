package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/news"
)

const (
	newsChannel = "998877665544332211"
	newsRole    = "112233445566778899"
	newsMessage = "123456789012345678"
)

type newsRequest struct {
	method, path string
	body         map[string]any
}

// newsRest returns a NewsPoster against a server that records every request and answers it with
// status: a created message with id newsMessage for 200.
func newsRest(t *testing.T, status int) (*NewsPoster, func() []newsRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []newsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		var body map[string]any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}
		mu.Lock()
		seen = append(seen, newsRequest{r.Method, r.URL.Path, body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := fmt.Fprintf(w, `{"id":"%s","channel_id":"%s","code":0,"message":"m"}`, newsMessage, newsChannel); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := newRestClient(makeFakeToken(gatewayAppID), srv.URL)
	if err != nil {
		t.Fatalf("newRestClient: %v", err)
	}
	t.Cleanup(func() { client.Close(context.Background()) })
	return NewNewsPoster(client.Rest), func() []newsRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]newsRequest(nil), seen...)
	}
}

func newsContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// An announcement may mention the release role and nobody else: no parsed mentions, no users,
// exactly that one role (ADR-0019, point 6).
func TestNewsPoster_Positive_AnnouncementMentionsOnlyTheRole(t *testing.T) {
	p, seen := newsRest(t, http.StatusOK)
	id, err := p.PostAnnouncement(newsContext(t), newsChannel, newsRole, "<@&"+newsRole+"> New release")
	if err != nil || id != newsMessage {
		t.Fatalf("PostAnnouncement() = %q, %v; want %q, nil", id, err, newsMessage)
	}
	got := seen()
	if len(got) != 1 || got[0].method != http.MethodPost || got[0].path != "/channels/"+newsChannel+"/messages" {
		t.Fatalf("requests = %+v, want one POST to the channel's messages", got)
	}
	m, ok := got[0].body["allowed_mentions"].(map[string]any)
	if !ok || fmt.Sprint(m["parse"]) != "[]" || fmt.Sprint(m["users"]) != "[]" || fmt.Sprint(m["roles"]) != "["+newsRole+"]" {
		t.Fatalf("allowed_mentions = %v, want parse [], users [], roles [%s]", m, newsRole)
	}
}

// Publishing to following servers is the crosspost of the bot's own message.
func TestNewsPoster_Positive_Crosspost(t *testing.T) {
	p, seen := newsRest(t, http.StatusOK)
	if err := p.Crosspost(newsContext(t), newsChannel, newsMessage); err != nil {
		t.Fatalf("Crosspost() error = %v", err)
	}
	got := seen()
	if len(got) != 1 || got[0].method != http.MethodPost ||
		got[0].path != "/channels/"+newsChannel+"/messages/"+newsMessage+"/crosspost" {
		t.Fatalf("requests = %+v, want one POST to the message's crosspost", got)
	}
}

// A digest mentions nobody.
func TestNewsPoster_Positive_DigestMentionsNobody(t *testing.T) {
	p, seen := newsRest(t, http.StatusOK)
	if err := p.PostDigest(newsContext(t), newsChannel, "**News**"); err != nil {
		t.Fatalf("PostDigest() error = %v", err)
	}
	got := seen()
	if len(got) != 1 {
		t.Fatalf("requests = %+v, want one post", got)
	}
	m, ok := got[0].body["allowed_mentions"].(map[string]any)
	if !ok || fmt.Sprint(m["parse"]) != "[]" || fmt.Sprint(m["roles"]) != "[]" || fmt.Sprint(m["users"]) != "[]" {
		t.Fatalf("requests = %+v, want one post that mentions nobody", got)
	}
}

func TestNewsPoster_Negative_RefusedCallsAreTold(t *testing.T) {
	p, _ := newsRest(t, http.StatusForbidden)
	ctx := newsContext(t)
	_, postErr := p.PostAnnouncement(ctx, newsChannel, newsRole, "x")
	for name, err := range map[string]error{
		"announcement": postErr,
		"crosspost":    p.Crosspost(ctx, newsChannel, newsMessage),
		"digest":       p.PostDigest(ctx, newsChannel, "x"),
	} {
		if !errors.Is(err, ErrDiscordRefused) {
			t.Errorf("%s: error = %v, want ErrDiscordRefused", name, err)
		}
	}
}

// An id that is not a Discord id is refused before any request.
func TestNewsPoster_Boundary_BadIDsMakeNoRequest(t *testing.T) {
	p, seen := newsRest(t, http.StatusOK)
	ctx := newsContext(t)
	_, e1 := p.PostAnnouncement(ctx, "announcements", newsRole, "x")
	_, e2 := p.PostAnnouncement(ctx, newsChannel, "everyone", "x")
	e3 := p.Crosspost(ctx, newsChannel, "")
	e4 := p.PostDigest(ctx, "-1", "x")
	e5 := p.PostDigest(ctx, "0", "x")
	for i, err := range []error{e1, e2, e3, e4, e5} {
		if err == nil {
			t.Errorf("call %d: error = nil, want one", i+1)
		}
	}
	if got := seen(); len(got) != 0 {
		t.Fatalf("requests = %+v, want none", got)
	}
}

var (
	_ news.Poster    = (*NewsPoster)(nil)
	_ news.Announcer = (*NewsPoster)(nil)
)
