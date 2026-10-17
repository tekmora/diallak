package model

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGateway is a test gateway that serves the guardrail route and the chat route. Its masking
// replaces e-mail addresses and the planted name, as a Presidio guardrail would.
type fakeGateway struct {
	mu sync.Mutex
	// maskStatus, when not 0, is the status of every guardrail reply.
	maskStatus int
	// maskBody, when not "", is the body of every guardrail reply.
	maskBody string
	// maskBlock makes the guardrail route wait until the request ends.
	maskBlock bool
	// unmasked makes the guardrail route return the text unchanged.
	unmasked bool

	maskPaths []string
	maskReqs  []guardrailRequest
	maskAuth  []string
	chatUsers []string
}

var (
	plantedEmail = regexp.MustCompile(`[a-z.]+@[a-z.]+`)
	plantedName  = regexp.MustCompile(`Jane Roe`)
)

func (f *fakeGateway) mask(text string) string {
	if f.unmasked {
		return text
	}
	text = plantedEmail.ReplaceAllString(text, "<EMAIL_ADDRESS>")
	return plantedName.ReplaceAllString(text, "<PERSON>")
}

// readJSON decodes the request body into v, or answers 400 and reports false.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
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

// writeReply writes s; a failed write only ends the test request.
func writeReply(w io.Writer, s string) {
	if _, err := io.WriteString(w, s); err != nil {
		return
	}
}

func (f *fakeGateway) serveMask(w http.ResponseWriter, r *http.Request) {
	var req guardrailRequest
	if !readJSON(w, r, &req) {
		return
	}
	f.mu.Lock()
	f.maskPaths = append(f.maskPaths, r.URL.Path)
	f.maskReqs = append(f.maskReqs, req)
	f.maskAuth = append(f.maskAuth, r.Header.Get("Authorization"))
	status, reply, block := f.maskStatus, f.maskBody, f.maskBlock
	f.mu.Unlock()
	if block {
		<-r.Context().Done()
		return
	}
	if status != 0 {
		w.WriteHeader(status)
	}
	if reply == "" {
		out, err := json.Marshal(map[string]string{"response_text": f.mask(req.Text)})
		if err != nil {
			http.Error(w, "marshal", http.StatusInternalServerError)
			return
		}
		reply = string(out)
	}
	writeReply(w, reply)
}

func (f *fakeGateway) serveChat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Messages) > 0 {
		f.mu.Lock()
		f.chatUsers = append(f.chatUsers, req.Messages[len(req.Messages)-1].Content)
		f.mu.Unlock()
	}
	w.Header().Set(PolicyHeader, "example-policy")
	w.Header().Set(GuardrailHeader, "presidio-pii")
	out, err := json.Marshal(chatResponse{Choices: []chatChoice{
		{Message: chatMessage{Role: "assistant", Content: "Answer [1]."}},
	}})
	if err != nil {
		http.Error(w, "marshal", http.StatusInternalServerError)
		return
	}
	writeReply(w, string(out))
}

func (f *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, guardrailPath) {
		f.serveMask(w, r)
		return
	}
	f.serveChat(w, r)
}

func (f *fakeGateway) chats() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.chatUsers...)
}

func (f *fakeGateway) masks() []guardrailRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]guardrailRequest(nil), f.maskReqs...)
}

// newMaskingClient serves gw and returns a client with a guardrail whose base URL ends in "/v1".
func newMaskingClient(t *testing.T, gw *fakeGateway) *Client {
	t.Helper()
	server := httptest.NewServer(gw)
	t.Cleanup(server.Close)
	c, err := New(Config{
		BaseURL:   server.URL + "/v1",
		Alias:     "router/test",
		Policy:    "example-policy",
		Guardrail: "presidio-pii",
		APIKey:    "test-secret-key",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	return c
}

func newMaskingGate(t *testing.T, c *Client) *Gate {
	t.Helper()
	g, err := NewGate(GateConfig{Endpoint: c, Masker: c, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("NewGate() unexpected error: %v", err)
	}
	return g
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

const plantedQuestion = "I am Jane Roe, write to jane.roe@example.com: why does the import stop?"

func plantedPrompt(masked string) string { return "Question: " + masked }

// The planted case of #69: a question with a name and an e-mail address reaches the model masked,
// and the trace holds the same text and the counts.
func TestCompleteMasked_Positive_PlantedNameAndAddressReachTheModelMasked(t *testing.T) {
	gw := &fakeGateway{}
	c := newMaskingClient(t, gw)
	g := newMaskingGate(t, c)
	if err := g.Start(context.Background()); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}

	res, trace, err := g.CompleteMasked(context.Background(), "System", plantedQuestion, plantedPrompt)
	if err != nil {
		t.Fatalf("CompleteMasked() unexpected error: %v", err)
	}
	if res != "Answer [1]." {
		t.Errorf("result = %q", res)
	}
	verifyPlantedChats(t, gw.chats())
	verifyPlantedTrace(t, trace)
}

const plantedMasked = "I am <PERSON>, write to <EMAIL_ADDRESS>: why does the import stop?"

func verifyPlantedChats(t *testing.T, chats []string) {
	t.Helper()
	wantMasked := plantedMasked
	// The start's chat probe is the first chat request.
	if len(chats) != 2 || chats[1] != plantedPrompt(wantMasked) {
		t.Fatalf("chat requests = %q, want the probe and %q", chats, plantedPrompt(wantMasked))
	}
	for _, sent := range chats {
		if strings.Contains(sent, "jane.roe@example.com") || strings.Contains(sent, "Jane Roe") {
			t.Errorf("model received unmasked data: %q", sent)
		}
	}
}

func verifyPlantedTrace(t *testing.T, trace Trace) {
	t.Helper()
	wantMasked := plantedMasked
	if trace.Mask == nil || trace.Mask.Text != wantMasked || !trace.Mask.Changed || trace.Mask.Failed {
		t.Fatalf("trace.Mask = %+v, want the masked text", trace.Mask)
	}
	want := []KindCount{{Kind: "EMAIL_ADDRESS", Count: 1}, {Kind: "PERSON", Count: 1}}
	if !equalKinds(trace.Mask.Kinds, want) {
		t.Errorf("kinds = %+v, want %+v", trace.Mask.Kinds, want)
	}
	if !trace.Replied || len(trace.Guardrails) != 1 {
		t.Errorf("trace = %+v, want replied with the guardrail header", trace)
	}
}

func equalKinds(a, b []KindCount) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMask_Positive_RequestShape(t *testing.T) {
	gw := &fakeGateway{}
	c := newMaskingClient(t, gw)
	got, err := c.Mask(context.Background(), "mail a.b@example.org")
	if err != nil {
		t.Fatalf("Mask() unexpected error: %v", err)
	}
	if got != "mail <EMAIL_ADDRESS>" {
		t.Errorf("Mask() = %q", got)
	}
	gw.mu.Lock()
	defer gw.mu.Unlock()
	if len(gw.maskPaths) != 1 || gw.maskPaths[0] != guardrailPath {
		t.Errorf("paths = %q, want %q without /v1", gw.maskPaths, guardrailPath)
	}
	if gw.maskReqs[0].GuardrailName != "presidio-pii" || gw.maskReqs[0].Text != "mail a.b@example.org" {
		t.Errorf("request = %+v", gw.maskReqs[0])
	}
	if gw.maskAuth[0] != "Bearer test-secret-key" {
		t.Errorf("Authorization = %q", gw.maskAuth[0])
	}
}

func TestGuardrailURL_Boundary_TrimsOnlyATrailingV1(t *testing.T) {
	cases := map[string]string{
		"https://gw.example/v1":     "https://gw.example" + guardrailPath,
		"https://gw.example":        "https://gw.example" + guardrailPath,
		"https://gw.example/v1/x":   "https://gw.example/v1/x" + guardrailPath,
		"https://gw.example/api/v1": "https://gw.example/api" + guardrailPath,
	}
	for in, want := range cases {
		if got := guardrailURL(in); got != want {
			t.Errorf("guardrailURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMask_Negative_RepliesWithoutText(t *testing.T) {
	tests := []struct {
		name string
		gw   *fakeGateway
		want error
	}{
		{"no field", &fakeGateway{maskBody: `{"other":"x"}`}, ErrMaskEmpty},
		{"null", &fakeGateway{maskBody: `{"response_text":null}`}, ErrMaskEmpty},
		{"empty", &fakeGateway{maskBody: `{"response_text":""}`}, ErrMaskEmpty},
		{"blank", &fakeGateway{maskBody: `{"response_text":"  \n"}`}, ErrMaskEmpty},
		{"not json", &fakeGateway{maskBody: `<html>`}, ErrMaskEmpty},
		{"oversize", &fakeGateway{maskBody: `{"response_text":"` + strings.Repeat("a", MaxMaskedBytes+1) + `"}`}, ErrMaskEmpty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newMaskingClient(t, tt.gw)
			if _, err := c.Mask(context.Background(), "text"); !errors.Is(err, tt.want) {
				t.Errorf("Mask() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestMask_Boundary_LongestText(t *testing.T) {
	text := strings.Repeat("a", MaxMaskedBytes)
	c := newMaskingClient(t, &fakeGateway{maskBody: `{"response_text":"` + text + `"}`})
	got, err := c.Mask(context.Background(), "text")
	if err != nil || got != text {
		t.Errorf("Mask() = %d bytes, %v; want %d bytes", len(got), err, MaxMaskedBytes)
	}
	if MaskTimeout != 5*time.Second {
		t.Errorf("MaskTimeout = %v, want 5s (#69)", MaskTimeout)
	}
}

func TestMask_Negative_StatusAndTimeoutClassify(t *testing.T) {
	refused := newMaskingClient(t, &fakeGateway{maskStatus: http.StatusForbidden, maskBody: `{"error":"no"}`})
	_, err := refused.Mask(context.Background(), "text")
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusForbidden || Classify(err) == ClassUnavailable {
		t.Errorf("4xx error = %v, want a status error that is not unavailable", err)
	}

	broken := newMaskingClient(t, &fakeGateway{maskStatus: http.StatusBadGateway, maskBody: `{}`})
	if _, err := broken.Mask(context.Background(), "text"); Classify(err) != ClassUnavailable {
		t.Errorf("5xx error = %v, want unavailable", err)
	}

	slow := newMaskingClient(t, &fakeGateway{maskBlock: true})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := slow.Mask(ctx, "text"); !isTimeout(err) {
		t.Errorf("blocked error = %v, want a timeout", err)
	}
}

func TestProbeMask_Negative_UnmaskedAddress(t *testing.T) {
	c := newMaskingClient(t, &fakeGateway{unmasked: true})
	if err := c.ProbeMask(context.Background()); !errors.Is(err, ErrMaskProbeUnmasked) {
		t.Errorf("ProbeMask() = %v, want ErrMaskProbeUnmasked", err)
	}
	ok := newMaskingClient(t, &fakeGateway{})
	if err := ok.ProbeMask(context.Background()); err != nil {
		t.Errorf("ProbeMask() = %v, want nil", err)
	}
}

func TestNewMaskTrace_Positive_CountsBothForms(t *testing.T) {
	m := NewMaskTrace("a b c d", "<PERSON> [EMAIL_ADDRESS_REDACTED] <PERSON> [PHONE_NUMBER_REDACTED]")
	want := []KindCount{{"EMAIL_ADDRESS", 1}, {"PERSON", 2}, {"PHONE_NUMBER", 1}}
	if !equalKinds(m.Kinds, want) || !m.Changed {
		t.Errorf("trace = %+v, want %+v", m, want)
	}
}

// The planted case of #69: a question that quotes a placeholder is not counted as masked data.
func TestNewMaskTrace_Negative_QuotedPlaceholderIsNotCounted(t *testing.T) {
	q := "Why does the log show <EMAIL_ADDRESS> and [PERSON_REDACTED]?"
	m := NewMaskTrace(q, q)
	if len(m.Kinds) != 0 || m.Changed {
		t.Errorf("trace = %+v, want no kinds and unchanged", m)
	}
	m = NewMaskTrace(q+" Mail x@y.z", q+" Mail <EMAIL_ADDRESS>")
	if want := []KindCount{{"EMAIL_ADDRESS", 1}}; !equalKinds(m.Kinds, want) {
		t.Errorf("kinds = %+v, want %+v", m.Kinds, want)
	}
}

func TestNewMaskTrace_Boundary_ChangedWithoutPlaceholdersAndKindCap(t *testing.T) {
	m := NewMaskTrace("call 555 0100", "call ***")
	if !m.Changed || len(m.Kinds) != 0 {
		t.Errorf("trace = %+v, want changed and no kinds", m)
	}
	var b strings.Builder
	for i := range maxMaskKinds + 5 {
		b.WriteString("<K" + strings.Repeat("A", i+1) + "> ")
	}
	if m := NewMaskTrace("x", b.String()); len(m.Kinds) != maxMaskKinds {
		t.Errorf("kinds = %d, want the cap %d", len(m.Kinds), maxMaskKinds)
	}
	long := "<" + strings.Repeat("A", 65) + ">"
	if m := NewMaskTrace("x", long); len(m.Kinds) != 0 {
		t.Errorf("a 65-character kind counted: %+v", m.Kinds)
	}
}
