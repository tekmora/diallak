package ruleanswer

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tekmora/diallak/internal/corpus"
)

func fixtureTrigger() Trigger {
	return Trigger{
		ID: "release-check", Repository: "tekmora/diallak",
		Keywords: Keywords{All: []string{"release"}, Any: []string{"verify", "image"}},
		Commands: []string{"ask"}, Aliases: []string{"Diallak"}, ForumTags: []string{"Diallak"},
		Answer: Location{Path: "README.md", Heading: "Release"}, Links: []string{},
		MustMatch:    []string{"How do I verify the Diallak release image?"},
		MustNotMatch: []string{"Why does the Diallak release fail?"},
	}
}

func fixtureTable(trigger ...Trigger) *Table { return &Table{Version: 1, Triggers: trigger} }

func knownNames() Known {
	return Known{Commands: []string{"ask"}, ForumTags: []string{"Diallak"}}
}

func hasFinding(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func TestLoad_Positive_StrictValidTable(t *testing.T) {
	raw := `{"version":1,"triggers":[]}`
	table, err := Load([]byte(raw))
	if err != nil || table.Version != 1 {
		t.Fatalf("Load = (%v, %v), want version 1", table, err)
	}
}

func TestLoad_Negative_UnknownFieldAndTrailingData(t *testing.T) {
	for _, raw := range []string{`{"version":1,"triggers":[],"extra":true}`, `{"version":1,"triggers":[]} {}`} {
		if _, err := Load([]byte(raw)); err == nil {
			t.Errorf("Load(%s) error = nil", raw)
		}
	}
}

func TestLoad_Boundary_WhitespaceAfterDocument(t *testing.T) {
	if _, err := Load([]byte("{\"version\":1,\"triggers\":[]}  \n")); err != nil {
		t.Fatalf("Load whitespace suffix: %v", err)
	}
}

func TestMatch_Positive_EntryPointAndTokenPhrase(t *testing.T) {
	table := fixtureTable(fixtureTrigger())
	for _, q := range []Query{
		{Text: "How do I verify the Diallak release image?", Command: "ask"},
		{Text: "How do I verify the Diallak release image?", Forum: true, Tags: []string{"Diallak"}},
	} {
		got, ok := table.Match(q)
		if !ok || got.ID != "release-check" {
			t.Fatalf("Match(%+v) = (%q, %t)", q, got.ID, ok)
		}
	}
}

func TestMatch_Negative_WrongEntryOrMissingKeyword(t *testing.T) {
	table := fixtureTable(fixtureTrigger())
	for _, q := range []Query{
		{Text: "How do I verify the Diallak release image?", Command: "bug"},
		{Text: "How do I verify a product?", Command: "ask"},
		{Text: "How do I verify the Diallak release image?", Forum: true, Tags: nil},
	} {
		if _, ok := table.Match(q); ok {
			t.Errorf("Match(%+v) unexpectedly matched", q)
		}
	}
}

func TestMatch_Boundary_NilTableAndEmptyAny(t *testing.T) {
	if _, ok := (*Table)(nil).Match(Query{Text: "Diallak release", Command: "ask"}); ok {
		t.Fatal("nil table matched")
	}
	trigger := fixtureTrigger()
	trigger.Keywords.Any = nil
	if _, ok := fixtureTable(trigger).Match(Query{Text: "Diallak release", Command: "ask"}); !ok {
		t.Fatal("empty any list should allow all")
	}
}

func TestValidate_Positive_ReplaysBothEntryPointKinds(t *testing.T) {
	if findings := Validate(fixtureTable(fixtureTrigger()), knownNames()); len(findings) != 0 {
		t.Fatalf("Validate findings = %+v", findings)
	}
}

func TestValidate_Negative_AmbiguityStaleAndUnknownFields(t *testing.T) {
	first := fixtureTrigger()
	second := fixtureTrigger()
	second.ID = "release-check-two"
	second.Commands = []string{"ask"}
	second.ForumTags = nil
	findings := Validate(fixtureTable(first, second), knownNames())
	if !hasFinding(findings, "ambiguous_match") {
		t.Fatal("missing ambiguity finding")
	}

	bad := fixtureTrigger()
	bad.MustMatch = []string{"How do I start the bot?"}
	bad.MustNotMatch = []string{"How do I verify the Diallak release image?"}
	bad.Commands = []string{"unknown"}
	bad.ForumTags = []string{"Unknown"}
	bad.Keywords.All = []string{"release"}
	bad.MustNotMatch = []string{"How do I verify the Diallak release image?"}
	findings = Validate(fixtureTable(bad), knownNames())
	for _, code := range []string{"must_match", "must_not_match", "command", "forum_tag"} {
		if !hasFinding(findings, code) {
			t.Errorf("missing %s finding: %+v", code, findings)
		}
	}
	stopword := fixtureTrigger()
	stopword.Keywords.All = []string{"the and of"}
	if !hasFinding(Validate(fixtureTable(stopword), knownNames()), "keyword") {
		t.Fatal("stopword-only keyword finding missing")
	}
}

func TestValidate_Boundary_DuplicateIDAndMaximumCount(t *testing.T) {
	first := fixtureTrigger()
	second := fixtureTrigger()
	if !hasFinding(Validate(fixtureTable(first, second), knownNames()), "duplicate_id") {
		t.Fatal("duplicate id finding missing")
	}
	tooMany := make([]Trigger, MaxTriggers+1)
	if !hasFinding(Validate(fixtureTable(tooMany...), knownNames()), "trigger_count") {
		t.Fatal("trigger count finding missing")
	}
}

func TestValidate_Positive_SupportHelpKeyword(t *testing.T) {
	trigger := fixtureTrigger()
	trigger.ID = "diallak-support"
	trigger.Keywords.Any = []string{"get help"}
	if hasFinding(Validate(fixtureTable(trigger), knownNames()), "bare_support_keyword") {
		t.Fatal("help-seeking keyword should be allowed")
	}
}

func TestValidate_Negative_BareSupportKeyword(t *testing.T) {
	for _, tc := range []struct {
		name, keyword, group string
	}{
		{"support alone", "support", "any"},
		{"alias then support", "Where is Diallak support", "any"},
		{"support then alias", "support for Diallak", "all"},
		{"multi-token alias", "support for Diallak Tools", "any"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trigger := fixtureBareSupportTrigger(tc.name, tc.keyword, tc.group)
			checkBareSupportFindings(t, Validate(fixtureTable(trigger), knownNames()), tc.group)
		})
	}
}

func fixtureBareSupportTrigger(name, keyword, group string) Trigger {
	trigger := fixtureTrigger()
	trigger.ID = "diallak-support"
	if name == "multi-token alias" {
		trigger.Aliases = append(trigger.Aliases, "Diallak Tools")
	}
	if group == "all" {
		trigger.Keywords.All = []string{keyword}
	} else {
		trigger.Keywords.Any = []string{keyword}
	}
	return trigger
}

func checkBareSupportFindings(t *testing.T, findings []Finding, group string) {
	t.Helper()
	count := 0
	for _, finding := range findings {
		if finding.Code == "bare_support_keyword" {
			count++
			if want := "triggers[0].keywords." + group + "[0]"; finding.Path != want {
				t.Errorf("finding path = %q, want %q", finding.Path, want)
			}
		}
	}
	if count != 1 {
		t.Errorf("bare_support_keyword findings = %d, want 1", count)
	}
}

func TestValidate_Boundary_NonSupportTriggerAllowsBareSupport(t *testing.T) {
	trigger := fixtureTrigger()
	trigger.Keywords.Any = []string{"Diallak support"}
	if hasFinding(Validate(fixtureTable(trigger), knownNames()), "bare_support_keyword") {
		t.Fatal("non-support trigger should allow bare support keyword")
	}
}

func TestResolve_Positive_ConcatenatesCorpusOrderAndLinks(t *testing.T) {
	trigger := fixtureTrigger()
	trigger.Links = []string{"docs/guide.md"}
	chunks := []corpus.Chunk{
		{Repository: trigger.Repository, Revision: "abcdef0123456789", Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Section: 1, Text: "first"},
		{Repository: trigger.Repository, Revision: "abcdef0123456789", Path: "docs/guide.md", Heading: "Guide", Text: "link"},
		{Repository: trigger.Repository, Revision: "abcdef0123456789", Path: "docs/guide.md", Heading: "More", Text: "link two"},
		{Repository: trigger.Repository, Revision: "abcdef0123456789", Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Section: 1, Text: "second"},
	}
	resolved, off, err := Resolve(fixtureTable(trigger), chunks, "https://example.test/{repository}/{revision}/{path}")
	if err != nil || len(off) != 0 {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, off, err)
	}
	if got := resolved.Answers[trigger.ID].Text; got != "first\n\nsecond" {
		t.Errorf("text = %q", got)
	}
	if len(resolved.Answers[trigger.ID].Sources) != 2 {
		t.Errorf("sources = %+v", resolved.Answers[trigger.ID].Sources)
	}
	if len(resolved.Answers[trigger.ID].Links) != 1 {
		t.Errorf("links = %+v", resolved.Answers[trigger.ID].Links)
	}
}

func TestResolve_Negative_MissingHeadingDisablesTrigger(t *testing.T) {
	trigger := fixtureTrigger()
	resolved, disabled, err := Resolve(fixtureTable(trigger), []corpus.Chunk{{Repository: trigger.Repository, Path: "README.md", Heading: "Other"}}, "")
	if err != nil || len(disabled) != 1 || disabled[0].Reason != "answer_missing" || len(resolved.Answers) != 0 || resolved.DisabledCount != 1 {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, disabled, err)
	}
}

func TestResolve_Positive_RemovesSponsorshipParagraph(t *testing.T) {
	trigger := fixtureTrigger()
	chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "Ask for help in Discussions.\n\n[Sponsor us](https://github.com/sponsors/example)"}}
	resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(issues) != 0 || resolved.Answers[trigger.ID].Text != "Ask for help in Discussions." {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, issues, err)
	}
}

func TestResolve_Negative_SponsorshipOnlyDisablesTrigger(t *testing.T) {
	for _, url := range []string{"https://github.com/sponsors/example", "https://ko-fi.com/example", "https://opencollective.com/example", "https://liberapay.com/example", "https://patreon.com/example", "https://buymeacoffee.com/example"} {
		trigger := fixtureTrigger()
		chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "[Sponsor](" + url + ")"}}
		resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
		if err != nil || len(issues) != 1 || issues[0] != (Issue{trigger.ID, "sponsorship_only"}) || resolved.DisabledCount != 1 || len(resolved.Answers) != 0 {
			t.Errorf("Resolve(%s) = (%v, %v, %v)", url, resolved, issues, err)
		}
	}
}

func TestResolve_Boundary_SponsorshipListItem(t *testing.T) {
	trigger := fixtureTrigger()
	chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "- Ask for help in Discussions.\n- [Sponsor](https://ko-fi.com/example)"}}
	resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(issues) != 0 || resolved.Answers[trigger.ID].Text != "- Ask for help in Discussions." {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, issues, err)
	}
}

func TestResolve_Negative_MissingLinkIsDropped(t *testing.T) {
	trigger := fixtureTrigger()
	trigger.Links = []string{"docs/guide.md"}
	chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "kept text"}}
	resolved, disabled, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(disabled) != 1 || disabled[0].Reason != "link_missing" || len(resolved.Answers[trigger.ID].Links) != 0 || resolved.DisabledCount != 0 {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, disabled, err)
	}
	if resolved.Answers[trigger.ID].Text != "kept text" {
		t.Error("answer text lost")
	}
}

func TestResolve_Boundary_AbsentRepositoryIsOff(t *testing.T) {
	resolved, off, err := Resolve(fixtureTable(fixtureTrigger()), nil, "")
	if err != nil || resolved == nil || len(off) != 1 || off[0].TriggerID != "release-check" {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, off, err)
	}
}

func TestMatch_Boundary_AliasScope(t *testing.T) {
	table := fixtureTable(fixtureTrigger())
	if _, ok := table.Match(Query{Text: "verify release", Command: "ask"}); ok {
		t.Fatal("command without alias matched")
	}
	if _, ok := table.Match(Query{Text: "verify release", Forum: true, Tags: []string{"Diallak"}}); !ok {
		t.Fatal("scoped forum did not match")
	}
}

func TestValidate_Negative_CrossProductAmbiguity(t *testing.T) {
	first := fixtureTrigger()
	second := fixtureTrigger()
	second.ID = "other-release"
	second.Repository = "example/other"
	second.Aliases = []string{"Other"}
	second.ForumTags = nil
	first.MustMatch = []string{"verify Diallak Other release"}
	if !hasFinding(Validate(fixtureTable(first, second), knownNames()), "ambiguous_match") {
		t.Fatal("missing cross-product ambiguity")
	}
}

func TestValidate_Negative_RequiresCommandAlias(t *testing.T) {
	trigger := fixtureTrigger()
	trigger.Aliases = nil
	if !hasFinding(Validate(fixtureTable(trigger), knownNames()), "aliases") {
		t.Fatal("missing alias finding")
	}
}

func TestValidate_Boundary_NearMiss(t *testing.T) {
	for _, tc := range []struct {
		name, negative string
		want           bool
	}{
		{"near miss", "Why does Diallak release fail?", false},
		{"far miss", "Where is the roadmap?", true},
		{"alias only", "Where is Diallak?", true},
		{"keyword only", "Why does the release fail?", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trigger := fixtureTrigger()
			trigger.MustNotMatch = []string{tc.negative}
			got := hasFinding(Validate(fixtureTable(trigger), knownNames()), "near_miss")
			if got != tc.want {
				t.Errorf("near_miss = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestMatch_Negative_MultipleForumProducts(t *testing.T) {
	a := fixtureTrigger()
	b := fixtureTrigger()
	b.ID = "other"
	b.Repository = "other/product"
	b.ForumTags = []string{"Other"}
	q := Query{Text: "verify release", Forum: true, Tags: []string{"Diallak", "Other"}}
	table := fixtureTable(a, b)
	if _, ok := table.Match(q); ok {
		t.Error("ambiguous forum post matched")
	}
	resolved := &Resolved{Table: table, Answers: map[string]ResolvedAnswer{a.ID: {Trigger: a}, b.ID: {Trigger: b}}}
	if _, ok := resolved.Match(q); ok {
		t.Error("ambiguous resolved forum post matched")
	}
}

func TestValidate_Boundary_TriggerAndFindingCaps(t *testing.T) {
	table := fixtureTable(make([]Trigger, MaxTriggers)...)
	if !hasFinding(Validate(table, knownNames()), "id") {
		t.Error("limit must still validate")
	}
	table.Triggers = append(table.Triggers, Trigger{})
	got := Validate(table, knownNames())
	if len(got) != 1 || got[0].Code != "trigger_count" {
		t.Errorf("above limit findings = %+v", got)
	}
	v := validator{}
	for i := 0; i < MaxFindings+1; i++ {
		v.add("x", "p", "m")
	}
	if len(v.findings) != MaxFindings {
		t.Errorf("finding count = %d", len(v.findings))
	}
}

func TestResolve_Positive_IndentedCodePreserved(t *testing.T) {
	input := "    code line\n    second line"
	if got := filterSponsorship(input); got != input {
		t.Errorf("got %q", got)
	}
}

func TestFilterSponsorship_Positive_ListUnchanged(t *testing.T) {
	input := "\n- first\n- second\n\n"
	if got := filterSponsorship(input); got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestFilterSponsorship_Positive_RemoveMiddleItem(t *testing.T) {
	input := "- first\n- [Sponsor](https://ko-fi.com/example)\n- last"
	want := "- first\n- last"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Negative_NestedSponsorPreservesParent(t *testing.T) {
	input := "- parent\n  - ordinary\n  - [Sponsor](https://ko-fi.com/example)"
	want := "- parent\n  - ordinary"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Negative_NestedSponsorBetweenItems(t *testing.T) {
	input := "- parent\n\t1. first\n\t2. [Sponsor](https://ko-fi.com/example)\n\t3. last"
	want := "- parent\n\t1. first\n\t3. last"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Negative_NestedSponsorContinuation(t *testing.T) {
	input := "- parent\n  - [Sponsor](https://ko-fi.com/example)\n    continuation\n  - ordinary"
	want := "- parent\n  - ordinary"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Positive_RemovesSponsorSubtree(t *testing.T) {
	input := "- [Sponsor](https://ko-fi.com/example)\n  - Gold tier\n    benefits"
	if got := filterSponsorship(input); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFilterSponsorship_Positive_RemovesSubtreeBetweenItems(t *testing.T) {
	input := "- ordinary\n- [Sponsor](https://ko-fi.com/example)\n  - Gold tier\n    benefits\n- last"
	want := "- ordinary\n- last"
	if got := filterSponsorship(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFilterSponsorship_Boundary_SubtreeBeforeParagraph(t *testing.T) {
	input := "- [Sponsor](https://ko-fi.com/example)\n  - Gold tier\n    benefits\n\nparagraph"
	if got := filterSponsorship(input); got != "paragraph" {
		t.Errorf("got %q, want paragraph", got)
	}
}

func TestResolve_Boundary_LoadedIndentedCode(t *testing.T) {
	trigger := fixtureTrigger()
	fsys := fstest.MapFS{"README.md": &fstest.MapFile{Data: []byte("# Release\n\n    first line\n    second line\n")}}
	src := corpus.Source{Repository: trigger.Repository, Revision: "main", Patterns: []string{"README.md"}}
	chunks, err := corpus.Load(context.Background(), fsys, src, corpus.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(issues) != 0 {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, issues, err)
	}
	if got, want := resolved.Answers[trigger.ID].Text, "    first line\n    second line"; got != want {
		t.Errorf("answer = %q, want %q", got, want)
	}
}

func TestValidate_Positive_MultiProductForumReplay(t *testing.T) {
	a := fixtureTrigger()
	a.ID, a.Repository, a.ForumTags = "amussis-support", "tekmora/amussis", []string{"Amussis"}
	a.Commands, a.Aliases = nil, nil
	a.Keywords = Keywords{All: []string{"get help"}, Any: []string{"please"}}
	a.MustMatch, a.MustNotMatch = []string{"please get help"}, []string{"get help"}
	b := a
	b.ID, b.Repository, b.ForumTags = "vmafx-support", "tekmora/vmafx", []string{"VMAFx"}
	table := fixtureTable(a, b)
	if findings := Validate(table, Known{ForumTags: []string{"Amussis", "VMAFx"}}); len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	resolved := &Resolved{Table: table, Answers: map[string]ResolvedAnswer{a.ID: {Trigger: a}, b.ID: {Trigger: b}}}
	q := Query{Text: "please get help", Forum: true, Tags: []string{"Amussis", "VMAFx"}}
	if _, ok := table.Match(q); ok {
		t.Error("multi-tag table match")
	}
	if _, ok := resolved.Match(q); ok {
		t.Error("multi-tag resolved match")
	}
	for _, trigger := range []Trigger{a, b} {
		q.Tags = trigger.ForumTags
		if got, ok := table.Match(q); !ok || got.ID != trigger.ID {
			t.Errorf("single-tag table = %q, %t", got.ID, ok)
		}
		if got, ok := resolved.Match(q); !ok || got.Trigger.ID != trigger.ID {
			t.Errorf("single-tag resolved = %q, %t", got.Trigger.ID, ok)
		}
	}
}

func TestReplayMultiTag_Negative_OtherProductWins(t *testing.T) {
	a := fixtureTrigger()
	b := fixtureTrigger()
	b.ID, b.Repository, b.ForumTags = "other", "tekmora/other", []string{"Other"}
	a.Keywords = Keywords{All: []string{"different"}}
	v := validator{table: fixtureTable(a, b)}
	v.replayMultiTag("triggers[0]", &a, 0, Query{Text: "verify release", Forum: true, Tags: []string{"Diallak"}})
	if !hasFinding(v.findings, "ambiguous_match") {
		t.Fatalf("findings = %+v", v.findings)
	}
}

func TestLoad_Negative_DuplicateObjectKeys(t *testing.T) {
	for _, tc := range []struct{ raw, key string }{
		{`{"version":1,"version":1,"triggers":[]}`, "version"},
		{`{"version":1,"triggers":[{"id":"a","id":"b"}]}`, "id"},
		{`{"version":1,"triggers":[{"keywords":{"all":[],"all":[]}}]}`, "all"},
	} {
		_, err := Load([]byte(tc.raw))
		if err == nil || !strings.Contains(err.Error(), tc.key) {
			t.Errorf("duplicate %s: %v", tc.key, err)
		}
	}
	if _, err := Load([]byte(`{"version":1,"triggers":[{"id":"a"},{"id":"b"}]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Negative_NonCanonicalObjectKeys(t *testing.T) {
	for _, tc := range []struct{ raw, key string }{
		{`{"version":2,"VERSION":1,"triggers":[]}`, "VERSION"},
		{`{"Version":1,"triggers":[]}`, "Version"},
		{`{"version":1,"triggers":[{"Keywords":{}}]}`, "Keywords"},
		{"{\"version\":1,\"triggers\":[{\"Keywords\":{}}]}", "Keywords"},
	} {
		_, err := Load([]byte(tc.raw))
		if err == nil || !strings.Contains(err.Error(), tc.key) {
			t.Errorf("key %q: %v", tc.key, err)
		}
	}
}

func TestValidate_Boundary_ListSize(t *testing.T) {
	trigger := fixtureTrigger()
	trigger.Links = make([]string, MaxListItems)
	for i := range trigger.Links {
		trigger.Links[i] = "README.md"
	}
	if got := Validate(fixtureTable(trigger), knownNames()); hasFinding(got, "list_size") {
		t.Fatalf("at cap: %+v", got)
	}
	trigger.Links = append(trigger.Links, "README.md")
	if got := Validate(fixtureTable(trigger), knownNames()); !hasFinding(got, "list_size") {
		t.Fatalf("above cap: %+v", got)
	}
}

func TestCheckJSONKeys_Boundary_Depth(t *testing.T) {
	for _, depth := range []int{maxJSONDepth, maxJSONDepth + 1} {
		raw := strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth)
		err := checkJSONKeys([]byte(raw))
		if (err != nil) != (depth > maxJSONDepth) {
			t.Errorf("depth %d: %v", depth, err)
		}
	}
}

func TestValidate_Negative_AllListSizes(t *testing.T) {
	trigger := fixtureTrigger()
	lists := []struct {
		name  string
		items *[]string
	}{
		{"keywords.all", &trigger.Keywords.All}, {"keywords.any", &trigger.Keywords.Any},
		{"keywords.none", &trigger.Keywords.None}, {"commands", &trigger.Commands},
		{"aliases", &trigger.Aliases}, {"forum_tags", &trigger.ForumTags},
		{"links", &trigger.Links}, {"must_match", &trigger.MustMatch},
		{"must_not_match", &trigger.MustNotMatch},
	}
	for _, list := range lists {
		t.Run(list.name, func(t *testing.T) {
			old := *list.items
			*list.items = make([]string, MaxListItems+1)
			findings := Validate(fixtureTable(trigger), knownNames())
			*list.items = old
			if len(findings) != 1 || findings[0].Code != "list_size" || !strings.HasSuffix(findings[0].Path, "."+list.name) {
				t.Fatalf("findings = %+v", findings)
			}
		})
	}
}

func TestResolve_Positive_SponsorshipSentences(t *testing.T) {
	trigger := fixtureTrigger()
	for _, text := range []string{
		"Ask in Discussions for help. You can also [sponsor us](https://github.com/sponsors/example).",
		"You can also [sponsor us](https://github.com/sponsors/example). Ask in Discussions for help.",
		"Ask in Discussions for help. You can also\n[sponsor us](https://github.com/sponsors/example).",
		"Ask in Discussions for help. Sponsor at https://ko-fi.com/example e.g. monthly.",
	} {
		chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: text}}
		resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
		if err != nil || len(issues) != 0 || resolved.Answers[trigger.ID].Text != "Ask in Discussions for help." {
			t.Errorf("Resolve(%q) = (%v, %v, %v)", text, resolved, issues, err)
		}
	}
}

func TestResolve_Positive_SponsorshipLowercaseSentence(t *testing.T) {
	trigger := fixtureTrigger()
	text := "Sponsor https://ko-fi.com/example. iOS users can ask in Discussions."
	chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: text}}
	resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(issues) != 0 || resolved.Answers[trigger.ID].Text != "iOS users can ask in Discussions." {
		t.Errorf("Resolve(%q) = (%v, %v, %v)", text, resolved, issues, err)
	}
}

func TestFilterSponsorship_Negative_IndentedCodeUnchanged(t *testing.T) {
	text := "Keep this paragraph.\n\n    example. Sponsor https://ko-fi.com/example\n    more code\n\n    safe code\n    more safe code"
	want := "Keep this paragraph.\n\n    safe code\n    more safe code"
	if got := filterSponsorship(text); got != want {
		t.Errorf("filterSponsorship() = %q; want %q", got, want)
	}
}

func TestFilterSponsorship_Negative_IndentedCodeListLikeLines(t *testing.T) {
	text := "Keep this paragraph.\n\n    keep\n    - example\n    https://ko-fi.com/example\n\n    safe code\n    - safe list-like code\n    more safe code"
	want := "Keep this paragraph.\n\n    safe code\n    - safe list-like code\n    more safe code"
	if got := filterSponsorship(text); got != want {
		t.Errorf("filterSponsorship() = %q; want %q", got, want)
	}
}

func TestFilterSponsorshipSentences_Boundary_NextRune(t *testing.T) {
	for _, suffix := range []string{"Next", "iOS users can ask in Discussions.", "1 next", "[next]", "(next)", "\"Next\"", "'Next'"} {
		text := "Sponsor https://ko-fi.com/example. " + suffix
		if got := filterSponsorshipSentences(text); got != suffix {
			t.Errorf("filterSponsorshipSentences(%q) = %q; want %q", text, got, suffix)
		}
	}
	if got := filterSponsorshipSentences("Sponsor https://ko-fi.com/example."); got != "" {
		t.Errorf("sponsorship at end = %q", got)
	}
}

func TestFilterSponsorshipSentences_Boundary_Abbreviations(t *testing.T) {
	for _, tc := range []struct{ word, want string }{
		{"i.e.", ""}, {"U.S.", ""}, {"(e.g.", ""},
		{"a.", "keep"}, {"J.", "keep"}, {"example.", "keep"},
	} {
		text := "Sponsor https://ko-fi.com/example " + tc.word + " keep"
		if got := filterSponsorshipSentences(text); got != tc.want {
			t.Errorf("filterSponsorshipSentences(%q) = %q; want %q", text, got, tc.want)
		}
	}
}

func TestFilterSponsorship_Boundary_IndentedList(t *testing.T) {
	text := "Keep.\n\n    - first\n    - Sponsor https://ko-fi.com/example\n    - last"
	want := "Keep.\n\n    - first\n    - last"
	if got := filterSponsorship(text); got != want {
		t.Errorf("filterSponsorship() = %q; want %q", got, want)
	}
}

func TestResolve_Negative_AmbiguousSection(t *testing.T) {
	trigger := fixtureTrigger()
	chunks := []corpus.Chunk{
		{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Section: 1, Text: "development"},
		{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Section: 2, Text: "production"},
	}
	resolved, issues, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(issues) != 1 || issues[0].Reason != "answer_ambiguous" || resolved.DisabledCount != 1 || len(resolved.Answers) != 0 {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, issues, err)
	}
}

func TestHasNegation_Positive_Words(t *testing.T) {
	for _, word := range []string{"not", "no", "never", "cannot", "can't", "cant", "don't", "dont", "doesn't", "doesnt", "didn't", "didnt", "won't", "wont", "isn't", "isnt", "aren't", "arent", "shouldn't", "shouldnt", "couldn't", "wouldn’t", "wasnt", "werent", "havent", "hasnt", "hadnt", "mustnt", "neednt", "couldnt", "wouldnt", "nothing", "none", "neither", "nor"} {
		if !hasNegation("Question " + strings.ToUpper(word) + " start?") {
			t.Errorf("missed %q", word)
		}
	}
}

func TestHasNegation_Negative_Words(t *testing.T) {
	for _, text := range []string{"notice", "know", "start Diallak"} {
		if hasNegation(text) {
			t.Errorf("unexpected negation in %q", text)
		}
	}
}

func TestHasNegation_Boundary_Text(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{{"don’t", true}, {"No. start", true}, {"", false}, {"notice know", false}, {"want", false}, {"front", false}, {"client", false}} {
		if got := hasNegation(tc.text); got != tc.want {
			t.Errorf("hasNegation(%q) = %t", tc.text, got)
		}
	}
}

func TestMatch_Negative_ForumNegation(t *testing.T) {
	trigger := fixtureTrigger()
	if _, ok := fixtureTable(trigger).Match(Query{Text: "Do not verify the Diallak release image", Forum: true, Tags: trigger.ForumTags}); ok {
		t.Fatal("negated forum question matched")
	}
}
