package rules

import (
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
		{"Diallak crashes when I run it", ""},
		{"Why does Diallak not start after the update?", ""},
		{"How do I start contributing to Epitrop?", "epitrop-contribute"},
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
	}
}
