package serverdef

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The tests in this file pin where news goes (ADR-0019, points 2 and 3; issue #96): a feed posts
// into the channel of the area it serves, own releases and advisories into the announcement
// channel, and a topic without an area has no place.

func newsFeeds(n int, area string) []NewsFeed {
	feeds := make([]NewsFeed, 0, n)
	for i := range n {
		feeds = append(feeds, NewsFeed{
			ID:   fmt.Sprintf("feed-%02d", i),
			URL:  fmt.Sprintf("https://example.org/feed-%02d.xml", i),
			Area: area,
		})
	}
	return feeds
}

func newsRepositories(n int) []string {
	repos := make([]string, 0, n)
	for i := range n {
		repos = append(repos, fmt.Sprintf("tekmora/repo-%02d", i))
	}
	return repos
}

// setBotAccess replaces the bot's entry in channel id's access with one that allows allow and
// denies deny.
func setBotAccess(t *testing.T, def *Definition, id string, allow, deny []string) {
	t.Helper()
	ch := channelOf(t, def, id)
	access := []Access{{Role: "bot", Allow: allow, Deny: deny}}
	for _, a := range ch.Access {
		if a.Role != "bot" {
			access = append(access, a)
		}
	}
	ch.Access = access
}

func newsFindings(findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if strings.HasPrefix(f.Path, "news") {
			out = append(out, f)
		}
	}
	return out
}

func TestValidate_Positive_CommittedNewsBlock(t *testing.T) {
	def := valid(t)
	if def.News == nil {
		t.Fatal("the committed definition holds no news block")
	}
	if got := newsFindings(Validate(def)); len(got) != 0 {
		t.Errorf("the committed news block has findings %+v", got)
	}
	want := []string{"tekmora/diallak", "tekmora/epitrop"}
	if !slices.Equal(def.News.Repositories, want) {
		t.Errorf("own repositories = %v, want %v", def.News.Repositories, want)
	}
	if len(def.News.Feeds) != 0 {
		t.Errorf("feeds = %+v; the operator has named no source yet", def.News.Feeds)
	}
}

func TestNewsPlace_Positive_EachTopicToItsPlace(t *testing.T) {
	def := valid(t)
	def.News = &News{
		Repositories: []string{"tekmora/diallak"},
		Feeds: []NewsFeed{
			{ID: "ai", URL: "https://example.org/ai.xml", Area: "ai-news-tools"},
			{ID: "video", URL: "https://example.org/video.xml", Area: "media-video"},
		},
	}
	if got := newsFindings(Validate(def)); len(got) != 0 {
		t.Fatalf("findings %+v", got)
	}
	for feed, want := range map[string]string{"ai": "ai-news-tools", "video": "media-video"} {
		got, ok := def.FeedChannel(feed)
		if !ok || got != want {
			t.Errorf("FeedChannel(%q) = %q, %v; want %q", feed, got, ok, want)
		}
	}
	got, ok := def.AnnouncementChannel()
	if !ok || got != "announcements" {
		t.Errorf("AnnouncementChannel() = %q, %v; want announcements", got, ok)
	}
}

func TestNewsPlace_Negative_UnknownFeedHasNoPlace(t *testing.T) {
	def := valid(t)
	def.News = &News{Feeds: newsFeeds(1, "ai-news-tools")}
	if got, ok := def.FeedChannel("frameworks"); ok {
		t.Errorf("FeedChannel(frameworks) = %q, want no place", got)
	}
	def.News = nil
	if got, ok := def.FeedChannel("feed-00"); ok {
		t.Errorf("FeedChannel without a news block = %q, want no place", got)
	}
}

func TestValidate_Negative_NewsFeeds(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"topic without an area", func(t *testing.T, def *Definition) {
			def.News.Feeds = []NewsFeed{{ID: "frameworks", URL: "https://example.org/f.xml", Area: "frameworks"}}
		}, CodeNews, "news.feeds[frameworks].area", "frameworks"},
		{"no area", func(t *testing.T, def *Definition) {
			def.News.Feeds = []NewsFeed{{ID: "ai", URL: "https://example.org/f.xml"}}
		}, CodeNews, "news.feeds[ai].area", "area"},
		{"plain http", func(t *testing.T, def *Definition) {
			def.News.Feeds = []NewsFeed{{ID: "ai", URL: "http://example.org/f.xml", Area: "ai-news-tools"}}
		}, CodeNews, "news.feeds[ai].url", "https"},
		{"relative address", func(t *testing.T, def *Definition) {
			def.News.Feeds = []NewsFeed{{ID: "ai", URL: "/f.xml", Area: "ai-news-tools"}}
		}, CodeNews, "news.feeds[ai].url", "https"},
		{"https without a host", func(t *testing.T, def *Definition) {
			def.News.Feeds = []NewsFeed{{ID: "ai", URL: "https:///f.xml", Area: "ai-news-tools"}}
		}, CodeNews, "news.feeds[ai].url", "https"},
		{"bad id", func(t *testing.T, def *Definition) {
			def.News.Feeds = []NewsFeed{{ID: "AI", URL: "https://example.org/f.xml", Area: "ai-news-tools"}}
		}, CodeID, "news.feeds[AI]", "regex"},
		{"duplicate id", func(t *testing.T, def *Definition) {
			def.News.Feeds = append(newsFeeds(1, "ai-news-tools"), newsFeeds(1, "media-video")...)
		}, CodeID, "news.feeds[feed-00]", "duplicate"},
		{"too many feeds", func(t *testing.T, def *Definition) {
			def.News.Feeds = newsFeeds(MaxNewsFeeds+1, "ai-news-tools")
		}, CodeNews, "news.feeds", "20"},
		{"too many repositories", func(t *testing.T, def *Definition) {
			def.News.Repositories = newsRepositories(MaxNewsRepositories + 1)
		}, CodeNews, "news.repositories", "10"},
		{"repository not owner/name", func(t *testing.T, def *Definition) {
			def.News.Repositories = []string{"diallak"}
		}, CodeNews, "news.repositories[diallak]", "owner/name"},
		{"repository twice", func(t *testing.T, def *Definition) {
			def.News.Repositories = []string{"tekmora/diallak", "tekmora/diallak"}
		}, CodeNews, "news.repositories[tekmora/diallak]", "twice"},
		{"repositories without an announcement channel", func(t *testing.T, def *Definition) {
			def.News.Repositories = []string{"tekmora/diallak"}
			channelOf(t, def, "announcements").Type = "text"
		}, CodeNews, "news.repositories", "announcement"},
		{"two announcement channels", func(t *testing.T, def *Definition) {
			def.News.Repositories = []string{"tekmora/diallak"}
			channelOf(t, def, "home-lab").Type = "announcement"
		}, CodeNews, "news.repositories", "announcement"},
		// The bot posts and crossposts its own announcements: Discord asks SEND_MESSAGES for both
		// (discord-api-docs 2d337db, "Crosspost Message").
		{"bot cannot see the announcement channel", func(t *testing.T, def *Definition) {
			setBotAccess(t, def, "announcements", []string{"SEND_MESSAGES"}, []string{"VIEW_CHANNEL"})
		}, CodeNews, "news.repositories", "VIEW_CHANNEL"},
		{"bot cannot post into the announcement channel", func(t *testing.T, def *Definition) {
			setBotAccess(t, def, "announcements", []string{"VIEW_CHANNEL"}, []string{})
		}, CodeNews, "news.repositories", "SEND_MESSAGES"},
	})
}

func TestValidate_Boundary_NewsLimits(t *testing.T) {
	def := valid(t)
	def.News = &News{
		Repositories: newsRepositories(MaxNewsRepositories),
		Feeds:        newsFeeds(MaxNewsFeeds, "ai-news-tools"),
	}
	if got := newsFindings(Validate(def)); len(got) != 0 {
		t.Errorf("%d feeds and %d repositories have findings %+v", MaxNewsFeeds, MaxNewsRepositories, got)
	}
	if got, ok := def.FeedChannel(fmt.Sprintf("feed-%02d", MaxNewsFeeds-1)); !ok || got != "ai-news-tools" {
		t.Errorf("the last feed has place %q, %v", got, ok)
	}
}

// Issue #180: the bot finds the news places on the server by the names the definition gives them.
func TestNewsNames_Positive_PlacesByName(t *testing.T) {
	def := valid(t)
	def.News = &News{
		Repositories: []string{"tekmora/diallak"},
		Feeds:        []NewsFeed{{ID: "ai", URL: "https://example.org/ai.xml", Area: "ai-news-tools"}},
	}
	got, ok := def.NewsNames()
	if !ok {
		t.Fatal("NewsNames() not ok")
	}
	if got.Announcement != "announcements" || got.Role != "Notify: releases" {
		t.Errorf("NewsNames() = %+v, want the announcement channel and the release role by name", got)
	}
	if name := got.Channels["ai-news-tools"]; name != channelOf(t, def, "ai-news-tools").Name || len(got.Channels) != 1 {
		t.Errorf("NewsNames().Channels = %v, want only the feed's channel by name", got.Channels)
	}
}

func TestNewsNames_Negative_MissingPlace(t *testing.T) {
	def := valid(t)
	def.Roles = slices.DeleteFunc(def.Roles, func(r Role) bool { return r.ID == ReleaseRole })
	if _, ok := def.NewsNames(); ok {
		t.Error("NewsNames() ok without the release role")
	}
	def = valid(t)
	for i := range def.Channels {
		if def.Channels[i].Type == "announcement" {
			def.Channels[i].Type = "text"
		}
	}
	if _, ok := def.NewsNames(); ok {
		t.Error("NewsNames() ok without an announcement channel")
	}
	def = valid(t)
	def.News = nil
	if got, ok := def.NewsNames(); !ok || len(got.Channels) != 0 {
		t.Errorf("NewsNames() without a news block = %+v, %v; want the places and no feed channel", got, ok)
	}
}

// Own repositories need the release role their posts mention.
func TestValidate_Negative_NewsNeedsReleaseRole(t *testing.T) {
	def := valid(t)
	def.Roles = slices.DeleteFunc(def.Roles, func(r Role) bool { return r.ID == ReleaseRole })
	def.RoleOrder = slices.DeleteFunc(def.RoleOrder, func(id string) bool { return id == ReleaseRole })
	found := false
	for _, f := range newsFindings(Validate(def)) {
		found = found || strings.Contains(f.Message, ReleaseRole)
	}
	if !found {
		t.Errorf("findings %+v, want one naming %s", newsFindings(Validate(def)), ReleaseRole)
	}
}

func TestNewsNames_Positive_DiscussionsChannel(t *testing.T) {
	def := valid(t)
	def.News.DiscussionsChannel = "ai-news-tools"
	got, ok := def.NewsNames()
	if !ok || got.Discussions != channelOf(t, def, "ai-news-tools").Name {
		t.Errorf("NewsNames().Discussions = %q, %v; want %q", got.Discussions, ok, channelOf(t, def, "ai-news-tools").Name)
	}
	def.News.DiscussionsChannel = ""
	got, ok = def.NewsNames()
	if !ok || got.Discussions != "" {
		t.Errorf("NewsNames().Discussions when unset = %q, %v; want empty", got.Discussions, ok)
	}
}

func TestValidate_Positive_DiscussionsChannel(t *testing.T) {
	def := valid(t)
	def.News.DiscussionsChannel = "ai-news-tools"
	if got := newsFindings(Validate(def)); len(got) != 0 {
		t.Errorf("Validate() with valid text discussions channel findings = %+v", got)
	}
}

func TestValidate_Negative_DiscussionsChannelRefused(t *testing.T) {
	cases := []struct {
		name, chID, wantPart string
	}{
		{"unknown id", "nonexistent-channel", "not a channel of the definition"},
		{"announcement type", "announcements", "want text"},
		{"forum type", "support", "want text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := valid(t)
			def.News.DiscussionsChannel = tc.chID
			findings := newsFindings(Validate(def))
			found := false
			for _, f := range findings {
				if f.Path == "news.discussions_channel" && strings.Contains(f.Message, tc.wantPart) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: findings = %+v, want news.discussions_channel containing %q", tc.name, findings, tc.wantPart)
			}
		})
	}
}

// The shipped definition posts the bridge into the lounge, a text channel (ledger diallak-99-channel).
func TestNews_Positive_ShippedDiscussionsChannelIsLounge(t *testing.T) {
	def := valid(t)
	if def.News.DiscussionsChannel != "lounge" {
		t.Fatalf("news.discussions_channel = %q, want lounge", def.News.DiscussionsChannel)
	}
	if ch := channelOf(t, def, "lounge"); ch.Type != "text" {
		t.Errorf("lounge type = %q, want text", ch.Type)
	}
}
