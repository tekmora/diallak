package answer

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/eval"
	"github.com/tekmora/diallak/internal/retrieval"
)

func loadSnatchArrFixture(t *testing.T) (eval.Set, *retrieval.Index) {
	t.Helper()
	setFS := os.DirFS("../eval/testdata/snatcharr")
	set, err := eval.LoadSet(setFS, "cases.json")
	if err != nil {
		t.Fatalf("eval.LoadSet error: %v", err)
	}

	corpusFS := os.DirFS("../eval/testdata/snatcharr/corpus")
	chunks, err := corpus.Load(context.Background(), corpusFS, set.Corpus, corpus.DefaultLimits())
	if err != nil {
		t.Fatalf("corpus.Load error: %v", err)
	}

	ix, err := retrieval.Build(chunks)
	if err != nil {
		t.Fatalf("retrieval.Build error: %v", err)
	}
	return set, ix
}

func TestIntegration_SnatchArr_OffTopicRefusalAndAnswerableCitations(t *testing.T) {
	set, ix := loadSnatchArrFixture(t)
	gen := &fakeGenerator{scripted: "Answer [1]."}

	p, err := New(Config{
		Retriever: ix,
		Generator: gen,
		Options:   retrieval.DefaultOptions(),
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	verifyOffTopicCases(t, p, gen, set.Cases)
	verifyAnswerableCases(t, p, ix, set.Cases)
}

func verifyOffTopicCases(t *testing.T, p *Pipeline, gen *fakeGenerator, cases []eval.Case) {
	t.Helper()
	offTopicCount := 0
	for _, c := range cases {
		if c.Kind != eval.KindOffTopic {
			continue
		}
		offTopicCount++
		initialCalls := gen.callCount()
		ans, askErr := p.Ask(context.Background(), c.Question)
		if askErr != nil {
			t.Fatalf("case %s Ask() unexpected error: %v", c.ID, askErr)
		}
		if ans.Kind != KindRefusal || ans.Reason != ReasonNoSource {
			t.Errorf("case %s Outcome = %v/%v, want refusal/no_source", c.ID, ans.Kind, ans.Reason)
		}
		if gen.callCount() != initialCalls {
			t.Errorf("case %s called generator (%d calls), want 0", c.ID, gen.callCount()-initialCalls)
		}
	}
	if offTopicCount == 0 {
		t.Fatal("zero off-topic cases verified")
	}
}

func verifyAnswerableCases(t *testing.T, p *Pipeline, ix *retrieval.Index, cases []eval.Case) {
	t.Helper()
	answerableHits := 0
	for _, c := range cases {
		if c.Kind != eval.KindAnswerable {
			continue
		}
		hits, sErr := ix.Search(c.Question, retrieval.DefaultOptions())
		if sErr != nil {
			t.Fatalf("case %s Search() error: %v", c.ID, sErr)
		}
		if len(hits) == 0 {
			continue
		}
		answerableHits++

		ans, askErr := p.Ask(context.Background(), c.Question)
		if askErr != nil {
			t.Fatalf("case %s Ask() unexpected error: %v", c.ID, askErr)
		}
		if ans.Kind != KindAnswer {
			t.Errorf("case %s Kind = %v, want answer", c.ID, ans.Kind)
		}
		if len(ans.Sources) != 1 || ans.Sources[0].N != 1 {
			t.Errorf("case %s Sources = %+v, want exactly source 1", c.ID, ans.Sources)
		}
		rendered := Render(ans, RenderOptions{})
		if !strings.HasPrefix(rendered, LabelAnswer) {
			t.Errorf("case %s rendered prefix want LabelAnswer, got: %s", c.ID, rendered)
		}
		if !strings.Contains(rendered, "revision 9e70c2e03d22") {
			t.Errorf("case %s rendered missing revision: %s", c.ID, rendered)
		}
	}
	if answerableHits < 28 {
		t.Errorf("answerableHits = %d, want at least 28", answerableHits)
	}
}
