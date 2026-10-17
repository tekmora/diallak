package serverplan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	// KindVerificationLevel is reported when the server's verification level differs.
	KindVerificationLevel = "verification-level"
	// KindVerificationNotRead is reported when the definition states a level and the read holds none.
	KindVerificationNotRead = "verification-not-read"
	// KindFilterRulesNotRead is reported when the definition states rules and the read could not list them.
	KindFilterRulesNotRead = "filter-rules-not-read"
	// KindFilterRuleMissing is reported when a rule of the definition has no match on the server.
	KindFilterRuleMissing = "filter-rule-missing"
	// KindFilterRuleAmbiguous is reported when the server holds more than one rule of a rule's name.
	KindFilterRuleAmbiguous = "filter-rule-ambiguous"
	// KindFilterRuleTrigger is reported when a rule has another trigger on the server.
	KindFilterRuleTrigger = "filter-rule-trigger"
	// KindFilterRuleDiffers is reported when a rule differs in a field it can change.
	KindFilterRuleDiffers = "filter-rule-differs"
	// KindFilterRuleExtra is reported for a rule only the server holds.
	KindFilterRuleExtra = "filter-rule-extra"
)

// compareSettings compares the verification level and the filter rules. A
// definition without settings compares neither.
func compareSettings(def *serverdef.Definition, s *Snapshot) []Difference {
	if def.Settings == nil {
		return nil
	}
	var diffs []Difference
	if d := compareVerification(def.Settings.Verification, s); d != nil {
		diffs = append(diffs, *d)
	}
	if !s.FilterRulesRead {
		if len(def.Settings.FilterRules) > 0 {
			diffs = append(diffs, Difference{Kind: KindFilterRulesNotRead, Object: "settings.filter_rules"})
		}
		return diffs
	}
	for i := range def.Settings.FilterRules {
		if d := compareFilterRule(def, &def.Settings.FilterRules[i], s); d != nil {
			diffs = append(diffs, *d)
		}
	}
	return append(diffs, extraFilterRules(def, s)...)
}

func compareVerification(v *serverdef.Verification, s *Snapshot) *Difference {
	switch {
	case v == nil || v.Level == s.VerificationLevel:
		return nil
	case s.VerificationLevel == "":
		return &Difference{Kind: KindVerificationNotRead, Object: "settings.verification"}
	default:
		return &Difference{Kind: KindVerificationLevel, Object: "settings.verification",
			Detail: fmt.Sprintf("%s, server %s", v.Level, s.VerificationLevel)}
	}
}

// filterRuleObject names a rule of the definition.
func filterRuleObject(id string) string {
	return fmt.Sprintf("settings.filter_rules[%s]", id)
}

// matchFilterRules returns the server's rules of the rule's exact name.
func matchFilterRules(name string, s *Snapshot) []*FilterRule {
	var matched []*FilterRule
	for i := range s.FilterRules {
		if s.FilterRules[i].Name == name {
			matched = append(matched, &s.FilterRules[i])
		}
	}
	return matched
}

func compareFilterRule(def *serverdef.Definition, r *serverdef.FilterRule, s *Snapshot) *Difference {
	object := filterRuleObject(r.ID)
	matched := matchFilterRules(r.Name, s)
	switch {
	case len(matched) == 0:
		return &Difference{Kind: KindFilterRuleMissing, Object: object}
	case len(matched) > 1:
		return &Difference{Kind: KindFilterRuleAmbiguous, Object: object, Detail: fmt.Sprintf("%d", len(matched))}
	case matched[0].Trigger != r.Trigger:
		return &Difference{Kind: KindFilterRuleTrigger, Object: object, Detail: fmt.Sprintf("%s, server %s", r.Trigger, matched[0].Trigger)}
	}
	fields := filterRuleFieldsDiffering(serverFormOf(def, r), matched[0])
	if len(fields) == 0 {
		return nil
	}
	return &Difference{Kind: KindFilterRuleDiffers, Object: object, Detail: strings.Join(fields, ", ")}
}

// serverFormOf returns a rule of the definition as the server holds it: roles
// and channels by name.
func serverFormOf(def *serverdef.Definition, r *serverdef.FilterRule) FilterRule {
	sr := FilterRule{
		Name: r.Name, Trigger: r.Trigger, Enabled: r.Enabled,
		Keywords: r.Keywords, Patterns: r.Patterns, Presets: r.Presets, AllowList: r.AllowList,
		MentionLimit: r.MentionLimit, RaidProtection: r.RaidProtection,
	}
	for _, a := range r.Actions {
		action := FilterAction{Type: a.Type, TimeoutSeconds: a.TimeoutSeconds, Message: a.Message}
		if a.Channel != "" {
			action.Channel = defChannelName(def, a.Channel)
		}
		sr.Actions = append(sr.Actions, action)
	}
	for _, id := range r.ExemptRoles {
		sr.ExemptRoles = append(sr.ExemptRoles, getRoleName(def, id))
	}
	for _, id := range r.ExemptChannels {
		sr.ExemptChannels = append(sr.ExemptChannels, defChannelName(def, id))
	}
	return sr
}

func defChannelName(def *serverdef.Definition, id string) string {
	for i := range def.Channels {
		if def.Channels[i].ID == id {
			return def.Channels[i].Name
		}
	}
	return ""
}

// filterRuleFieldsDiffering names the fields in which two rules differ, in a
// fixed order. Lists compare as sets.
func filterRuleFieldsDiffering(want FilterRule, got *FilterRule) []string {
	checks := []struct {
		field string
		same  bool
	}{
		{"enabled", want.Enabled == got.Enabled},
		{"keywords", sameSet(want.Keywords, got.Keywords)},
		{"patterns", sameSet(want.Patterns, got.Patterns)},
		{"presets", sameSet(want.Presets, got.Presets)},
		{"allow_list", sameSet(want.AllowList, got.AllowList)},
		{"mention_limit", want.MentionLimit == got.MentionLimit},
		{"raid_protection", want.RaidProtection == got.RaidProtection},
		{"actions", sameSet(actionKeys(want.Actions), actionKeys(got.Actions))},
		{"exempt_roles", sameSet(want.ExemptRoles, got.ExemptRoles)},
		{"exempt_channels", sameSet(want.ExemptChannels, got.ExemptChannels)},
	}
	var fields []string
	for _, c := range checks {
		if !c.same {
			fields = append(fields, c.field)
		}
	}
	return fields
}

func actionKeys(actions []FilterAction) []string {
	keys := make([]string, 0, len(actions))
	for _, a := range actions {
		keys = append(keys, fmt.Sprintf("%q %q %d %q", a.Type, a.Channel, a.TimeoutSeconds, a.Message))
	}
	return keys
}

// sameSet reports whether two lists hold the same entries, each as often, in any order.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa, sb := slices.Clone(a), slices.Clone(b)
	slices.Sort(sa)
	slices.Sort(sb)
	return slices.Equal(sa, sb)
}

// extraFilterRules reports each name of a rule only the server holds, once.
func extraFilterRules(def *serverdef.Definition, s *Snapshot) []Difference {
	known := make(map[string]struct{}, len(def.Settings.FilterRules)+len(s.FilterRules))
	for i := range def.Settings.FilterRules {
		known[def.Settings.FilterRules[i].Name] = struct{}{}
	}
	var diffs []Difference
	for i := range s.FilterRules {
		name := s.FilterRules[i].Name
		if _, ok := known[name]; ok {
			continue
		}
		known[name] = struct{}{}
		diffs = append(diffs, Difference{Kind: KindFilterRuleExtra, Object: fmt.Sprintf("server.filter_rules[%s]", name)})
	}
	return diffs
}
