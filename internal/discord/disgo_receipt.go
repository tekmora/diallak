package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
)

// receiptAdapter is a press of a receipt button, made against Discord.
type receiptAdapter struct {
	client      *bot.Client
	interaction discord.ComponentInteraction
	respond     func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error
}

func (a *receiptAdapter) Show(ctx context.Context, text, directKey string) error {
	mentions := defaultAllowedMentions()
	return a.respond(discord.InteractionResponseTypeCreateMessage, discord.MessageCreate{
		Content:         text,
		Components:      receiptComponents(receiptDirectCustomIDPrefix, ReceiptDirectButtonLabel, directKey),
		Flags:           discord.MessageFlagEphemeral,
		AllowedMentions: &mentions,
	}, rest.WithCtx(ctx))
}

func (a *receiptAdapter) Defer(ctx context.Context) error {
	return a.respond(discord.InteractionResponseTypeDeferredCreateMessage, buildDeferPayload(), rest.WithCtx(ctx))
}

func (a *receiptAdapter) Edit(ctx context.Context, text string) error {
	_, err := a.client.Rest.UpdateInteractionResponse(
		a.interaction.ApplicationID(),
		a.interaction.Token(),
		buildReplyPayload(text, ""),
		rest.WithCtx(ctx),
	)
	return err
}

func (a *receiptAdapter) SendDirect(ctx context.Context, text string) error {
	ch, err := a.client.Rest.CreateDMChannel(a.interaction.User().ID, rest.WithCtx(ctx))
	if err != nil {
		return directError("create direct message channel", err)
	}
	mentions := defaultAllowedMentions()
	_, err = a.client.Rest.CreateMessage(ch.ID(), discord.MessageCreate{
		Content:         text,
		AllowedMentions: &mentions,
	}, rest.WithCtx(ctx))
	if err != nil {
		return directError("create direct message", err)
	}
	return nil
}

// directError reports ErrDirectClosed when Discord refused the direct message because the member's
// direct messages are closed (code 50007), and the failed call otherwise.
func directError(call string, err error) error {
	var restErr *rest.Error
	if errors.As(err, &restErr) && restErr.Code == rest.JSONErrorCodeCannotSendMessagesToThisUser {
		return ErrDirectClosed
	}
	return fmt.Errorf("%s: %w", call, err)
}

// receiptComponents is the row with one button carrying key, or no row when key is empty.
func receiptComponents(prefix, label, key string) []discord.LayoutComponent {
	if key == "" {
		return nil
	}
	return []discord.LayoutComponent{
		discord.NewActionRow(discord.NewSecondaryButton(label, prefix+key)),
	}
}

// receiptButton returns the prefix and the key of a press of a receipt button.
func receiptButton(i discord.ComponentInteraction) (string, string, bool) {
	data, ok := i.Data.(discord.ButtonInteractionData)
	if !ok {
		return "", "", false
	}
	return receiptButtonID(data.CustomID())
}

// receiptButtonID splits the custom id of a receipt button into its prefix and its key.
func receiptButtonID(id string) (string, string, bool) {
	for _, prefix := range []string{receiptDirectCustomIDPrefix, receiptCustomIDPrefix} {
		if key, found := strings.CutPrefix(id, prefix); found {
			return prefix, key, true
		}
	}
	return "", "", false
}
