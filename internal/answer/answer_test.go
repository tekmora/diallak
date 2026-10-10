package answer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/retrieval"
)

type stubRetriever struct {
	hits []retrieval.Hit
	err  error
}

func (s *stubRetriever) Search(_ string, _ retrieval.Options) ([]retrieval.Hit, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.hits, nil
}

type fakeGenerator struct {
	mu       sync.Mutex
	calls    int
	scripted string
	err      error
}

func (f *fakeGenerator) Complete(_ context.Context, _, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.scripted, nil
}

func (f *fakeGenerator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func sampleHit(id, title, heading, text, rev string) retrieval.Hit {
	return retrieval.Hit{
		Chunk: corpus.Chunk{
			ID:         id,
			Repository: "tekmora/test-repo",
			Revision:   rev,
			Path:       "docs/" + id + ".md",
			Title:      title,
			Heading:    heading,
			Text:       text,
		},
		Score: 0.9,
	}
}

func TestNew_Positive_ValidConfigurations(t *testing.T) {
	ret := &stubRetriever{}
	gen := &fakeGenerator{}

	tests := []struct {
		name          string
		cfg           Config
		wantExcerpt   int
		wantAnswer    int
		wantGenerator bool
	}{
		{
			name: "defaults applied",
			cfg: Config{
				Retriever: ret,
			},
			wantExcerpt:   DefaultMaxExcerptRunes,
			wantAnswer:    DefaultMaxAnswerRunes,
			wantGenerator: false,
		},
		{
			name: "custom limits and generator",
			cfg: Config{
				Retriever:       ret,
				Generator:       gen,
				MaxExcerptRunes: 300,
				MaxAnswerRunes:  800,
			},
			wantExcerpt:   300,
			wantAnswer:    800,
			wantGenerator: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.cfg)
			if err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}
			if p.maxExcerptRunes != tc.wantExcerpt {
				t.Errorf("maxExcerptRunes = %d, want %d", p.maxExcerptRunes, tc.wantExcerpt)
			}
			if p.maxAnswerRunes != tc.wantAnswer {
				t.Errorf("maxAnswerRunes = %d, want %d", p.maxAnswerRunes, tc.wantAnswer)
			}
			if (p.generator != nil) != tc.wantGenerator {
				t.Errorf("generator set = %v, want %v", p.generator != nil, tc.wantGenerator)
			}
		})
	}
}

func TestNew_Negative_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "nil retriever",
			cfg: Config{
				Retriever: nil,
			},
			wantErr: "retriever is required",
		},
		{
			name: "negative max excerpt runes",
			cfg: Config{
				Retriever:       &stubRetriever{},
				MaxExcerptRunes: -1,
			},
			wantErr: "negative",
		},
		{
			name: "negative max answer runes",
			cfg: Config{
				Retriever:      &stubRetriever{},
				MaxAnswerRunes: -1,
			},
			wantErr: "negative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if err == nil {
				t.Fatal("New() want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestAsk_Positive_BranchA_NoSourceRefusal(t *testing.T) {
	ret := &stubRetriever{hits: []retrieval.Hit{}}
	gen := &fakeGenerator{scripted: "Should not be called [1]"}
	p, err := New(Config{Retriever: ret, Generator: gen})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ans, err := p.Ask(context.Background(), "Where is the manual?")
	if err != nil {
		t.Fatalf("Ask() unexpected error: %v", err)
	}
	if ans.Kind != KindRefusal {
		t.Errorf("Kind = %v, want %v", ans.Kind, KindRefusal)
	}
	if ans.Reason != ReasonNoSource {
		t.Errorf("Reason = %v, want %v", ans.Reason, ReasonNoSource)
	}
	if gen.callCount() != 0 {
		t.Errorf("generator call count = %d, want 0", gen.callCount())
	}
}

func TestAsk_Positive_BranchB_SourcesExcerptRunesCut(t *testing.T) {
	longText := strings.Repeat("a", 500)
	hit := sampleHit("install", "Install Guide", "Prerequisites", longText, "abcdef1234567890")
	ret := &stubRetriever{hits: []retrieval.Hit{hit}}
	p, err := New(Config{
		Retriever:       ret,
		MaxExcerptRunes: 50,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ans, err := p.Ask(context.Background(), "How to install?")
	if err != nil {
		t.Fatalf("Ask() unexpected error: %v", err)
	}
	if len(ans.Sources) != 1 {
		t.Fatalf("len(Sources) = %d, want 1", len(ans.Sources))
	}
	src := ans.Sources[0]
	if src.N != 1 {
		t.Errorf("N = %d, want 1", src.N)
	}
	wantExcerpt := strings.Repeat("a", 50) + "..."
	if src.Excerpt != wantExcerpt {
		t.Errorf("Excerpt = %q, want %q", src.Excerpt, wantExcerpt)
	}
	if src.Revision != "abcdef1234567890" {
		t.Errorf("Revision = %q, want abcdef1234567890", src.Revision)
	}
	if !strings.HasPrefix(src.URL, "https://github.com/") {
		t.Errorf("URL = %q, want GitHub URL", src.URL)
	}
}

func TestAsk_Positive_BranchC_NoModelConfiguredExcerpts(t *testing.T) {
	hit1 := sampleHit("doc1", "Doc One", "Section 1", "Content 1", "rev111111111111")
	hit2 := sampleHit("doc2", "Doc Two", "Section 2", "Content 2", "rev222222222222")
	ret := &stubRetriever{hits: []retrieval.Hit{hit1, hit2}}
	p, err := New(Config{
		Retriever: ret,
		Generator: nil,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ans, err := p.Ask(context.Background(), "General query?")
	if err != nil {
		t.Fatalf("Ask() unexpected error: %v", err)
	}
	if ans.Kind != KindExcerpts {
		t.Errorf("Kind = %v, want %v", ans.Kind, KindExcerpts)
	}
	if ans.Reason != ReasonNoModel {
		t.Errorf("Reason = %v, want %v", ans.Reason, ReasonNoModel)
	}
	if len(ans.Sources) != 2 {
		t.Fatalf("len(Sources) = %d, want 2", len(ans.Sources))
	}
}

func TestAsk_Negative_BranchD_ModelUnavailableExcerptsAndSlog(t *testing.T) {
	hit := sampleHit("doc1", "Doc One", "Head", "Text content", "rev111111111111")
	ret := &stubRetriever{hits: []retrieval.Hit{hit}}
	gen := &fakeGenerator{err: errors.New("connection failed")}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	p, err := New(Config{
		Retriever: ret,
		Generator: gen,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	const secretQuestion = "Secret Question 12345?"
	ans, err := p.Ask(context.Background(), secretQuestion)
	if err != nil {
		t.Fatalf("Ask() want nil error on model failure, got: %v", err)
	}
	if ans.Kind != KindExcerpts {
		t.Errorf("Kind = %v, want %v", ans.Kind, KindExcerpts)
	}
	if ans.Reason != ReasonModelUnavailable {
		t.Errorf("Reason = %v, want %v", ans.Reason, ReasonModelUnavailable)
	}
	if len(ans.Sources) != 1 {
		t.Errorf("len(Sources) = %d, want 1", len(ans.Sources))
	}

	stats := p.Stats()
	if stats.ModelFailures != 1 {
		t.Errorf("ModelFailures = %d, want 1", stats.ModelFailures)
	}

	logOut := logBuf.String()
	if !strings.Contains(logOut, "reason=model_unavailable") {
		t.Errorf("log output missing reason: %s", logOut)
	}
	if !strings.Contains(logOut, "error_class=error") {
		t.Errorf("log output missing error_class: %s", logOut)
	}
	if strings.Contains(logOut, secretQuestion) {
		t.Fatalf("log output leaked question: %s", logOut)
	}
}

func TestAsk_Negative_BranchD_DeadlineTimeout(t *testing.T) {
	hit := sampleHit("doc1", "Doc One", "Head", "Text", "rev111111111111")
	ret := &stubRetriever{hits: []retrieval.Hit{hit}}
	gen := &fakeGenerator{err: context.DeadlineExceeded}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	p, err := New(Config{
		Retriever: ret,
		Generator: gen,
		Logger:    logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ans, err := p.Ask(context.Background(), "Question?")
	if err != nil {
		t.Fatalf("Ask() failed: %v", err)
	}
	if ans.Kind != KindExcerpts || ans.Reason != ReasonModelUnavailable {
		t.Errorf("Outcome = %v/%v, want excerpts/model_unavailable", ans.Kind, ans.Reason)
	}

	logOut := logBuf.String()
	if !strings.Contains(logOut, "error_class=timeout") {
		t.Errorf("log output want error_class=timeout, got: %s", logOut)
	}
}

func TestAsk_Positive_BranchE_NoAnswerSentinelRefusal(t *testing.T) {
	hit := sampleHit("doc1", "Doc One", "Head", "Text", "rev111111111111")
	ret := &stubRetriever{hits: []retrieval.Hit{hit}}
	gen := &fakeGenerator{scripted: "  NO_ANSWER  \n"}

	p, err := New(Config{
		Retriever: ret,
		Generator: gen,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ans, err := p.Ask(context.Background(), "Unknown detail?")
	if err != nil {
		t.Fatalf("Ask() failed: %v", err)
	}
	if ans.Kind != KindRefusal {
		t.Errorf("Kind = %v, want %v", ans.Kind, KindRefusal)
	}
	if ans.Reason != ReasonNotInSources {
		t.Errorf("Reason = %v, want %v", ans.Reason, ReasonNotInSources)
	}
}

func TestAsk_Negative_BranchF_UncitedOrInvalidCitationsRefusal(t *testing.T) {
	hits := []retrieval.Hit{
		sampleHit("doc1", "Doc One", "H1", "T1", "rev111111111111"),
		sampleHit("doc2", "Doc Two", "H2", "T2", "rev222222222222"),
		sampleHit("doc3", "Doc Three", "H3", "T3", "rev333333333333"),
	}

	tests := []struct {
		name       string
		scripted   string
		wantReason Reason
	}{
		{
			name:       "zero citations",
			scripted:   "This is an answer with no citations whatsoever.",
			wantReason: ReasonUncited,
		},
		{
			name:       "citation index zero [0]",
			scripted:   "Answer citing non-existent [0].",
			wantReason: ReasonUncited,
		},
		{
			name:       "citation out of bounds [4] with 3 sources",
			scripted:   "Answer citing out-of-range [4].",
			wantReason: ReasonUncited,
		},
		{
			name:       "spaced brackets not a citation [ 1 ]",
			scripted:   "Answer with bracket spaces [ 1 ] only.",
			wantReason: ReasonUncited,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ret := &stubRetriever{hits: hits}
			gen := &fakeGenerator{scripted: tc.scripted}
			p, err := New(Config{Retriever: ret, Generator: gen})
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ans, err := p.Ask(context.Background(), "Question?")
			if err != nil {
				t.Fatalf("Ask() failed: %v", err)
			}
			if ans.Kind != KindRefusal || ans.Reason != tc.wantReason {
				t.Errorf("Outcome = %v/%v, want refusal/%v", ans.Kind, ans.Reason, tc.wantReason)
			}
		})
	}
}

func TestAsk_Positive_BranchG_ValidAnswerCitesSources(t *testing.T) {
	hits := []retrieval.Hit{
		sampleHit("doc1", "Doc One", "H1", "T1", "rev111111111111"),
		sampleHit("doc2", "Doc Two", "H2", "T2", "rev222222222222"),
		sampleHit("doc3", "Doc Three", "H3", "T3", "rev333333333333"),
	}

	tests := []struct {
		name        string
		scripted    string
		wantSources []int
	}{
		{
			name:        "leading zero citation [01] counts as 1",
			scripted:    "According to [01], the system is configured.",
			wantSources: []int{1},
		},
		{
			name:        "adjacent citations [1][2]",
			scripted:    "Cross-referencing [1][2] confirms the setting.",
			wantSources: []int{1, 2},
		},
		{
			name:        "reordered and duplicated citations [3] [1] [3]",
			scripted:    "Found in [3] and [1], verified again in [3].",
			wantSources: []int{1, 3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ret := &stubRetriever{hits: hits}
			gen := &fakeGenerator{scripted: tc.scripted}
			p, err := New(Config{Retriever: ret, Generator: gen})
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ans, err := p.Ask(context.Background(), "Question?")
			if err != nil {
				t.Fatalf("Ask() failed: %v", err)
			}
			if ans.Kind != KindAnswer || ans.Reason != ReasonNone {
				t.Fatalf("Outcome = %v/%v, want answer/none", ans.Kind, ans.Reason)
			}
			if len(ans.Sources) != len(tc.wantSources) {
				t.Fatalf("got %d sources, want %d", len(ans.Sources), len(tc.wantSources))
			}
			for i, wantN := range tc.wantSources {
				if ans.Sources[i].N != wantN {
					t.Errorf("Sources[%d].N = %d, want %d", i, ans.Sources[i].N, wantN)
				}
			}
		})
	}
}

func TestAsk_Negative_InvalidInputs(t *testing.T) {
	ret := &stubRetriever{}
	p, err := New(Config{Retriever: ret})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	tests := []struct {
		name     string
		question string
		wantErr  string
	}{
		{
			name:     "empty question",
			question: "",
			wantErr:  "empty",
		},
		{
			name:     "whitespace only",
			question: "   \t\n  ",
			wantErr:  "empty",
		},
		{
			name:     "invalid UTF-8",
			question: "Bad bytes \xff\xfe",
			wantErr:  "not valid UTF-8",
		},
		{
			name:     "exceeds max question runes",
			question: strings.Repeat("q", MaxQuestionRunes+1),
			wantErr:  "exceeds maximum",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, askErr := p.Ask(context.Background(), tc.question)
			if askErr == nil {
				t.Fatal("Ask() want error, got nil")
			}
			if !strings.Contains(askErr.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", askErr.Error(), tc.wantErr)
			}
		})
	}
}

func TestAsk_Negative_SearchError(t *testing.T) {
	ret := &stubRetriever{err: errors.New("database locked")}
	p, err := New(Config{Retriever: ret})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, askErr := p.Ask(context.Background(), "Question?")
	if askErr == nil {
		t.Fatal("Ask() want error when search fails, got nil")
	}
	if !strings.Contains(askErr.Error(), "retrieve: database locked") {
		t.Errorf("unexpected error: %v", askErr)
	}
}

func TestAsk_Boundary_QuestionLengthLimits(t *testing.T) {
	hit := sampleHit("doc1", "Doc One", "H1", "T1", "rev111111111111")
	ret := &stubRetriever{hits: []retrieval.Hit{hit}}
	p, err := New(Config{Retriever: ret})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Boundary: 1 rune
	if _, err := p.Ask(context.Background(), "?"); err != nil {
		t.Errorf("Ask() with 1 rune failed: %v", err)
	}

	// Boundary: exactly MaxQuestionRunes (500 runes)
	q500 := strings.Repeat("a", MaxQuestionRunes)
	if _, err := p.Ask(context.Background(), q500); err != nil {
		t.Errorf("Ask() with 500 runes failed: %v", err)
	}

	// Boundary: MaxQuestionRunes + 1
	q501 := strings.Repeat("a", MaxQuestionRunes+1)
	if _, err := p.Ask(context.Background(), q501); err == nil {
		t.Error("Ask() with 501 runes passed, want error")
	}
}

func TestAsk_NegativeControl_UncitedModelTextNeverAppearsInReply(t *testing.T) {
	hit := sampleHit("doc1", "Doc One", "H1", "Safe documentation text.", "rev111111111111")
	ret := &stubRetriever{hits: []retrieval.Hit{hit}}

	const secretUncitedAnswer = "CLASSIFIED_MODEL_HALLUCINATION_42"
	gen := &fakeGenerator{scripted: secretUncitedAnswer}

	p, err := New(Config{Retriever: ret, Generator: gen})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ans, err := p.Ask(context.Background(), "What is the secret?")
	if err != nil {
		t.Fatalf("Ask() unexpected error: %v", err)
	}
	if ans.Kind != KindRefusal {
		t.Errorf("Kind = %v, want refusal", ans.Kind)
	}
	if ans.Reason != ReasonUncited {
		t.Errorf("Reason = %v, want uncited", ans.Reason)
	}
	if strings.Contains(ans.Text, secretUncitedAnswer) {
		t.Fatalf("ans.Text leaked uncited model text: %q", ans.Text)
	}

	rendered := Render(ans)
	if strings.Contains(rendered, secretUncitedAnswer) {
		t.Fatalf("rendered output leaked uncited model text: %q", rendered)
	}
	wantRendered := LabelRefusal + "\n\n" + LabelFooter
	if rendered != wantRendered {
		t.Errorf("rendered = %q, want %q", rendered, wantRendered)
	}
}

func TestPipeline_Stats_Sums(t *testing.T) {
	hit := sampleHit("doc1", "Doc One", "H1", "T1", "rev111111111111")
	ret := &stubRetriever{hits: []retrieval.Hit{hit}}
	gen := &fakeGenerator{scripted: "Answer [1]."}

	p, err := New(Config{Retriever: ret, Generator: gen})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Initial stats
	initStats := p.Stats()
	if initStats.Asked != 0 {
		t.Errorf("initial asked = %d, want 0", initStats.Asked)
	}

	// 1 answered
	if _, err := p.Ask(context.Background(), "Question 1"); err != nil {
		t.Fatalf("Ask 1 failed: %v", err)
	}
	// 1 refusal (empty sources)
	ret.hits = []retrieval.Hit{}
	if _, err := p.Ask(context.Background(), "Question 2"); err != nil {
		t.Fatalf("Ask 2 failed: %v", err)
	}
	// 1 excerpt (model error)
	ret.hits = []retrieval.Hit{hit}
	gen.err = errors.New("timeout")
	if _, err := p.Ask(context.Background(), "Question 3"); err != nil {
		t.Fatalf("Ask 3 failed: %v", err)
	}
	// 1 error (invalid input - must NOT increment Asked/Answered/Excerpts/Refused)
	if _, err := p.Ask(context.Background(), ""); err == nil {
		t.Fatal("Ask empty question want error, got nil")
	}

	s := p.Stats()
	if s.Answered != 1 {
		t.Errorf("Answered = %d, want 1", s.Answered)
	}
	if s.Refused != 1 {
		t.Errorf("Refused = %d, want 1", s.Refused)
	}
	if s.Excerpts != 1 {
		t.Errorf("Excerpts = %d, want 1", s.Excerpts)
	}
	if s.ModelFailures != 1 {
		t.Errorf("ModelFailures = %d, want 1", s.ModelFailures)
	}
	if s.Asked != s.Answered+s.Excerpts+s.Refused {
		t.Errorf("Stats sum mismatch: Asked %d != %d (%d+%d+%d)",
			s.Asked, s.Answered+s.Excerpts+s.Refused, s.Answered, s.Excerpts, s.Refused)
	}
}

func TestPipeline_Positive_ConcurrentSafe(t *testing.T) {
	hit := sampleHit("doc1", "Doc One", "H1", "T1", "rev111111111111")
	ret := &stubRetriever{hits: []retrieval.Hit{hit}}
	gen := &fakeGenerator{scripted: "Answer [1]."}

	p, err := New(Config{Retriever: ret, Generator: gen})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(workerID int) {
			defer wg.Done()
			q := fmt.Sprintf("Question from worker %d?", workerID)
			ans, aErr := p.Ask(context.Background(), q)
			if aErr != nil {
				t.Errorf("worker %d Ask() failed: %v", workerID, aErr)
			}
			if ans.Kind != KindAnswer {
				t.Errorf("worker %d Kind = %v, want answer", workerID, ans.Kind)
			}
		}(i)
	}
	wg.Wait()

	s := p.Stats()
	if s.Asked != workers || s.Answered != workers {
		t.Errorf("Stats Asked = %d, Answered = %d, want %d", s.Asked, s.Answered, workers)
	}
}

func TestRender_Positive_AllKinds(t *testing.T) {
	src1 := Source{
		N:        1,
		Title:    "Guide",
		Heading:  "Intro",
		URL:      "https://github.com/repo/blob/rev/docs/intro.md",
		Revision: "abcdef1234567890",
		Excerpt:  "Intro excerpt text.",
	}
	src2 := Source{
		N:        2,
		Title:    "Reference",
		Heading:  "Config",
		URL:      "https://github.com/repo/blob/rev/docs/config.md",
		Revision: "abcdef1234567890",
		Excerpt:  "Config excerpt text.",
	}

	// KindAnswer
	ans := Answer{
		Kind:    KindAnswer,
		Reason:  ReasonNone,
		Text:    "This is the model generated text.",
		Sources: []Source{src1, src2},
	}
	rendAns := Render(ans)
	if !strings.HasPrefix(rendAns, LabelAnswer) {
		t.Errorf("Render(KindAnswer) want prefix LabelAnswer, got: %s", rendAns)
	}
	if !strings.Contains(rendAns, "Sources:\n[1] Guide - Intro <https://github.com/repo/blob/rev/docs/intro.md> (revision abcdef123456)") {
		t.Errorf("Render(KindAnswer) missing source 1 line: %s", rendAns)
	}

	// KindExcerpts without model unavailable
	exc := Answer{
		Kind:    KindExcerpts,
		Reason:  ReasonNoModel,
		Sources: []Source{src1},
	}
	rendExc := Render(exc)
	if !strings.HasPrefix(rendExc, LabelExcerpts) {
		t.Errorf("Render(KindExcerpts) want prefix LabelExcerpts, got: %s", rendExc)
	}
	if strings.Contains(rendExc, LabelModelUnavailable) {
		t.Errorf("Render(KindExcerpts) should not contain LabelModelUnavailable: %s", rendExc)
	}

	// KindExcerpts with model unavailable
	excUnavail := Answer{
		Kind:    KindExcerpts,
		Reason:  ReasonModelUnavailable,
		Sources: []Source{src1},
	}
	rendUnavail := Render(excUnavail)
	if !strings.Contains(rendUnavail, LabelModelUnavailable) {
		t.Errorf("Render(KindExcerpts) missing LabelModelUnavailable: %s", rendUnavail)
	}

	// KindRefusal
	ref := Answer{
		Kind:   KindRefusal,
		Reason: ReasonNoSource,
	}
	wantRefusal := LabelRefusal + "\n\n" + LabelFooter
	if Render(ref) != wantRefusal {
		t.Errorf("Render(KindRefusal) = %q, want %q", Render(ref), wantRefusal)
	}
}

func TestRender_Boundary_TrimmingOverMaxRenderRunes(t *testing.T) {
	// Build 10 sources each with 300 runes excerpt
	sources := make([]Source, 10)
	for i := 0; i < 10; i++ {
		sources[i] = Source{
			N:        i + 1,
			Title:    fmt.Sprintf("Title %d", i+1),
			Heading:  fmt.Sprintf("Heading %d", i+1),
			URL:      fmt.Sprintf("https://github.com/repo/blob/rev/docs/file%d.md", i+1),
			Revision: "abcdef1234567890",
			Excerpt:  strings.Repeat("e", 300),
		}
	}

	// Test KindAnswer trimming
	ans := Answer{
		Kind:    KindAnswer,
		Reason:  ReasonNone,
		Text:    strings.Repeat("t", 1000),
		Sources: sources,
	}
	renderedAns := Render(ans)
	if len([]rune(renderedAns)) > MaxRenderRunes {
		t.Errorf("renderedAns rune count %d > %d", len([]rune(renderedAns)), MaxRenderRunes)
	}
	if !strings.Contains(renderedAns, "[1] Title 1 - Heading 1") {
		t.Errorf("renderedAns dropped first source line: %s", renderedAns)
	}

	// Test KindExcerpts trimming
	exc := Answer{
		Kind:    KindExcerpts,
		Reason:  ReasonNoModel,
		Sources: sources,
	}
	renderedExc := Render(exc)
	if len([]rune(renderedExc)) > MaxRenderRunes {
		t.Errorf("renderedExc rune count %d > %d", len([]rune(renderedExc)), MaxRenderRunes)
	}
	if !strings.Contains(renderedExc, "[1] Title 1 - Heading 1") {
		t.Errorf("renderedExc dropped first source line: %s", renderedExc)
	}
}

func TestRender_Boundary_EmptyHeadingAndRevision(t *testing.T) {
	src := Source{
		N:        1,
		Title:    "TopLevel",
		Heading:  "",
		URL:      "https://github.com/repo/blob/rev/README.md",
		Revision: "shortrev",
		Excerpt:  "Some content.",
	}
	ans := Answer{
		Kind:    KindAnswer,
		Sources: []Source{src},
	}
	rend := Render(ans)
	// Empty heading -> omit " - <Heading>"
	if !strings.Contains(rend, "[1] TopLevel <https://github.com/repo/blob/rev/README.md> (revision shortrev)") {
		t.Errorf("unexpected rendered output for empty heading and short revision: %s", rend)
	}
}

func TestTexts_Positive_ExactStringPins(t *testing.T) {
	const (
		wantLabelAnswer    = "AI-generated answer. It can be wrong: check the sources below."
		wantDefaultFooter  = "Automated reply. A person can help in the support channel."
		wantDefaultRefusal = "I found no documented answer to that. Please ask in the support channel so a person can help."
	)

	if LabelAnswer != wantLabelAnswer {
		t.Errorf("LabelAnswer = %q, want %q", LabelAnswer, wantLabelAnswer)
	}
	if DefaultFooter != wantDefaultFooter {
		t.Errorf("DefaultFooter = %q, want %q", DefaultFooter, wantDefaultFooter)
	}
	if LabelFooter != wantDefaultFooter {
		t.Errorf("LabelFooter = %q, want %q", LabelFooter, wantDefaultFooter)
	}
	if DefaultRefusal != wantDefaultRefusal {
		t.Errorf("DefaultRefusal = %q, want %q", DefaultRefusal, wantDefaultRefusal)
	}
	if LabelRefusal != wantDefaultRefusal {
		t.Errorf("LabelRefusal = %q, want %q", LabelRefusal, wantDefaultRefusal)
	}
	if got := Footer(""); got != wantDefaultFooter {
		t.Errorf("Footer(\"\") = %q, want %q", got, wantDefaultFooter)
	}
	if got := Refusal(""); got != wantDefaultRefusal {
		t.Errorf("Refusal(\"\") = %q, want %q", got, wantDefaultRefusal)
	}
	if got := Footer(DefaultPlace); got != wantDefaultFooter {
		t.Errorf("Footer(DefaultPlace) = %q, want %q", got, wantDefaultFooter)
	}
	if got := Refusal(DefaultPlace); got != wantDefaultRefusal {
		t.Errorf("Refusal(DefaultPlace) = %q, want %q", got, wantDefaultRefusal)
	}
}

func TestRender_Positive_ModelWrittenAnswerFirstLineIsMarker(t *testing.T) {
	ans := Answer{
		Kind:    KindAnswer,
		Text:    "Model generated text citing [1].",
		Sources: []Source{{N: 1, Title: "Doc", Path: "doc.md", Revision: "rev1"}},
	}
	rendered := Render(ans)
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 || lines[0] != LabelAnswer {
		t.Fatalf("first line = %q, want exact marker LabelAnswer %q", lines[0], LabelAnswer)
	}
}

func TestRender_Positive_FooterIsLastLineForAllFiveCases(t *testing.T) {
	src := Source{N: 1, Title: "Doc", Heading: "H", URL: "https://example.com", Revision: "rev1", Excerpt: "text"}
	cases := []struct {
		name string
		ans  Answer
	}{
		{
			name: "case 1: answer",
			ans:  Answer{Kind: KindAnswer, Text: "Answer text citing [1].", Sources: []Source{src}},
		},
		{
			name: "case 2: excerpts normal",
			ans:  Answer{Kind: KindExcerpts, Reason: ReasonNoModel, Sources: []Source{src}},
		},
		{
			name: "case 3: excerpts with model unavailable",
			ans:  Answer{Kind: KindExcerpts, Reason: ReasonModelUnavailable, Sources: []Source{src}},
		},
		{
			name: "case 4: refusal",
			ans:  Answer{Kind: KindRefusal, Reason: ReasonNoSource},
		},
		{
			name: "case 5: unknown kind",
			ans:  Answer{Kind: Kind("unknown_kind")},
		},
	}

	places := []struct {
		desc       string
		opt        RenderOptions
		wantFooter string
	}{
		{
			desc:       "default place",
			opt:        RenderOptions{},
			wantFooter: "Automated reply. A person can help in the support channel.",
		},
		{
			desc:       "custom place #help-desk",
			opt:        RenderOptions{Place: "#help-desk"},
			wantFooter: "Automated reply. A person can help in #help-desk.",
		},
	}

	for _, c := range cases {
		for _, p := range places {
			t.Run(c.name+"_"+p.desc, func(t *testing.T) {
				rendered := Render(c.ans, p.opt)
				if !strings.HasSuffix(rendered, "\n\n"+p.wantFooter) {
					t.Fatalf("Render() output does not end with blank line and footer %q:\n%s", p.wantFooter, rendered)
				}
				lines := strings.Split(rendered, "\n")
				lastLine := lines[len(lines)-1]
				if lastLine != p.wantFooter {
					t.Errorf("last line = %q, want %q", lastLine, p.wantFooter)
				}
				if len(lines) < 3 || lines[len(lines)-2] != "" {
					t.Errorf("expected blank line before footer, got line: %q", lines[len(lines)-2])
				}
			})
		}
	}
}

func TestAppendFooter_Positive(t *testing.T) {
	if got := AppendFooter("Base message", ""); got != "Base message\n\n"+DefaultFooter {
		t.Errorf("AppendFooter default = %q, want Base message\\n\\n%s", got, DefaultFooter)
	}
	if got := AppendFooter("Base message", "#help-desk"); got != "Base message\n\nAutomated reply. A person can help in #help-desk." {
		t.Errorf("AppendFooter custom = %q", got)
	}
	if got := AppendFooter("", ""); got != DefaultFooter {
		t.Errorf("AppendFooter empty text = %q, want %q", got, DefaultFooter)
	}
}

func TestRender_Boundary_LimitWith64RuneMultiBytePlace(t *testing.T) {
	// 64 runes using multi-byte character (each rune is 3 bytes)
	place64 := strings.Repeat("界", 64)
	if utf8.RuneCountInString(place64) != 64 {
		t.Fatalf("place64 runes = %d, want 64", utf8.RuneCountInString(place64))
	}
	wantFooter := Footer(place64)

	t.Run("long answer with max sources", func(t *testing.T) {
		sources := make([]Source, retrieval.MaxLimit)
		for i := range sources {
			sources[i] = Source{
				N:        i + 1,
				Title:    fmt.Sprintf("Title %d with long description", i+1),
				Heading:  "Deep Subheading in Documentation",
				URL:      fmt.Sprintf("https://github.com/tekmora/diallak/blob/rev/docs/section%d.md", i+1),
				Revision: "abcdef1234567890",
			}
		}
		longText := strings.Repeat("Long detailed answer text citing [1][2][3][4][5][6][7][8][9][10]. ", 50)
		ans := Answer{
			Kind:    KindAnswer,
			Text:    longText,
			Sources: sources,
		}

		result := Render(ans, RenderOptions{Place: place64})
		runeCount := utf8.RuneCountInString(result)
		if runeCount > MaxRenderRunes {
			t.Fatalf("RuneCountInString = %d, exceeds MaxRenderRunes %d", runeCount, MaxRenderRunes)
		}
		if !strings.HasSuffix(result, "\n\n"+wantFooter) {
			t.Fatalf("result does not end with full footer %q; tail is: %q", wantFooter, result[len(result)-len(wantFooter)-10:])
		}
		if !strings.HasPrefix(result, LabelAnswer) {
			t.Fatalf("result does not start with LabelAnswer")
		}
	})

	t.Run("long excerpts with max sources", func(t *testing.T) {
		sources := make([]Source, retrieval.MaxLimit)
		for i := range sources {
			sources[i] = Source{
				N:        i + 1,
				Title:    fmt.Sprintf("Title %d with long description", i+1),
				Heading:  "Deep Subheading in Documentation",
				URL:      fmt.Sprintf("https://github.com/tekmora/diallak/blob/rev/docs/section%d.md", i+1),
				Revision: "abcdef1234567890",
				Excerpt:  strings.Repeat("Long excerpt line from section. ", 20),
			}
		}
		exc := Answer{
			Kind:    KindExcerpts,
			Sources: sources,
		}

		result := Render(exc, RenderOptions{Place: place64})
		runeCount := utf8.RuneCountInString(result)
		if runeCount > MaxRenderRunes {
			t.Fatalf("RuneCountInString = %d, exceeds MaxRenderRunes %d", runeCount, MaxRenderRunes)
		}
		if !strings.HasSuffix(result, "\n\n"+wantFooter) {
			t.Fatalf("result does not end with full footer %q", wantFooter)
		}
		if !strings.HasPrefix(result, LabelExcerpts) {
			t.Fatalf("result does not start with LabelExcerpts")
		}
	})
}
