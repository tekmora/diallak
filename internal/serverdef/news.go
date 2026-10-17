package serverdef

const (
	// MaxNewsFeeds caps the feeds of the news function (ADR-0019, point 2).
	MaxNewsFeeds = 20
	// MaxNewsRepositories caps the own repositories: four unauthenticated requests an hour each
	// within GitHub's 60 (ADR-0019, point 1).
	MaxNewsRepositories = 10
)

// News lists what the news function reads (ADR-0019, point 2). Own releases and security
// advisories go to the announcement channel; every other feed goes to the area it serves.
type News struct {
	// Repositories are own public repositories as "owner/name".
	Repositories []string `json:"repositories"`
	// Feeds are the other feeds, each with the area it serves.
	Feeds []NewsFeed `json:"feeds"`
}

// NewsFeed is one feed and the area whose channel gets its digest.
type NewsFeed struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Area string `json:"area"`
}

// FeedChannel returns the id of the channel that gets the digest of feed id: the first channel
// of the area the feed serves. ok is false for a feed the definition does not list or whose area
// has no channel, so a topic without an area has no place.
func (d *Definition) FeedChannel(id string) (channel string, ok bool) {
	if d.News == nil {
		return "", false
	}
	for _, f := range d.News.Feeds {
		if f.ID != id {
			continue
		}
		for _, ch := range d.Channels {
			if ch.Area == f.Area {
				return ch.ID, true
			}
		}
		return "", false
	}
	return "", false
}

// AnnouncementChannel returns the id of the channel of type announcement, where own releases and
// security advisories go. ok is false unless the definition has exactly one.
func (d *Definition) AnnouncementChannel() (channel string, ok bool) {
	count := 0
	for _, ch := range d.Channels {
		if ch.Type == "announcement" {
			channel = ch.ID
			count++
		}
	}
	if count != 1 {
		return "", false
	}
	return channel, true
}
