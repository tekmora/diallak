package answer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/retrieval"
)

// recordingRetriever keeps the options of the last search and every query.
type recordingRetriever struct {
	hits    []retrieval.Hit
	opt     retrieval.Options
	queries []string
}

func (r *recordingRetriever) Search(q string, opt retrieval.Options) ([]retrieval.Hit, error) {
	r.opt = opt
	r.queries = append(r.queries, q)
	return r.hits, nil
}

func manyHits(n int) []retrieval.Hit {
	hits := make([]retrieval.Hit, n)
	for i := range hits {
		id := fmt.Sprintf("doc%d", i+1)
		hits[i] = sampleHit(id, "Title "+id, "Head", "Text of "+id, "rev111111111111")
	}
	return hits
}

func TestAsk_Negative_ModelErrorTextIsNeverLogged(t *testing.T) {
	const question = "Private question 98765?"
	// An endpoint can echo the prompt in its error. The log line must carry the error class only.
	gen := &fakeGenerator{err: fmt.Errorf("upstream rejected prompt %q with marker-4711", question)}
	var logBuf bytes.Buffer
	p, err := New(Config{
		Retriever: &stubRetriever{hits: manyHits(1)},
		Generator: gen,
		Logger:    slog.New(slog.NewTextHandler(&logBuf, nil)),
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if _, err := p.Ask(context.Background(), question); err != nil {
		t.Fatalf("Ask() failed: %v", err)
	}
	logOut := logBuf.String()
	if !strings.Contains(logOut, "error_class=error") {
		t.Fatalf("log output misses the error class: %s", logOut)
	}
	if strings.Contains(logOut, "98765") || strings.Contains(logOut, "marker-4711") {
		t.Fatalf("log output carries the model error text: %s", logOut)
	}
}

func TestAsk_Negative_CallerCancelledIsAnErrorNotAModelFailure(t *testing.T) {
	gen := &fakeGenerator{err: context.Canceled}
	p, err := New(Config{Retriever: &stubRetriever{hits: manyHits(1)}, Generator: gen})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, askErr := p.Ask(ctx, "Question?")
	if !errors.Is(askErr, context.Canceled) {
		t.Fatalf("Ask() error = %v, want context.Canceled", askErr)
	}
	if stats := p.Stats(); stats.ModelFailures != 0 || stats.Excerpts != 0 || stats.Asked != 0 {
		t.Fatalf("Stats = %+v, want no outcome and no model failure for an abandoned call", stats)
	}
}

func TestAsk_Boundary_HitsBeyondMaxLimitAreNotOffered(t *testing.T) {
	hits := manyHits(retrieval.MaxLimit + 2)

	// The prompt offers MaxLimit sources, so an id above it was never offered.
	gen := &fakeGenerator{scripted: fmt.Sprintf("Answer [%d].", retrieval.MaxLimit+1)}
	p, err := New(Config{Retriever: &stubRetriever{hits: hits}, Generator: gen})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ans, err := p.Ask(context.Background(), "Question?")
	if err != nil {
		t.Fatalf("Ask() failed: %v", err)
	}
	if ans.Kind != KindRefusal || ans.Reason != ReasonUncited {
		t.Fatalf("Outcome = %v/%v, want refusal/uncited for an id that was not offered", ans.Kind, ans.Reason)
	}

	noModel, err := New(Config{Retriever: &stubRetriever{hits: hits}})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	excerpts, err := noModel.Ask(context.Background(), "Question?")
	if err != nil {
		t.Fatalf("Ask() failed: %v", err)
	}
	if len(excerpts.Sources) != retrieval.MaxLimit {
		t.Fatalf("len(Sources) = %d, want %d", len(excerpts.Sources), retrieval.MaxLimit)
	}
}

func TestNew_Boundary_ZeroOptionsUseRetrievalDefaults(t *testing.T) {
	rec := &recordingRetriever{}
	p, err := New(Config{Retriever: rec})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if _, err := p.Ask(context.Background(), "Question?"); err != nil {
		t.Fatalf("Ask() failed: %v", err)
	}
	if rec.opt != retrieval.DefaultOptions() {
		t.Fatalf("options = %+v, want retrieval.DefaultOptions() %+v", rec.opt, retrieval.DefaultOptions())
	}

	custom := retrieval.Options{Limit: 1, MinScore: 0}
	rec2 := &recordingRetriever{}
	p2, err := New(Config{Retriever: rec2, Options: custom})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if _, err := p2.Ask(context.Background(), "Question?"); err != nil {
		t.Fatalf("Ask() failed: %v", err)
	}
	if rec2.opt != custom {
		t.Fatalf("options = %+v, want the configured %+v", rec2.opt, custom)
	}
}

func TestRender_Boundary_NeverExceedsMaxRenderRunes(t *testing.T) {
	sources := make([]Source, 3)
	for i := range sources {
		sources[i] = Source{
			N: i + 1, Title: fmt.Sprintf("Title %d", i+1), Heading: "Head",
			URL: fmt.Sprintf("https://github.com/o/r/blob/rev/docs/f%d.md", i+1), Revision: "abcdef1234567890",
		}
	}

	// A long answer is cut; every cited source stays, because the text refers to it.
	long := Render(Answer{Kind: KindAnswer, Text: strings.Repeat("word ", 1000) + "[1][2][3]", Sources: sources}, RenderOptions{})
	if n := utf8.RuneCountInString(long); n > DefaultMaxRenderRunes {
		t.Fatalf("answer render has %d runes, limit %d", n, DefaultMaxRenderRunes)
	}
	if !strings.HasPrefix(long, LabelAnswer) {
		t.Fatalf("answer render lost its label: %.80s", long)
	}
	for i := range sources {
		if !strings.Contains(long, fmt.Sprintf("[%d] Title %d - Head", i+1, i+1)) {
			t.Fatalf("answer render dropped cited source %d", i+1)
		}
	}
	if !strings.Contains(long, "...") {
		t.Fatalf("answer render does not mark the cut")
	}

	// A single source line longer than the limit cannot be kept whole; the limit still holds.
	huge := Source{N: 1, Title: "T", URL: "https://example.invalid/" + strings.Repeat("a", 3000), Revision: "abc"}
	for _, a := range []Answer{
		{Kind: KindAnswer, Text: "Answer [1].", Sources: []Source{huge}},
		{Kind: KindExcerpts, Reason: ReasonNoModel, Sources: []Source{huge}},
	} {
		if n := utf8.RuneCountInString(Render(a, RenderOptions{})); n > DefaultMaxRenderRunes {
			t.Fatalf("%s render has %d runes, limit %d", a.Kind, n, DefaultMaxRenderRunes)
		}
	}
}
