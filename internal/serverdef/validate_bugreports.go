package serverdef

import "fmt"

func (v *validator) checkBugReports() {
	if v.def.BugReports == nil {
		return
	}
	repo := v.def.BugReports.Repository
	if !ValidRepository(repo) {
		v.add(CodeBugReports, "bug_reports.repository", fmt.Sprintf("repository %q is not owner/name", repo))
	}
}
