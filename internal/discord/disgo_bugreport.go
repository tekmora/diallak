package discord

import (
	"context"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
)

// bugAdapter adapts Discord slash command interactions for BugHandler.
type bugAdapter struct {
	interaction discord.Interaction
	data        discord.SlashCommandInteractionData
	respond     func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error
}

func (a *bugAdapter) GuildID() uint64 {
	return extractGuildID(a.interaction.GuildID())
}

func (a *bugAdapter) Title() string {
	return a.data.String(OptionBugTitle)
}

func (a *bugAdapter) Steps() string {
	return a.data.String(OptionBugSteps)
}

func (a *bugAdapter) Show(ctx context.Context, text string) error {
	return a.respond(discord.InteractionResponseTypeCreateMessage, buildBugReplyPayload(text), rest.WithCtx(ctx))
}

func buildBugReplyPayload(text string) discord.MessageCreate {
	mentions := defaultAllowedMentions()
	return discord.MessageCreate{
		Content:         text,
		Flags:           discord.MessageFlagEphemeral,
		AllowedMentions: &mentions,
	}
}

func buildBugCommandCreate() discord.ApplicationCommandCreate {
	minLen := 1
	maxTitle := MaxBugTitleRunes
	maxSteps := MaxBugStepsRunes
	return discord.SlashCommandCreate{
		Name:        BugCommandName,
		Description: BugCommandDescription,
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        OptionBugTitle,
				Description: "The title of the bug report",
				Required:    true,
				MinLength:   &minLen,
				MaxLength:   &maxTitle,
			},
			discord.ApplicationCommandOptionString{
				Name:        OptionBugSteps,
				Description: "The steps to reproduce the bug",
				Required:    true,
				MinLength:   &minLen,
				MaxLength:   &maxSteps,
			},
		},
	}
}
