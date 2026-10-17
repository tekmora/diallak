package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// areasAdapter is an areas command or a change of one of its menus, delivered by the library.
// command tells the two apart: a command is answered with a new message, a change with an
// update of the message its menu sits on.
type areasAdapter struct {
	client      *bot.Client
	interaction discord.Interaction
	respond     func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error
	command     bool
}

func (a *areasAdapter) GuildID() uint64 {
	return extractGuildID(a.interaction.GuildID())
}

// interactionMemberID returns the id of the member who sent the interaction, or 0 outside a server.
func interactionMemberID(in discord.Interaction) uint64 {
	m := in.Member()
	if m == nil {
		return 0
	}
	return uint64(m.User.ID)
}

// interactionMemberRoleIDs returns the role ids of the member who sent the interaction, or nil
// outside a server.
func interactionMemberRoleIDs(in discord.Interaction) []uint64 {
	m := in.Member()
	if m == nil {
		return nil
	}
	ids := make([]uint64, len(m.RoleIDs))
	for i, id := range m.RoleIDs {
		ids[i] = uint64(id)
	}
	return ids
}

func (a *areasAdapter) MemberID() uint64 {
	return interactionMemberID(a.interaction)
}

func (a *areasAdapter) MemberRoleIDs() []uint64 {
	return interactionMemberRoleIDs(a.interaction)
}

func (a *areasAdapter) Pending() bool {
	m := a.interaction.Member()
	return m != nil && m.Pending
}

func (a *areasAdapter) Show(ctx context.Context, v AreasView) error {
	mentions := defaultAllowedMentions()
	if a.command {
		return a.respond(discord.InteractionResponseTypeCreateMessage, discord.MessageCreate{
			Content:         v.Text,
			Components:      areaComponents(v),
			Flags:           discord.MessageFlagEphemeral,
			AllowedMentions: &mentions,
		}, rest.WithCtx(ctx))
	}
	return a.respond(discord.InteractionResponseTypeUpdateMessage, areaUpdate(v), rest.WithCtx(ctx))
}

func (a *areasAdapter) Acknowledge(ctx context.Context) error {
	return a.respond(discord.InteractionResponseTypeDeferredUpdateMessage, nil, rest.WithCtx(ctx))
}

func (a *areasAdapter) Edit(ctx context.Context, v AreasView) error {
	_, err := a.client.Rest.UpdateInteractionResponse(
		a.interaction.ApplicationID(),
		a.interaction.Token(),
		areaUpdate(v),
		rest.WithCtx(ctx),
	)
	return err
}

func areaUpdate(v AreasView) discord.MessageUpdate {
	mentions := defaultAllowedMentions()
	components := areaComponents(v)
	return discord.MessageUpdate{
		Content:         &v.Text,
		Components:      &components,
		AllowedMentions: &mentions,
	}
}

// areaComponents puts each menu of v in a row of its own. Every menu accepts an empty choice.
func areaComponents(v AreasView) []discord.LayoutComponent {
	rows := make([]discord.LayoutComponent, 0, len(v.Menus))
	minValues := 0
	for _, m := range v.Menus {
		options := make([]discord.StringSelectMenuOption, len(m.Options))
		for i, o := range m.Options {
			options[i] = discord.StringSelectMenuOption{Label: o.Label, Value: o.Value, Default: o.Default}
		}
		rows = append(rows, discord.NewActionRow(discord.StringSelectMenuComponent{
			CustomID:    m.CustomID,
			Placeholder: m.Placeholder,
			MinValues:   &minValues,
			MaxValues:   m.MaxValues,
			Options:     options,
		}))
	}
	return rows
}

// areaChange returns the custom id and the values of a change of an areas menu.
func areaChange(i discord.ComponentInteraction) (string, []string, bool) {
	data, ok := i.Data.(discord.StringSelectMenuInteractionData)
	if !ok || !strings.HasPrefix(data.CustomID(), areasCustomIDPrefix) {
		return "", nil, false
	}
	return data.CustomID(), data.Values, true
}

// restRoleCalls makes the calls of a change against the server, each with the fixed reason the
// audit log shows.
type restRoleCalls struct {
	rest    rest.Rest
	guildID snowflake.ID
}

func (c restRoleCalls) ReadRoles(ctx context.Context) ([]discord.Role, error) {
	roles, err := c.rest.GetRoles(c.guildID, rest.WithCtx(ctx))
	if err != nil {
		return nil, callError("get roles", err, "")
	}
	return roles, nil
}

func (c restRoleCalls) AddRole(ctx context.Context, memberID, roleID uint64) error {
	err := c.rest.AddMemberRole(c.guildID, snowflake.ID(memberID), snowflake.ID(roleID),
		rest.WithCtx(ctx), rest.WithReason(areasAuditReason))
	if err != nil {
		return callError("add member role", err, "")
	}
	return nil
}

func (c restRoleCalls) RemoveRole(ctx context.Context, memberID, roleID uint64) error {
	err := c.rest.RemoveMemberRole(c.guildID, snowflake.ID(memberID), snowflake.ID(roleID),
		rest.WithCtx(ctx), rest.WithReason(areasAuditReason))
	if err != nil {
		return callError("remove member role", err, "")
	}
	return nil
}

// readAreaRoles reads the server's roles once at start, so the first answer to the areas command
// needs no call.
func readAreaRoles(ctx context.Context, client *bot.Client, cfg BotConfig, opts clientOptions) error {
	readCtx, cancel := context.WithTimeout(ctx, opts.callTimeout())
	defer cancel()
	roles, err := restRoleCalls{rest: client.Rest, guildID: snowflake.ID(cfg.GuildID)}.ReadRoles(readCtx)
	if err != nil {
		return fmt.Errorf("read roles for the areas command: %w", sanitizeToken(err, cfg.Token))
	}
	cfg.Areas.SetRoles(roles)
	return nil
}
