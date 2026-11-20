package serverplan

import (
	"errors"
	"fmt"
	"strings"
)

// Invite holds only the fields used to compare an invite.
type Invite struct {
	// Code is the invite code.
	Code string `json:"code"`
	// Channel is the channel name.
	Channel string `json:"channel"`
	// ChannelUnlisted is true when the channel was not listed; Channel holds the reported name.
	ChannelUnlisted bool `json:"channel_unlisted,omitempty"`
	// MaxAge is the lifetime in seconds.
	MaxAge int `json:"max_age"`
	// MaxUses is the use limit.
	MaxUses int `json:"max_uses"`
	// Temporary reports whether membership is temporary.
	Temporary bool `json:"temporary"`
	// ByBot reports whether the bot created the invite.
	ByBot bool `json:"by_bot"`
}

func checkInvites(s *Snapshot) []error {
	var errs []error
	if !s.InvitesRead && len(s.Invites) > 0 {
		errs = append(errs, errors.New("invites listed while marked as not read"))
	}
	for i, invite := range s.Invites {
		if strings.TrimSpace(invite.Code) == "" {
			errs = append(errs, fmt.Errorf("empty invite code at index %d", i))
		}
		if invite.ByBot && strings.TrimSpace(invite.Channel) == "" {
			errs = append(errs, fmt.Errorf("empty invite channel at index %d", i))
		}
	}
	return errs
}
