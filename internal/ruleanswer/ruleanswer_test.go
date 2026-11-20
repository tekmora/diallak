package ruleanswer

import (
	"strings"
	"testing"

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
	table, err := Load(strings.NewReader(raw))
	if err != nil || table.Version != 1 {
		t.Fatalf("Load = (%v, %v), want version 1", table, err)
	}
}

func TestLoad_Negative_UnknownFieldAndTrailingData(t *testing.T) {
	for _, raw := range []string{`{"version":1,"triggers":[],"extra":true}`, `{"version":1,"triggers":[]} {}`} {
		if _, err := Load(strings.NewReader(raw)); err == nil {
			t.Errorf("Load(%s) error = nil", raw)
		}
	}
}

func TestLoad_Boundary_WhitespaceAfterDocument(t *testing.T) {
	if _, err := Load(strings.NewReader("{\"version\":1,\"triggers\":[]}  \n")); err != nil {
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
		{Repository: trigger.Repository, Revision: "abcdef0123456789", Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "first"},
		{Repository: trigger.Repository, Revision: "abcdef0123456789", Path: "docs/guide.md", Heading: "Guide", Text: "link"},
		{Repository: trigger.Repository, Revision: "abcdef0123456789", Path: "docs/guide.md", Heading: "More", Text: "link two"},
		{Repository: trigger.Repository, Revision: "abcdef0123456789", Path: trigger.Answer.Path, Heading: trigger.Answer.Heading, Text: "second"},
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
	chunks := []corpus.Chunk{{Repository: trigger.Repository, Path: trigger.Answer.Path, Heading: trigger.Answer.Heading}}
	resolved, disabled, err := Resolve(fixtureTable(trigger), chunks, "")
	if err != nil || len(disabled) != 1 || disabled[0].Reason != "link_missing" || len(resolved.Answers[trigger.ID].Links) != 0 || resolved.DisabledCount != 0 {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, disabled, err)
	}
}

func TestResolve_Boundary_AbsentRepositoryIsOff(t *testing.T) {
	resolved, off, err := Resolve(fixtureTable(fixtureTrigger()), nil, "")
	if err != nil || resolved == nil || len(off) != 1 || off[0].TriggerID != "release-check" {
		t.Fatalf("Resolve = (%v, %v, %v)", resolved, off, err)
	}
}

func TestMatch_AliasScope(t *testing.T) {
	table := fixtureTable(fixtureTrigger())
	if _, ok := table.Match(Query{Text: "verify release", Command: "ask"}); ok {
		t.Fatal("command without alias matched")
	}
	if _, ok := table.Match(Query{Text: "verify release", Forum: true, Tags: []string{"Diallak"}}); !ok {
		t.Fatal("scoped forum did not match")
	}
}

func TestValidate_CrossProductAmbiguity(t *testing.T) {
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

func TestValidate_RequiresCommandAlias(t *testing.T) {
	trigger := fixtureTrigger()
	trigger.Aliases = nil
	if !hasFinding(Validate(fixtureTable(trigger), knownNames()), "aliases") {
		t.Fatal("missing alias finding")
	}
}

func TestValidateNearMiss(t *testing.T) {
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
