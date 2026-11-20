// Package ruleanswer loads, validates, matches, and resolves fixed documentation answers.
package ruleanswer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/retrieval"
)

// MaxTriggers limits the number of triggers in a table.
const MaxTriggers = 128

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

// Validate checks shape, references, and replay examples. Findings are capped at MaxFindings.
func Validate(t *Table, known Known) []Finding {
	if t == nil {
		return []Finding{{Code: "table", Path: "$", Message: "table is nil"}}
	}
	v := validator{table: t, known: known, seen: make(map[string]bool)}
	v.checkVersion()
	if len(t.Triggers) <= MaxTriggers {
		for i := range t.Triggers {
			v.checkTrigger(i, &t.Triggers[i])
		}
	}
	return v.findings
}

type validator struct {
	table    *Table
	known    Known
	seen     map[string]bool
	findings []Finding
}

func (v *validator) add(code, p, message string) {
	if len(v.findings) < MaxFindings {
		v.findings = append(v.findings, Finding{Code: code, Path: p, Message: message})
	}
}

func (v *validator) checkVersion() {
	if v.table.Version != 1 {
		v.add("version", "version", "must be 1")
	}
	if len(v.table.Triggers) > MaxTriggers {
		v.add("trigger_count", "triggers", fmt.Sprintf("exceeds %d", MaxTriggers))
	}
}

func (v *validator) checkTrigger(i int, t *Trigger) {
	p := fmt.Sprintf("triggers[%d]", i)
	v.checkIdentity(p, t)
	v.checkKeywords(p, t.Keywords)
	v.checkBareSupportKeywords(p, t)
	v.checkEntryPoints(p, t)
	v.checkLocations(p, t)
	v.checkExamples(p, t)
}

func (v *validator) checkIdentity(p string, t *Trigger) {
	if !idPattern.MatchString(t.ID) || utf8.RuneCountInString(t.ID) > 64 {
		v.add("id", p+".id", "must be a lowercase slug of at most 64 runes")
	}
	if v.seen[t.ID] {
		v.add("duplicate_id", p+".id", "must be unique")
	}
	v.seen[t.ID] = true
	if !corpus.ValidRepository(t.Repository) {
		v.add("repository", p+".repository", "does not use corpus repository syntax")
	}
}

func (v *validator) checkKeywords(p string, k Keywords) {
	if len(k.All)+len(k.Any) == 0 {
		v.add("keywords_all", p+".keywords", "all or any must not be empty")
	}
	for _, group := range []struct {
		name    string
		entries []string
	}{{"all", k.All}, {"any", k.Any}, {"none", k.None}} {
		for i, entry := range group.entries {
			if utf8.RuneCountInString(entry) > 64 || len(retrieval.Tokenize(entry)) == 0 {
				v.add("keyword", fmt.Sprintf("%s.keywords.%s[%d]", p, group.name, i), "must be at most 64 runes and contain a searchable token")
			}
		}
	}
}

func (v *validator) checkBareSupportKeywords(p string, t *Trigger) {
	if !strings.HasSuffix(t.ID, "-support") {
		return
	}
	for _, group := range []struct {
		name    string
		entries []string
	}{{"all", t.Keywords.All}, {"any", t.Keywords.Any}} {
		for i, entry := range group.entries {
			if bareSupportKeyword(retrieval.Tokenize(entry), t.Aliases) {
				v.add("bare_support_keyword", fmt.Sprintf("%s.keywords.%s[%d]", p, group.name, i), "must include more than an alias and support")
			}
		}
	}
}

func bareSupportKeyword(tokens, aliases []string) bool {
	if len(tokens) == 1 && tokens[0] == "support" {
		return true
	}
	for _, alias := range aliases {
		aliasTokens := retrieval.Tokenize(alias)
		if len(aliasTokens) == 0 || len(tokens) != len(aliasTokens)+1 {
			continue
		}
		if tokens[0] == "support" && slices.Equal(tokens[1:], aliasTokens) ||
			tokens[len(tokens)-1] == "support" && slices.Equal(tokens[:len(tokens)-1], aliasTokens) {
			return true
		}
	}
	return false
}

func (v *validator) checkEntryPoints(p string, t *Trigger) {
	if len(t.Commands) > 0 && len(t.Aliases) == 0 {
		v.add("aliases", p+".aliases", "required for commands")
	}
	for _, alias := range t.Aliases {
		if utf8.RuneCountInString(alias) > 64 || len(retrieval.Tokenize(alias)) == 0 {
			v.add("alias", p+".aliases", "must be at most 64 runes and contain a searchable token")
		}
	}
	if len(t.Commands)+len(t.ForumTags) == 0 {
		v.add("entry_point", p, "needs a command or forum tag")
	}
	for _, name := range t.Commands {
		if !slices.Contains(v.known.Commands, name) {
			v.add("command", p+".commands", "unknown command "+name)
		}
	}
	for _, name := range t.ForumTags {
		if !slices.Contains(v.known.ForumTags, name) {
			v.add("forum_tag", p+".forum_tags", "unknown forum tag "+name)
		}
	}
}

func cleanMarkdownPath(p string) bool {
	return fs.ValidPath(p) && path.Ext(p) == ".md" && path.Clean(p) == p
}

func cleanRelativePath(p string) bool { return fs.ValidPath(p) && path.Clean(p) == p }

func (v *validator) checkLocations(p string, t *Trigger) {
	if !cleanMarkdownPath(t.Answer.Path) || strings.TrimSpace(t.Answer.Heading) == "" {
		v.add("answer", p+".answer", "needs a clean relative Markdown path and non-empty heading")
	}
	for i, link := range t.Links {
		if !cleanRelativePath(link) {
			v.add("link", fmt.Sprintf("%s.links[%d]", p, i), "must be a clean relative path")
		}
	}
}

func (v *validator) checkExamples(p string, t *Trigger) {
	if len(t.MustMatch) == 0 || len(t.MustNotMatch) == 0 {
		v.add("examples", p, "must_match and must_not_match each need an example")
	}
	if !hasNearMiss(t) {
		v.add("near_miss", p+".must_not_match", "needs a negative example containing an alias and a keyword phrase")
	}
	for i, text := range t.MustMatch {
		v.replayMatch(p, t, i, text)
	}
	for i, text := range t.MustNotMatch {
		for _, q := range triggerQueries(*t, text) {
			if t.matchesQuery(q, retrieval.Tokenize(q.Text)) {
				v.add("must_not_match", fmt.Sprintf("%s.must_not_match[%d]", p, i), "example matches this trigger")
				break
			}
		}
	}
}

func hasNearMiss(t *Trigger) bool {
	for _, example := range t.MustNotMatch {
		tokens := retrieval.Tokenize(example)
		if len(t.Commands) > 0 && !matchesAny(tokens, t.Aliases) {
			continue
		}
		if matchesAny(tokens, t.Keywords.All) || matchesAny(tokens, t.Keywords.Any) {
			return true
		}
	}
	return false
}

func triggerQueries(t Trigger, text string) []Query {
	qs := make([]Query, 0, len(t.Commands)+len(t.ForumTags))
	for _, command := range t.Commands {
		qs = append(qs, Query{Text: text, Command: command})
	}
	for _, tag := range t.ForumTags {
		qs = append(qs, Query{Text: text, Forum: true, Tags: []string{tag}})
	}
	return qs
}

func (v *validator) replayMatch(p string, target *Trigger, i int, text string) {
	for _, q := range triggerQueries(*target, text) {
		if q.Forum {
			v.replayMultiTag(p, target, i, q)
		}
		matches := 0
		tokens := retrieval.Tokenize(q.Text)
		for j := range v.table.Triggers {
			candidate := &v.table.Triggers[j]
			if candidate.matchesQuery(q, tokens) {
				matches++
				if candidate.ID != target.ID {
					v.add("ambiguous_match", fmt.Sprintf("%s.must_match[%d]", p, i), "example also matches trigger "+candidate.ID)
				}
			}
		}
		if matches != 1 {
			v.add("must_match", fmt.Sprintf("%s.must_match[%d]", p, i), "example must match exactly this trigger through each entry point")
		}
	}
}

// replayMultiTag checks the runtime decision for each other product tag and their union.
func (v *validator) replayMultiTag(p string, target *Trigger, i int, q Query) {
	tags := append([]string(nil), q.Tags...)
	for _, other := range v.table.Triggers {
		if other.Repository == target.Repository {
			continue
		}
		for _, tag := range other.ForumTags {
			if !slices.Contains(tags, tag) {
				tags = append(tags, tag)
			}
			v.checkMultiTag(p, target, i, q, []string{q.Tags[0], tag})
		}
	}
	v.checkMultiTag(p, target, i, q, tags)
}

func (v *validator) checkMultiTag(p string, target *Trigger, i int, q Query, tags []string) {
	q.Tags = tags
	if got, ok := v.table.match(q, retrieval.Tokenize(q.Text), nil); ok && got.ID != target.ID {
		v.add("ambiguous_match", fmt.Sprintf("%s.must_match[%d]", p, i), "multi-tag example resolves to trigger "+got.ID)
	}
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
	var blocks, lines []string
	flush := func() {
		block := strings.Join(lines, "\n")
		trimmed := strings.TrimSpace(block)
		if trimmed != "" && !sponsorshipLink.MatchString(trimmed) {
			blocks = append(blocks, block)
		}
		lines = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if listItem.MatchString(line) {
			flush()
		}
		lines = append(lines, line)
	}
	flush()
	return strings.Join(blocks, "\n\n")
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
