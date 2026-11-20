package main

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/discord"
	"github.com/tekmora/diallak/internal/ruleanswer"
	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/rules"
	"github.com/tekmora/diallak/server"
)

func TestRuleTable_Positive_CrossChecksServerCommandsTagsAndCorpus(t *testing.T) {
	table, err := rules.Table()
	if err != nil {
		t.Fatal(err)
	}
	definition, err := server.Definition()
	if err != nil {
		t.Fatal(err)
	}
	supportTags := ruleSupportTags(definition.Channels)
	commandNames := discord.RegisteredCommandNames()
	if findings := ruleanswer.Validate(table, ruleanswer.Known{Commands: commandNames, ForumTags: supportTags}); len(findings) != 0 {
		t.Fatalf("trigger validation findings = %+v", findings)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	chunks, err := corpus.Load(ctx, os.DirFS("../.."), corpus.Source{
		Repository: "tekmora/diallak", Revision: "fixture",
		Patterns: []string{"README.md", "docs/**/*.md"},
	}, corpus.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	resolved, issues, err := ruleanswer.Resolve(table, chunks, "")
	if err != nil {
		t.Fatal(err)
	}
	checkResolvedRuleAnswers(t, table, resolved, issues)
}

func checkResolvedRuleAnswers(t *testing.T, table *ruleanswer.Table, resolved *ruleanswer.Resolved, issues []ruleanswer.Issue) {
	t.Helper()
	if resolved.DisabledCount != len(table.Triggers)-3 {
		t.Errorf("disabled = %d, want %d", resolved.DisabledCount, len(table.Triggers)-3)
	}
	want := make(map[string]bool)
	for _, trigger := range table.Triggers {
		if trigger.Repository == "tekmora/diallak" {
			want[trigger.ID] = true
		}
	}
	if len(resolved.Answers) != len(want) {
		t.Errorf("resolved answers = %v, want IDs %v", resolved.Answers, want)
	}
	for id := range resolved.Answers {
		if !want[id] {
			t.Errorf("unexpected resolved answer %q", id)
		}
	}
	for _, issue := range issues {
		if issue.Reason == "link_missing" {
			t.Errorf("missing link: %+v", issue)
		}
	}
}

func ruleSupportTags(channels []serverdef.Channel) []string {
	var tags []string
	for _, channel := range channels {
		if channel.ID == "support" {
			for _, tag := range channel.Tags {
				tags = append(tags, tag.Name)
			}
		}
	}
	return tags
}

func TestRuleTable_LicenceSpellings(t *testing.T) {
	table, err := rules.Table()
	if err != nil {
		t.Fatal(err)
	}
	for _, trigger := range table.Triggers {
		if !strings.HasSuffix(trigger.ID, "-licence") || !slices.Contains(trigger.Commands, "ask") {
			continue
		}
		for _, spelling := range []string{"licence", "license"} {
			q := ruleanswer.Query{Text: "What is the " + trigger.Aliases[0] + " " + spelling, Command: "ask"}
			got, ok := table.Match(q)
			if !ok || got.ID != trigger.ID {
				t.Errorf("%s: got %s, %t", trigger.ID, got.ID, ok)
			}
		}
	}
}
