package eval

import (
	"bytes"
	"testing"
	"testing/fstest"

	"github.com/tekmora/diallak/internal/corpus"
)

func TestLoadSet_Boundary_ExactOneMebibyteAllowed(t *testing.T) {
	base := sampleSetJSON()
	total := 1 << 20
	if len(base) > total {
		t.Fatalf("sampleSetJSON exceeds %d bytes", total)
	}
	padding := bytes.Repeat([]byte(" "), total-len(base))
	data := append([]byte(base), padding...)
	if len(data) != total {
		t.Fatalf("data length = %d, want %d", len(data), total)
	}

	fsys := fstest.MapFS{
		"cases.json": &fstest.MapFile{Data: data},
	}
	set, err := LoadSet(fsys, "cases.json")
	if err != nil {
		t.Fatalf("LoadSet with exactly 1 MiB failed: %v", err)
	}
	if set.Version != 1 {
		t.Fatalf("set.Version = %d, want 1", set.Version)
	}
}

func TestOverlap_Boundary(t *testing.T) {
	chunks := []corpus.Chunk{
		{
			Path:    "docs/sample.md",
			Title:   "Document Title",
			Heading: "Section Header",
			Text:    "Alpha beta gamma delta",
		},
	}
	sources := []string{"docs/sample.md"}

	// Boundary 1: Empty question -> 0
	if got := Overlap("", chunks, sources); got != 0.0 {
		t.Errorf("Overlap empty question = %f, want 0.0", got)
	}
	// Also stopword-only question -> 0
	if got := Overlap("what is it", chunks, sources); got != 0.0 {
		t.Errorf("Overlap stopword question = %f, want 0.0", got)
	}

	// Boundary 2: All tokens shared -> 1
	if got := Overlap("alpha beta", chunks, sources); got != 1.0 {
		t.Errorf("Overlap all shared = %f, want 1.0", got)
	}

	// Boundary 3: Exactly half -> 0.5 passes Validate
	// "alpha" is in chunk; "omega" is not. Question has 2 distinct tokens, 1 matches.
	if got := Overlap("alpha omega", chunks, sources); got != 0.5 {
		t.Errorf("Overlap exactly half = %f, want 0.5", got)
	}

	set := Set{
		Cases: []Case{
			{ID: "para-half", Kind: KindParaphrase, Question: "alpha omega", Sources: sources},
		},
	}
	if err := Validate(set, chunks); err != nil {
		t.Errorf("Validate at 0.5 overlap failed: %v, want nil", err)
	}
}

func TestEmbeddingsRecordDue_Boundary(t *testing.T) {
	// Boundary 1: Exactly MaxParaphraseMisses misses -> false
	repExact := Report{
		Paraphrase:    10,
		ParaphraseHit: 10 - MaxParaphraseMisses,
	}
	if EmbeddingsRecordDue(repExact) {
		t.Errorf("EmbeddingsRecordDue at %d misses = true, want false", MaxParaphraseMisses)
	}

	// Boundary 2: One more than MaxParaphraseMisses -> true
	repOneMore := Report{
		Paraphrase:    10,
		ParaphraseHit: 10 - (MaxParaphraseMisses + 1),
	}
	if !EmbeddingsRecordDue(repOneMore) {
		t.Errorf("EmbeddingsRecordDue at %d misses = false, want true", MaxParaphraseMisses+1)
	}

	// Boundary 3: Zero misses -> false
	repZero := Report{
		Paraphrase:    10,
		ParaphraseHit: 10,
	}
	if EmbeddingsRecordDue(repZero) {
		t.Error("EmbeddingsRecordDue at 0 misses = true, want false")
	}
}
