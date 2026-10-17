package serverdef

import (
	"fmt"
	"net/url"
	"regexp"
)

// repositoryRegexp matches "owner/name" with GitHub's characters for both parts.
var repositoryRegexp = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)

func (v *validator) checkNews() {
	n := v.def.News
	if n == nil {
		return
	}
	checkNewsRepositories(v, n.Repositories)

	if len(n.Feeds) > MaxNewsFeeds {
		v.add(CodeNews, "news.feeds", fmt.Sprintf("%d feeds, at most %d", len(n.Feeds), MaxNewsFeeds))
	}
	ids := make([]string, 0, len(n.Feeds))
	areas := make(map[string]struct{}, len(v.def.Areas))
	for _, a := range v.def.Areas {
		areas[a.ID] = struct{}{}
	}
	for _, f := range n.Feeds {
		ids = append(ids, f.ID)
		p := fmt.Sprintf("news.feeds[%s]", f.ID)
		if !httpsAddress(f.URL) {
			v.add(CodeNews, p+".url", fmt.Sprintf("feed address %q is not an absolute https address", f.URL))
		}
		if f.Area == "" {
			v.add(CodeNews, p+".area", "feed names no area")
		} else if _, ok := areas[f.Area]; !ok {
			v.add(CodeNews, p+".area", fmt.Sprintf("feed area %q is not an area of the definition", f.Area))
		}
	}
	checkItemIDs(v, "news.feeds", ids)
}

func checkNewsRepositories(v *validator, repos []string) {
	if len(repos) == 0 {
		return
	}
	if len(repos) > MaxNewsRepositories {
		v.add(CodeNews, "news.repositories",
			fmt.Sprintf("%d repositories, at most %d", len(repos), MaxNewsRepositories))
	}
	if ch, ok := v.def.AnnouncementChannel(); !ok {
		v.add(CodeNews, "news.repositories", "own repositories need exactly one announcement channel")
	} else {
		checkAnnouncementBot(v, ch)
	}
	if findRole(v.def.Roles, ReleaseRole) == nil {
		v.add(CodeNews, "news.repositories", fmt.Sprintf("own repositories need the release role %q", ReleaseRole))
	}
	seen := make(map[string]struct{}, len(repos))
	for _, r := range repos {
		p := fmt.Sprintf("news.repositories[%s]", r)
		if !repositoryRegexp.MatchString(r) {
			v.add(CodeNews, p, fmt.Sprintf("repository %q is not owner/name", r))
		}
		if _, ok := seen[r]; ok {
			v.add(CodeNews, p, fmt.Sprintf("repository %q is listed twice", r))
		}
		seen[r] = struct{}{}
	}
}

// checkAnnouncementBot asks that the bot can post into announcement channel ch and crosspost
// its own posts: Discord asks SEND_MESSAGES for both (discord-api-docs 2d337db, "Crosspost
// Message").
func checkAnnouncementBot(v *validator, ch string) {
	botRole := findRoleByKind(v.def.Roles, "bot")
	if botRole == nil {
		return
	}
	eff := Effective(v.def, botRole.ID, ch)
	for _, perm := range []string{"VIEW_CHANNEL", "SEND_MESSAGES"} {
		if _, ok := eff[perm]; !ok {
			v.add(CodeNews, "news.repositories",
				fmt.Sprintf("bot lacks %s in announcement channel %s", perm, ch))
		}
	}
}

func httpsAddress(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}
