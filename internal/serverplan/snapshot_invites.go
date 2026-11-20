package serverplan

import (
	"errors"
	"fmt"
	"strings"
)

// Invite holds only the fields used to compare an invite.
type Invite struct {
	Code      string `json:"code"`
	Channel   string `json:"channel"`
	MaxAge    int    `json:"max_age"`
	MaxUses   int    `json:"max_uses"`
	Temporary bool   `json:"temporary"`
	ByBot     bool   `json:"by_bot"`
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
		if strings.TrimSpace(invite.Channel) == "" {
			errs = append(errs, fmt.Errorf("empty invite channel at index %d", i))
		}
	}
	return errs
}
