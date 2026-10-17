package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/tekmora/diallak/internal/answer"
)

// The tests of registering commands call registerCommandsWithOptions with the
// address of a fake. The command calls the exported RegisterCommands
// (cmd/diallak, defaultDependencies), which sets no address and uses the
// library's own. The tests in this file call that and send the address to a
// fake with redirectDiscord.

const (
	registerTokenID uint64 = 810000000000000001
	registerAppID   uint64 = 820000000000000001
	registerGuildID uint64 = 830000000000000001
)

// registerFake answers the application lookup with registerAppID and the
// command list with putStatus, and records every request.
type registerFake struct {
	putStatus int
	mu        sync.Mutex
	calls     []recordedRequest
}

func (f *registerFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	f.mu.Lock()
	f.calls = append(f.calls, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
	f.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/applications/@me":
		writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":"diallak"}`, registerAppID))
	case r.Method == http.MethodPut && f.putStatus == http.StatusOK:
		writeTestResponse(w, http.StatusOK, "[]")
	case r.Method == http.MethodPut:
		writeTestResponse(w, f.putStatus, `{"code":50001,"message":"Missing Access"}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *registerFake) recorded() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.calls...)
}

// newRegisterFixture starts a fake and sends the library's address to it.
func newRegisterFixture(t *testing.T, putStatus int) *registerFake {
	t.Helper()
	fake := &registerFake{putStatus: putStatus}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	redirectDiscord(t, srv.URL)
	return fake
}

// questionMaxLength returns the max_length of the question option in the
// registered command list.
func questionMaxLength(t *testing.T, calls []recordedRequest) int {
	t.Helper()
	for _, c := range calls {
		if c.method != http.MethodPut {
			continue
		}
		var cmds []struct {
			Options []struct {
				Name      string `json:"name"`
				MaxLength int    `json:"max_length"`
			} `json:"options"`
		}
		if err := json.Unmarshal(c.body, &cmds); err != nil {
			t.Fatalf("command list body: %v", err)
		}
		if len(cmds) != 2 || len(cmds[0].Options) != 1 || cmds[0].Options[0].Name != OptionQuestion {
			t.Fatalf("command list %s, want the ask command with the option %q and the areas command", c.body, OptionQuestion)
		}
		return cmds[0].Options[0].MaxLength
	}
	t.Fatalf("no command list was sent: %+v", calls)
	return 0
}

func TestRegisterCommands_Positive_ExportedEntryRegistersOverDiscordsAddress(t *testing.T) {
	fake := newRegisterFixture(t, http.StatusOK)

	if err := RegisterCommands(context.Background(), makeFakeToken(registerTokenID), registerGuildID, 300); err != nil {
		t.Fatalf("RegisterCommands: %v", err)
	}
	calls := fake.recorded()
	assertRecordedCommandPath(t, calls, registerTokenID, registerAppID, registerGuildID)
	if got := questionMaxLength(t, calls); got != 300 {
		t.Errorf("question max_length = %d, want the configured 300", got)
	}
}

func TestRegisterCommands_Negative_ExportedEntryReportsARefusedList(t *testing.T) {
	fake := newRegisterFixture(t, http.StatusForbidden)

	err := RegisterCommands(context.Background(), makeFakeToken(registerTokenID), registerGuildID, 0)
	if err == nil {
		t.Fatal("RegisterCommands reported success for a command list Discord refused")
	}
	if got := questionMaxLength(t, fake.recorded()); got != answer.MaxQuestionRunes {
		t.Errorf("question max_length = %d, want the default %d", got, answer.MaxQuestionRunes)
	}
}

func TestRegisterCommands_Boundary_ExportedEntryTakesTheLimitAndRefusesOneMore(t *testing.T) {
	fake := newRegisterFixture(t, http.StatusOK)
	token := makeFakeToken(registerTokenID)

	if err := RegisterCommands(context.Background(), token, registerGuildID, answer.MaxQuestionRunes); err != nil {
		t.Fatalf("RegisterCommands at the limit: %v", err)
	}
	if got := questionMaxLength(t, fake.recorded()); got != answer.MaxQuestionRunes {
		t.Errorf("question max_length = %d, want %d", got, answer.MaxQuestionRunes)
	}

	sent := len(fake.recorded())
	if err := RegisterCommands(context.Background(), token, registerGuildID, answer.MaxQuestionRunes+1); err == nil {
		t.Error("RegisterCommands took a question limit one over the maximum")
	}
	if got := len(fake.recorded()); got != sent {
		t.Errorf("RegisterCommands sent %d requests for a limit over the maximum, want none", got-sent)
	}
}
