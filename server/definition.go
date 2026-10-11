// Package server provides access to the embedded community server definition.
package server

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

//go:embed definition.json
var definitionJSON string

// Definition loads and validates the embedded server definition.
// It returns an error naming the count when validation returns any findings.
func Definition() (*serverdef.Definition, error) {
	def, err := serverdef.Load(strings.NewReader(definitionJSON))
	if err != nil {
		return nil, fmt.Errorf("load embedded definition: %w", err)
	}
	findings := serverdef.Validate(def)
	if len(findings) > 0 {
		return nil, fmt.Errorf("validate definition: %d findings", len(findings))
	}
	return def, nil
}
