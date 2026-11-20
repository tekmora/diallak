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
var sponsorshipLink = regexp.MustCompile(`(?i)https?://(?:www\.)?(?:github\.com/sponsors/|ko-fi\.com/|opencollective\.com/|liberapay\.com/|patreon\.com/|buymeacoffee\.com/)`)
var listItem = regexp.MustCompile(`^(?:[-+*] |[0-9]+[.)] )`)

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

func (t Trigger) matchesQuery(q Query, tokens []string) bool {
	return t.entryPoints(q, tokens) && t.matches(tokens)
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
		answer, foundRepo := resolveAnswer(trigger, chunks)
		if !foundRepo {
			issues = append(issues, Issue{trigger.ID, "repository_absent"})
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

func sponsorshipLines(lines []string) []bool {
	removed := make([]bool, len(lines))
	start := 0
	flush := func(end int) {
		if sponsorshipLink.MatchString(strings.Join(lines[start:end], "\n")) {
			for i := start; i < end; i++ {
				removed[i] = true
			}
		}
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" || listItem.MatchString(line) {
			flush(i)
			start = i
			if strings.TrimSpace(line) == "" {
				start = i + 1
			}
		}
	}
	flush(len(lines))
	return removed
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

func resolveAnswer(trigger Trigger, chunks []corpus.Chunk) (ResolvedAnswer, bool) {
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
		texts = append(texts, chunk.Text)
		if len(answer.Sources) == 0 {
			answer.Sources = append(answer.Sources, chunk)
		}
	}
	answer.Text = strings.Join(texts, "\n\n")
	return answer, foundRepo
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
