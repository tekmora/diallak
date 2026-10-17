package discord

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/model"
	"github.com/tekmora/diallak/internal/retrieval"
)

// maskingTestGateway masks e-mail addresses and the planted name on the guardrail route and
// records what the chat route received.
type maskingTestGateway struct {
	mu    sync.Mutex
	chats []string
}

var (
	e2eEmail = regexp.MustCompile(`[a-z.]+@[a-z.]+`)
	e2eName  = regexp.MustCompile(`Jane Roe`)
)

func (g *maskingTestGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/guardrails/apply_guardrail" {
		var req struct{ Text string }
		if !e2eReadJSON(w, r, &req) {
			return
		}
		masked := e2eName.ReplaceAllString(e2eEmail.ReplaceAllString(req.Text, "<EMAIL_ADDRESS>"), "<PERSON>")
		e2eWriteJSON(w, map[string]string{"response_text": masked})
		return
	}
	var req struct{ Messages []struct{ Content string } }
	if !e2eReadJSON(w, r, &req) || len(req.Messages) == 0 {
		return
	}
	g.mu.Lock()
	g.chats = append(g.chats, req.Messages[len(req.Messages)-1].Content)
	g.mu.Unlock()
	w.Header().Set(model.PolicyHeader, "diallak")
	w.Header().Set(model.GuardrailHeader, "presidio-pii")
	e2eWriteJSON(w, map[string]any{
		"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "Free the disk [1]."}}},
	})
}

// e2eReadJSON decodes the request body into v, or answers 400 and reports false.
func e2eReadJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(r.Body)
	if err == nil {
		err = json.Unmarshal(body, v)
	}
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return false
	}
	return true
}

// e2eWriteJSON writes v; a failed write only ends the test request.
func e2eWriteJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encode", http.StatusInternalServerError)
	}
}

type oneHitRetriever struct{}

func (oneHitRetriever) Search(string, retrieval.Options) ([]retrieval.Hit, error) {
	return []retrieval.Hit{{Chunk: corpus.Chunk{
		ID: "c1", Repository: "tekmora/test-repo", Revision: "abc123", Path: "docs/import.md",
		Title: "Import", Heading: "Stops", Text: "The import stops when the disk is full.",
	}, Score: 0.9}}, nil
}

// newMaskingAsker wires the model client, the gate and the answer pipeline as the app does.
func newMaskingAsker(t *testing.T, gw *maskingTestGateway) *answer.Pipeline {
	t.Helper()
	server := httptest.NewServer(gw)
	t.Cleanup(server.Close)
	client, err := model.New(model.Config{
		BaseURL: server.URL + "/v1", Alias: "router/test", Policy: "diallak", Guardrail: "presidio-pii",
	}, server.Client())
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	gate, err := model.NewGate(model.GateConfig{Endpoint: client, Masker: client, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewGate: %v", err)
	}
	if err := gate.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	p, err := answer.New(answer.Config{Retriever: oneHitRetriever{}, Masker: gate, Options: retrieval.DefaultOptions()})
	if err != nil {
		t.Fatalf("answer.New: %v", err)
	}
	return p
}

// The planted case of #69, end to end: a question with a name and an e-mail address reaches the
// test model masked, and the receipt shows the same text and counts.
func TestAskReceipt_Positive_PlantedQuestionReachesTheModelMaskedAndTheReceiptShowsIt(t *testing.T) {
	gw := &maskingTestGateway{}
	receipts, _ := newTestReceipts(t, 0)
	in := &fakeInteraction{guildID: 123, question: "I am Jane Roe (jane.roe@example.com): why does the import stop?"}
	newReceiptHandler(t, receipts, newMaskingAsker(t, gw)).HandleAsk(context.Background(), in)

	verifyMaskedChats(t, gw)
	verifyMaskedReceipt(t, receipts, in)
}

const e2eMasked = "I am <PERSON> (<EMAIL_ADDRESS>): why does the import stop?"

func verifyMaskedChats(t *testing.T, gw *maskingTestGateway) {
	t.Helper()
	const masked = e2eMasked
	gw.mu.Lock()
	chats := append([]string(nil), gw.chats...)
	gw.mu.Unlock()
	if len(chats) != 2 || !strings.Contains(chats[1], masked) {
		t.Fatalf("chat requests = %q, want the start probe and the masked question", chats)
	}
	for _, c := range chats {
		if strings.Contains(c, "Jane Roe") || strings.Contains(c, "jane.roe@example.com") {
			t.Errorf("the model received unmasked data: %q", c)
		}
	}
}

func verifyMaskedReceipt(t *testing.T, receipts *Receipts, in *fakeInteraction) {
	t.Helper()
	const masked = e2eMasked
	if len(in.receiptKeys) != 1 || in.receiptKeys[0] == "" {
		t.Fatalf("receipt keys = %q, want one", in.receiptKeys)
	}
	e, ok := receipts.get(in.receiptKeys[0])
	if !ok {
		t.Fatal("receipt not held")
	}
	text := receiptText(e)
	for _, want := range []string{"```\n" + masked + "\n```", "Replaced by the masking: `EMAIL_ADDRESS` 1, `PERSON` 1."} {
		if !strings.Contains(text, want) {
			t.Errorf("receipt lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Jane Roe") || strings.Contains(text, "jane.roe@example.com") {
		t.Errorf("receipt holds the question as written:\n%s", text)
	}
}
