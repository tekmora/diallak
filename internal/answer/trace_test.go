package answer

import (
	"context"
	"errors"
	"testing"

	"github.com/tekmora/diallak/internal/model"
	"github.com/tekmora/diallak/internal/retrieval"
)

// askWithTrace asks one question against gen, or in excerpt mode when gen is nil.
func askWithTrace(t *testing.T, gen Generator) Answer {
	t.Helper()
	hit := sampleHit("doc1", "Doc One", "Head", "Text", "rev111111111111")
	p, err := New(Config{Retriever: &stubRetriever{hits: []retrieval.Hit{hit}}, Generator: gen})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ans, err := p.Ask(context.Background(), "How do I install it?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	return ans
}

func TestAsk_Positive_EveryReplyAfterAModelCallCarriesItsTrace(t *testing.T) {
	trace := model.Trace{Sent: true, Replied: true, Guardrails: []string{"presidio-pii"}}
	cases := []struct {
		name string
		gen  *fakeGenerator
		want Kind
	}{
		{"answer", &fakeGenerator{scripted: "Install it as [1] says.", trace: trace}, KindAnswer},
		{"refusal", &fakeGenerator{scripted: "NO_ANSWER", trace: trace}, KindRefusal},
		{"excerpts after a failure", &fakeGenerator{err: errors.New("boom"), trace: trace}, KindExcerpts},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ans := askWithTrace(t, tc.gen)
			if ans.Kind != tc.want || !ans.Trace.Sent || len(ans.Trace.Guardrails) != 1 {
				t.Errorf("answer %s with trace %+v, want %s with the generator's trace", ans.Kind, ans.Trace, tc.want)
			}
		})
	}
}

func TestAsk_Negative_ExcerptModeCarriesNoTrace(t *testing.T) {
	if ans := askWithTrace(t, nil); ans.Kind != KindExcerpts || ans.Trace.Sent {
		t.Errorf("answer %s with trace %+v, want excerpts and nothing sent", ans.Kind, ans.Trace)
	}
}
