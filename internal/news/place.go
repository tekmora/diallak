package news

import "time"

// HoldForPlace holds items for the digest of channel when the place is open on the server and
// returns how many it held. For a place that is not open the items are dropped: Record has marked
// them seen, so an area that opens later starts with the next day's news, not with a backlog
// (ADR-0019, point 3). An empty channel is no place and holds nothing.
func HoldForPlace(s Store, channel string, open bool, items []Item, now time.Time) int {
	if !open || channel == "" || len(items) == 0 {
		return 0
	}
	s.Hold(channel, items, now)
	return min(len(items), MaxItems)
}
