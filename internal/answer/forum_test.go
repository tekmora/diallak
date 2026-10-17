package answer

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/retrieval"
)

// ADR-0009, "Where the text goes": the forum reply holds excerpts only, and the text of a post never
// reaches the model endpoint, whatever the model settings are.
func TestExcerpts_Positive_NeverCallsTheGenerator(t *testing.T) {
	gen := &fakeGenerator{scripted: "An answer [1]"}
	p, err := New(Config{
		Retriever: &stubRetriever{hits: []retrieval.Hit{sampleHit("install", "Install", "Linux", "Run the installer.", "abc123")}},
		Generator: gen,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := p.Excerpts(context.Background(), "How do I install it on Linux?")
	if err != nil {
		t.Fatalf("Excerpts: %v", err)
	}
	if gen.callCount() != 0 {
		t.Errorf("generator called %d times, want 0", gen.callCount())
	}
	if got.Kind != KindExcerpts || got.Reason != ReasonNoModel || len(got.Sources) != 1 {
		t.Errorf("Excerpts = %+v, want one excerpt with reason %q", got, ReasonNoModel)
	}
	if s := p.Stats(); s.Asked != 1 || s.Excerpts != 1 {
		t.Errorf("Stats = %+v, want asked 1 and excerpts 1", s)
	}
}

// ADR-0009 case 9: without a passage above the threshold the result is a refusal, which the forum
// does not post.
func TestExcerpts_Negative_NoPassageIsARefusal(t *testing.T) {
	p, err := New(Config{Retriever: &stubRetriever{}, Generator: &fakeGenerator{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := p.Excerpts(context.Background(), "Something the documentation does not hold")
	if err != nil {
		t.Fatalf("Excerpts: %v", err)
	}
	if got.Kind != KindRefusal || got.Reason != ReasonNoSource {
		t.Errorf("Excerpts = %+v, want a refusal with reason %q", got, ReasonNoSource)
	}
	if RenderForum(got, 0) != "" {
		t.Errorf("RenderForum(refusal) = %q, want empty", RenderForum(got, 0))
	}
}

func TestExcerpts_Negative_InvalidQuestionIsAnError(t *testing.T) {
	p, err := New(Config{Retriever: &stubRetriever{}, MaxQuestionRunes: 10})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, q := range []string{"", "   ", "\xff", strings.Repeat("a", 11)} {
		if _, err := p.Excerpts(context.Background(), q); err == nil {
			t.Errorf("Excerpts(%q) error = nil, want an error", q)
		}
	}
}

// ADR-0009 case 13: the first line and the last line of every forum reply are exactly the two texts
// of the record.
func TestRenderForum_Positive_FirstAndLastLine(t *testing.T) {
	a := Answer{Kind: KindExcerpts, Reason: ReasonNoModel, Sources: []Source{
		{N: 1, Title: "Install", URL: "https://example.org/install", Revision: "abc123", Excerpt: "Run the installer."},
	}}
	got := RenderForum(a, 0)
	lines := strings.Split(got, "\n")
	if lines[0] != LabelExcerpts {
		t.Errorf("first line = %q, want %q", lines[0], LabelExcerpts)
	}
	if lines[len(lines)-1] != FooterForum {
		t.Errorf("last line = %q, want %q", lines[len(lines)-1], FooterForum)
	}
	if !strings.Contains(got, "Run the installer.") {
		t.Errorf("RenderForum = %q, want the excerpt", got)
	}
	if FooterForum != "Automated first reply. A person can help here in this post." {
		t.Errorf("FooterForum = %q, want the text of ADR-0009", FooterForum)
	}
}

// A forum reply keeps the limit, and the footer stays whole when excerpts are dropped.
func TestRenderForum_Boundary_LimitKeepsTheFooter(t *testing.T) {
	long := strings.Repeat("x", 400)
	a := Answer{Kind: KindExcerpts, Reason: ReasonNoModel}
	for i := 1; i <= 5; i++ {
		a.Sources = append(a.Sources, Source{N: i, Title: "T", URL: "https://example.org", Revision: "r", Excerpt: long})
	}
	got := RenderForum(a, MinRenderRunes)
	if n := utf8.RuneCountInString(got); n > MinRenderRunes {
		t.Errorf("RenderForum length = %d runes, want at most %d", n, MinRenderRunes)
	}
	if !strings.HasSuffix(got, "\n\n"+FooterForum) || !strings.HasPrefix(got, LabelExcerpts) {
		t.Errorf("RenderForum = %q, want the label first and the footer last", got)
	}
}

// Only an excerpt reply has a forum form: a model answer must never be posted there.
func TestRenderForum_Negative_OtherKindsRenderNothing(t *testing.T) {
	for _, k := range []Kind{KindAnswer, KindRefusal, Kind("other")} {
		a := Answer{Kind: k, Text: "text", Sources: []Source{{N: 1, Title: "T", URL: "u", Revision: "r"}}}
		if got := RenderForum(a, 0); got != "" {
			t.Errorf("RenderForum(kind %q) = %q, want empty", k, got)
		}
	}
}
