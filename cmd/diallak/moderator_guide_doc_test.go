package main

import (
	"os"
	"strings"
	"testing"
)

// The two commands of ADR-0021, the only way a moderator acts (ADR-0015). The guide names them by
// these names; the commands themselves come with issue 89.
var moderatorCommands = []string{"`Remove message`", "`/timeout`"}

// readDoc reads a page of the documentation, relative to the repository root.
func readDoc(t *testing.T, path string) string {
	t.Helper()
	doc, err := os.ReadFile("../../" + path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(doc)
}

// ADR-0005 lets moderation start only when the moderator guide exists. The guide names the two
// commands as the only way to act, the limit of a timeout, what the trail records and how long it
// is kept, and it points to the server rules a member reads.
func TestModeratorGuide_Positive_NamesCommandsTrailAndLimits(t *testing.T) {
	guide := readDoc(t, "docs/moderator-guide.md")
	for _, command := range moderatorCommands {
		if !strings.Contains(guide, command) {
			t.Errorf("the moderator guide does not name the command %s", command)
		}
	}
	for _, phrase := range []string{
		"only way",
		"28 days",
		"reason",
		"90 days",
		"(compliance/server-rules.md)",
		"(adr/0021-moderation-commands-and-trail.md)",
	} {
		if !strings.Contains(guide, phrase) {
			t.Errorf("the moderator guide does not contain %q", phrase)
		}
	}
}

// A member reads the server rules, not the guide. The rules tell the member how to object to an
// act of a moderator and link the guide that binds the moderators.
func TestModeratorGuide_Positive_ServerRulesSayHowToObject(t *testing.T) {
	rules := readDoc(t, "docs/compliance/server-rules.md")
	for _, phrase := range []string{"(../moderator-guide.md)", "To object to"} {
		if !strings.Contains(rules, phrase) {
			t.Errorf("the server rules do not contain %q", phrase)
		}
	}
}

// The operator accepted the guide with ADR-0021. The guide is final: it no longer calls itself a
// draft, and the record it names is accepted, in its own header and in the index.
func TestModeratorGuide_Negative_FinalAndRecordAccepted(t *testing.T) {
	guide := readDoc(t, "docs/moderator-guide.md")
	if strings.Contains(guide, "Draft.") {
		t.Error("the moderator guide still calls itself a draft")
	}
	record := readDoc(t, "docs/adr/0021-moderation-commands-and-trail.md")
	if !strings.Contains(record, "- **Status**: Accepted\n") {
		t.Error("ADR-0021 is not accepted")
	}
	index := readDoc(t, "docs/adr/README.md")
	if !strings.Contains(index, "| The moderation commands and their trail | Accepted |") {
		t.Error("the index of records does not list ADR-0021 as accepted")
	}
}
