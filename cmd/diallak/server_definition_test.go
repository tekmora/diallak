package main

import (
	"os"
	"path/filepath"
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
