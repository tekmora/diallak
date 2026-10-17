package serverplan

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

// otherName stands for a trigger or an action the platform has and the table does not name.
const otherName = "other"

// checkSettings reports a verification level, a filter rule or an action the
// snapshot cannot hold.
func checkSettings(s *Snapshot) []error {
	var errs []error
	if s.VerificationLevel != "" && !slices.Contains(serverdef.VerificationLevels(), s.VerificationLevel) {
		errs = append(errs, fmt.Errorf("unknown verification level %q", s.VerificationLevel))
	}
	if !s.FilterRulesRead && len(s.FilterRules) > 0 {
		errs = append(errs, errors.New("filter rules listed while marked as not read"))
	}
	triggers := append(serverdef.FilterTriggers(), otherName)
	actions := append(serverdef.FilterActionTypes(), otherName)
	for i := range s.FilterRules {
		r := &s.FilterRules[i]
		if strings.TrimSpace(r.Name) == "" {
			errs = append(errs, fmt.Errorf("empty filter rule name at index %d", i))
		}
		if !slices.Contains(triggers, r.Trigger) {
			errs = append(errs, fmt.Errorf("filter rule %q: unknown trigger %q", r.Name, r.Trigger))
		}
		for _, a := range r.Actions {
			if !slices.Contains(actions, a.Type) {
				errs = append(errs, fmt.Errorf("filter rule %q: unknown action %q", r.Name, a.Type))
			}
		}
	}
	return errs
}
