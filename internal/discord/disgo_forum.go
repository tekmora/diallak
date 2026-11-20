package discord

import (
	"context"
	"errors"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// errNoMessageContentRight stops the start when the forum answer is configured and the application
// may not read message text (ADR-0009 case 11).
var errNoMessageContentRight = errors.New("forum answer: the application lacks the message content right; " +
	"switch it on in the developer portal or leave the forum unconfigured")

// hasMessageContentRight reports whether the application may read message text. Either flag grants
// it; the limited one is the form an application in fewer than 100 servers holds.
func hasMessageContentRight(flags discord.ApplicationFlags) bool {
	return flags.Has(discord.ApplicationFlagGatewayMessageContent) ||
		flags.Has(discord.ApplicationFlagGatewayMessageContentLimited)
}

// handleThreadCreate hands a created thread to the forum answer. The handler compares parent, type,
// mark and tags before it reads anything.
func (d *dispatcher) handleThreadCreate(ctx context.Context, e *events.ThreadCreate) {
	if !d.dispatching.Load() {
		return
	}
	d.cfg.Forum.Dispatch(ctx, forumPostOf(e), e.Thread.Name(), restForumCalls{rest: e.Client().Rest})
}

func forumPostOf(e *events.ThreadCreate) ForumPost {
	tags := make([]uint64, len(e.Thread.AppliedTags))
	for i, t := range e.Thread.AppliedTags {
		tags[i] = uint64(t)
	}
	parentID := uint64(0)
	if parent := e.Thread.ParentID(); parent != nil {
		parentID = uint64(*parent)
	}
	return ForumPost{
		ThreadID:     uint64(e.ThreadID),
		ParentID:     parentID,
		PublicThread: e.Thread.Type() == discord.ChannelTypeGuildPublicThread,
		NewlyCreated: e.NewlyCreated,
		TagIDs:       tags,
		TagNames:     forumTagNames(e, parentID, tags),
	}
}

func forumTagNames(e *events.ThreadCreate, parentID uint64, ids []uint64) []string {
	channel, ok := e.Client().Caches.Channel(snowflake.ID(parentID))
	if !ok {
		return nil
	}
	return forumTagNamesFrom(channel, ids)
}

func forumTagNamesFrom(channel discord.GuildChannel, ids []uint64) []string {
	var forum discord.GuildForumChannel
	switch value := channel.(type) {
	case discord.GuildForumChannel:
		forum = value
	case *discord.GuildForumChannel:
		if value == nil {
			return nil
		}
		forum = *value
	default:
		return nil
	}
	names := make([]string, len(ids))
	for i, id := range ids {
		found := false
		for _, tag := range forum.AvailableTags {
			if uint64(tag.ID) == id {
				names[i] = tag.Name
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return names
}

// restForumCalls are the forum calls over the platform's HTTP API. The first message of a forum
// post has the id of the post.
type restForumCalls struct {
	rest rest.Rest
}

func (c restForumCalls) FirstMessage(ctx context.Context, threadID uint64) (ForumMessage, error) {
	id := snowflake.ID(threadID)
	m, err := c.rest.GetMessage(id, id, rest.WithCtx(ctx))
	if err != nil {
		return ForumMessage{}, err
	}
	return ForumMessage{
		Content:         m.Content,
		FromApplication: m.Author.Bot || m.WebhookID != nil || m.ApplicationID != nil,
	}, nil
}

func (c restForumCalls) Reply(ctx context.Context, threadID uint64, text string) error {
	mentions := defaultAllowedMentions()
	_, err := c.rest.CreateMessage(snowflake.ID(threadID),
		discord.MessageCreate{Content: text, AllowedMentions: &mentions}, rest.WithCtx(ctx))
	return err
}
