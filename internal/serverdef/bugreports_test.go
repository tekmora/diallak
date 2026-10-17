package serverdef

import (
	"slices"
	"testing"
)

func TestBugReports_Positive_ShippedDefinition(t *testing.T) {
	def := valid(t)
	if def.BugReports == nil {
		t.Fatal("the committed definition holds no bug_reports block")
	}
	if def.BugReports.Repository != "tekmora/diallak" {
		t.Errorf("bug_reports.repository = %q, want tekmora/diallak", def.BugReports.Repository)
	}
	for _, f := range Validate(def) {
		if f.Code == CodeBugReports {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
}

func TestBugReports_Negative_InvalidRepository(t *testing.T) {
	def := valid(t)
	def.BugReports = &BugReports{Repository: "not a repo"}
	findings := Validate(def)
	idx := slices.IndexFunc(findings, func(f Finding) bool {
		return f.Code == CodeBugReports && f.Path == "bug_reports.repository"
	})
	if idx == -1 {
		t.Fatalf("expected finding CodeBugReports at bug_reports.repository, got %+v", findings)
	}
}
