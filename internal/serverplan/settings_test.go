package serverplan

import (
	"slices"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

// serverSettingsOf returns the verification level and the filter rules a server
// holds when it is set up as def states, roles and channels by name.
func serverSettingsOf(t *testing.T, def *serverdef.Definition) (string, []FilterRule) {
	t.Helper()
	if def.Settings == nil {
		return "", nil
	}
	level := ""
	if def.Settings.Verification != nil {
		level = def.Settings.Verification.Level
	}
	rules := make([]FilterRule, 0, len(def.Settings.FilterRules))
	for i := range def.Settings.FilterRules {
		rules = append(rules, serverRuleOf(t, def, &def.Settings.FilterRules[i]))
	}
	return level, rules
}

func serverRuleOf(t *testing.T, def *serverdef.Definition, r *serverdef.FilterRule) FilterRule {
	t.Helper()
	sr := FilterRule{
		Name:           r.Name,
		Trigger:        r.Trigger,
		Enabled:        r.Enabled,
		Keywords:       slices.Clone(r.Keywords),
		Patterns:       slices.Clone(r.Patterns),
		Presets:        slices.Clone(r.Presets),
		AllowList:      slices.Clone(r.AllowList),
		MentionLimit:   r.MentionLimit,
		RaidProtection: r.RaidProtection,
	}
	for _, a := range r.Actions {
		action := FilterAction{Type: a.Type, TimeoutSeconds: a.TimeoutSeconds, Message: a.Message}
		if a.Channel != "" {
			action.Channel = channelNameOf(t, def, a.Channel)
		}
		sr.Actions = append(sr.Actions, action)
	}
	for _, id := range r.ExemptRoles {
		sr.ExemptRoles = append(sr.ExemptRoles, getRoleName(def, id))
	}
	for _, id := range r.ExemptChannels {
		sr.ExemptChannels = append(sr.ExemptChannels, channelNameOf(t, def, id))
	}
	return sr
}

func channelNameOf(t *testing.T, def *serverdef.Definition, id string) string {
	t.Helper()
	for i := range def.Channels {
		if def.Channels[i].ID == id {
			return def.Channels[i].Name
		}
	}
	t.Fatalf("channel %q not in definition", id)
	return ""
}

// serverRule returns the server's rule of the given name.
func serverRule(t *testing.T, s *Snapshot, name string) *FilterRule {
	t.Helper()
	for i := range s.FilterRules {
		if s.FilterRules[i].Name == name {
			return &s.FilterRules[i]
		}
	}
	t.Fatalf("filter rule %q not in snapshot", name)
	return nil
}

// defRule returns the definition's rule of the given id.
func defRule(t *testing.T, def *serverdef.Definition, id string) *serverdef.FilterRule {
	t.Helper()
	for i := range def.Settings.FilterRules {
		if def.Settings.FilterRules[i].ID == id {
			return &def.Settings.FilterRules[i]
		}
	}
	t.Fatalf("filter rule %q not in definition", id)
	return nil
}

func withoutServerRule(s *Snapshot, name string) {
	kept := s.FilterRules[:0]
	for _, r := range s.FilterRules {
		if r.Name != name {
			kept = append(kept, r)
		}
	}
	s.FilterRules = kept
}

// keywordRuleInDef turns the definition's spam rule into a keyword rule and
// the server's rule along with it.
func keywordRuleInDef(t *testing.T, def *serverdef.Definition, s *Snapshot) {
	t.Helper()
	r := defRule(t, def, "spam")
	r.Trigger = "keyword"
	r.Keywords = []string{"buy now", "free nitro"}
	r.Patterns = []string{"discord\\.gift/\\w+"}
	r.AllowList = []string{"nitro"}
	sr := serverRule(t, s, "Spam")
	*sr = serverRuleOf(t, def, r)
}

type settingsDiffCase struct {
	name   string
	mutate func(t *testing.T, def *serverdef.Definition, s *Snapshot)
	kind   string
	object string
	word   string
}

func findDiff(diffs []Difference, kind, object string) *Difference {
	for i := range diffs {
		if diffs[i].Kind == kind && diffs[i].Object == object {
			return &diffs[i]
		}
	}
	return nil
}

func runSettingsDiffCases(t *testing.T, cases []settingsDiffCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := loadDef(t)
			s := snapshotOf(t, def)
			tc.mutate(t, def, s)
			diffs, err := Compare(def, s)
			if err != nil {
				t.Fatalf("Compare failed: %v", err)
			}
			d := findDiff(diffs, tc.kind, tc.object)
			if d == nil {
				t.Fatalf("no %s %s in %+v", tc.kind, tc.object, diffs)
			}
			if !strings.Contains(d.Detail, tc.word) {
				t.Errorf("detail %q lacks %q", d.Detail, tc.word)
			}
		})
	}
}

func TestCompare_Positive_SettingsAsDefined(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	keywordRuleInDef(t, def, s)
	r := defRule(t, def, "mention-spam")
	r.ExemptRoles = []string{"moderator", "maintainer"}
	r.ExemptChannels = []string{"mod-queue"}
	r.Actions = append(r.Actions, serverdef.FilterAction{Type: "timeout", TimeoutSeconds: 60})
	*serverRule(t, s, "Mass mentions") = serverRuleOf(t, def, r)

	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 0 {
		t.Fatalf("expected 0 differences, got %+v", diffs)
	}
}

func TestCompare_Positive_NoSettingsNothingCompared(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	def.Settings = nil
	s.VerificationLevel = ""
	s.FilterRulesRead = false
	s.FilterRules = nil
	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 0 {
		t.Fatalf("expected 0 differences, got %+v", diffs)
	}
}

func TestCompare_Negative_VerificationLevel(t *testing.T) {
	runSettingsDiffCases(t, []settingsDiffCase{
		{"other level", func(_ *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.VerificationLevel = "low"
		}, KindVerificationLevel, "settings.verification", "medium, server low"},
		{"level not read", func(_ *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.VerificationLevel = ""
		}, KindVerificationNotRead, "settings.verification", ""},
	})
}

func TestCompare_Negative_FilterRuleKinds(t *testing.T) {
	runSettingsDiffCases(t, []settingsDiffCase{
		{"rules not read", func(_ *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.FilterRulesRead = false
			s.FilterRules = nil
		}, KindFilterRulesNotRead, "settings.filter_rules", ""},
		{"rule missing", func(_ *testing.T, _ *serverdef.Definition, s *Snapshot) {
			withoutServerRule(s, "Spam")
		}, KindFilterRuleMissing, "settings.filter_rules[spam]", ""},
		{"rule name twice", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.FilterRules = append(s.FilterRules, *serverRule(t, s, "Spam"))
		}, KindFilterRuleAmbiguous, "settings.filter_rules[spam]", ""},
		{"other trigger", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Spam").Trigger = "other"
		}, KindFilterRuleTrigger, "settings.filter_rules[spam]", "spam, server other"},
		{"rule only on the server", func(_ *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.FilterRules = append(s.FilterRules, FilterRule{Name: "Links", Trigger: "keyword", Keywords: []string{"http"}})
		}, KindFilterRuleExtra, "server.filter_rules[Links]", ""},
	})
}

func TestCompare_Negative_FilterRuleFields(t *testing.T) {
	runSettingsDiffCases(t, []settingsDiffCase{
		{"enabled", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Spam").Enabled = false
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "enabled"},
		{"keywords", func(t *testing.T, def *serverdef.Definition, s *Snapshot) {
			keywordRuleInDef(t, def, s)
			serverRule(t, s, "Spam").Keywords = []string{"buy now"}
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "keywords"},
		{"patterns", func(t *testing.T, def *serverdef.Definition, s *Snapshot) {
			keywordRuleInDef(t, def, s)
			serverRule(t, s, "Spam").Patterns = nil
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "patterns"},
		{"allow list", func(t *testing.T, def *serverdef.Definition, s *Snapshot) {
			keywordRuleInDef(t, def, s)
			serverRule(t, s, "Spam").AllowList = []string{"nitro", "gift"}
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "allow_list"},
		{"presets", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Slurs and sexual content").Presets = []string{"slurs"}
		}, KindFilterRuleDiffers, "settings.filter_rules[abuse-presets]", "presets"},
		{"mention limit", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Mass mentions").MentionLimit = 11
		}, KindFilterRuleDiffers, "settings.filter_rules[mention-spam]", "mention_limit"},
		{"raid protection", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Mass mentions").RaidProtection = false
		}, KindFilterRuleDiffers, "settings.filter_rules[mention-spam]", "raid_protection"},
		{"exempt roles", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Spam").ExemptRoles = []string{"Moderator"}
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "exempt_roles"},
		{"exempt channels", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Spam").ExemptChannels = []string{"mod-queue"}
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "exempt_channels"},
	})
}

func TestCompare_Negative_FilterRuleActions(t *testing.T) {
	runSettingsDiffCases(t, []settingsDiffCase{
		{"action missing", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			r := serverRule(t, s, "Spam")
			r.Actions = r.Actions[:1]
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "actions"},
		{"action only on the server", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			r := serverRule(t, s, "Spam")
			r.Actions = append(r.Actions, FilterAction{Type: "block_interaction"})
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "actions"},
		{"alert to another channel", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Spam").Actions[1].Channel = "mod-log"
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "actions"},
		{"other message", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Spam").Actions[0].Message = "Not here."
		}, KindFilterRuleDiffers, "settings.filter_rules[spam]", "actions"},
		{"other timeout", func(t *testing.T, def *serverdef.Definition, s *Snapshot) {
			r := defRule(t, def, "mention-spam")
			r.Actions = append(r.Actions, serverdef.FilterAction{Type: "timeout", TimeoutSeconds: 60})
			*serverRule(t, s, "Mass mentions") = serverRuleOf(t, def, r)
			sr := serverRule(t, s, "Mass mentions")
			sr.Actions[2].TimeoutSeconds = 61
		}, KindFilterRuleDiffers, "settings.filter_rules[mention-spam]", "actions"},
	})
}

// The order of lists does not count, an empty and an absent list are the same,
// and every field that differs is named once, in a fixed order.
func TestCompare_Boundary_FilterRuleFields(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	keywordRuleInDef(t, def, s)
	sr := serverRule(t, s, "Spam")
	slices.Reverse(sr.Keywords)
	slices.Reverse(sr.Actions)
	sr.ExemptRoles = []string{}
	pr := serverRule(t, s, "Slurs and sexual content")
	slices.Reverse(pr.Presets)
	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if len(diffs) != 0 {
		t.Fatalf("expected 0 differences, got %+v", diffs)
	}

	mr := serverRule(t, s, "Mass mentions")
	mr.RaidProtection = false
	mr.Enabled = false
	mr.MentionLimit = 9
	diffs, err = Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	d := findDiff(diffs, KindFilterRuleDiffers, "settings.filter_rules[mention-spam]")
	if d == nil || d.Detail != "enabled, mention_limit, raid_protection" {
		t.Fatalf("differs = %+v, want detail %q", d, "enabled, mention_limit, raid_protection")
	}
}

// A rule matches by its exact name: another case is another rule.
func TestCompare_Boundary_FilterRuleNameExact(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	serverRule(t, s, "Spam").Name = "spam"
	diffs, err := Compare(def, s)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}
	if findDiff(diffs, KindFilterRuleMissing, "settings.filter_rules[spam]") == nil ||
		findDiff(diffs, KindFilterRuleExtra, "server.filter_rules[spam]") == nil {
		t.Fatalf("want missing and extra, got %+v", diffs)
	}
}

func TestSnapshot_Negative_SettingsChecked(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(s *Snapshot)
		word   string
	}{
		{"unknown level", func(s *Snapshot) { s.VerificationLevel = "highest" }, "verification level"},
		{"rules while not read", func(s *Snapshot) { s.FilterRulesRead = false }, "not read"},
		{"blank rule name", func(s *Snapshot) { s.FilterRules[0].Name = " " }, "empty filter rule name"},
		{"unknown trigger", func(s *Snapshot) { s.FilterRules[0].Trigger = "links" }, "trigger"},
		{"unknown action", func(s *Snapshot) { s.FilterRules[0].Actions[0].Type = "ban" }, "action"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := loadDef(t)
			s := snapshotOf(t, def)
			tc.mutate(s)
			err := Check(s)
			if err == nil || !strings.Contains(err.Error(), tc.word) {
				t.Fatalf("Check = %v, want an error naming %q", err, tc.word)
			}
		})
	}
}

func TestSnapshot_Boundary_SettingsOptional(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.FilterRules[0].Trigger = "other"
	s.FilterRules[0].Actions[0].Type = "other"
	if err := Check(s); err != nil {
		t.Fatalf("Check refused the platform's unnamed trigger and action: %v", err)
	}
	s.VerificationLevel = ""
	s.FilterRulesRead = false
	s.FilterRules = nil
	if err := Check(s); err != nil {
		t.Fatalf("Check refused a snapshot without settings: %v", err)
	}
}
