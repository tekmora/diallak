// Package eval loads evaluation question sets and measures retrieval against quality bars.
package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/retrieval"
)

// Kind classifies the expected retrieval behavior for an evaluation case.
type Kind string

const (
	// KindAnswerable indicates the question is answered by the documentation corpus.
	KindAnswerable Kind = "answerable"
	// KindOffTopic indicates the question is unrelated to the project and must be refused.
	KindOffTopic Kind = "off_topic"
	// KindUndocumented indicates the question is about the project but not in the docs.
	KindUndocumented Kind = "undocumented"
	// KindParaphrase indicates the documentation answers it, but the question uses other words than the files that answer it; measured against a threshold, not a pass bar.
	KindParaphrase Kind = "paraphrase"
)

const (
	// MaxParaphraseOverlap is the maximum allowed share of distinct query tokens that may occur in source chunks.
	// A paraphrase case shares at most half of its distinct terms with its sources.
	// Source: chosen, not measured.
	MaxParaphraseOverlap = 0.5

	// MaxParaphraseMisses is the maximum number of missed paraphrase cases tolerated before a decision record on embeddings is due.
	// Source: chosen, not measured.
	MaxParaphraseMisses = 3
)

// Case represents a single evaluation query with expected behavior.
type Case struct {
	ID       string   `json:"id"`
	Kind     Kind     `json:"kind"`
	Question string   `json:"question"`
	Sources  []string `json:"sources,omitempty"`
}

// Set groups evaluation cases with their target corpus specification.
type Set struct {
	Version int           `json:"version"`
	Corpus  corpus.Source `json:"corpus"`
	Cases   []Case        `json:"cases"`
}

// LoadSet reads and validates an evaluation set JSON file within bounded memory (1 MiB).
func LoadSet(fsys fs.FS, name string) (_ Set, err error) {
	f, err := fsys.Open(name)
	if err != nil {
		return Set{}, fmt.Errorf("open %q: %w", name, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close %q: %w", name, closeErr))
		}
	}()

	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return Set{}, fmt.Errorf("read %q: %w", name, err)
	}
	if len(data) > 1<<20 {
		return Set{}, fmt.Errorf("file %q exceeds 1 MiB limit", name)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var set Set
	if err := dec.Decode(&set); err != nil {
		return Set{}, fmt.Errorf("decode %q: %w", name, err)
	}

	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return Set{}, fmt.Errorf("file %q contains trailing data", name)
	}

	if err := validateSet(set); err != nil {
		return Set{}, err
	}
	return set, nil
}

func validateSet(set Set) error {
	if set.Version != 1 {
		return fmt.Errorf("unsupported version %d, want 1", set.Version)
	}
	if len(set.Cases) == 0 {
		return errors.New("zero cases in set")
	}

	seenIDs := make(map[string]struct{}, len(set.Cases))
	for _, c := range set.Cases {
		if err := validateCase(c, seenIDs); err != nil {
			return err
		}
	}
	return nil
}

func isSupportedKind(k Kind) bool {
	switch k {
	case KindAnswerable, KindParaphrase, KindOffTopic, KindUndocumented:
		return true
	default:
		return false
	}
}

func validateCase(c Case, seenIDs map[string]struct{}) error {
	if c.ID == "" {
		return errors.New("empty case ID in set")
	}
	if _, ok := seenIDs[c.ID]; ok {
		return fmt.Errorf("duplicate case ID %q", c.ID)
	}
	seenIDs[c.ID] = struct{}{}

	if !isSupportedKind(c.Kind) {
		return fmt.Errorf("case %q has unknown kind %q", c.ID, c.Kind)
	}
	if strings.TrimSpace(c.Question) == "" {
		return fmt.Errorf("case %q has empty question", c.ID)
	}
	needsSources := c.Kind == KindAnswerable || c.Kind == KindParaphrase
	if needsSources && len(c.Sources) == 0 {
		return fmt.Errorf("case %q: %s without sources", c.ID, c.Kind)
	}
	if !needsSources && len(c.Sources) > 0 {
		return fmt.Errorf("case %q: non-answerable with sources", c.ID)
	}
	return nil
}

func addChunkTokens(set map[string]struct{}, texts ...string) {
	for _, text := range texts {
		for _, tok := range retrieval.Tokenize(text) {
			set[tok] = struct{}{}
		}
	}
}

func collectSourceTokens(chunks []corpus.Chunk, srcSet map[string]struct{}) map[string]struct{} {
	sourceTokens := make(map[string]struct{})
	for _, c := range chunks {
		if _, ok := srcSet[c.Path]; !ok {
			continue
		}
		addChunkTokens(sourceTokens, c.Text, c.Heading, c.Title)
	}
	return sourceTokens
}

// Overlap returns the share of distinct retrieval.Tokenize tokens of question
// that occur in the tokens of any chunk whose Path is in sources:
//
//	overlap = |tokens(question) ∩ tokens(sources)| / |tokens(question)|
//
// Tokens in sources are drawn from Text, Heading, and Title of matching chunks.
// If question produces zero tokens, Overlap returns 0.
func Overlap(question string, chunks []corpus.Chunk, sources []string) float64 {
	qTokens := retrieval.Tokenize(question)
	if len(qTokens) == 0 {
		return 0
	}
	qDistinct := make(map[string]struct{}, len(qTokens))
	for _, tok := range qTokens {
		qDistinct[tok] = struct{}{}
	}

	srcSet := make(map[string]struct{}, len(sources))
	for _, s := range sources {
		srcSet[s] = struct{}{}
	}

	sourceTokens := collectSourceTokens(chunks, srcSet)
	shared := 0
	for tok := range qDistinct {
		if _, ok := sourceTokens[tok]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(qDistinct))
}

// Validate verifies that every source path listed in set exists in chunks,
// and that no paraphrase case exceeds MaxParaphraseOverlap.
func Validate(set Set, chunks []corpus.Chunk) error {
	paths := make(map[string]struct{}, len(chunks))
	for _, c := range chunks {
		paths[c.Path] = struct{}{}
	}
	for _, c := range set.Cases {
		for _, s := range c.Sources {
			if _, ok := paths[s]; !ok {
				return fmt.Errorf("case %q references source %q not in chunks", c.ID, s)
			}
		}
		if c.Kind == KindParaphrase {
			overlap := Overlap(c.Question, chunks, c.Sources)
			if overlap > MaxParaphraseOverlap {
				return fmt.Errorf("case %q: overlap %.4f exceeds maximum %.4f", c.ID, overlap, MaxParaphraseOverlap)
			}
		}
	}
	return nil
}

// Retriever executes search queries against an index.
type Retriever interface {
	Search(query string, opt retrieval.Options) ([]retrieval.Hit, error)
}

// Outcome records retrieval results and pass/fail evaluation for one case.
type Outcome struct {
	Case Case
	Hits []retrieval.Hit
	Pass bool
}

// Report aggregates evaluation outcomes across an entire test set.
type Report struct {
	Outcomes            []Outcome
	Answerable          int
	AnswerableHit       int
	OffTopic            int
	OffTopicRefused     int
	Undocumented        int
	UndocumentedRefused int
	Paraphrase          int
	ParaphraseHit       int
}

// EmbeddingsRecordDue reports whether the number of missed paraphrase cases exceeds MaxParaphraseMisses.
func EmbeddingsRecordDue(rep Report) bool {
	return rep.Paraphrase-rep.ParaphraseHit > MaxParaphraseMisses
}

// Run executes all cases in set against retriever r using opt.
func Run(set Set, r Retriever, opt retrieval.Options) (Report, error) {
	var rep Report
	for _, c := range set.Cases {
		hits, err := r.Search(c.Question, opt)
		if err != nil {
			return Report{}, fmt.Errorf("search for case %q: %w", c.ID, err)
		}
		pass := evaluateCase(c, hits, &rep)
		rep.Outcomes = append(rep.Outcomes, Outcome{
			Case: c,
			Hits: hits,
			Pass: pass,
		})
	}
	return rep, nil
}

func evaluateCase(c Case, hits []retrieval.Hit, rep *Report) bool {
	switch c.Kind {
	case KindAnswerable:
		rep.Answerable++
		for _, h := range hits {
			if slices.Contains(c.Sources, h.Chunk.Path) {
				rep.AnswerableHit++
				return true
			}
		}
		return false
	case KindParaphrase:
		rep.Paraphrase++
		for _, h := range hits {
			if slices.Contains(c.Sources, h.Chunk.Path) {
				rep.ParaphraseHit++
				return true
			}
		}
		return false
	case KindOffTopic:
		rep.OffTopic++
		if len(hits) == 0 {
			rep.OffTopicRefused++
			return true
		}
		return false
	case KindUndocumented:
		rep.Undocumented++
		if len(hits) == 0 {
			rep.UndocumentedRefused++
			return true
		}
		return false
	default:
		return false
	}
}

// Bars specifies acceptance thresholds for evaluation runs.
type Bars struct {
	MinAnswerableHit   int
	MinOffTopicRefused int
}

// Check evaluates report against bars, returning an error detailing every failing case if unmet.
func Check(rep Report, bars Bars) error {
	failed := rep.AnswerableHit < bars.MinAnswerableHit || rep.OffTopicRefused < bars.MinOffTopicRefused
	if !failed {
		return nil
	}

	var msgs []string
	if rep.AnswerableHit < bars.MinAnswerableHit {
		msgs = append(msgs, fmt.Sprintf("answerable hit %d < bar %d", rep.AnswerableHit, bars.MinAnswerableHit))
	}
	if rep.OffTopicRefused < bars.MinOffTopicRefused {
		msgs = append(msgs, fmt.Sprintf("off_topic refused %d < bar %d", rep.OffTopicRefused, bars.MinOffTopicRefused))
	}

	for _, out := range rep.Outcomes {
		if out.Pass || (out.Case.Kind != KindAnswerable && out.Case.Kind != KindOffTopic) {
			continue
		}
		if len(out.Hits) > 0 {
			top := out.Hits[0]
			msgs = append(msgs, fmt.Sprintf("case %s (%s): top hit %s (score %.4f)", out.Case.ID, out.Case.Kind, top.Chunk.Path, top.Score))
		} else {
			msgs = append(msgs, fmt.Sprintf("case %s (%s): no hits (score 0.0000)", out.Case.ID, out.Case.Kind))
		}
	}
	return errors.New(strings.Join(msgs, "; "))
}
