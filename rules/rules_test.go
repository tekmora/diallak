package rules

import (
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/ruleanswer"
)

func TestTablePrecision(t *testing.T) {
	table, err := Table()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ question, want string }{
		{"How do I run the Diallak tests?", ""},
		{"How do I not start Diallak?", ""},
		{"How do I start Diallak?", "diallak-start"},
		{"Diallak crashes when I run it", ""},
		{"Why does Diallak not start after the update?", ""},
		{"How do I start contributing to Epitrop?", "epitrop-contribute"},
		{"Does Amussis support ARM64?", ""},
		{"Does Pelorus support Windows?", ""},
		{"Is VMAFx support for HDR planned?", ""},
		{"Can VMAFx help with HDR playback?", ""},
		{"How do I get help with Amussis?", "amussis-support"},
		{"How do I set up Snatcharr?", "snatcharr-start"},
		{"How do I verify the Diallak image signature?", "diallak-verify-release"},
		{"Where can I get support for Pelorus?", ""},
		{"How do I get help with VMAFx?", "vmafx-support"},
		{"Where can I get help with SnatchArr?", "snatcharr-support"},
		{"How do I install Amussis?", ""},
		{"Does SnatchArr support Usenet?", ""},
		{"How can I install Diallak?", "diallak-start"},
		{"Can I contribute to VMAFx?", "vmafx-contribute"},
		{"Diallak install fails with an error", ""},
	} {
		got, ok := table.Match(ruleanswer.Query{Text: tc.question, Command: "ask"})
		if (tc.want == "" && ok) || (tc.want != "" && (!ok || got.ID != tc.want)) {
			t.Errorf("%q matched %s, %t; want %q", tc.question, got.ID, ok, tc.want)
		}
	}
}

func TestTableExamplesEveryEntryPoint(t *testing.T) {
	table, err := Table()
	if err != nil {
		t.Fatal(err)
	}
	for _, trigger := range table.Triggers {
		for _, example := range trigger.MustMatch {
			checkExampleEntryPoints(t, table, trigger, example)
		}
	}
}

func checkExampleEntryPoints(t *testing.T, table *ruleanswer.Table, trigger ruleanswer.Trigger, example string) {
	t.Helper()
	for _, command := range trigger.Commands {
		got, ok := table.Match(ruleanswer.Query{Text: example, Command: command})
		if !ok || got.ID != trigger.ID {
			t.Errorf("%s command %s: got %s, %t", trigger.ID, command, got.ID, ok)
		}
	}
	for _, tag := range trigger.ForumTags {
		got, ok := table.Match(ruleanswer.Query{Text: example, Forum: true, Tags: []string{tag}})
		if !ok || got.ID != trigger.ID {
			t.Errorf("%s tag %s: got %s, %t", trigger.ID, tag, got.ID, ok)
		}
	}
}

func TestTable_Negative_ReportsFirstFinding(t *testing.T) {
	old := triggerJSON
	t.Cleanup(func() { triggerJSON = old })
	triggerJSON = `{"version":2,"triggers":[]}`
	_, err := Table()
	if err == nil || !strings.Contains(err.Error(), "1 findings") || !strings.Contains(err.Error(), "version: must be 1") {
		t.Fatalf("Table error = %v", err)
	}
}

func TestTable_Boundary_TriggerAndListSizes(t *testing.T) {
	table, err := Table()
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Triggers) < 1 || len(table.Triggers) > ruleanswer.MaxTriggers {
		t.Errorf("trigger count = %d", len(table.Triggers))
	}
	for _, trigger := range table.Triggers {
		lists := map[string][]string{
			"keywords.all": trigger.Keywords.All, "keywords.any": trigger.Keywords.Any,
			"keywords.none": trigger.Keywords.None, "commands": trigger.Commands,
			"aliases": trigger.Aliases, "forum_tags": trigger.ForumTags, "links": trigger.Links,
			"must_match": trigger.MustMatch, "must_not_match": trigger.MustNotMatch,
		}
		for name, items := range lists {
			if len(items) > ruleanswer.MaxListItems {
				t.Errorf("%s %s: %d items", trigger.ID, name, len(items))
			}
		}
	}
}
