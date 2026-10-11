package serverdef

import (
	"fmt"
	"path/filepath"
	"strings"
)

func isInvalidRecordPath(record string) bool {
	return record == "" || filepath.IsAbs(record) || strings.HasPrefix(record, "/") || strings.Contains(record, "..")
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
}

func (v *validator) checkAutomationRecords() {
	for _, auto := range v.def.Automation {
		checkSingleAutomation(v, auto)
	}
}
