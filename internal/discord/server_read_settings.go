package discord

import (
	"context"
	"fmt"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverplan"
)

// otherName stands for a trigger, an action or a preset the platform has and the table does not name.
const otherName = "other"

// The names of the platform's numbers, read in Discord's developer documentation,
// developers/resources/guild.mdx and developers/resources/auto-moderation.mdx.
var (
	verificationLevelNames = map[discord.VerificationLevel]string{
		discord.VerificationLevelNone:     "none",
		discord.VerificationLevelLow:      "low",
		discord.VerificationLevelMedium:   "medium",
		discord.VerificationLevelHigh:     "high",
		discord.VerificationLevelVeryHigh: "very_high",
	}
	filterTriggerNames = map[discord.AutoModerationTriggerType]string{
		discord.AutoModerationTriggerTypeKeyword:        "keyword",
		discord.AutoModerationTriggerTypeSpam:           "spam",
		discord.AutoModerationTriggerTypeKeywordPresent: "keyword_preset",
		discord.AutoModerationTriggerTypeMentionSpam:    "mention_spam",
		discord.AutoModerationTriggerTypeMemberProfile:  "member_profile",
	}
	filterPresetNames = map[discord.AutoModerationKeywordPreset]string{
		discord.AutoModerationKeywordPresetProfanity:     "profanity",
		discord.AutoModerationKeywordPresetSexualContent: "sexual_content",
		discord.AutoModerationKeywordPresetSlurs:         "slurs",
	}
	filterActionNames = map[discord.AutoModerationActionType]string{
		discord.AutoModerationActionTypeBlockMessage:           "block_message",
		discord.AutoModerationActionTypeSendAlertMessage:       "alert",
		discord.AutoModerationActionTypeTimeout:                "timeout",
		discord.AutoModerationActionTypeBlockMemberInteraction: "block_interaction",
	}
)

// nameOf returns the name the table holds for v, or otherName.
func nameOf[K comparable](names map[K]string, v K) string {
	if n, ok := names[v]; ok {
		return n
	}
	return otherName
}

// callerManagesGuild reports whether the roles the bot holds give it the right
// to read the server's filter rules. A role the server's list does not have
// gives nothing; the snapshot reports that inconsistency.
func callerManagesGuild(data *serverData, guildID uint64) bool {
	byID := make(map[snowflake.ID]discord.Permissions, len(data.guild.Roles))
	for _, r := range data.guild.Roles {
		byID[r.ID] = r.Permissions
	}
	perms := byID[snowflake.ID(guildID)]
	for _, id := range data.member.RoleIDs {
		perms |= byID[id]
	}
	return perms.Has(discord.PermissionAdministrator) || perms.Has(discord.PermissionManageGuild)
}

// fetchFilterRules reads the server's filter rules when the bot may read them;
// otherwise the snapshot marks them as not read.
func fetchFilterRules(ctx context.Context, client *bot.Client, token string, guildID uint64, opts clientOptions, data *serverData) error {
	if !callerManagesGuild(data, guildID) {
		return nil
	}
	rulesCtx, rulesCancel := context.WithTimeout(ctx, opts.callTimeout())
	rules, err := client.Rest.GetAutoModerationRules(snowflake.ID(guildID), rest.WithCtx(rulesCtx))
	rulesCancel()
	if err != nil {
		return callError("get filter rules", err, token)
	}
	data.rules = rules
	data.rulesRead = true
	return nil
}

// readSettings fills the verification level and the filter rules into snap, by name.
func readSettings(snap *serverplan.Snapshot, data *serverData, roleByID map[snowflake.ID]discord.Role) error {
	// A level the table does not name stays empty: the plan then sets no level.
	snap.VerificationLevel = verificationLevelNames[data.guild.VerificationLevel]
	snap.FilterRulesRead = data.rulesRead
	channelByID := make(map[snowflake.ID]string, len(data.channels))
	for _, ch := range data.channels {
		channelByID[ch.ID()] = ch.Name()
	}
	for i := range data.rules {
		rule, err := filterRuleOf(&data.rules[i], roleByID, channelByID)
		if err != nil {
			return err
		}
		snap.FilterRules = append(snap.FilterRules, rule)
	}
	return nil
}

// filterRuleOf names the parts of one platform rule. A channel the bot cannot
// see has no name here: the rule then differs from the definition and the plan
// sets it.
func filterRuleOf(r *discord.AutoModerationRule, roleByID map[snowflake.ID]discord.Role, channelByID map[snowflake.ID]string) (serverplan.FilterRule, error) {
	md := r.TriggerMetadata
	rule := serverplan.FilterRule{
		Name:           r.Name,
		Trigger:        nameOf(filterTriggerNames, r.TriggerType),
		Enabled:        r.Enabled,
		Keywords:       md.KeywordFilter,
		Patterns:       md.RegexPatterns,
		AllowList:      md.AllowList,
		MentionLimit:   md.MentionTotalLimit,
		RaidProtection: md.MentionRaidProtectionEnabled,
	}
	for _, p := range md.Presets {
		rule.Presets = append(rule.Presets, nameOf(filterPresetNames, p))
	}
	for _, a := range r.Actions {
		rule.Actions = append(rule.Actions, filterActionOf(a, channelByID))
	}
	for _, id := range r.ExemptRoles {
		role, ok := roleByID[id]
		if !ok {
			return serverplan.FilterRule{}, fmt.Errorf("filter rule %q exempts a role that the server's role list does not have; the server changed during the read", r.Name)
		}
		rule.ExemptRoles = append(rule.ExemptRoles, role.Name)
	}
	for _, id := range r.ExemptChannels {
		rule.ExemptChannels = append(rule.ExemptChannels, channelByID[id])
	}
	return rule, nil
}

func filterActionOf(a discord.AutoModerationAction, channelByID map[snowflake.ID]string) serverplan.FilterAction {
	act := serverplan.FilterAction{Type: nameOf(filterActionNames, a.Type)}
	if a.Metadata == nil {
		return act
	}
	switch a.Type {
	case discord.AutoModerationActionTypeSendAlertMessage:
		act.Channel = channelByID[a.Metadata.ChannelID]
	case discord.AutoModerationActionTypeTimeout:
		act.TimeoutSeconds = a.Metadata.DurationSeconds
	case discord.AutoModerationActionTypeBlockMessage:
		if a.Metadata.CustomMessage != nil {
			act.Message = *a.Metadata.CustomMessage
		}
	}
	return act
}
