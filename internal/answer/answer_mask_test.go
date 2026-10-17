package answer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/model"
	"github.com/tekmora/diallak/internal/retrieval"
)

// fakeMasker masks like the gateway would and records what it was given.
type fakeMasker struct {
	masked    string
	err       error
	reply     string
	questions []string
	prompts   []string
}

func (f *fakeMasker) CompleteMasked(_ context.Context, _, question string, prompt func(string) string) (string, model.Trace, error) {
	f.questions = append(f.questions, question)
	if f.err != nil {
		return "", model.Trace{Sent: true, Mask: &model.MaskTrace{Failed: true}}, f.err
	}
	f.prompts = append(f.prompts, prompt(f.masked))
	mask := model.NewMaskTrace(question, f.masked)
	return f.reply, model.Trace{Sent: true, Replied: true, Mask: &mask}, nil
}

const maskedQuestion = "Why does <PERSON> see the import stop?"

func newMaskedPipeline(t *testing.T, m *fakeMasker) (*Pipeline, *recordingRetriever) {
	t.Helper()
	ret := &recordingRetriever{hits: []retrieval.Hit{
		sampleHit("c1", "Import", "Stops", "The import stops when the disk is full.", "abc123"),
	}}
	p, err := New(Config{Retriever: ret, Masker: m, Options: retrieval.DefaultOptions()})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return p, ret
}

// Retrieval runs on the question as written; the prompt for the model holds the masked text only.
func TestAsk_Positive_MaskedTextBuildsThePrompt(t *testing.T) {
	m := &fakeMasker{masked: maskedQuestion, reply: "The disk is full [1]."}
	p, ret := newMaskedPipeline(t, m)
	question := "Why does Jane Roe see the import stop?"

	ans, err := p.Ask(context.Background(), question)
	if err != nil {
		t.Fatalf("Ask() error: %v", err)
	}
	if ans.Kind != KindAnswer {
		t.Fatalf("Kind = %v, want answer", ans.Kind)
	}
	if len(ret.queries) != 1 || !strings.Contains(ret.queries[0], "Jane Roe") {
		t.Errorf("retrieval queries = %q, want the question as written", ret.queries)
	}
	if len(m.prompts) != 1 || !strings.Contains(m.prompts[0], maskedQuestion) || strings.Contains(m.prompts[0], "Jane Roe") {
		t.Errorf("prompt = %q, want the masked text and not the name", m.prompts)
	}
	if ans.Trace.Mask == nil || ans.Trace.Mask.Text != maskedQuestion {
		t.Errorf("trace mask = %+v, want the masked text", ans.Trace.Mask)
	}
}

func TestAsk_Negative_MaskFailureAnswersWithExcerpts(t *testing.T) {
	m := &fakeMasker{err: errors.New("mask question: guardrail reply holds no masked text")}
	p, _ := newMaskedPipeline(t, m)

	ans, err := p.Ask(context.Background(), "Why does the import stop?")
	if err != nil {
		t.Fatalf("Ask() error: %v", err)
	}
	if ans.Kind != KindExcerpts || ans.Reason != ReasonModelUnavailable {
		t.Errorf("answer = %v/%v, want excerpts for an unavailable model", ans.Kind, ans.Reason)
	}
	if ans.Trace.Mask == nil || !ans.Trace.Mask.Failed {
		t.Errorf("trace = %+v, want a failed mask", ans.Trace)
	}
	if len(m.prompts) != 0 {
		t.Errorf("prompts = %q, want none", m.prompts)
	}
}

// Excerpts never asks the masking route either: the forum answer sends no text out.
func TestExcerpts_Boundary_NoMaskingRequest(t *testing.T) {
	m := &fakeMasker{masked: maskedQuestion}
	p, _ := newMaskedPipeline(t, m)
	if _, err := p.Excerpts(context.Background(), "Why does the import stop?"); err != nil {
		t.Fatalf("Excerpts() error: %v", err)
	}
	if len(m.questions) != 0 {
		t.Errorf("masking requests = %q, want none", m.questions)
	}
}
