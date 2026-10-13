// Package serverplan provides offline comparison, planning, and rendering against server snapshots.
package serverplan

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

// Snapshot represents the observed state of a Discord server as data.
type Snapshot struct {
	Features         []string   `json:"features"`
	Caller           Caller     `json:"caller"`
	Roles            []Role     `json:"roles"`
	Categories       []Category `json:"categories"`
	Channels         []Channel  `json:"channels"`
	ChannelsComplete bool       `json:"channels_complete"`
}

// Caller identifies the inspecting identity, its role, and its effective permissions.
// Position is the place number the platform gives the caller's role; the
// everyone role holds 0, and roles may share a number. The order step numbers
// the roles below the caller's role from 1 without gaps, so it needs fewer
// roles there than this number.
type Caller struct {
	Role        string   `json:"role"`
	Position    int      `json:"position"`
	Permissions []string `json:"permissions"`
}

// Role describes an observed role and its server-wide permissions.
// UnknownBits holds the numbers of the bits the role has set that the
// permission table does not name, in rising order.
type Role struct {
	Name        string   `json:"name"`
	Everyone    bool     `json:"everyone"`
	Managed     bool     `json:"managed"`
	ShownApart  bool     `json:"shown_apart,omitempty"`
	Mentionable bool     `json:"mentionable,omitempty"`
	Permissions []string `json:"permissions"`
	UnknownBits []int    `json:"unknown_bits,omitempty"`
}

// Category represents a channel category in the snapshot.
type Category struct {
	Name string `json:"name"`
}

// Channel represents an observed channel, its type, parent category, and permission overwrites.
type Channel struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Category      string   `json:"category"`
	Access        []Access `json:"access"`
	MemberEntries int      `json:"member_entries"`
}

// Access describes the permissions granted or denied to a role in a channel.
// UnknownAllow and UnknownDeny hold the numbers of the bits the entry has set
// that the permission table does not name, in rising order.
type Access struct {
	Role         string   `json:"role"`
	Allow        []string `json:"allow"`
	Deny         []string `json:"deny"`
	UnknownAllow []int    `json:"unknown_allow,omitempty"`
	UnknownDeny  []int    `json:"unknown_deny,omitempty"`
}

var allowedChannelTypes = map[string]struct{}{
	"text":         {},
	"announcement": {},
	"forum":        {},
	"voice":        {},
	"other":        {},
}

// LoadSnapshot decodes a snapshot strictly from r and verifies its structural invariants.
func LoadSnapshot(r io.Reader) (*Snapshot, error) {
	var s Snapshot
	if err := serverdef.DecodeStrict(r, "snapshot", &s); err != nil {
		return nil, err
	}
	if err := Check(&s); err != nil {
		return nil, fmt.Errorf("check snapshot: %w", err)
	}
	return &s, nil
}

// Check validates that s satisfies all structural invariants required of a snapshot.
func Check(s *Snapshot) error {
	if s == nil {
		return errors.New("check snapshot: nil snapshot")
	}
	var errs []error
	if err := checkEveryoneRole(s.Roles); err != nil {
		errs = append(errs, err)
	}
	if err := checkCallerRole(s.Roles, s.Caller.Role); err != nil {
		errs = append(errs, err)
	}
	if s.Caller.Position < 1 {
		errs = append(errs, fmt.Errorf("caller position %d is below 1; only the everyone role stands on 0", s.Caller.Position))
	}
	errs = append(errs, checkNamesAndTypes(s)...)
	errs = append(errs, checkPermissions(s)...)
	return errors.Join(errs...)
}

func checkEveryoneRole(roles []Role) error {
	count := 0
	lastIsEveryone := false
	for i := range roles {
		if roles[i].Everyone {
			count++
		}
	}
	if len(roles) > 0 && roles[len(roles)-1].Everyone {
		lastIsEveryone = true
	}
	if count != 1 {
		return fmt.Errorf("snapshot must have exactly one role with everyone=true, found %d", count)
	}
	if !lastIsEveryone {
		return errors.New("role with everyone=true must be the last entry in roles")
	}
	return nil
}

func checkCallerRole(roles []Role, callerRole string) error {
	matchCount := 0
	isEveryone := false
	for i := range roles {
		if roles[i].Name == callerRole {
			matchCount++
			if roles[i].Everyone {
				isEveryone = true
			}
		}
	}
	if matchCount == 0 {
		return fmt.Errorf("caller role %q names no role in snapshot", callerRole)
	}
	if matchCount > 1 {
		return fmt.Errorf("caller role %q names multiple roles in snapshot", callerRole)
	}
	if isEveryone {
		return fmt.Errorf("caller role %q must not be the everyone role", callerRole)
	}
	return nil
}

func checkNamesAndTypes(s *Snapshot) []error {
	var errs []error
	for i := range s.Roles {
		if strings.TrimSpace(s.Roles[i].Name) == "" {
			errs = append(errs, fmt.Errorf("empty role name at index %d", i))
		}
	}
	for i := range s.Categories {
		if strings.TrimSpace(s.Categories[i].Name) == "" {
			errs = append(errs, fmt.Errorf("empty category name at index %d", i))
		}
	}
	for i := range s.Channels {
		errs = append(errs, checkChannelShape(i, &s.Channels[i])...)
	}
	for i := range s.Features {
		if strings.TrimSpace(s.Features[i]) == "" {
			errs = append(errs, fmt.Errorf("empty feature name at index %d", i))
		}
	}
	return errs
}

// checkChannelShape reports a blank name, an unknown type and a negative
// member entry count of the channel at index i.
func checkChannelShape(i int, ch *Channel) []error {
	var errs []error
	if strings.TrimSpace(ch.Name) == "" {
		errs = append(errs, fmt.Errorf("empty channel name at index %d", i))
	}
	if _, ok := allowedChannelTypes[ch.Type]; !ok {
		errs = append(errs, fmt.Errorf("unknown channel type %q in channel %q", ch.Type, ch.Name))
	}
	if ch.MemberEntries < 0 {
		errs = append(errs, fmt.Errorf("channel %q has negative member entries %d", ch.Name, ch.MemberEntries))
	}
	return errs
}

func checkPermList(perms []string, vocab map[string]struct{}, ctx string) []error {
	var errs []error
	seen := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		if _, exists := seen[p]; exists {
			errs = append(errs, fmt.Errorf("duplicate permission %q in %s", p, ctx))
		}
		seen[p] = struct{}{}
		if _, valid := vocab[p]; !valid {
			errs = append(errs, fmt.Errorf("unknown permission %q in %s", p, ctx))
		}
	}
	return errs
}

// permissionBits is the number of bits a permission value holds here.
const permissionBits = 64

// checkUnknownBits accepts only bit numbers the permission table does not
// name, each once and in rising order.
func checkUnknownBits(bits []int, ctx string) []error {
	var errs []error
	last := -1
	for _, b := range bits {
		if b < 0 || b >= permissionBits {
			errs = append(errs, fmt.Errorf("unknown bit %d in %s is outside 0 to %d", b, ctx, permissionBits-1))
			continue
		}
		if b <= last {
			errs = append(errs, fmt.Errorf("unknown bit %d in %s is not in rising order", b, ctx))
		}
		last = b
		if names, _ := serverdef.PermissionNames(uint64(1) << b); len(names) > 0 {
			errs = append(errs, fmt.Errorf("unknown bit %d in %s is the permission %q", b, ctx, names[0]))
		}
	}
	return errs
}

func checkPermissions(s *Snapshot) []error {
	vocabList := serverdef.Permissions()
	vocab := make(map[string]struct{}, len(vocabList))
	for _, v := range vocabList {
		vocab[v] = struct{}{}
	}

	var errs []error
	errs = append(errs, checkPermList(s.Caller.Permissions, vocab, "caller permissions")...)
	for i := range s.Roles {
		ctx := fmt.Sprintf("role %q", s.Roles[i].Name)
		errs = append(errs, checkPermList(s.Roles[i].Permissions, vocab, ctx)...)
		errs = append(errs, checkUnknownBits(s.Roles[i].UnknownBits, ctx)...)
	}
	for i := range s.Channels {
		ch := &s.Channels[i]
		for j := range ch.Access {
			acc := &ch.Access[j]
			allowCtx := fmt.Sprintf("channel %q access[%s].allow", ch.Name, acc.Role)
			denyCtx := fmt.Sprintf("channel %q access[%s].deny", ch.Name, acc.Role)
			errs = append(errs, checkPermList(acc.Allow, vocab, allowCtx)...)
			errs = append(errs, checkPermList(acc.Deny, vocab, denyCtx)...)
			errs = append(errs, checkUnknownBits(acc.UnknownAllow, allowCtx)...)
			errs = append(errs, checkUnknownBits(acc.UnknownDeny, denyCtx)...)
		}
	}
	return errs
}
