package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

// TestServerDefinition_Positive_CommittedDefinition verifies that the declarative
// server definition file exists, parses strictly, produces zero validation findings,
// and that all referenced automation records exist in the repository.
func TestServerDefinition_Positive_CommittedDefinition(t *testing.T) {
	file, err := os.Open("../../server/definition.json")
	if err != nil {
		t.Fatalf("open server definition: %v", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Errorf("close server definition: %v", closeErr)
		}
	}()

	def, err := serverdef.Load(file)
	if err != nil {
		t.Fatalf("Load server definition: %v", err)
	}

	findings := serverdef.Validate(def)
	if len(findings) != 0 {
		t.Fatalf("Validate returned %d findings: %+v", len(findings), findings)
	}

	for _, auto := range def.Automation {
		if auto.Record != "" {
			targetPath := filepath.Join("../..", auto.Record)
			if _, statErr := os.Stat(targetPath); statErr != nil {
				t.Errorf("automation %q record file %q does not exist: %v", auto.ID, auto.Record, statErr)
			}
		}
	}
}

// The documentation page lists what the validator refuses and how many entries each part of the
// file holds. This test keeps both lists in step with the code and the file.
func TestServerDefinitionDoc_Negative_NamesEveryCodeAndCount(t *testing.T) {
	doc, err := os.ReadFile("../../docs/server-definition.md")
	if err != nil {
		t.Fatalf("read documentation page: %v", err)
	}
	source, err := os.ReadFile("../../internal/serverdef/serverdef.go")
	if err != nil {
		t.Fatalf("read validator source: %v", err)
	}
	codes := regexp.MustCompile(`(?m)^\tCode[A-Za-z]+ += "([a-z-]+)"$`).FindAllSubmatch(source, -1)
	if len(codes) < 20 {
		t.Fatalf("found %d finding codes in the source; the pattern of this test no longer fits", len(codes))
	}
	for _, code := range codes {
		if !bytes.Contains(doc, []byte("`"+string(code[1])+"`")) {
			t.Errorf("the documentation page does not name the finding code %q", code[1])
		}
	}

	file, err := os.Open("../../server/definition.json")
	if err != nil {
		t.Fatalf("open server definition: %v", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Errorf("close server definition: %v", closeErr)
		}
	}()
	def, err := serverdef.Load(file)
	if err != nil {
		t.Fatalf("Load server definition: %v", err)
	}
	if def.Settings == nil {
		t.Fatalf("the server definition holds no settings")
	}
	counts := []struct {
		part  string
		count string
	}{
		{"metrics", strconv.Itoa(len(def.Metrics))},
		{"roles", strconv.Itoa(len(def.Roles))},
		{"role_order", strconv.Itoa(len(def.RoleOrder))},
		{"categories", strconv.Itoa(len(def.Categories))},
		{"channels", strconv.Itoa(len(def.Channels))},
		{"areas", strconv.Itoa(len(def.Areas))},
		{"choices", strconv.Itoa(len(def.Choices.Questions)) + " questions"},
		{"automation", strconv.Itoa(len(def.Automation))},
		{"settings", strconv.Itoa(len(def.Settings.FilterRules)) + " filter rules and the verification level"},
		{"news", newsCount(def.News)},
	}
	for _, row := range counts {
		pattern := "(?m)^\\| `" + row.part + "` \\| [^|]+ \\| " + regexp.QuoteMeta(row.count) + " \\|$"
		if !regexp.MustCompile(pattern).Match(doc) {
			t.Errorf("the documentation page has no row for %s with the count %s", row.part, row.count)
		}
	}
}

// newsCount is the count the documentation page gives for the part news, "missing" without one.
func newsCount(news *serverdef.News) string {
	if news == nil {
		return "missing"
	}
	return strconv.Itoa(len(news.Repositories)) + " repositories and " + strconv.Itoa(len(news.Feeds)) + " feeds"
}
