package main

import (
	"context"
	"os"
	"strings"
	"testing"

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
	chunks, err := corpus.Load(context.Background(), os.DirFS("../.."), corpus.Source{
		Repository: "tekmora/diallak", Revision: "fixture",
		Patterns: []string{"README.md", "docs/**/*.md"},
	}, corpus.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, off, err := ruleanswer.Resolve(table, chunks, ""); err != nil || len(off) != len(table.Triggers)-3 {
		t.Fatalf("Resolve own README = off %v, err %v", off, err)
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
		if !strings.HasSuffix(trigger.ID, "-licence") {
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
