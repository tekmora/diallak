package serverplan

import (
	"fmt"
	"slices"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	// ActionSetVerificationLevel sets the server's verification level.
	ActionSetVerificationLevel = "set-verification-level"
	// ActionCreateFilterRule creates a missing filter rule.
	ActionCreateFilterRule = "create-filter-rule"
	// ActionSetFilterRule changes the fields of a filter rule.
	ActionSetFilterRule = "set-filter-rule"
	// ActionConvertFilterRule stands for a rule with another trigger; always refused.
	ActionConvertFilterRule = "convert-filter-rule"

	// ReasonTriggerDiffers refuses a rule whose trigger differs: the platform does not change a trigger.
	ReasonTriggerDiffers = "trigger-differs"
)

// addSettingsCandidates plans the verification level and the filter rules. It
// runs last: a rule's alert goes to a channel the plan may create before it.
func (ctx *planContext) addSettingsCandidates() {
	if ctx.def.Settings == nil {
		return
	}
	for _, d := range ctx.diffs {
		switch d.Kind {
		case KindVerificationLevel, KindVerificationNotRead:
			ctx.evaluateCandidate(candidate{
				action:       ActionSetVerificationLevel,
				object:       d.Object,
				needs:        []string{"MANAGE_GUILD"},
				detail:       d.Detail,
				isNotVisible: d.Kind == KindVerificationNotRead,
			})
		case KindFilterRulesNotRead:
			for i := range ctx.def.Settings.FilterRules {
				r := &ctx.def.Settings.FilterRules[i]
				ctx.evaluateCandidate(ctx.filterRuleCandidate(ActionSetFilterRule, r.ID, ""))
			}
		case KindFilterRuleMissing, KindFilterRuleDiffers, KindFilterRuleTrigger, KindFilterRuleAmbiguous:
			ctx.addFilterRuleCandidate(d)
		}
	}
}

func (ctx *planContext) addFilterRuleCandidate(d Difference) {
	id := extractBracketID(d.Object)
	switch d.Kind {
	case KindFilterRuleMissing:
		c := ctx.filterRuleCandidate(ActionCreateFilterRule, id, "")
		if r := ctx.defFilterRule(id); r != nil {
			c.detail = r.Name
		}
		ctx.evaluateCandidate(c)
	case KindFilterRuleDiffers:
		ctx.evaluateCandidate(ctx.filterRuleCandidate(ActionSetFilterRule, id, d.Detail))
	case KindFilterRuleTrigger:
		ctx.evaluateCandidate(candidate{action: ActionConvertFilterRule, object: d.Object, detail: d.Detail})
	default:
		ctx.evaluateCandidate(candidate{action: ActionResolveName, object: d.Object})
	}
}

// filterRuleCandidate builds a change of a rule; a rule that cannot be read is
// not visible.
func (ctx *planContext) filterRuleCandidate(action, id, detail string) candidate {
	needs := []string{"MANAGE_GUILD"}
	if r := ctx.defFilterRule(id); r != nil && slices.ContainsFunc(r.Actions, func(a serverdef.FilterAction) bool {
		return a.Type == "timeout"
	}) {
		needs = append(needs, "MODERATE_MEMBERS")
	}
	return candidate{
		action:       action,
		object:       filterRuleObject(id),
		needs:        needs,
		detail:       detail,
		ruleID:       id,
		isNotVisible: !ctx.snap.FilterRulesRead,
	}
}

func (ctx *planContext) defFilterRule(id string) *serverdef.FilterRule {
	for i := range ctx.def.Settings.FilterRules {
		if ctx.def.Settings.FilterRules[i].ID == id {
			return &ctx.def.Settings.FilterRules[i]
		}
	}
	return nil
}

// ruleNamesAmbiguous reports whether a rule names a role or a channel the
// server holds more than once: its alert channel or an exemption.
func (ctx *planContext) ruleNamesAmbiguous(id string) bool {
	r := ctx.defFilterRule(id)
	if r == nil {
		return false
	}
	for _, a := range r.Actions {
		if a.Channel != "" && ctx.isAmbiguous(fmt.Sprintf("channels[%s]", a.Channel)) {
			return true
		}
	}
	for _, roleID := range r.ExemptRoles {
		if ctx.isAmbiguous(fmt.Sprintf("roles[%s]", roleID)) {
			return true
		}
	}
	for _, chID := range r.ExemptChannels {
		if ctx.isAmbiguous(fmt.Sprintf("channels[%s]", chID)) {
			return true
		}
	}
	return false
}

// isSettingsAction reports whether the bot may plan the action with its own
// rights: it holds MANAGE_GUILD for the settings (ADR-0015).
func isSettingsAction(action string) bool {
	return action == ActionSetVerificationLevel || action == ActionCreateFilterRule || action == ActionSetFilterRule
}
