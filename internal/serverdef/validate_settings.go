package serverdef

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	verificationLevels = []string{"none", "low", "medium", "high", "very_high"}
	filterTriggers     = []string{"keyword", "spam", "keyword_preset", "mention_spam", "member_profile"}
	filterPresets      = []string{"profanity", "sexual_content", "slurs"}
	filterActionTypes  = []string{"block_message", "alert", "timeout", "block_interaction"}
	settingsBases      = []string{"decided", "proposal"}
)

// settingsRefs holds what a filter rule may name: roles and channels by id.
type settingsRefs struct {
	roles      map[string]struct{}
	channelIDs map[string]struct{}
	channels   map[string]Channel
}

func (v *validator) checkSettings() {
	s := v.def.Settings
	if s == nil {
		return
	}
	checkVerification(v, s.Verification)

	ids := make([]string, 0, len(s.FilterRules))
	names := make(map[string]int, len(s.FilterRules))
	for _, rule := range s.FilterRules {
		ids = append(ids, rule.ID)
		names[rule.Name]++
	}
	checkItemIDs(v, "settings.filter_rules", ids)
	checkFilterRulesPerServer(v, s.FilterRules)

	refs := settingsRefs{
		roles:      make(map[string]struct{}, len(v.def.Roles)),
		channelIDs: make(map[string]struct{}, len(v.def.Channels)),
		channels:   make(map[string]Channel, len(v.def.Channels)),
	}
	for _, r := range v.def.Roles {
		refs.roles[r.ID] = struct{}{}
	}
	for _, ch := range v.def.Channels {
		refs.channelIDs[ch.ID] = struct{}{}
		refs.channels[ch.ID] = ch
	}
	for _, rule := range s.FilterRules {
		p := fmt.Sprintf("settings.filter_rules[%s]", rule.ID)
		if names[rule.Name] > 1 {
			v.add(CodeFilterRule, p+".name", fmt.Sprintf("rule name %q is used twice", rule.Name))
		}
		checkFilterRuleTexts(v, p, rule)
		checkFilterKeywords(v, p, rule)
		checkFilterAllowList(v, p, rule)
		checkFilterPresets(v, p, rule)
		checkFilterMentions(v, p, rule)
		checkFilterActions(v, p, rule, refs.channels)
		checkExemptList(v, p+".exempt_roles", "roles", rule.ExemptRoles, MaxExemptRoles, refs.roles)
		checkExemptList(v, p+".exempt_channels", "channels", rule.ExemptChannels, MaxExemptChannels, refs.channelIDs)
	}
}

func checkVerification(v *validator, ver *Verification) {
	if ver == nil {
		return
	}
	if !inSet(ver.Level, verificationLevels) {
		v.add(CodeEnum, "settings.verification.level", fmt.Sprintf("invalid verification level %q", ver.Level))
	}
	if !inSet(ver.Basis, settingsBases) {
		v.add(CodeEnum, "settings.verification.basis", fmt.Sprintf("invalid basis %q", ver.Basis))
	}
	if strings.TrimSpace(ver.Reasoning) == "" {
		v.add(CodeText, "settings.verification.reasoning", "verification reasoning must not be empty")
	}
}

func checkFilterRulesPerServer(v *validator, rules []FilterRule) {
	counts := make(map[string]int, len(filterRulesPerServer))
	for _, rule := range rules {
		counts[rule.Trigger]++
	}
	for _, trigger := range filterTriggers {
		if limit := filterRulesPerServer[trigger]; counts[trigger] > limit {
			v.add(CodeFilterRule, "settings.filter_rules",
				fmt.Sprintf("a server holds at most %d %s rules, found %d", limit, trigger, counts[trigger]))
		}
	}
}

func checkFilterRuleTexts(v *validator, p string, rule FilterRule) {
	if strings.TrimSpace(rule.Name) == "" {
		v.add(CodeText, p+".name", "filter rule name must not be empty")
	}
	if strings.TrimSpace(rule.Purpose) == "" {
		v.add(CodeText, p+".purpose", "filter rule purpose must not be empty")
	}
	if strings.TrimSpace(rule.Reasoning) == "" {
		v.add(CodeText, p+".reasoning", "filter rule reasoning must not be empty")
	}
	if !inSet(rule.Trigger, filterTriggers) {
		v.add(CodeEnum, p+".trigger", fmt.Sprintf("invalid filter trigger %q", rule.Trigger))
	}
	if !inSet(rule.Basis, settingsBases) {
		v.add(CodeEnum, p+".basis", fmt.Sprintf("invalid basis %q", rule.Basis))
	}
}

func checkFilterKeywords(v *validator, p string, rule FilterRule) {
	if rule.Trigger != "keyword" && rule.Trigger != "member_profile" {
		if len(rule.Keywords) > 0 {
			v.add(CodeFilterRule, p+".keywords", "only a keyword or member_profile rule reads keywords")
		}
		if len(rule.Patterns) > 0 {
			v.add(CodeFilterRule, p+".patterns", "only a keyword or member_profile rule reads patterns")
		}
		return
	}
	if len(rule.Keywords) == 0 && len(rule.Patterns) == 0 {
		v.add(CodeFilterRule, p+".keywords", "a keyword or member_profile rule needs keywords or patterns")
	}
	checkStringList(v, p+".keywords", "keywords", rule.Keywords, MaxKeywords, MaxKeywordRunes)
	checkStringList(v, p+".patterns", "patterns", rule.Patterns, MaxPatterns, MaxPatternRunes)
}

func checkFilterAllowList(v *validator, p string, rule FilterRule) {
	switch rule.Trigger {
	case "keyword", "member_profile":
		checkStringList(v, p+".allow_list", "allow-list entries", rule.AllowList, MaxAllowList, MaxKeywordRunes)
	case "keyword_preset":
		checkStringList(v, p+".allow_list", "allow-list entries", rule.AllowList, MaxPresetAllowList, MaxKeywordRunes)
	default:
		if len(rule.AllowList) > 0 {
			v.add(CodeFilterRule, p+".allow_list", "only a keyword, keyword_preset or member_profile rule reads an allow list")
		}
	}
}

// checkStringList refuses a list longer than maxLen and an entry that is blank or longer than maxRunes.
func checkStringList(v *validator, path, what string, list []string, maxLen, maxRunes int) {
	if len(list) > maxLen {
		v.add(CodeFilterRule, path, fmt.Sprintf("a rule holds at most %d %s, found %d", maxLen, what, len(list)))
	}
	for _, entry := range list {
		if strings.TrimSpace(entry) == "" || utf8.RuneCountInString(entry) > maxRunes {
			v.add(CodeFilterRule, path, fmt.Sprintf("each of the %s has between 1 and %d characters", what, maxRunes))
			return
		}
	}
}

func checkFilterPresets(v *validator, p string, rule FilterRule) {
	if rule.Trigger != "keyword_preset" {
		if len(rule.Presets) > 0 {
			v.add(CodeFilterRule, p+".presets", "only a keyword_preset rule reads presets")
		}
		return
	}
	if len(rule.Presets) == 0 {
		v.add(CodeFilterRule, p+".presets", "a keyword_preset rule needs at least one preset")
	}
	seen := make(map[string]struct{}, len(rule.Presets))
	for _, preset := range rule.Presets {
		if !inSet(preset, filterPresets) {
			v.add(CodeEnum, p+".presets", fmt.Sprintf("invalid preset %q", preset))
		}
		if _, ok := seen[preset]; ok {
			v.add(CodeFilterRule, p+".presets", fmt.Sprintf("preset %q is listed twice", preset))
		}
		seen[preset] = struct{}{}
	}
}

func checkFilterMentions(v *validator, p string, rule FilterRule) {
	if rule.Trigger != "mention_spam" {
		if rule.MentionLimit != 0 {
			v.add(CodeFilterRule, p+".mention_limit", "only a mention_spam rule reads a mention limit")
		}
		if rule.RaidProtection {
			v.add(CodeFilterRule, p+".raid_protection", "only a mention_spam rule reads raid protection")
		}
		return
	}
	if rule.MentionLimit < 1 || rule.MentionLimit > MaxMentionLimit {
		v.add(CodeFilterRule, p+".mention_limit",
			fmt.Sprintf("a mention limit lies between 1 and %d, found %d", MaxMentionLimit, rule.MentionLimit))
	}
}

func checkFilterActions(v *validator, p string, rule FilterRule, channels map[string]Channel) {
	if len(rule.Actions) == 0 {
		v.add(CodeFilterRule, p+".actions", "a filter rule needs at least one action")
	}
	seen := make(map[string]struct{}, len(rule.Actions))
	for _, action := range rule.Actions {
		ap := fmt.Sprintf("%s.actions[%s]", p, action.Type)
		if !inSet(action.Type, filterActionTypes) {
			v.add(CodeEnum, ap, fmt.Sprintf("invalid action type %q", action.Type))
		}
		if _, ok := seen[action.Type]; ok {
			v.add(CodeFilterRule, ap, fmt.Sprintf("action %q is listed twice", action.Type))
		}
		seen[action.Type] = struct{}{}
		checkAlertAction(v, ap, action, channels)
		checkTimeoutAction(v, ap, rule.Trigger, action)
		checkBlockAction(v, ap, action)
	}
}

func checkAlertAction(v *validator, ap string, action FilterAction, channels map[string]Channel) {
	if action.Type != "alert" {
		if action.Channel != "" {
			v.add(CodeFilterRule, ap+".channel", "only an alert action names a channel")
		}
		return
	}
	if action.Channel == "" {
		v.add(CodeFilterRule, ap+".channel", "an alert needs the channel it is written to")
		return
	}
	ch, ok := channels[action.Channel]
	if !ok {
		v.add(CodeRef, ap+".channel", fmt.Sprintf("unknown channel %q", action.Channel))
		return
	}
	if !ch.Staff {
		v.add(CodeFilterRule, ap+".channel",
			fmt.Sprintf("an alert carries the flagged message and goes to a staff channel only; %q is not one", ch.ID))
	}
}

func checkTimeoutAction(v *validator, ap, trigger string, action FilterAction) {
	if action.Type != "timeout" {
		if action.TimeoutSeconds != 0 {
			v.add(CodeFilterRule, ap+".timeout_seconds", "only a timeout action has a length")
		}
		return
	}
	if trigger != "keyword" && trigger != "mention_spam" {
		v.add(CodeFilterRule, ap, "the platform sets a timeout on a keyword or mention_spam rule only")
	}
	if action.TimeoutSeconds < 1 || action.TimeoutSeconds > MaxTimeoutSeconds {
		v.add(CodeFilterRule, ap+".timeout_seconds",
			fmt.Sprintf("a timeout lasts between 1 and %d seconds, found %d", MaxTimeoutSeconds, action.TimeoutSeconds))
	}
}

func checkBlockAction(v *validator, ap string, action FilterAction) {
	if action.Type != "block_message" {
		if action.Message != "" {
			v.add(CodeFilterRule, ap+".message", "only a block_message action shows a message")
		}
		return
	}
	if utf8.RuneCountInString(action.Message) > MaxBlockMessageRunes {
		v.add(CodeFilterRule, ap+".message",
			fmt.Sprintf("the message a blocked member is shown has at most %d characters", MaxBlockMessageRunes))
	}
}

// checkExemptList refuses more than maxLen exemptions, an exemption listed twice and one that names nothing.
func checkExemptList(v *validator, path, what string, ids []string, maxLen int, known map[string]struct{}) {
	if len(ids) > maxLen {
		v.add(CodeFilterRule, path, fmt.Sprintf("a rule exempts at most %d %s, found %d", maxLen, what, len(ids)))
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		ep := fmt.Sprintf("%s[%s]", path, id)
		if _, ok := known[id]; !ok {
			v.add(CodeRef, ep, fmt.Sprintf("unknown exempt %s entry %q", what, id))
		}
		if _, ok := seen[id]; ok {
			v.add(CodeFilterRule, ep, fmt.Sprintf("%q is listed twice", id))
		}
		seen[id] = struct{}{}
	}
}
