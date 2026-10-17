package discord

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/news"
	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	newsAnnouncementID = 501
	newsRoleID         = 502
	newsFeedChannelID  = 503
)

func newsNames() serverdef.NewsNames {
	return serverdef.NewsNames{
		Announcement: "announcements",
		Role:         "Notify: releases",
		Channels:     map[string]string{"ai-news-tools": "ai-news-tools", "media-video": "media-video"},
	}
}

func newsServer(t *testing.T, roles, channels string) *NewsConfig {
	t.Helper()
	client, _, _ := recordingRest(t, map[string]string{
		fmt.Sprintf("/guilds/%d/roles", modGuildID):    roles,
		fmt.Sprintf("/guilds/%d/channels", modGuildID): channels,
	})
	nc := &NewsConfig{Runner: &news.Runner{}, Names: newsNames()}
	nc.wire(client.Rest, modGuildID, time.Second)
	return nc
}

var (
	newsRoles    = fmt.Sprintf(`[{"id":"%d","name":"Notify: releases","permissions":"0"}]`, newsRoleID)
	newsChannels = fmt.Sprintf(`[{"id":"%d","guild_id":"%d","type":5,"name":"announcements"},`+
		`{"id":"%d","guild_id":"%d","type":0,"name":"ai-news-tools"}]`,
		newsAnnouncementID, modGuildID, newsFeedChannelID, modGuildID)
)

// Issue #180: the runner finds its places by name on every read and posts through Discord.
func TestNewsConfig_Positive_PlacesByName(t *testing.T) {
	nc := newsServer(t, newsRoles, newsChannels)
	if _, ok := nc.Runner.Announcer.(*NewsPoster); !ok {
		t.Errorf("Announcer = %T, want *NewsPoster", nc.Runner.Announcer)
	}
	if _, ok := nc.Runner.Poster.(*NewsPoster); !ok {
		t.Errorf("Poster = %T, want *NewsPoster", nc.Runner.Poster)
	}
	got, err := nc.Runner.ReadPlaces(context.Background())
	if err != nil {
		t.Fatalf("ReadPlaces() error = %v", err)
	}
	want := news.Place{Channel: fmt.Sprint(newsAnnouncementID), Role: fmt.Sprint(newsRoleID)}
	if got.Announcement != want {
		t.Errorf("Announcement = %+v, want %+v", got.Announcement, want)
	}
	if len(got.Channels) != 1 || got.Channels["ai-news-tools"] != fmt.Sprint(newsFeedChannelID) {
		t.Errorf("Channels = %v, want only the open feed channel", got.Channels)
	}
}

// Without the announcement channel or the release role the round reads no source; a name held
// twice is no place either.
func TestNewsConfig_Negative_MissingPlace(t *testing.T) {
	twice := fmt.Sprintf(`[{"id":"%d","name":"Notify: releases","permissions":"0"},`+
		`{"id":"%d","name":"Notify: releases","permissions":"0"}]`, newsRoleID, newsRoleID+10)
	cases := map[string][2]string{
		"no role":         {`[]`, newsChannels},
		"role held twice": {twice, newsChannels},
		"no announcement": {newsRoles, `[]`},
		"roles not read":  {`{"message":"no"}`, newsChannels},
	}
	for name, c := range cases {
		nc := newsServer(t, c[0], c[1])
		if _, err := nc.Runner.ReadPlaces(context.Background()); err == nil {
			t.Errorf("%s: ReadPlaces() succeeded", name)
		}
	}
}

// A feed channel held twice is not open, and the announcement still is.
func TestNewsConfig_Boundary_FeedChannelHeldTwice(t *testing.T) {
	channels := fmt.Sprintf(`[{"id":"%d","guild_id":"%d","type":5,"name":"announcements"},`+
		`{"id":"%d","guild_id":"%d","type":0,"name":"ai-news-tools"},{"id":"%d","guild_id":"%d","type":0,"name":"ai-news-tools"}]`,
		newsAnnouncementID, modGuildID, newsFeedChannelID, modGuildID, newsFeedChannelID+10, modGuildID)
	got, err := newsServer(t, newsRoles, channels).Runner.ReadPlaces(context.Background())
	if err != nil || len(got.Channels) != 0 || got.Announcement.Channel != fmt.Sprint(newsAnnouncementID) {
		t.Fatalf("ReadPlaces() = %+v, %v; want the announcement and no feed channel", got, err)
	}
}

// The news runner starts with the bot and ends with it.
func TestStartNews_Positive_EndsWithContext(t *testing.T) {
	client, _, _ := recordingRest(t, map[string]string{})
	s, err := news.OpenFileStore(t.TempDir() + "/news.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg := BotConfig{GuildID: modGuildID, News: &NewsConfig{Runner: &news.Runner{Store: s}, Names: newsNames()},
		Logger: slog.New(slog.DiscardHandler)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runNews(ctx, client.Rest, cfg, clientOptions{})
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runNews did not end with the context")
	}
}
