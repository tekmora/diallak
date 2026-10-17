package discord

import (
	"context"
	"fmt"
	"strconv"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// NewsPoster posts the news digests and the announcements of own releases and advisories
// (ADR-0019, points 4 and 6). It implements news.Poster and news.Announcer.
type NewsPoster struct {
	rest rest.Rest
}

// NewNewsPoster returns a NewsPoster that calls Discord through r.
func NewNewsPoster(r rest.Rest) *NewsPoster {
	return &NewsPoster{rest: r}
}

// PostDigest posts content into channelID; the post mentions nobody.
func (p *NewsPoster) PostDigest(ctx context.Context, channelID, content string) error {
	channel, err := newsID("channel", channelID)
	if err != nil {
		return err
	}
	mentions := defaultAllowedMentions()
	_, err = p.rest.CreateMessage(channel, discord.MessageCreate{
		Content:         content,
		AllowedMentions: &mentions,
	}, rest.WithCtx(ctx))
	return discordCallError("create digest message", err)
}

// PostAnnouncement posts content into channelID and returns the message's id. The post may
// mention role roleID and nothing else.
func (p *NewsPoster) PostAnnouncement(ctx context.Context, channelID, roleID, content string) (string, error) {
	channel, err := newsID("channel", channelID)
	if err != nil {
		return "", err
	}
	role, err := newsID("role", roleID)
	if err != nil {
		return "", err
	}
	mentions := defaultAllowedMentions()
	mentions.Roles = []snowflake.ID{role}
	msg, err := p.rest.CreateMessage(channel, discord.MessageCreate{
		Content:         content,
		AllowedMentions: &mentions,
	}, rest.WithCtx(ctx))
	if err != nil {
		return "", discordCallError("create announcement message", err)
	}
	return msg.ID.String(), nil
}

// Crosspost publishes the bot's own message messageID of announcement channel channelID to the
// servers that follow the channel.
func (p *NewsPoster) Crosspost(ctx context.Context, channelID, messageID string) error {
	channel, err := newsID("channel", channelID)
	if err != nil {
		return err
	}
	message, err := newsID("message", messageID)
	if err != nil {
		return err
	}
	_, err = p.rest.CrosspostMessage(channel, message, rest.WithCtx(ctx))
	return discordCallError("crosspost message", err)
}

// newsID reads a Discord id written in decimal; zero is no id.
func newsID(what, s string) (snowflake.ID, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("%s %q is not a Discord id", what, s)
	}
	return snowflake.ID(n), nil
}
