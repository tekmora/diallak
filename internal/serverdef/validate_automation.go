package serverdef

import (
	"fmt"
	"path/filepath"
	"strings"
)

func isInvalidRecordPath(record string) bool {
	return record == "" || filepath.IsAbs(record) || strings.HasPrefix(record, "/") || strings.Contains(record, "..")
}

// checkObservedAutomation validates that observed automations are exclusively performed
// by the platform and do not declare an audit decision record.
func checkObservedAutomation(v *validator, auto Automation, p string) {
	if auto.State != "observed" {
		return
	}
	if auto.By != "platform" {
		v.add(CodeAutomationRecord, p+".state",
			"only platform automation can have observed state")
	}
	if auto.Record != "" {
		v.add(CodeAutomationRecord, p+".record",
			"observed automation cannot have a record")
	}
}

func checkSingleAutomation(v *validator, auto Automation) {
	p := fmt.Sprintf("automation[%s]", auto.ID)
	if (auto.By == "bot" || auto.By == "platform") && (auto.State == "built" || auto.State == "decided") {
		if isInvalidRecordPath(auto.Record) {
			v.add(CodeAutomationRecord, p+".record",
				"automation record must be a relative path without '..'")
		}
	}
	if auto.By == "bot" && auto.State == "planned" && !auto.NeedsRecord {
		v.add(CodeAutomationRecord, p+".needs_record",
			"planned bot automation must set needs_record to true")
	}
	checkObservedAutomation(v, auto, p)
}

func (v *validator) checkAutomationRecords() {
	for _, auto := range v.def.Automation {
		checkSingleAutomation(v, auto)
	}
}
