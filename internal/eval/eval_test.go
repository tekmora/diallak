package eval

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/retrieval"
)

type stubRetriever struct {
	hits []retrieval.Hit
	err  error
}

func (s *stubRetriever) SearchMode(_ context.Context, _ string, _ retrieval.Options) ([]retrieval.Hit, retrieval.Mode, error) {
	return s.hits, retrieval.ModeKeyword, s.err
}

func sampleSetJSON() string {
	return `{
  "version": 1,
  "corpus": {
    "repository": "owner/repo",
    "revision": "main",
    "paths": ["README.md"]
  },
  "cases": [
    {"id": "ans-1", "kind": "answerable", "question": "How to deploy?", "sources": ["docs/deploy.md"]},
    {"id": "off-1", "kind": "off_topic", "question": "Weather in Paris?"},
    {"id": "undoc-1", "kind": "undocumented", "question": "Is Windows supported?"}
  ]
}`
}

func TestLoadSet_Positive_LoadsValidSet(t *testing.T) {
	fsys := fstest.MapFS{"cases.json": &fstest.MapFile{Data: []byte(sampleSetJSON())}}
	set, err := LoadSet(fsys, "cases.json")
	if err != nil {
		t.Fatalf("LoadSet error = %v, want nil", err)
	}
	if set.Version != 1 || len(set.Cases) != 3 {
		t.Errorf("LoadSet version = %d, cases = %d", set.Version, len(set.Cases))
	}
}

func TestLoadSet_Negative_ErrorsOnMalformedInputs(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr string
	}{
		{"version zero", `{"version": 0, "cases": [{"id": "1", "kind": "off_topic", "question": "q"}]}`, "unsupported version"},
		{"zero cases", `{"version": 1, "cases": []}`, "zero cases in set"},
		{"empty case id", `{"version": 1, "cases": [{"id": "", "kind": "off_topic", "question": "q"}]}`, "empty case ID"},
		{"duplicate id", `{"version": 1, "cases": [{"id": "c1", "kind": "off_topic", "question": "q"}, {"id": "c1", "kind": "off_topic", "question": "q2"}]}`, "duplicate case ID \"c1\""},
		{"unknown kind", `{"version": 1, "cases": [{"id": "c1", "kind": "random", "question": "q"}]}`, "unknown kind \"random\""},
		{"empty question", `{"version": 1, "cases": [{"id": "c1", "kind": "off_topic", "question": "   "}]}`, "empty question"},
		{"answerable without sources", `{"version": 1, "cases": [{"id": "c1", "kind": "answerable", "question": "q", "sources": []}]}`, "answerable without sources"},
		{"paraphrase without sources", `{"version": 1, "cases": [{"id": "c1", "kind": "paraphrase", "question": "q", "sources": []}]}`, "paraphrase without sources"},
		{"off topic with sources", `{"version": 1, "cases": [{"id": "c1", "kind": "off_topic", "question": "q", "sources": ["a.md"]}]}`, "non-answerable with sources"},
		{"trailing data", sampleSetJSON() + "\n extra", "trailing data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{"cases.json": &fstest.MapFile{Data: []byte(tc.json)}}
			_, err := LoadSet(fsys, "cases.json")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("LoadSet error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidate_Positive_AllSourcesExist(t *testing.T) {
	set := Set{
		Cases: []Case{
			{ID: "c1", Kind: KindAnswerable, Question: "q", Sources: []string{"docs/a.md", "README.md"}},
		},
	}
	chunks := []corpus.Chunk{
		{Path: "docs/a.md"},
		{Path: "README.md"},
	}
	if err := Validate(set, chunks); err != nil {
		t.Errorf("Validate = %v, want nil", err)
	}
}

func TestValidate_Negative_MissingSourceErrors(t *testing.T) {
	set := Set{
		Cases: []Case{
			{ID: "c1", Kind: KindAnswerable, Question: "q", Sources: []string{"docs/missing.md"}},
		},
	}
	chunks := []corpus.Chunk{{Path: "docs/a.md"}}
	if err := Validate(set, chunks); err == nil || !strings.Contains(err.Error(), "docs/missing.md") {
		t.Errorf("Validate missing source err = %v, want containing docs/missing.md", err)
	}
}

func TestValidate_Negative_ParaphraseHighOverlap(t *testing.T) {
	set := Set{
		Cases: []Case{
			{ID: "para-dup", Kind: KindParaphrase, Question: "How to deploy?", Sources: []string{"docs/deploy.md"}},
		},
	}
	chunks := []corpus.Chunk{{Path: "docs/deploy.md", Text: "How to deploy application on server"}}
	err := Validate(set, chunks)
	if err == nil {
		t.Fatal("Validate want error for high overlap paraphrase, got nil")
	}
	if !strings.Contains(err.Error(), "para-dup") {
		t.Errorf("Validate error %q does not name case para-dup", err.Error())
	}
}

func TestRun_Positive_EvaluatesKinds(t *testing.T) {
	set := Set{
		Cases: []Case{
			{ID: "ans-1", Kind: KindAnswerable, Question: "q", Sources: []string{"docs/deploy.md"}},
			{ID: "off-1", Kind: KindOffTopic, Question: "off"},
			{ID: "undoc-1", Kind: KindUndocumented, Question: "undoc"},
		},
	}
	stub := &stubRetriever{
		hits: []retrieval.Hit{
			{Chunk: corpus.Chunk{Path: "docs/deploy.md"}, Score: 0.8},
		},
	}
	rep, err := Run(context.Background(), set, stub, retrieval.DefaultOptions())
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if rep.AnswerableHit != 1 || rep.OffTopicRefused != 0 || rep.UndocumentedRefused != 0 {
		t.Errorf("Run unexpected report: %+v", rep)
	}
}

func TestCheck_Negative_ControlsProveGateFails(t *testing.T) {
	set := Set{
		Cases: []Case{
			{ID: "ans-1", Kind: KindAnswerable, Question: "q", Sources: []string{"docs/deploy.md"}},
			{ID: "off-1", Kind: KindOffTopic, Question: "off"},
		},
	}
	bars := Bars{MinAnswerableHit: 1, MinOffTopicRefused: 1}

	// Negative control (a): No hits fails answerable bar
	noHitsRetriever := &stubRetriever{hits: nil}
	repNoHits, err := Run(context.Background(), set, noHitsRetriever, retrieval.DefaultOptions())
	if err != nil {
		t.Fatalf("Run noHits err = %v", err)
	}
	errCheckNoHits := Check(repNoHits, bars)
	if errCheckNoHits == nil || !strings.Contains(errCheckNoHits.Error(), "ans-1") {
		t.Errorf("Check negative control (a) error = %v, want naming ans-1", errCheckNoHits)
	}

	// Negative control (b): Stub returning hits fails off-topic refusal bar
	allHitsRetriever := &stubRetriever{hits: []retrieval.Hit{{Chunk: corpus.Chunk{Path: "docs/other.md"}, Score: 0.9}}}
	repAllHits, err := Run(context.Background(), set, allHitsRetriever, retrieval.DefaultOptions())
	if err != nil {
		t.Fatalf("Run allHits err = %v", err)
	}
	errCheckAllHits := Check(repAllHits, bars)
	if errCheckAllHits == nil || !strings.Contains(errCheckAllHits.Error(), "off-1") {
		t.Errorf("Check negative control (b) error = %v, want naming off-1", errCheckAllHits)
	}
}
