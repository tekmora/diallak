package discord

import (
	"context"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/tekmora/diallak/internal/serverplan"
)

func fetchInvites(ctx context.Context, client *bot.Client, token string, guildID uint64, opts clientOptions, data *serverData) error {
	if !callerManagesGuild(data, guildID) {
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, opts.callTimeout())
	defer cancel()
	invites, err := client.Rest.GetGuildInvites(snowflake.ID(guildID), rest.WithCtx(callCtx))
	if err != nil {
		return callError("get invites", err, token)
	}
	data.invites = invites
	data.invitesRead = true
	return nil
}

func readInvites(snap *serverplan.Snapshot, data *serverData) {
	snap.InvitesRead = data.invitesRead
	names := make(map[snowflake.ID]string, len(data.channels))
	for _, ch := range data.channels {
		names[ch.ID()] = ch.Name()
	}
	for _, invite := range data.invites {
		var name string
		unlisted := false
		if invite.Channel != nil {
			var ok bool
			name, ok = names[invite.Channel.ID]
			if !ok {
				name = invite.Channel.Name
				unlisted = true
			}
		}
		snap.Invites = append(snap.Invites, serverplan.Invite{Code: invite.Code, Channel: name, ChannelUnlisted: unlisted, MaxAge: invite.MaxAge, MaxUses: invite.MaxUses, Temporary: invite.Temporary, ByBot: invite.Inviter != nil && invite.Inviter.ID == data.app.Bot.ID})
	}
}
