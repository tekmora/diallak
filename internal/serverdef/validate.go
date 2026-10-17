package serverdef

import "sort"

type validator struct {
	def      *Definition
	findings []Finding
}

func (v *validator) add(code, path, msg string) {
	v.findings = append(v.findings, Finding{
		Code:    code,
		Path:    path,
		Message: msg,
	})
}

// Validate executes all declarative validation rules against def.
// Findings are returned sorted by Path then Code, capped at MaxFindings.
func Validate(def *Definition) []Finding {
	if def == nil {
		return []Finding{}
	}

	v := &validator{def: def, findings: make([]Finding, 0, 32)}
	v.checkIDs()
	v.checkTexts()
	v.checkEnums()
	v.checkVersion()
	v.checkSingletons()
	v.checkReferences()
	v.checkChannelOwner()
	v.checkPermissions()
	v.checkAccessShape()
	v.checkMentionEveryone()
	v.checkOperatorOnly()
	v.checkModerationOnly()
	v.checkBotRights()
	v.checkTrail()
	v.checkNothingAttaches()
	v.checkRoleGrants()
	v.checkRoleOrder()
	v.checkChoiceLimits()
	v.checkTicketThreadReader()
	v.checkTicketShape()
	v.checkStaffVisibility()
	v.checkModeratorVisibility()
	v.checkFirstAnswer()
	v.checkVoiceClosed()
	v.checkAreaRules()
	v.checkRuleShape()
	v.checkAreaChannels()
	v.checkAreaConditions()
	v.checkAutomationRecords()
	v.checkSettings()

	sort.Slice(v.findings, func(i, j int) bool {
		if v.findings[i].Path != v.findings[j].Path {
			return v.findings[i].Path < v.findings[j].Path
		}
		return v.findings[i].Code < v.findings[j].Code
	})

	if len(v.findings) > MaxFindings {
		v.findings = v.findings[:MaxFindings]
	}
	return v.findings
}
