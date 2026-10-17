package discord

import (
	"context"
	"errors"
	"fmt"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
)

const (
	verificationObject = "settings.verification"
	filterRulePrefix   = "settings.filter_rules["
)

func validateVerificationChange(def *serverdef.Definition, c serverplan.Change) error {
	if c.Object != verificationObject {
		return fmt.Errorf("step %d %s %s: unexpected object for %s", c.Step, c.Action, c.Object, c.Action)
	}
	if def.Settings == nil || def.Settings.Verification == nil {
		return fmt.Errorf("step %d %s %s: the definition holds no verification level", c.Step, c.Action, c.Object)
	}
	return nil
}

func validateFilterRuleChange(def *serverdef.Definition, c serverplan.Change) error {
	id, err := extractBracketedID(c.Object, filterRulePrefix)
	if err != nil {
		return fmt.Errorf("step %d %s %s: %w", c.Step, c.Action, c.Object, err)
	}
	if _, ok := findFilterRuleInDef(def, id); !ok {
		return fmt.Errorf("step %d %s %s: filter rule %q not found in definition", c.Step, c.Action, c.Object, id)
	}
	return nil
}

func findFilterRuleInDef(def *serverdef.Definition, id string) (*serverdef.FilterRule, bool) {
	if def.Settings == nil {
		return nil, false
	}
	for i := range def.Settings.FilterRules {
		if def.Settings.FilterRules[i].ID == id {
			return &def.Settings.FilterRules[i], true
		}
	}
	return nil, false
}

func applySetVerificationLevel(ctx context.Context, client *bot.Client, guildID uint64, def *serverdef.Definition, reason string) error {
	if def.Settings == nil || def.Settings.Verification == nil {
		return errors.New("the definition holds no verification level")
	}
	level, ok := numberOf(verificationLevelNames, def.Settings.Verification.Level)
	if !ok {
		return fmt.Errorf("unknown verification level %q", def.Settings.Verification.Level)
	}
	update := discord.GuildUpdate{VerificationLevel: omit.NewPtr(level)}
	if _, err := client.Rest.UpdateGuild(snowflake.ID(guildID), update, rest.WithCtx(ctx), rest.WithReason(reason)); err != nil {
		return callError("set verification level", err, client.Token)
	}
	return nil
}

// platformFilterRule is a rule of the definition with its names turned into the platform's numbers and ids.
type platformFilterRule struct {
	event    discord.AutoModerationEventType
	trigger  discord.AutoModerationTriggerType
	metadata *discord.AutoModerationTriggerMetadata
	actions  []discord.AutoModerationAction
	roles    []snowflake.ID
	channels []snowflake.ID
}

func numberOf[K comparable](names map[K]string, name string) (K, bool) {
	for k, v := range names {
		if v == name {
			return k, true
		}
	}
	var zero K
	return zero, false
}

func buildPlatformFilterRule(def *serverdef.Definition, r *serverdef.FilterRule, idx *serverIndex) (platformFilterRule, error) {
	var out platformFilterRule
	trigger, ok := numberOf(filterTriggerNames, r.Trigger)
	if !ok {
		return out, fmt.Errorf("filter rule %q: unknown trigger %q", r.ID, r.Trigger)
	}
	out.trigger, out.event = trigger, discord.AutoModerationEventTypeMessageSend
	if trigger == discord.AutoModerationTriggerTypeMemberProfile {
		out.event = discord.AutoModerationEventTypeMemberUpdate
	}
	if trigger != discord.AutoModerationTriggerTypeSpam {
		md, err := triggerMetadataOf(r)
		if err != nil {
			return out, err
		}
		out.metadata = md
	}
	for _, a := range r.Actions {
		act, err := platformActionOf(def, r.ID, a, idx)
		if err != nil {
			return out, err
		}
		out.actions = append(out.actions, act)
	}
	var err error
	if out.roles, err = exemptRolesOf(def, r, idx); err != nil {
		return out, err
	}
	out.channels, err = exemptChannelsOf(def, r, idx)
	return out, err
}

func exemptRolesOf(def *serverdef.Definition, r *serverdef.FilterRule, idx *serverIndex) ([]snowflake.ID, error) {
	ids := []snowflake.ID{}
	for _, roleID := range r.ExemptRoles {
		role, ok := findRoleInDef(def, roleID)
		if !ok {
			return nil, fmt.Errorf("filter rule %q: role %q not found in definition", r.ID, roleID)
		}
		id, err := idx.lookupRole(role.Name)
		if err != nil {
			return nil, fmt.Errorf("filter rule %q: %w", r.ID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func exemptChannelsOf(def *serverdef.Definition, r *serverdef.FilterRule, idx *serverIndex) ([]snowflake.ID, error) {
	ids := []snowflake.ID{}
	for _, chID := range r.ExemptChannels {
		id, err := lookupDefChannel(def, chID, idx)
		if err != nil {
			return nil, fmt.Errorf("filter rule %q: %w", r.ID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func triggerMetadataOf(r *serverdef.FilterRule) (*discord.AutoModerationTriggerMetadata, error) {
	md := &discord.AutoModerationTriggerMetadata{
		KeywordFilter:                emptyIfNil(r.Keywords),
		RegexPatterns:                emptyIfNil(r.Patterns),
		AllowList:                    emptyIfNil(r.AllowList),
		Presets:                      []discord.AutoModerationKeywordPreset{},
		MentionTotalLimit:            r.MentionLimit,
		MentionRaidProtectionEnabled: r.RaidProtection,
	}
	for _, p := range r.Presets {
		n, ok := numberOf(filterPresetNames, p)
		if !ok {
			return nil, fmt.Errorf("filter rule %q: unknown preset %q", r.ID, p)
		}
		md.Presets = append(md.Presets, n)
	}
	return md, nil
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func platformActionOf(def *serverdef.Definition, ruleID string, a serverdef.FilterAction, idx *serverIndex) (discord.AutoModerationAction, error) {
	t, ok := numberOf(filterActionNames, a.Type)
	if !ok {
		return discord.AutoModerationAction{}, fmt.Errorf("filter rule %q: unknown action %q", ruleID, a.Type)
	}
	act := discord.AutoModerationAction{Type: t}
	switch t {
	case discord.AutoModerationActionTypeSendAlertMessage:
		id, err := lookupDefChannel(def, a.Channel, idx)
		if err != nil {
			return act, fmt.Errorf("filter rule %q: %w", ruleID, err)
		}
		act.Metadata = &discord.AutoModerationActionMetadata{ChannelID: id}
	case discord.AutoModerationActionTypeTimeout:
		act.Metadata = &discord.AutoModerationActionMetadata{DurationSeconds: a.TimeoutSeconds}
	case discord.AutoModerationActionTypeBlockMessage:
		if a.Message != "" {
			msg := a.Message
			act.Metadata = &discord.AutoModerationActionMetadata{CustomMessage: &msg}
		}
	}
	return act, nil
}

func lookupDefChannel(def *serverdef.Definition, id string, idx *serverIndex) (snowflake.ID, error) {
	ch, ok := findChannelInDef(def, id)
	if !ok {
		return 0, fmt.Errorf("channel %q not found in definition", id)
	}
	return idx.lookupChannel(ch.Name)
}

func applyCreateFilterRule(ctx context.Context, client *bot.Client, guildID uint64, def *serverdef.Definition, object string, idx *serverIndex, reason string) error {
	r, p, err := filterRuleForChange(def, object, idx)
	if err != nil {
		return err
	}
	enabled := r.Enabled
	create := discord.AutoModerationRuleCreate{
		Name: r.Name, EventType: p.event, TriggerType: p.trigger, TriggerMetadata: p.metadata,
		Actions: p.actions, Enabled: &enabled, ExemptRoles: p.roles, ExemptChannels: p.channels,
	}
	created, err := client.Rest.CreateAutoModerationRule(snowflake.ID(guildID), create, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("create filter rule", err, client.Token)
	}
	if created == nil || created.ID == 0 {
		return errors.New("create filter rule: returned rule id is 0")
	}
	idx.filterRules[r.Name] = append(idx.filterRules[r.Name], created.ID)
	return nil
}

// applySetFilterRule sends every part of the rule; the trigger stays as it is,
// since the platform changes no trigger of a rule.
func applySetFilterRule(ctx context.Context, client *bot.Client, guildID uint64, def *serverdef.Definition, object string, idx *serverIndex, reason string) error {
	r, p, err := filterRuleForChange(def, object, idx)
	if err != nil {
		return err
	}
	ids := idx.filterRules[r.Name]
	if len(ids) != 1 {
		return fmt.Errorf("filter rule %q is held %d times, want once", r.Name, len(ids))
	}
	name, enabled := r.Name, r.Enabled
	update := discord.AutoModerationRuleUpdate{
		Name: &name, EventType: &p.event, TriggerMetadata: p.metadata,
		Actions: &p.actions, Enabled: &enabled, ExemptRoles: &p.roles, ExemptChannels: &p.channels,
	}
	if _, err := client.Rest.UpdateAutoModerationRule(snowflake.ID(guildID), ids[0], update, rest.WithCtx(ctx), rest.WithReason(reason)); err != nil {
		return callError("set filter rule", err, client.Token)
	}
	return nil
}

func filterRuleForChange(def *serverdef.Definition, object string, idx *serverIndex) (*serverdef.FilterRule, platformFilterRule, error) {
	id, err := extractBracketedID(object, filterRulePrefix)
	if err != nil {
		return nil, platformFilterRule{}, err
	}
	r, ok := findFilterRuleInDef(def, id)
	if !ok {
		return nil, platformFilterRule{}, fmt.Errorf("filter rule %q not found in definition", id)
	}
	p, err := buildPlatformFilterRule(def, r, idx)
	return r, p, err
}
