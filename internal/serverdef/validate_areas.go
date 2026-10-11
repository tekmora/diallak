package serverdef

import (
	"fmt"
	"strings"
)

func (v *validator) checkAreaRules() {
	for _, a := range v.def.Areas {
		p := fmt.Sprintf("areas[%s]", a.ID)
		if a.Opening == nil {
			v.add(CodeAreaOpeningRule, p+".opening", "area missing opening rule")
		}
		if a.Closing == nil {
			v.add(CodeAreaClosingRule, p+".closing", "area missing closing rule")
		}
	}
}

func (v *validator) checkSingleRuleShape(path string, rule *Rule) {
	if rule == nil {
		return
	}
	if rule.Value < MinRuleValue {
		v.add(CodeRuleShape, path+".value", fmt.Sprintf("rule value %d must be at least 1", rule.Value))
	}
	if rule.WindowDays < MinWindowDays || rule.WindowDays > MaxWindowDays {
		v.add(CodeRuleShape, path+".window_days",
			fmt.Sprintf("rule window_days %d must be between 1 and 365", rule.WindowDays))
	}
	if rule.Metric == "members-with-role" {
		if r := findRole(v.def.Roles, rule.Subject); r == nil {
			v.add(CodeRuleShape, path+".subject",
				fmt.Sprintf("metric members-with-role subject %q is not a role", rule.Subject))
		}
	}
	if rule.Metric == "days-without-message" {
		if ch := findChannel(v.def.Channels, rule.Subject); ch == nil {
			v.add(CodeRuleShape, path+".subject",
				fmt.Sprintf("metric days-without-message subject %q is not a channel", rule.Subject))
		}
	}
}

func (v *validator) checkRuleShape() {
	for _, a := range v.def.Areas {
		p := fmt.Sprintf("areas[%s]", a.ID)
		v.checkSingleRuleShape(p+".opening", a.Opening)
		v.checkSingleRuleShape(p+".closing", a.Closing)
	}
}

func (v *validator) checkAreaChannels() {
	areaChannels := make(map[string][]Channel)
	for _, ch := range v.def.Channels {
		if ch.Area != "" {
			areaChannels[ch.Area] = append(areaChannels[ch.Area], ch)
		}
		if ch.Opens == "threshold" && ch.Area == "" {
			v.add(CodeAreaChannel, fmt.Sprintf("channels[%s].area", ch.ID),
				"channel with opens threshold must name an area")
		}
	}

	for _, a := range v.def.Areas {
		p := fmt.Sprintf("areas[%s]", a.ID)
		channels, ok := areaChannels[a.ID]
		if !ok || len(channels) == 0 {
			v.add(CodeAreaChannel, p, fmt.Sprintf("area %s is not named by any channel", a.ID))
			continue
		}
		for _, ch := range channels {
			v.checkAreaChannelDetails(a, ch)
		}
	}
}

func (v *validator) checkAreaChannelDetails(a Area, ch Channel) {
	chPath := fmt.Sprintf("channels[%s]", ch.ID)
	if ch.Opens != "threshold" {
		v.add(CodeAreaChannel, chPath+".opens", "area channel must have opens threshold")
	}
	effEveryone := Effective(v.def, "everyone", ch.ID)
	if _, ok := effEveryone["VIEW_CHANNEL"]; ok {
		v.add(CodeAreaChannel, fmt.Sprintf("%s.access[everyone]", chPath),
			"everyone must not have effective VIEW_CHANNEL in area channel")
	}
	effAreaRole := Effective(v.def, a.Role, ch.ID)
	if _, ok := effAreaRole["VIEW_CHANNEL"]; !ok {
		v.add(CodeAreaChannel, fmt.Sprintf("%s.access[%s]", chPath, a.Role),
			"area role must have effective VIEW_CHANNEL in area channel")
	}
}

func (v *validator) checkAreaConditions() {
	for _, a := range v.def.Areas {
		if a.Kind != "language" {
			continue
		}
		p := fmt.Sprintf("areas[%s].conditions", a.ID)
		if len(a.Conditions) == 0 {
			v.add(CodeAreaCondition, p, "language area must have at least one condition")
			continue
		}
		hasNonEmpty := false
		for _, cond := range a.Conditions {
			if strings.TrimSpace(cond) != "" {
				hasNonEmpty = true
				break
			}
		}
		if !hasNonEmpty {
			v.add(CodeAreaCondition, p, "language area conditions are all empty")
		}
	}
}
