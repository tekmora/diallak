package answer

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tekmora/diallak/internal/model"
	"github.com/tekmora/diallak/internal/retrieval"
)

// askThroughGate runs the client and the gate of the model package behind the
// pipeline against a stub endpoint. The start probe always names the policy;
// later replies name it only when named is true. It asks twice and returns
// both answers, the number of requests the stub got and the gate.
func askThroughGate(t *testing.T, named bool, replyText string) ([]Answer, int32, *model.Gate) {
	t.Helper()
	const policy = "example-policy"
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 || named {
			w.Header().Set(model.PolicyHeader, policy)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + replyText + `"}}]}`))
	}))
	defer srv.Close()

	client, err := model.New(model.Config{BaseURL: srv.URL, Alias: "test-alias", Policy: policy}, srv.Client())
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gate, err := model.NewGate(model.GateConfig{Endpoint: client, Logger: logger})
	if err != nil {
		t.Fatalf("model.NewGate: %v", err)
	}
	if err := gate.Start(context.Background()); err != nil {
		t.Fatalf("gate.Start: %v", err)
	}
	hit := sampleHit("doc1", "Doc One", "Head", "Text content", "rev111111111111")
	p, err := New(Config{Retriever: &stubRetriever{hits: []retrieval.Hit{hit}}, Generator: gate, Logger: logger})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	answers := make([]Answer, 0, 2)
	for i := 0; i < 2; i++ {
		ans, err := p.Ask(context.Background(), "How do I install it?")
		if err != nil {
			t.Fatalf("Ask %d: %v", i+1, err)
		}
		answers = append(answers, ans)
	}
	return answers, calls.Load(), gate
}

func TestAsk_Positive_ReplyThatNamesThePolicyIsAnAnswer(t *testing.T) {
	const replyText = "Install it with the package [1]."

	answers, calls, gate := askThroughGate(t, true, replyText)

	for i, ans := range answers {
		if ans.Kind != KindAnswer || ans.Text != replyText {
			t.Errorf("answer %d: kind=%v text=%q, want the model's text", i+1, ans.Kind, ans.Text)
		}
	}
	if calls != 3 {
		t.Errorf("requests = %d, want 3 (the probe and two questions)", calls)
	}
	if gate.State() != model.StateReady || gate.Reason() != "" {
		t.Errorf("gate state=%q reason=%q, want ready", gate.State(), gate.Reason())
	}
}

func TestAsk_Negative_ReplyWithoutThePolicyEndsInExcerpts(t *testing.T) {
	const replyText = "Text of a reply that names no policy [1]."

	answers, calls, gate := askThroughGate(t, false, replyText)

	for i, ans := range answers {
		if ans.Kind != KindExcerpts || ans.Reason != ReasonModelUnavailable {
			t.Errorf("answer %d: kind=%v reason=%v, want excerpts for an unavailable model", i+1, ans.Kind, ans.Reason)
		}
		if rendered := Render(ans, RenderOptions{}); strings.Contains(ans.Text+rendered, "names no policy") {
			t.Errorf("answer %d shows the text of the refused reply: %q", i+1, rendered)
		}
	}
	// The probe, then the one reply without the policy. The second question
	// never reaches the endpoint.
	if calls != 2 {
		t.Errorf("requests = %d, want 2", calls)
	}
	if gate.State() != model.StateNotReady || gate.Reason() != "policy" {
		t.Errorf("gate state=%q reason=%q, want not ready for policy", gate.State(), gate.Reason())
	}
}
