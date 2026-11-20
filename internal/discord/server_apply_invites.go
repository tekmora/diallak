package discord

import (
	"context"
	"fmt"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
)

func inviteForChange(def *serverdef.Definition, c serverplan.Change) (*serverdef.Invite, error) {
	id, err := extractBracketedID(c.Object, "invites[")
	if err != nil {
		return nil, fmt.Errorf("step %d %s: %w", c.Step, c.Action, err)
	}
	for i := range def.Invites {
		entry := &def.Invites[i]
		if entry.ID == id {
			if entry.Code != "" {
				return nil, fmt.Errorf("invite %q already has a recorded code", id)
			}
			return entry, nil
		}
	}
	return nil, fmt.Errorf("invite %q not found in definition", id)
}

func validateInviteChange(def *serverdef.Definition, c serverplan.Change) error {
	_, err := inviteForChange(def, c)
	return err
}

func applyCreateInvite(ctx context.Context, client *bot.Client, def *serverdef.Definition, c serverplan.Change, idx *serverIndex, reason string) error {
	entry, err := inviteForChange(def, c)
	if err != nil {
		return err
	}
	channelID, err := lookupDefChannel(def, entry.Channel, idx)
	if err != nil {
		return fmt.Errorf("resolve invite channel: %w", err)
	}
	maxAge, maxUses := entry.MaxAge, entry.MaxUses
	invite, err := client.Rest.CreateInvite(channelID, discord.InviteCreate{MaxAge: &maxAge, MaxUses: &maxUses, Temporary: entry.Temporary}, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("create invite", err, client.Token)
	}
	idx.createdInviteCode = invite.Code
	return nil
}
