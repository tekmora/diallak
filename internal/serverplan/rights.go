package serverplan

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

// heldRights is the set of rights the caller holds at a point of the plan.
// The snapshot's set joins the rights of @everyone with those of the caller's
// roles; a step that sets @everyone's rights changes the part that comes from
// @everyone. A right @everyone holds and the caller's own role does not counts
// as held through @everyone, even when another role of the caller may hold it
// as well: the snapshot does not say, and the plan then refuses rather than
// fail in the apply.
type heldRights struct {
	apart     map[string]struct{}
	everyone  map[string]struct{}
	removedBy map[string]int
}

func newHeldRights(s *Snapshot, callerIdx int) heldRights {
	everyoneSnap := map[string]struct{}{}
	for i := range s.Roles {
		if s.Roles[i].Everyone {
			for _, p := range s.Roles[i].Permissions {
				everyoneSnap[p] = struct{}{}
			}
			break
		}
	}
	var ownRole []string
	if callerIdx >= 0 {
		ownRole = s.Roles[callerIdx].Permissions
	}

	h := heldRights{
		apart:     make(map[string]struct{}, len(s.Caller.Permissions)),
		everyone:  make(map[string]struct{}, len(everyoneSnap)),
		removedBy: map[string]int{},
	}
	for _, p := range s.Caller.Permissions {
		_, viaEveryone := everyoneSnap[p]
		if viaEveryone && !slices.Contains(ownRole, p) {
			h.everyone[p] = struct{}{}
			continue
		}
		h.apart[p] = struct{}{}
	}
	return h
}

func (h heldRights) holds(right string) bool {
	if _, ok := h.apart[right]; ok {
		return true
	}
	_, ok := h.everyone[right]
	return ok
}

// setEveryone records that step gives @everyone the rights perms. A right the
// caller does not hold before the step does not count after it either: the
// step needs every right it adds, so such a right is one the snapshot did not
// give the caller.
func (h *heldRights) setEveryone(step int, perms []string) {
	next := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		if h.holds(p) {
			next[p] = struct{}{}
			delete(h.removedBy, p)
		}
	}
	for p := range h.everyone {
		if _, kept := next[p]; !kept {
			if _, ok := h.apart[p]; !ok {
				h.removedBy[p] = step
			}
		}
	}
	h.everyone = next
}

// lacking returns the sorted rights of needs the caller does not hold, and a
// clause per earlier step that took one of them from @everyone.
func (h heldRights) lacking(needs []string) (string, bool) {
	if h.holds("ADMINISTRATOR") {
		return "", false
	}
	var names []string
	byStep := map[int][]string{}
	for _, n := range needs {
		if h.holds(n) {
			continue
		}
		names = append(names, n)
		if step, ok := h.removedBy[n]; ok {
			byStep[step] = append(byStep[step], n)
		}
	}
	if len(names) == 0 {
		return "", false
	}
	sort.Strings(names)
	parts := []string{strings.Join(names, ", ")}
	steps := make([]int, 0, len(byStep))
	for step := range byStep {
		steps = append(steps, step)
	}
	sort.Ints(steps)
	for _, step := range steps {
		taken := byStep[step]
		sort.Strings(taken)
		parts = append(parts, fmt.Sprintf("step %d takes %s from @everyone", step, strings.Join(taken, ", ")))
	}
	return strings.Join(parts, "; "), true
}

// everyoneRole returns the definition's role of kind everyone, or nil.
func everyoneRole(def *serverdef.Definition) *serverdef.Role {
	for i := range def.Roles {
		if def.Roles[i].Kind == "everyone" {
			return &def.Roles[i]
		}
	}
	return nil
}

// everyoneID returns the id of the definition's role of kind everyone, or "".
func everyoneID(def *serverdef.Definition) string {
	if r := everyoneRole(def); r != nil {
		return r.ID
	}
	return ""
}
