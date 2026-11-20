package ruleanswer

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/retrieval"
)

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
