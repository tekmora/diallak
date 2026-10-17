package serverplan

import (
	"slices"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

func planOf(t *testing.T, def *serverdef.Definition, s *Snapshot) Result {
	t.Helper()
	res, err := Plan(def, s)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	return res
}

func changeOf(t *testing.T, res Result, action, object string) Change {
	t.Helper()
	for _, c := range res.Changes {
		if c.Action == action && c.Object == object {
			return c
		}
	}
	t.Fatalf("no change %s %s in %+v (refused %+v)", action, object, res.Changes, res.Refused)
	return Change{}
}

func dropCallerRight(s *Snapshot, right string) {
	s.Caller.Permissions = slices.DeleteFunc(s.Caller.Permissions, func(p string) bool { return p == right })
	for i := range s.Roles {
		if s.Roles[i].Name == s.Caller.Role {
			s.Roles[i].Permissions = slices.DeleteFunc(s.Roles[i].Permissions, func(p string) bool { return p == right })
		}
	}
}

// With its own rights the bot sets the level and creates and changes the rules.
func TestPlan_Positive_OwnModeSettings(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.VerificationLevel = "low"
	withoutServerRule(s, "Spam")
	serverRule(t, s, "Mass mentions").MentionLimit = 20

	res := planOf(t, def, s)
	if len(res.Refused) != 0 {
		t.Fatalf("expected 0 refused, got %+v", res.Refused)
	}
	want := []struct{ action, object, detail string }{
		{ActionSetVerificationLevel, "settings.verification", "medium, server low"},
		{ActionCreateFilterRule, "settings.filter_rules[spam]", "Spam"},
		{ActionSetFilterRule, "settings.filter_rules[mention-spam]", "mention_limit"},
	}
	if len(res.Changes) != len(want) {
		t.Fatalf("changes = %+v, want %d", res.Changes, len(want))
	}
	for i, w := range want {
		c := res.Changes[i]
		if c.Action != w.action || c.Object != w.object || c.Detail != w.detail {
			t.Errorf("change %d = %+v, want %s %s %q", i, c, w.action, w.object, w.detail)
		}
		if !slices.Equal(c.Needs, []string{"MANAGE_GUILD"}) {
			t.Errorf("change %d needs %v, want [MANAGE_GUILD]", i, c.Needs)
		}
	}
}

// A rule that times a member out needs the right to do so as well.
func TestPlan_Positive_TimeoutNeedsModerateMembers(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	r := defRule(t, def, "mention-spam")
	r.Actions = append(r.Actions, serverdef.FilterAction{Type: "timeout", TimeoutSeconds: 60})

	res := planOf(t, def, s)
	c := changeOf(t, res, ActionSetFilterRule, "settings.filter_rules[mention-spam]")
	if !slices.Equal(c.Needs, []string{"MANAGE_GUILD", "MODERATE_MEMBERS"}) {
		t.Errorf("needs %v, want [MANAGE_GUILD MODERATE_MEMBERS]", c.Needs)
	}
}

type settingsRefusalCase struct {
	name   string
	mutate func(t *testing.T, def *serverdef.Definition, s *Snapshot)
	action string
	object string
	reason string
}

func settingsRefusalCases() []settingsRefusalCase {
	return []settingsRefusalCase{
		{"caller lacks the right", func(_ *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.VerificationLevel = "low"
			dropCallerRight(s, "MANAGE_GUILD")
		}, ActionSetVerificationLevel, "settings.verification", ReasonLacksRight},
		{"level not read", func(_ *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.VerificationLevel = ""
		}, ActionSetVerificationLevel, "settings.verification", ReasonNotVisible},
		{"rules not read", func(_ *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.FilterRulesRead = false
			s.FilterRules = nil
		}, ActionSetFilterRule, "settings.filter_rules[abuse-presets]", ReasonNotVisible},
		{"other trigger", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Spam").Trigger = "keyword"
		}, ActionConvertFilterRule, "settings.filter_rules[spam]", ReasonTriggerDiffers},
		{"rule name twice", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			s.FilterRules = append(s.FilterRules, *serverRule(t, s, "Spam"))
		}, ActionResolveName, "settings.filter_rules[spam]", ReasonAmbiguousName},
		{"alert channel twice", func(t *testing.T, _ *serverdef.Definition, s *Snapshot) {
			serverRule(t, s, "Spam").Enabled = false
			for _, ch := range s.Channels {
				if ch.Name == "mod-queue" {
					s.Channels = append(s.Channels, ch)
					break
				}
			}
		}, ActionSetFilterRule, "settings.filter_rules[spam]", ReasonAmbiguousName},
		{"exempt role twice", func(t *testing.T, def *serverdef.Definition, s *Snapshot) {
			defRule(t, def, "spam").ExemptRoles = []string{"helper"}
			duplicateRole(t, s, "Helper")
		}, ActionSetFilterRule, "settings.filter_rules[spam]", ReasonAmbiguousName},
	}
}

func TestPlan_Negative_SettingsRefused(t *testing.T) {
	for _, tc := range settingsRefusalCases() {
		t.Run(tc.name, func(t *testing.T) {
			def := loadDef(t)
			s := snapshotOf(t, def)
			tc.mutate(t, def, s)
			res := planOf(t, def, s)
			ref, ok := refusalFor(res, tc.action, tc.object)
			if !ok {
				t.Fatalf("no refusal %s %s in %+v (changes %+v)", tc.action, tc.object, res.Refused, res.Changes)
			}
			if ref.Reason != tc.reason {
				t.Errorf("reason = %q, want %q", ref.Reason, tc.reason)
			}
			for _, c := range res.Changes {
				if c.Object == tc.object {
					t.Errorf("refused object also planned: %+v", c)
				}
			}
		})
	}
}

// A rule only on the server, and one the platform names no trigger for, are
// reported and left alone.
func TestPlan_Boundary_ExtraFilterRuleLeftAlone(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	s.FilterRules = append(s.FilterRules,
		FilterRule{Name: "Links", Trigger: "keyword", Keywords: []string{"http"}, Actions: []FilterAction{{Type: "block_message"}}},
		FilterRule{Name: "Platform rule", Trigger: "other", Actions: []FilterAction{{Type: "other"}}})
	res := planOf(t, def, s)
	if len(res.Changes) != 0 || len(res.Refused) != 0 {
		t.Fatalf("changes %+v, refused %+v, want none", res.Changes, res.Refused)
	}
}

// A definition without settings plans none, whatever the server holds.
func TestPlan_Boundary_NoSettingsPlansNone(t *testing.T) {
	def := loadDef(t)
	s := snapshotOf(t, def)
	def.Settings = nil
	s.VerificationLevel = "none"
	s.FilterRules = nil
	res := planOf(t, def, s)
	if len(res.Changes) != 0 || len(res.Refused) != 0 {
		t.Fatalf("changes %+v, refused %+v, want none", res.Changes, res.Refused)
	}
}
