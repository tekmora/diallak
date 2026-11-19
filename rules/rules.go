// Package rules exposes the validated embedded common-question table.
package rules

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/tekmora/diallak/internal/ruleanswer"
	"github.com/tekmora/diallak/server"
)

//go:embed triggers.json
var triggerJSON string

// Table loads and validates the embedded trigger table.
func Table() (*ruleanswer.Table, error) {
	table, err := ruleanswer.Load(strings.NewReader(triggerJSON))
	if err != nil {
		return nil, fmt.Errorf("load embedded trigger table: %w", err)
	}
	definition, err := server.Definition()
	if err != nil {
		return nil, fmt.Errorf("load server definition: %w", err)
	}
	var tags []string
	for _, channel := range definition.Channels {
		if channel.ID == "support" {
			for _, tag := range channel.Tags {
				tags = append(tags, tag.Name)
			}
		}
	}
	findings := ruleanswer.Validate(table, ruleanswer.Known{Commands: []string{"ask"}, ForumTags: tags})
	if len(findings) > 0 {
		return nil, fmt.Errorf("validate embedded trigger table: %d findings", len(findings))
	}
	return table, nil
}
