package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
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
	want := make(map[string]bool)
	for _, trigger := range table.Triggers {
		if trigger.Repository == "tekmora/diallak" {
			want[trigger.ID] = true
		}
	}
	if len(want) == 0 {
		t.Error("no triggers for tekmora/diallak")
	}
	if resolved.DisabledCount != len(table.Triggers)-len(want) {
		t.Errorf("disabled = %d, want %d", resolved.DisabledCount, len(table.Triggers)-len(want))
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

func TestRuleTable_Positive_LicenceSpellings(t *testing.T) {
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

var channelPlanProductRow = regexp.MustCompile(
	`^\|\s*[^|]+\s*\|\s*\[([^\]]+)\]\(([^)]+)\)\s*\|`,
)

func channelPlanRepositories(doc []byte) map[string]bool {
	plan := make(map[string]bool)
	for _, line := range strings.Split(string(doc), "\n") {
		sub := channelPlanProductRow.FindStringSubmatch(line)
		if sub == nil {
			continue
		}
		repo, addr := sub[1], sub[2]
		if addr != "https://github.com/"+repo {
			continue
		}
		plan[repo] = true
	}
	return plan
}

func triggerRepositoryFindings(table *ruleanswer.Table, plan map[string]bool) []string {
	var findings []string
	for _, trigger := range table.Triggers {
		if plan[trigger.Repository] {
			continue
		}
		findings = append(findings, fmt.Sprintf(
			"trigger %q: repository %q not in channel plan product table",
			trigger.ID, trigger.Repository,
		))
	}
	sort.Strings(findings)
	return findings
}

func TestRuleTable_Positive_TriggerRepositoriesInChannelPlan(t *testing.T) {
	doc, err := os.ReadFile("../../docs/channel-plan.md")
	if err != nil {
		t.Fatal(err)
	}
	plan := channelPlanRepositories(doc)
	if len(plan) != 13 {
		t.Fatalf("channel plan product repositories = %d, want 13", len(plan))
	}
	table, err := rules.Table()
	if err != nil {
		t.Fatal(err)
	}
	if findings := triggerRepositoryFindings(table, plan); len(findings) != 0 {
		t.Fatalf("trigger repository findings:\n%s", strings.Join(findings, "\n"))
	}
}

func TestRuleTable_Negative_PlantedTriggerOutsideChannelPlan(t *testing.T) {
	table, err := rules.Table()
	if err != nil {
		t.Fatal(err)
	}
	triggers := slices.Clone(table.Triggers)
	var plantedID string
	for i := range triggers {
		if triggers[i].Repository != "vidiko/vmafx" {
			continue
		}
		plantedID = triggers[i].ID
		triggers[i].Repository = "VMAFx/vmafx"
		break
	}
	if plantedID == "" {
		t.Fatal("no trigger with repository vidiko/vmafx")
	}
	cloned := &ruleanswer.Table{Version: table.Version, Triggers: triggers}
	doc, err := os.ReadFile("../../docs/channel-plan.md")
	if err != nil {
		t.Fatal(err)
	}
	plan := channelPlanRepositories(doc)
	findings := triggerRepositoryFindings(cloned, plan)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want 1", findings)
	}
	if !strings.Contains(findings[0], plantedID) {
		t.Fatalf("finding %q does not name trigger %q", findings[0], plantedID)
	}
}

func TestRuleTable_Boundary_TriggerRepositoryCaseMatters(t *testing.T) {
	table, err := rules.Table()
	if err != nil {
		t.Fatal(err)
	}
	triggers := slices.Clone(table.Triggers)
	var plantedID string
	for i := range triggers {
		if triggers[i].Repository != "lusoris/SnatchArr" {
			continue
		}
		plantedID = triggers[i].ID
		triggers[i].Repository = "lusoris/snatcharr"
		break
	}
	if plantedID == "" {
		t.Fatal("no trigger with repository lusoris/SnatchArr")
	}
	cloned := &ruleanswer.Table{Version: table.Version, Triggers: triggers}
	doc, err := os.ReadFile("../../docs/channel-plan.md")
	if err != nil {
		t.Fatal(err)
	}
	plan := channelPlanRepositories(doc)
	findings := triggerRepositoryFindings(cloned, plan)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want 1", findings)
	}
	if !strings.Contains(findings[0], plantedID) {
		t.Fatalf("finding %q does not name trigger %q", findings[0], plantedID)
	}
}
