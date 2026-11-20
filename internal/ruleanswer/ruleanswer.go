// Package ruleanswer loads, validates, matches, and resolves fixed documentation answers.
package ruleanswer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/retrieval"
)

// MaxTriggers limits the number of triggers in a table.
const MaxTriggers = 128

// MaxListItems limits each list in a trigger.
const MaxListItems = 32

// maxJSONDepth limits nested objects and arrays in a trigger table.
const maxJSONDepth = 16

// MaxFindings limits the findings returned by Validate.
const MaxFindings = MaxTriggers * 8

var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var jsonKeyPattern = regexp.MustCompile(`^[a-z][a-z_]*$`)
var sponsorshipLink = regexp.MustCompile(`(?i)https?://(?:www\.)?(?:github\.com/sponsors/|ko-fi\.com/|opencollective\.com/|liberapay\.com/|patreon\.com/|buymeacoffee\.com/)`)
var listItem = regexp.MustCompile(`^[ \t]*(?:[-+*] |[0-9]+[.)] )`)

// Table contains the versioned trigger collection.
type Table struct {
	Version  int       `json:"version"`
	Triggers []Trigger `json:"triggers"`
}

// Trigger describes one fixed answer and its matching conditions.
type Trigger struct {
	ID           string   `json:"id"`
	Repository   string   `json:"repository"`
	Keywords     Keywords `json:"keywords"`
	Commands     []string `json:"commands"`
	Aliases      []string `json:"aliases"`
	ForumTags    []string `json:"forum_tags"`
	Answer       Location `json:"answer"`
	Links        []string `json:"links"`
	MustMatch    []string `json:"must_match"`
	MustNotMatch []string `json:"must_not_match"`
}

// Keywords holds required, alternative, and excluded phrases.
type Keywords struct {
	All  []string `json:"all"`
	Any  []string `json:"any"`
	None []string `json:"none"`
}

// Location identifies a documentation section.
type Location struct {
	Path    string `json:"path"`
	Heading string `json:"heading"`
}

// Query identifies a question and its entry point.
type Query struct {
	Text    string
	Command string
	Forum   bool
	Tags    []string
}

// Finding describes a validation failure.
type Finding struct {
	Code    string
	Path    string
	Message string
}

// Known lists allowed command and forum tag names.
type Known struct {
	Commands  []string
	ForumTags []string
}

// Load reads a strict JSON table and rejects unknown fields and trailing data.
func Load(data []byte) (*Table, error) {
	if err := checkJSONKeys(data); err != nil {
		return nil, fmt.Errorf("decode trigger table: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var table Table
	if err := dec.Decode(&table); err != nil {
		return nil, fmt.Errorf("decode trigger table: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("decode trigger table: trailing JSON value")
		}
		return nil, fmt.Errorf("decode trigger table trailing data: %w", err)
	}
	return &table, nil
}

// jsonFrame tracks keys only for the current object; arrays have a nil set.
type jsonFrame struct {
	keys    map[string]bool
	keyNext bool
}

func checkJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	stack := make([]jsonFrame, 0, maxJSONDepth)
	for i := 0; i <= len(data); i++ {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			stack = stack[:len(stack)-1]
			continue
		}
		isKey, err := checkJSONTokenKey(stack, tok)
		if err != nil {
			return err
		}
		if isKey {
			continue
		}
		stack, err = pushJSONContainer(stack, tok)
		if err != nil {
			return err
		}
	}
	return errors.New("JSON token limit exceeded")
}

func pushJSONContainer(stack []jsonFrame, tok json.Token) ([]jsonFrame, error) {
	d, ok := tok.(json.Delim)
	if !ok || (d != '{' && d != '[') {
		return stack, nil
	}
	if len(stack) == maxJSONDepth {
		return nil, fmt.Errorf("JSON nesting exceeds %d", maxJSONDepth)
	}
	frame := jsonFrame{}
	if d == '{' {
		frame.keys = make(map[string]bool)
		frame.keyNext = true
	}
	return append(stack, frame), nil
}

func checkJSONTokenKey(stack []jsonFrame, tok json.Token) (bool, error) {
	if len(stack) == 0 || stack[len(stack)-1].keys == nil {
		return false, nil
	}
	frame := &stack[len(stack)-1]
	if !frame.keyNext {
		frame.keyNext = true
		return false, nil
	}
	key, ok := tok.(string)
	if !ok {
		return false, errors.New("expected object key")
	}
	if !jsonKeyPattern.MatchString(key) {
		return false, fmt.Errorf("invalid object key %q", key)
	}
	if frame.keys[key] {
		return false, fmt.Errorf("duplicate key %q", key)
	}
	frame.keys[key] = true
	frame.keyNext = false
	return true, nil
}

func (t Trigger) entryPoints(q Query, tokens []string) bool {
	if q.Forum {
		for _, tag := range t.ForumTags {
			if slices.Contains(q.Tags, tag) {
				return true
			}
		}
		return false
	}
	return slices.Contains(t.Commands, q.Command) && matchesAny(tokens, t.Aliases)
}

func matchesAny(question, phrases []string) bool {
	for _, phrase := range phrases {
		if containsPhrase(question, retrieval.Tokenize(phrase)) {
			return true
		}
	}
	return false
}

// Match returns the first table-order trigger whose entry point and keywords match.
func (t *Table) Match(q Query) (Trigger, bool) {
	if t == nil {
		return Trigger{}, false
	}
	return t.match(q, retrieval.Tokenize(q.Text), nil)
}

func (t *Table) match(q Query, tokens []string, available map[string]ResolvedAnswer) (Trigger, bool) {
	var first Trigger
	var repository string
	found := false
	for _, trigger := range t.Triggers {
		if !trigger.matchesQuery(q, tokens) {
			continue
		}
		if repository != "" && q.Forum && repository != trigger.Repository {
			return Trigger{}, false
		}
		repository = trigger.Repository
		if !found {
			if available == nil {
				first, found = trigger, true
			} else if _, ok := available[trigger.ID]; ok {
				first, found = trigger, true
			}
		}
	}
	return first, found
}

func hasNegation(text string) bool {
	text = strings.ReplaceAll(strings.ToLower(text), "’", "'")
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' })
	negations := []string{"not", "no", "never", "nothing", "none", "neither", "nor", "cannot", "can't", "cant", "don't", "dont", "doesn't", "doesnt", "didn't", "didnt", "won't", "wont", "isn't", "isnt", "aren't", "arent", "shouldn't", "shouldnt", "couldnt", "wouldnt", "wasnt", "werent", "havent", "hasnt", "hadnt", "mustnt", "neednt"}
	for _, word := range words {
		if slices.Contains(negations, word) || strings.HasSuffix(word, "n't") {
			return true
		}
	}
	return false
}

func (t Trigger) matchesQuery(q Query, tokens []string) bool {
	return !hasNegation(q.Text) && t.entryPoints(q, tokens) && t.matches(tokens)
}

func (t Trigger) matches(question []string) bool {
	for _, entry := range t.Keywords.All {
		if !containsPhrase(question, retrieval.Tokenize(entry)) {
			return false
		}
	}
	if len(t.Keywords.Any) > 0 {
		found := false
		for _, entry := range t.Keywords.Any {
			found = found || containsPhrase(question, retrieval.Tokenize(entry))
		}
		if !found {
			return false
		}
	}
	for _, entry := range t.Keywords.None {
		if containsPhrase(question, retrieval.Tokenize(entry)) {
			return false
		}
	}
	return true
}

func containsPhrase(haystack, needle []string) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	for start := 0; start <= len(haystack)-len(needle); start++ {
		if slices.Equal(haystack[start:start+len(needle)], needle) {
			return true
		}
	}
	return false
}

// ResolvedAnswer contains a trigger and its resolved documentation.
type ResolvedAnswer struct {
	Trigger Trigger
	Text    string
	Sources []corpus.Chunk
	Links   []string
}

// Resolved holds answers resolved against the corpus.
type Resolved struct {
	Table         *Table
	Answers       map[string]ResolvedAnswer
	DisabledCount int
}

// Match returns the first matching trigger that resolved against the loaded corpus.
func (r *Resolved) Match(q Query) (ResolvedAnswer, bool) {
	if r == nil || r.Table == nil {
		return ResolvedAnswer{}, false
	}
	trigger, ok := r.Table.match(q, retrieval.Tokenize(q.Text), r.Answers)
	if !ok {
		return ResolvedAnswer{}, false
	}
	answer, ok := r.Answers[trigger.ID]
	return answer, ok
}

// Issue records a trigger or link unavailable in the loaded corpus.
type Issue struct {
	TriggerID string
	Reason    string
}

// Resolve binds triggers to corpus chunks and reports missing documentation.
func Resolve(t *Table, chunks []corpus.Chunk, linkTemplate string) (*Resolved, []Issue, error) {
	if t == nil {
		return nil, nil, errors.New("resolve trigger table: table is nil")
	}
	r := &Resolved{Table: t, Answers: make(map[string]ResolvedAnswer)}
	var issues []Issue
	for _, trigger := range t.Triggers {
		answer, foundRepo, ambiguous := resolveAnswer(trigger, chunks)
		if !foundRepo {
			issues = append(issues, Issue{trigger.ID, "repository_absent"})
			r.DisabledCount++
			continue
		}
		if ambiguous {
			issues = append(issues, Issue{trigger.ID, "answer_ambiguous"})
			r.DisabledCount++
			continue
		}
		if len(answer.Sources) == 0 {
			issues = append(issues, Issue{trigger.ID, "answer_missing"})
			r.DisabledCount++
			continue
		}
		if strings.TrimSpace(answer.Text) != "" {
			answer.Text = filterSponsorship(answer.Text)
			if answer.Text == "" {
				issues = append(issues, Issue{trigger.ID, "sponsorship_only"})
				r.DisabledCount++
				continue
			}
		}
		var missing []Issue
		var linkSources []corpus.Chunk
		answer.Links, linkSources, missing = resolveLinks(trigger, chunks, linkTemplate)
		answer.Sources = append(answer.Sources, linkSources...)
		issues = append(issues, missing...)
		r.Answers[trigger.ID] = answer
	}
	return r, issues, nil
}

func filterSponsorship(text string) string {
	if !sponsorshipLink.MatchString(text) {
		return text
	}
	lines := strings.Split(text, "\n")
	return joinWithoutSponsorship(lines, sponsorshipLines(lines))
}

// sponsorshipLines modifies lines owned by filterSponsorship. For a multi-line
// non-list paragraph, it keeps filtered text in the first line and marks the
// other lines as removed. Indented code retains indented list-like lines;
// paragraphs starting with a list item still split at list items.
func sponsorshipLines(lines []string) []bool {
	removed := make([]bool, len(lines))
	start := 0

	for i, line := range lines {
		if strings.TrimSpace(line) == "" || (listItem.MatchString(line) && !codeParagraphContinues(lines, start, i)) {
			filterSponsorshipParagraph(lines, removed, start, i)
			start = i
			if strings.TrimSpace(line) == "" {
				start = i + 1
			}
		}
	}
	filterSponsorshipParagraph(lines, removed, start, len(lines))
	removeSponsorshipSubtrees(lines, removed)
	return removed
}

func codeParagraphContinues(lines []string, start, next int) bool {
	return start < next && corpus.IndentColumn(lines[start]) >= 4 &&
		!listItem.MatchString(lines[start]) && corpus.IndentColumn(lines[next]) >= 4
}

// filterSponsorshipParagraph modifies lines owned by filterSponsorship. For a
// multi-line non-list paragraph, it keeps filtered text in the first line and
// marks the other lines as removed. Indented code with a sponsorship link is
// removed whole without changing its text.
func filterSponsorshipParagraph(lines []string, removed []bool, start, end int) {
	if start == end {
		return
	}
	paragraph := strings.Join(lines[start:end], "\n")
	if !sponsorshipLink.MatchString(paragraph) {
		return
	}
	if corpus.IndentColumn(lines[start]) >= 4 {
		for i := start; i < end; i++ {
			removed[i] = true
		}
		return
	}
	if !listItem.MatchString(lines[start]) {
		lines[start] = filterSponsorshipSentences(paragraph)
	}
	removed[start] = listItem.MatchString(lines[start]) || strings.TrimSpace(lines[start]) == ""
	for i := start + 1; i < end; i++ {
		removed[i] = true
	}
}

// filterSponsorshipSentences treats a line without end punctuation as part of
// the sentence continuing on the next line, removing it with a sponsorship sentence.
// Inside a paragraph, letter-dot abbreviations with at least two pairs do not
// end a sentence.
func filterSponsorshipSentences(paragraph string) string {
	var kept strings.Builder
	start := 0
	for i, r := range paragraph {
		if !sponsorshipSentenceBoundary(paragraph, start, i, r) {
			continue
		}
		next := i + 1
		sentence := paragraph[start:next]
		if !sponsorshipLink.MatchString(sentence) {
			kept.WriteString(sentence)
		}
		start = next
	}
	if tail := paragraph[start:]; !sponsorshipLink.MatchString(tail) {
		kept.WriteString(tail)
	}
	return strings.TrimSpace(kept.String())
}

func sponsorshipSentenceBoundary(paragraph string, start, i int, r rune) bool {
	if r != '.' && r != '!' && r != '?' {
		return false
	}
	next := i + 1
	if next == len(paragraph) {
		return true
	}
	following, _ := utf8.DecodeRuneInString(paragraph[next:])
	return unicode.IsSpace(following) && (r != '.' || !letterDotAbbreviation(paragraph[start:next]))
}

func letterDotAbbreviation(sentence string) bool {
	start := len(sentence)
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(sentence[:start])
		if unicode.IsSpace(r) {
			break
		}
		start -= size
	}
	word := strings.TrimLeftFunc(sentence[start:], func(r rune) bool {
		return r == '(' || r == '[' || unicode.Is(unicode.Quotation_Mark, r)
	})
	pairs := 0
	expectLetter := true
	for _, r := range word {
		if expectLetter {
			if !unicode.IsLetter(r) {
				return false
			}
		} else {
			if r != '.' {
				return false
			}
			pairs++
		}
		expectLetter = !expectLetter
	}
	return expectLetter && pairs >= 2
}

func removeSponsorshipSubtrees(lines []string, removed []bool) {
	active := false
	indent := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		column := corpus.IndentColumn(line)
		if active && column > indent {
			removed[i] = true
			continue
		}
		active = removed[i] && listItem.MatchString(line)
		if active {
			indent = column
		}
	}
}

func joinWithoutSponsorship(lines []string, removed []bool) string {
	var kept, blanks []string
	removedBlock := false
	for i, line := range lines {
		if removed[i] {
			removedBlock = true
			continue
		}
		if strings.TrimSpace(line) == "" {
			blanks = append(blanks, line)
			continue
		}
		if len(kept) > 0 {
			if removedBlock && len(blanks) > 1 {
				blanks = blanks[:1]
			}
			kept = append(kept, blanks...)
		}
		kept = append(kept, line)
		blanks = nil
		removedBlock = false
	}
	return strings.Join(kept, "\n")
}

func resolveAnswer(trigger Trigger, chunks []corpus.Chunk) (ResolvedAnswer, bool, bool) {
	answer := ResolvedAnswer{Trigger: trigger}
	var texts []string
	foundRepo := false
	for _, chunk := range chunks {
		if chunk.Repository != trigger.Repository {
			continue
		}
		foundRepo = true
		if chunk.Path != trigger.Answer.Path || chunk.Heading != trigger.Answer.Heading {
			continue
		}
		if len(answer.Sources) > 0 && answer.Sources[0].Section != chunk.Section {
			return ResolvedAnswer{}, true, true
		}
		texts = append(texts, chunk.Text)
		if len(answer.Sources) == 0 {
			answer.Sources = append(answer.Sources, chunk)
		}
	}
	answer.Text = strings.Join(texts, "\n\n")
	return answer, foundRepo, false
}

func resolveLinks(trigger Trigger, chunks []corpus.Chunk, template string) ([]string, []corpus.Chunk, []Issue) {
	var links []string
	var sources []corpus.Chunk
	var issues []Issue
	seen := make(map[string]bool)
	for _, link := range trigger.Links {
		if seen[link] {
			continue
		}
		seen[link] = true
		found := false
		for _, chunk := range chunks {
			if chunk.Repository == trigger.Repository && chunk.Path == link {
				links = append(links, chunk.FormatURL(template))
				if chunk.Path != trigger.Answer.Path {
					sources = append(sources, chunk)
				}
				found = true
				break
			}
		}
		if !found {
			issues = append(issues, Issue{trigger.ID, "link_missing"})
		}
	}
	return links, sources, issues
}
