package main

import (
	"strings"
	"testing"
)

// The organisation's code of conduct (ADR-0008). It lives once, in the organisation's .github
// repository, and every text that names it links this address.
const codeOfConductURL = "https://github.com/tekmora/.github/blob/main/CODE_OF_CONDUCT.md"

// ADR-0008 lets the server rules point to the code of conduct, and rule 1 is that pointer. The rule
// links the code of conduct at the organisation.
func TestServerRules_Positive_RuleOneLinksTheCodeOfConduct(t *testing.T) {
	rules := readDoc(t, "docs/compliance/server-rules.md")
	start := strings.Index(rules, "1. **The code of conduct applies here.**")
	end := strings.Index(rules, "2. **")
	if start < 0 || end < start {
		t.Fatal("the server rules have no rule 1 on the code of conduct")
	}
	if !strings.Contains(rules[start:end], "("+codeOfConductURL+")") {
		t.Errorf("rule 1 does not link %s", codeOfConductURL)
	}
}

// The code of conduct exists, so no text still waits for it: the rules, the moderator guide and the
// index of compliance texts link it, and none says it does not exist or that a link follows.
func TestServerRules_Negative_NoTextWaitsForTheCodeOfConduct(t *testing.T) {
	for _, path := range []string{
		"docs/compliance/server-rules.md",
		"docs/compliance/README.md",
		"docs/moderator-guide.md",
	} {
		doc := readDoc(t, path)
		if !strings.Contains(doc, "("+codeOfConductURL+")") {
			t.Errorf("%s does not link the code of conduct", path)
		}
		for _, stale := range []string{
			"does not exist yet",
			"The link follows",
			"once the code of conduct exists",
			"Still to come before moderation",
		} {
			if strings.Contains(doc, stale) {
				t.Errorf("%s still says %q", path, stale)
			}
		}
	}
}

// The operator accepted the code of conduct and kept the points it does not cover as additions
// for the server only. The rules no longer call any part of it proposed, name the additions as
// decided, and leave no open point on either question.
func TestServerRules_Positive_DecidedAdditionsAndAcceptedCodeOfConduct(t *testing.T) {
	rules := readDoc(t, "docs/compliance/server-rules.md")
	for _, stale := range []string{"proposed", "Our Pledge", "decides whether they become"} {
		if strings.Contains(rules, stale) {
			t.Errorf("the server rules still say %q", stale)
		}
	}
	start := strings.Index(rules, "additions for the server only")
	end := strings.Index(rules, "## How the rules are applied")
	if start < 0 || end < start {
		t.Fatal("the server rules do not name the additions for the server")
	}
	for _, addition := range []string{"gory or shocking", "No spam", "No scams", "pirated software", "Nothing illegal"} {
		if !strings.Contains(rules[start:end], addition) {
			t.Errorf("the additions for the server lack %q", addition)
		}
	}
}
