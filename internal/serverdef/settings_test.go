package serverdef

import (
	"strings"
	"testing"
)

// The tests in this file pin the conditions of the filter rules and the verification level. The
// limits are Discord's: developers/resources/auto-moderation.mdx at commit 37148620d232.

func filterRuleOf(t *testing.T, def *Definition, id string) *FilterRule {
	t.Helper()
	if def.Settings == nil {
		t.Fatalf("definition has no settings")
	}
	for i := range def.Settings.FilterRules {
		if def.Settings.FilterRules[i].ID == id {
			return &def.Settings.FilterRules[i]
		}
	}
	t.Fatalf("definition has no filter rule %q", id)
	return nil
}

func actionOf(t *testing.T, rule *FilterRule, kind string) *FilterAction {
	t.Helper()
	for i := range rule.Actions {
		if rule.Actions[i].Type == kind {
			return &rule.Actions[i]
		}
	}
	t.Fatalf("filter rule %q has no action %q", rule.ID, kind)
	return nil
}

func repeated(entry string, count int) []string {
	list := make([]string, count)
	for i := range list {
		list[i] = entry
	}
	return list
}

// keywordRule turns the spam rule into a keyword rule with one keyword.
func keywordRule(t *testing.T, def *Definition) *FilterRule {
	t.Helper()
	rule := filterRuleOf(t, def, "spam")
	rule.Trigger = "keyword"
	rule.Keywords = []string{"buy-followers"}
	return rule
}

func TestValidate_Positive_SettingsProposal(t *testing.T) {
	def := valid(t)
	if def.Settings == nil || def.Settings.Verification == nil {
		t.Fatalf("the committed definition holds no settings proposal")
	}
	if len(def.Settings.FilterRules) == 0 {
		t.Fatalf("the committed definition holds no filter rule")
	}
	for _, rule := range def.Settings.FilterRules {
		if rule.Basis != "proposal" {
			t.Errorf("filter rule %s has basis %q; the rule data is a proposal until the operator read it", rule.ID, rule.Basis)
		}
		if !strings.HasPrefix(rule.Reasoning, "Chosen, not measured.") {
			t.Errorf("filter rule %s does not say that its values are chosen, not measured", rule.ID)
		}
	}
	if def.Settings.Verification.Basis != "proposal" {
		t.Errorf("verification level has basis %q, want proposal", def.Settings.Verification.Basis)
	}
}

func TestValidate_Positive_NoSettingsNothingCompared(t *testing.T) {
	def := valid(t)
	def.Settings = nil
	for _, f := range Validate(def) {
		if strings.HasPrefix(f.Path, "settings") {
			t.Errorf("a definition without settings has the finding %+v", f)
		}
	}
}

func TestValidate_Negative_VerificationLevel(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"unknown level", func(t *testing.T, def *Definition) {
			def.Settings.Verification.Level = "extreme"
		}, CodeEnum, "settings.verification.level", "extreme"},
		{"unknown basis", func(t *testing.T, def *Definition) {
			def.Settings.Verification.Basis = "guessed"
		}, CodeEnum, "settings.verification.basis", "guessed"},
		{"no reasoning", func(t *testing.T, def *Definition) {
			def.Settings.Verification.Reasoning = " "
		}, CodeText, "settings.verification.reasoning", "reasoning"},
	})
}

func TestValidate_Negative_FilterRuleTexts(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"bad id", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").ID = "Spam"
		}, CodeID, "settings.filter_rules[Spam]", "regex"},
		{"duplicate id", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").ID = "mention-spam"
		}, CodeID, "settings.filter_rules[mention-spam]", "duplicate"},
		{"no name", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Name = ""
		}, CodeText, "settings.filter_rules[spam].name", "name"},
		{"duplicate name", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Name = filterRuleOf(t, def, "mention-spam").Name
		}, CodeFilterRule, "settings.filter_rules[spam].name", "twice"},
		{"no purpose", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Purpose = ""
		}, CodeText, "settings.filter_rules[spam].purpose", "purpose"},
		{"no reasoning", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Reasoning = ""
		}, CodeText, "settings.filter_rules[spam].reasoning", "reasoning"},
		{"unknown trigger", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Trigger = "links"
		}, CodeEnum, "settings.filter_rules[spam].trigger", "links"},
		{"unknown basis", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Basis = "measured"
		}, CodeEnum, "settings.filter_rules[spam].basis", "measured"},
	})
}

func TestValidate_Negative_FilterRulesPerServer(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"second spam rule", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "mention-spam").Trigger = "spam"
			filterRuleOf(t, def, "mention-spam").MentionLimit = 0
			filterRuleOf(t, def, "mention-spam").RaidProtection = false
		}, CodeFilterRule, "settings.filter_rules", "at most 1 spam"},
		{"seventh keyword rule", func(t *testing.T, def *Definition) {
			base := *keywordRule(t, def)
			for i := 0; i < 6; i++ {
				extra := base
				extra.ID = "keyword-" + string(rune('a'+i))
				extra.Name = "Keyword " + string(rune('a'+i))
				def.Settings.FilterRules = append(def.Settings.FilterRules, extra)
			}
		}, CodeFilterRule, "settings.filter_rules", "at most 6 keyword"},
	})
}

func TestValidate_Boundary_SixKeywordRules(t *testing.T) {
	def := valid(t)
	base := *keywordRule(t, def)
	for i := 0; i < 5; i++ {
		extra := base
		extra.ID = "keyword-" + string(rune('a'+i))
		extra.Name = "Keyword " + string(rune('a'+i))
		def.Settings.FilterRules = append(def.Settings.FilterRules, extra)
	}
	if hasFinding(Validate(def), CodeFilterRule, "settings.filter_rules", "at most") {
		t.Errorf("six keyword rules are refused; Discord allows six")
	}
}

func TestValidate_Negative_FilterRuleMetadata(t *testing.T) {
	p := "settings.filter_rules[spam]"
	runPlanted(t, []plantedCase{
		{"keywords on a spam rule", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Keywords = []string{"x"}
		}, CodeFilterRule, p + ".keywords", "keyword or member_profile"},
		{"patterns on a spam rule", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Patterns = []string{"x+"}
		}, CodeFilterRule, p + ".patterns", "keyword or member_profile"},
		{"keyword rule without keywords or patterns", func(t *testing.T, def *Definition) {
			keywordRule(t, def).Keywords = nil
		}, CodeFilterRule, p + ".keywords", "needs"},
		{"member profile rule without keywords or patterns", func(t *testing.T, def *Definition) {
			rule := keywordRule(t, def)
			rule.Trigger = "member_profile"
			rule.Keywords = nil
			rule.Actions = []FilterAction{{Type: "block_interaction"}}
		}, CodeFilterRule, p + ".keywords", "needs"},
		{"too many keywords", func(t *testing.T, def *Definition) {
			keywordRule(t, def).Keywords = repeated("x", MaxKeywords+1)
		}, CodeFilterRule, p + ".keywords", "1000"},
		{"long keyword", func(t *testing.T, def *Definition) {
			keywordRule(t, def).Keywords = []string{strings.Repeat("x", MaxKeywordRunes+1)}
		}, CodeFilterRule, p + ".keywords", "60"},
		{"empty keyword", func(t *testing.T, def *Definition) {
			keywordRule(t, def).Keywords = []string{" "}
		}, CodeFilterRule, p + ".keywords", "60"},
		{"too many patterns", func(t *testing.T, def *Definition) {
			keywordRule(t, def).Patterns = repeated("x+", MaxPatterns+1)
		}, CodeFilterRule, p + ".patterns", "10"},
		{"long pattern", func(t *testing.T, def *Definition) {
			keywordRule(t, def).Patterns = []string{strings.Repeat("x", MaxPatternRunes+1)}
		}, CodeFilterRule, p + ".patterns", "260"},
		{"presets on a spam rule", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Presets = []string{"slurs"}
		}, CodeFilterRule, p + ".presets", "keyword_preset"},
		{"allow list on a spam rule", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").AllowList = []string{"x"}
		}, CodeFilterRule, p + ".allow_list", "keyword"},
		{"too long allow list on a keyword rule", func(t *testing.T, def *Definition) {
			keywordRule(t, def).AllowList = repeated("x", MaxAllowList+1)
		}, CodeFilterRule, p + ".allow_list", "100"},
		{"long allow list entry", func(t *testing.T, def *Definition) {
			keywordRule(t, def).AllowList = []string{strings.Repeat("x", MaxKeywordRunes+1)}
		}, CodeFilterRule, p + ".allow_list", "60"},
		{"mention limit on a spam rule", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").MentionLimit = 5
		}, CodeFilterRule, p + ".mention_limit", "mention_spam"},
		{"raid protection on a spam rule", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").RaidProtection = true
		}, CodeFilterRule, p + ".raid_protection", "mention_spam"},
	})
}

func TestValidate_Negative_FilterRulePresetsAndMentions(t *testing.T) {
	preset := "settings.filter_rules[abuse-presets]"
	mention := "settings.filter_rules[mention-spam]"
	runPlanted(t, []plantedCase{
		{"preset rule without presets", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "abuse-presets").Presets = nil
		}, CodeFilterRule, preset + ".presets", "needs"},
		{"unknown preset", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "abuse-presets").Presets = []string{"slurs", "politics"}
		}, CodeEnum, preset + ".presets", "politics"},
		{"preset twice", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "abuse-presets").Presets = []string{"slurs", "slurs"}
		}, CodeFilterRule, preset + ".presets", "twice"},
		{"too long allow list on a preset rule", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "abuse-presets").AllowList = repeated("x", MaxPresetAllowList+1)
		}, CodeFilterRule, preset + ".allow_list", "1000"},
		{"mention limit zero", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "mention-spam").MentionLimit = 0
		}, CodeFilterRule, mention + ".mention_limit", "between 1 and 50"},
		{"mention limit above the maximum", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "mention-spam").MentionLimit = MaxMentionLimit + 1
		}, CodeFilterRule, mention + ".mention_limit", "between 1 and 50"},
	})
}

func TestValidate_Boundary_FilterRuleLimits(t *testing.T) {
	def := valid(t)
	keyword := keywordRule(t, def)
	keyword.Keywords = repeated(strings.Repeat("x", MaxKeywordRunes), MaxKeywords)
	keyword.Patterns = repeated(strings.Repeat("x", MaxPatternRunes), MaxPatterns)
	keyword.AllowList = repeated(strings.Repeat("y", MaxKeywordRunes), MaxAllowList)
	filterRuleOf(t, def, "abuse-presets").AllowList = repeated("y", MaxPresetAllowList)
	filterRuleOf(t, def, "mention-spam").MentionLimit = MaxMentionLimit
	timeout := FilterAction{Type: "timeout", TimeoutSeconds: MaxTimeoutSeconds}
	keyword.Actions = append(keyword.Actions, timeout)
	actionOf(t, keyword, "block_message").Message = strings.Repeat("z", MaxBlockMessageRunes)
	for _, f := range Validate(def) {
		if f.Code == CodeFilterRule {
			t.Errorf("a value at Discord's limit is refused: %+v", f)
		}
	}

	def = valid(t)
	filterRuleOf(t, def, "mention-spam").MentionLimit = 1
	filterRuleOf(t, def, "mention-spam").Actions = append(filterRuleOf(t, def, "mention-spam").Actions,
		FilterAction{Type: "timeout", TimeoutSeconds: 1})
	for _, f := range Validate(def) {
		if f.Code == CodeFilterRule {
			t.Errorf("a value at the lower limit is refused: %+v", f)
		}
	}
}

func TestValidate_Negative_FilterActions(t *testing.T) {
	p := "settings.filter_rules[spam]"
	runPlanted(t, []plantedCase{
		{"no action", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").Actions = nil
		}, CodeFilterRule, p + ".actions", "at least one"},
		{"unknown action", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "block_message").Type = "ban"
		}, CodeEnum, p + ".actions[ban]", "ban"},
		{"action twice", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "block_message").Type = "alert"
		}, CodeFilterRule, p + ".actions[alert]", "twice"},
		{"alert without channel", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "alert").Channel = ""
		}, CodeFilterRule, p + ".actions[alert].channel", "needs"},
		{"alert to an unknown channel", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "alert").Channel = "nowhere"
		}, CodeRef, p + ".actions[alert].channel", "nowhere"},
		{"alert to a channel members see", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "alert").Channel = "tickets"
		}, CodeFilterRule, p + ".actions[alert].channel", "staff"},
		{"channel on a block", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "block_message").Channel = "mod-queue"
		}, CodeFilterRule, p + ".actions[block_message].channel", "alert"},
		{"timeout on a spam rule", func(t *testing.T, def *Definition) {
			rule := filterRuleOf(t, def, "spam")
			rule.Actions = append(rule.Actions, FilterAction{Type: "timeout", TimeoutSeconds: 60})
		}, CodeFilterRule, p + ".actions[timeout]", "keyword or mention_spam"},
		{"timeout without length", func(t *testing.T, def *Definition) {
			rule := keywordRule(t, def)
			rule.Actions = append(rule.Actions, FilterAction{Type: "timeout"})
		}, CodeFilterRule, p + ".actions[timeout].timeout_seconds", "between 1 and 2419200"},
		{"timeout above four weeks", func(t *testing.T, def *Definition) {
			rule := keywordRule(t, def)
			rule.Actions = append(rule.Actions, FilterAction{Type: "timeout", TimeoutSeconds: MaxTimeoutSeconds + 1})
		}, CodeFilterRule, p + ".actions[timeout].timeout_seconds", "between 1 and 2419200"},
		{"length on a block", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "block_message").TimeoutSeconds = 60
		}, CodeFilterRule, p + ".actions[block_message].timeout_seconds", "timeout"},
		{"long block message", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "block_message").Message = strings.Repeat("z", MaxBlockMessageRunes+1)
		}, CodeFilterRule, p + ".actions[block_message].message", "150"},
		{"message on an alert", func(t *testing.T, def *Definition) {
			actionOf(t, filterRuleOf(t, def, "spam"), "alert").Message = "flagged"
		}, CodeFilterRule, p + ".actions[alert].message", "block_message"},
	})
}

func TestValidate_Negative_FilterRuleExemptions(t *testing.T) {
	p := "settings.filter_rules[spam]"
	runPlanted(t, []plantedCase{
		{"unknown exempt role", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").ExemptRoles = []string{"ghost"}
		}, CodeRef, p + ".exempt_roles[ghost]", "ghost"},
		{"unknown exempt channel", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").ExemptChannels = []string{"nowhere"}
		}, CodeRef, p + ".exempt_channels[nowhere]", "nowhere"},
		{"exempt role twice", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").ExemptRoles = []string{"moderator", "moderator"}
		}, CodeFilterRule, p + ".exempt_roles[moderator]", "twice"},
		{"too many exempt roles", func(t *testing.T, def *Definition) {
			ids := make([]string, 0, MaxExemptRoles+1)
			for i := 0; i < MaxExemptRoles+1; i++ {
				ids = append(ids, def.Roles[i].ID)
			}
			filterRuleOf(t, def, "spam").ExemptRoles = ids
		}, CodeFilterRule, p + ".exempt_roles", "20"},
		{"too many exempt channels", func(t *testing.T, def *Definition) {
			filterRuleOf(t, def, "spam").ExemptChannels = repeated("mod-queue", MaxExemptChannels+1)
		}, CodeFilterRule, p + ".exempt_channels", "50"},
	})
}

func TestValidate_Boundary_FilterRuleExemptions(t *testing.T) {
	def := valid(t)
	ids := make([]string, 0, MaxExemptRoles)
	for i := 0; i < MaxExemptRoles; i++ {
		ids = append(ids, def.Roles[i].ID)
	}
	rule := filterRuleOf(t, def, "spam")
	rule.ExemptRoles = ids
	rule.ExemptChannels = []string{"mod-queue", "mod-log"}
	for _, f := range Validate(def) {
		if strings.HasPrefix(f.Path, "settings") {
			t.Errorf("twenty exempt roles and two exempt channels are refused: %+v", f)
		}
	}
}
