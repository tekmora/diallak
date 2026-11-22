package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	minWorkflowYAMLFiles = 1
	maxWorkflowYAMLFiles = 64
)

var workflowRestatedVersionRE = regexp.MustCompile(
	`\b(gosec|govulncheck|x/tools|x/vuln) v[0-9]+\.[0-9]+`,
)

type workflowVersionFinding struct {
	Line int
	Text string
}

func workflowVersionFindings(fileText string) []workflowVersionFinding {
	lines := strings.Split(fileText, "\n")
	out := make([]workflowVersionFinding, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		locs := workflowRestatedVersionRE.FindAllStringIndex(line, -1)
		for j := 0; j < len(locs); j++ {
			out = append(out, workflowVersionFinding{
				Line: i + 1,
				Text: line[locs[j][0]:locs[j][1]],
			})
		}
	}
	return out
}

var toolsModFloorRE = regexp.MustCompile(
	`must stay at (v[0-9]+\.[0-9]+\.[0-9]+) or newer`,
)

var toolsModSelectedRE = regexp.MustCompile(
	`(?m)^\s*(?:require\s+)?golang\.org/x/tools (v[0-9]+\.[0-9]+\.[0-9]+)`,
)

type semverTriple struct {
	major int
	minor int
	patch int
}

func parseSemverTriple(v string) (semverTriple, error) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return semverTriple{}, fmt.Errorf("version %q is not MAJOR.MINOR.PATCH", v)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semverTriple{}, fmt.Errorf("major: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semverTriple{}, fmt.Errorf("minor: %w", err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semverTriple{}, fmt.Errorf("patch: %w", err)
	}
	return semverTriple{major: major, minor: minor, patch: patch}, nil
}

func semverAtLeast(selected, floor semverTriple) bool {
	if selected.major != floor.major {
		return selected.major > floor.major
	}
	if selected.minor != floor.minor {
		return selected.minor > floor.minor
	}
	return selected.patch >= floor.patch
}

func toolsModToolsFloorAndSelected(goModText string) (floor, selected semverTriple, err error) {
	floorMatch := toolsModFloorRE.FindStringSubmatch(goModText)
	if floorMatch == nil {
		return semverTriple{}, semverTriple{}, fmt.Errorf("tools go.mod header has no x/tools floor sentence")
	}
	floor, err = parseSemverTriple(floorMatch[1])
	if err != nil {
		return semverTriple{}, semverTriple{}, err
	}
	selectedMatch := toolsModSelectedRE.FindStringSubmatch(goModText)
	if selectedMatch == nil {
		return semverTriple{}, semverTriple{}, fmt.Errorf("tools go.mod has no golang.org/x/tools require line")
	}
	selected, err = parseSemverTriple(selectedMatch[1])
	if err != nil {
		return semverTriple{}, semverTriple{}, err
	}
	return floor, selected, nil
}

func TestWorkflowFiles_Negative_NoRestatedScannerVersions(t *testing.T) {
	matches, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil {
		t.Fatalf("glob yml: %v", err)
	}
	yamlMatches, err := filepath.Glob("../../.github/workflows/*.yaml")
	if err != nil {
		t.Fatalf("glob yaml: %v", err)
	}
	matches = append(matches, yamlMatches...)
	if len(matches) < minWorkflowYAMLFiles {
		t.Fatalf("workflow file count %d is below %d under .github/workflows (*.yml, *.yaml)",
			len(matches), minWorkflowYAMLFiles)
	}
	if len(matches) > maxWorkflowYAMLFiles {
		t.Fatalf("workflow file count %d exceeds bound %d", len(matches), maxWorkflowYAMLFiles)
	}
	for i := 0; i < len(matches); i++ {
		path := matches[i]
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		findings := workflowVersionFindings(string(body))
		if len(findings) > 0 {
			t.Fatalf("%s restates a scanner/tools version at line %d: %q", path, findings[0].Line, findings[0].Text)
		}
	}
}

func TestToolsGoMod_Positive_SelectedToolsAtOrAboveFloor(t *testing.T) {
	body, err := os.ReadFile("../../tools/go/go.mod")
	if err != nil {
		t.Fatalf("read tools/go/go.mod: %v", err)
	}
	floor, selected, err := toolsModToolsFloorAndSelected(string(body))
	if err != nil {
		t.Fatalf("parse tools/go/go.mod: %v", err)
	}
	if !semverAtLeast(selected, floor) {
		t.Fatalf("selected %d.%d.%d is below floor %d.%d.%d",
			selected.major, selected.minor, selected.patch,
			floor.major, floor.minor, floor.patch)
	}
}

func assertWorkflowFindingCount(t *testing.T, text string, wantLen int) {
	t.Helper()
	got := workflowVersionFindings(text)
	if len(got) != wantLen {
		t.Fatalf("findings len = %d, want %d (got %v)", len(got), wantLen, got)
	}
}

func assertToolsModFloorSelected(t *testing.T, goMod string, wantErr, wantPass bool) {
	t.Helper()
	floor, selected, err := toolsModToolsFloorAndSelected(goMod)
	if wantErr {
		if err == nil {
			t.Fatal("expected parse error, got nil")
		}
		return
	}
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	pass := semverAtLeast(selected, floor)
	if pass != wantPass {
		t.Fatalf("selected %d.%d.%d vs floor %d.%d.%d: pass=%v want %v",
			selected.major, selected.minor, selected.patch,
			floor.major, floor.minor, floor.patch, pass, wantPass)
	}
}

func TestWorkflowVersionFindings_Table(t *testing.T) {
	// The comment security.yml carried before this check, verbatim: three
	// restated versions (gosec, x/tools twice).
	const oldSecurityComment = `      # The scanners come from the pinned tools module, not from ` + "`@latest`" + `:
      # gosec v2.29.0 built with its own golang.org/x/tools v0.49.0 cannot read
      # the export data the Go 1.27.2 compiler writes and fails on every
      # package. tools/go/go.mod selects x/tools v0.50.0 for both scanners.`
	const newSecurityComment = `# The scanners come from the pinned tools module, not from @latest:
# tools/go/go.mod selects the golang.org/x/tools version both use, and
# its header documents which floor that version keeps and why.`

	t.Run("old_security_yml_comment", func(t *testing.T) {
		assertWorkflowFindingCount(t, oldSecurityComment, 3)
	})
	t.Run("new_security_yml_comment", func(t *testing.T) {
		assertWorkflowFindingCount(t, newSecurityComment, 0)
	})
}

func TestToolsModFloorSelected_Table(t *testing.T) {
	const header = "// golang.org/x/tools must stay at v0.50.0 or newer: reason.\n"
	cases := []struct {
		name     string
		goMod    string
		wantErr  bool
		wantPass bool
	}{
		{
			name:     "floor_boundary_equal",
			goMod:    header + "require (\n\tgolang.org/x/tools v0.50.0 // indirect\n)\n",
			wantPass: true,
		},
		{
			name:     "floor_below_selected_fail",
			goMod:    header + "require (\n\tgolang.org/x/tools v0.49.9 // indirect\n)\n",
			wantPass: false,
		},
		{
			name:     "floor_above_selected_pass",
			goMod:    header + "require (\n\tgolang.org/x/tools v0.53.0 // indirect\n)\n",
			wantPass: true,
		},
		{
			name:    "header_missing_floor",
			goMod:   "module tools\nrequire golang.org/x/tools v0.53.0 // indirect\n",
			wantErr: true,
		},
		{
			name:    "missing_require_line",
			goMod:   header + "module tools\n",
			wantErr: true,
		},
	}
	for i := 0; i < len(cases); i++ {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			assertToolsModFloorSelected(t, tc.goMod, tc.wantErr, tc.wantPass)
		})
	}
}
