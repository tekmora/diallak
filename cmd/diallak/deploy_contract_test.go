package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type deployContractRow struct {
	variable string
	kind     string
	needed   string
}

// loadContractRows extracts all rows from the cluster deployment contract table in docs/hosting.md.
func loadContractRows(t *testing.T) []deployContractRow {
	t.Helper()
	data, err := os.ReadFile("../../docs/hosting.md")
	if err != nil {
		t.Fatalf("read hosting.md: %v", err)
	}
	re := regexp.MustCompile(`(?m)^\|\s*` + "`" + `(DIALLAK_[A-Z0-9_]+)` + "`" + `\s*\|\s*([^|]+)\s*\|\s*([^|]+?)\s*\|\s*$`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatalf("no rows found matching deployment contract table in docs/hosting.md")
	}
	rows := make([]deployContractRow, 0, len(matches))
	for _, m := range matches {
		rows = append(rows, deployContractRow{
			variable: m[1],
			kind:     strings.TrimSpace(m[2]),
			needed:   strings.TrimSpace(m[3]),
		})
	}
	return rows
}

// loadCodeVariables scans non-test .go files under internal/config and cmd/diallak for DIALLAK_* literals.
func loadCodeVariables(t *testing.T) map[string]bool {
	t.Helper()
	codeVars := make(map[string]bool)
	dirs := []string{"../../internal/config", "../../cmd/diallak"}
	for _, d := range dirs {
		if err := scanGoDir(d, codeVars); err != nil {
			t.Fatalf("scan %s: %v", d, err)
		}
	}
	return codeVars
}

// scanGoDir walks a directory and records quoted DIALLAK_* string literals from non-test Go source files.
func scanGoDir(dir string, codeVars map[string]bool) error {
	varPattern := regexp.MustCompile(`"(DIALLAK_[A-Z0-9_]+)"`)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		matches := varPattern.FindAllStringSubmatch(string(content), -1)
		for _, m := range matches {
			codeVars[m[1]] = true
		}
		return nil
	})
}

// expectedContractKind returns the pod projection kind required for a given configuration variable.
func expectedContractKind(varName string) string {
	switch {
	case varName == "DIALLAK_CORPUS_DIR":
		return "read-only volume"
	case varName == "DIALLAK_NEWS_STATE_FILE":
		return "writable volume"
	case strings.HasSuffix(varName, "_FILE"):
		return "secret file"
	default:
		return "environment"
	}
}

// TestDeployContract_Positive_TableNamesEveryVariable verifies set equality between the contract table
// and every DIALLAK_* variable read in internal/config and cmd/diallak.
func TestDeployContract_Positive_TableNamesEveryVariable(t *testing.T) {
	rows := loadContractRows(t)
	tableVars := make(map[string]bool)
	for _, r := range rows {
		tableVars[r.variable] = true
	}
	codeVars := loadCodeVariables(t)

	var missing []string
	for v := range codeVars {
		if !tableVars[v] {
			missing = append(missing, v)
		}
	}
	sort.Strings(missing)

	var extra []string
	for v := range tableVars {
		if !codeVars[v] {
			extra = append(extra, v)
		}
	}
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("deployment contract table missing variables from code: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("deployment contract table has extra variables not in code: %v", extra)
	}
}

// TestDeployContract_Negative_SecretFilesAreSecretFiles ensures credentials arrive as secret files,
// volumes are correctly categorized, and plain settings arrive via environment.
func TestDeployContract_Negative_SecretFilesAreSecretFiles(t *testing.T) {
	rows := loadContractRows(t)
	for _, r := range rows {
		want := expectedContractKind(r.variable)
		if r.kind != want {
			t.Errorf("variable %s has kind %q, want %q", r.variable, r.kind, want)
		}
	}
}

// TestDeployContract_Boundary_KindsAreFromTheList ensures all rows use allowed kind values and no
// variable appears more than once.
func TestDeployContract_Boundary_KindsAreFromTheList(t *testing.T) {
	rows := loadContractRows(t)
	validKinds := map[string]bool{
		"secret file":      true,
		"environment":      true,
		"read-only volume": true,
		"writable volume":  true,
	}
	seen := make(map[string]bool)
	for _, r := range rows {
		if !validKinds[r.kind] {
			t.Errorf("variable %s has unknown kind %q", r.variable, r.kind)
		}
		if seen[r.variable] {
			t.Errorf("table has duplicate variable: %s", r.variable)
		}
		seen[r.variable] = true
	}
}
