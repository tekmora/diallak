package main

import (
	"os"
	"regexp"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

// The page on applying the definition has a table that gives each right the label it carries in
// Discord's settings, because the operator switches rights on by label and the tool prints
// programming names. This test keeps the table in step with the definition: every right the
// definition names on a role or on a channel's entry, and MANAGE_CHANNELS, has a row with a label.
func TestServerApplyDoc_Negative_LabelsEveryRight(t *testing.T) {
	doc, err := os.ReadFile("../../docs/server-apply.md")
	if err != nil {
		t.Fatalf("read documentation page: %v", err)
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

	named := map[string]struct{}{"MANAGE_CHANNELS": {}}
	for _, role := range def.Roles {
		for _, right := range role.Permissions {
			named[right] = struct{}{}
		}
	}
	for _, channel := range def.Channels {
		for _, entry := range channel.Access {
			for _, right := range entry.Allow {
				named[right] = struct{}{}
			}
			for _, right := range entry.Deny {
				named[right] = struct{}{}
			}
		}
	}
	if len(named) < 10 {
		t.Fatalf("the definition names %d rights; the walk of this test no longer fits", len(named))
	}
	for right := range named {
		row := regexp.MustCompile("(?m)^\\| `" + regexp.QuoteMeta(right) + "` \\| [^|`]*[A-Za-z][^|`]* \\| [^|]+ \\|$")
		if !row.Match(doc) {
			t.Errorf("the documentation page has no row with a settings label for the right %q", right)
		}
	}
}
