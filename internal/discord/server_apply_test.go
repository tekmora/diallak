package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

type fakeRole struct {
	id          snowflake.ID
	name        string
	position    int
	permissions string
	managed     bool
	botID       *snowflake.ID
	hoist       bool
	mentionable bool
}

func (r *fakeRole) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID          snowflake.ID `json:"id"`
		Name        string       `json:"name"`
		Position    int          `json:"position"`
		Permissions string       `json:"permissions"`
		Managed     bool         `json:"managed"`
		Hoist       bool         `json:"hoist"`
		Mentionable bool         `json:"mentionable"`
		Tags        *struct {
			BotID *snowflake.ID `json:"bot_id"`
		} `json:"tags"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.id = raw.ID
	r.name = raw.Name
	r.position = raw.Position
	r.permissions = raw.Permissions
	r.managed = raw.Managed
	r.hoist = raw.Hoist
	r.mentionable = raw.Mentionable
	if raw.Tags != nil && raw.Tags.BotID != nil {
		r.botID = raw.Tags.BotID
	}
	return nil
}

type fakeOverwrite struct {
	id    snowflake.ID
	t     int
	allow string
	deny  string
}

type fakeChannel struct {
	id         snowflake.ID
	t          int
	name       string
	parentID   *snowflake.ID
	position   int
	overwrites []fakeOverwrite
}

func (c *fakeChannel) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID         snowflake.ID  `json:"id"`
		Type       int           `json:"type"`
		Name       string        `json:"name"`
		ParentID   *snowflake.ID `json:"parent_id"`
		Position   int           `json:"position"`
		Overwrites []struct {
			ID    snowflake.ID `json:"id"`
			Type  int          `json:"type"`
			Allow string       `json:"allow"`
			Deny  string       `json:"deny"`
		} `json:"permission_overwrites"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.id = raw.ID
	c.t = raw.Type
	c.name = raw.Name
	c.parentID = raw.ParentID
	c.position = raw.Position
	c.overwrites = make([]fakeOverwrite, len(raw.Overwrites))
	for i, o := range raw.Overwrites {
		c.overwrites[i] = fakeOverwrite{
			id:    o.ID,
			t:     o.Type,
			allow: o.Allow,
			deny:  o.Deny,
		}
	}
	return nil
}

type recordedCall struct {
	method      string
	path        string
	body        []byte
	auditReason string
}

type fakePlatform struct {
	mu                 sync.Mutex
	t                  *testing.T
	guildID            uint64
	botUserID          uint64
	botRoleID          uint64
	roles              []fakeRole
	channels           []fakeChannel
	nextID             uint64
	recorded           []recordedCall
	writeCount         int
	failWrite          int
	failStatus         int
	failBody           string
	failTransportWrite int
	hangWrite          int
	hangRelease        chan struct{}
	onWrite            func(writeCount int)
	getRolesOverride   []fakeRole
	delaySuffix        string
	delay              time.Duration
}

func newFakePlatform(t *testing.T, guildID, botUserID, botRoleID uint64) *fakePlatform {
	return &fakePlatform{
		t:           t,
		guildID:     guildID,
		botUserID:   botUserID,
		botRoleID:   botRoleID,
		nextID:      50000,
		hangRelease: make(chan struct{}),
	}
}

func (f *fakePlatform) getRecorded() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	res := make([]recordedCall, len(f.recorded))
	copy(res, f.recorded)
	return res
}

func (f *fakePlatform) resetRecorded() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = nil
}

func (f *fakePlatform) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.handleRequest(w, r)
	}))
}

func (f *fakePlatform) handleRequest(w http.ResponseWriter, r *http.Request) {
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		f.t.Errorf("read body: %v", readErr)
	}
	audit := r.Header.Get("X-Audit-Log-Reason")

	f.mu.Lock()
	if r.Method == http.MethodDelete {
		f.mu.Unlock()
		f.t.Errorf("unexpected DELETE request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}

	f.recorded = append(f.recorded, recordedCall{
		method:      r.Method,
		path:        r.URL.Path,
		body:        body,
		auditReason: audit,
	})

	isWrite := r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodPut
	if isWrite {
		f.writeCount++
		wc := f.writeCount
		if f.onWrite != nil {
			f.onWrite(wc)
		}
		if f.failTransportWrite == wc {
			f.mu.Unlock()
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, hijackErr := hj.Hijack()
				if hijackErr != nil {
					f.t.Errorf("hijack: %v", hijackErr)
					return
				}
				if conn != nil {
					if closeErr := conn.Close(); closeErr != nil {
						f.t.Logf("conn.Close: %v", closeErr)
					}
				}
				return
			}
		}
		if f.hangWrite == wc {
			f.mu.Unlock()
			select {
			case <-f.hangRelease:
			case <-r.Context().Done():
			}
			return
		}
		if f.failWrite == wc {
			f.mu.Unlock()
			writeTestResponse(w, f.failStatus, f.failBody)
			return
		}
	}
	delay := time.Duration(0)
	if f.delaySuffix != "" && strings.HasSuffix(r.URL.Path, f.delaySuffix) {
		delay = f.delay
	}
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	f.routeRequest(w, r, body)
}

func (f *fakePlatform) routeRequest(w http.ResponseWriter, r *http.Request, body []byte) {
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/applications/@me":
		f.handleGetApplication(w)
	case r.Method == http.MethodGet && path == fmt.Sprintf("/guilds/%d", f.guildID):
		f.handleGetGuild(w)
	case r.Method == http.MethodGet && path == fmt.Sprintf("/guilds/%d/members/%d", f.guildID, f.botUserID):
		f.handleGetMember(w)
	case r.Method == http.MethodGet && path == fmt.Sprintf("/guilds/%d/channels", f.guildID):
		f.handleGetChannels(w)
	case r.Method == http.MethodGet && path == fmt.Sprintf("/guilds/%d/roles", f.guildID):
		f.handleGetRoles(w)
	case r.Method == http.MethodPost && path == fmt.Sprintf("/guilds/%d/roles", f.guildID):
		f.handleCreateRole(w, body)
	case r.Method == http.MethodPatch && strings.HasPrefix(path, fmt.Sprintf("/guilds/%d/roles/", f.guildID)):
		f.handleUpdateRole(w, path, body)
	case r.Method == http.MethodPatch && path == fmt.Sprintf("/guilds/%d/roles", f.guildID):
		f.handleUpdateRolePositions(w, body)
	case r.Method == http.MethodPost && path == fmt.Sprintf("/guilds/%d/channels", f.guildID):
		f.handleCreateChannel(w, body)
	case r.Method == http.MethodPatch && strings.HasPrefix(path, "/channels/"):
		f.handleUpdateChannel(w, path, body)
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/channels/") && strings.Contains(path, "/permissions/"):
		f.handleUpdateOverwrite(w, path, body)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakePlatform) handleGetApplication(w http.ResponseWriter) {
	writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"9001","name":"diallak-app","bot":{"id":"%d","username":"diallak"}}`, f.botUserID))
}

func (f *fakePlatform) handleGetGuild(w http.ResponseWriter) {
	f.mu.Lock()
	rolesJSON := f.rolesToJSONLocked()
	f.mu.Unlock()

	writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":"Test Hub","features":["COMMUNITY","NEWS"],"roles":%s}`, f.guildID, rolesJSON))
}

func (f *fakePlatform) handleGetMember(w http.ResponseWriter) {
	writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"user":{"id":"%d"},"roles":["%d"]}`, f.botUserID, f.botRoleID))
}

func (f *fakePlatform) handleGetChannels(w http.ResponseWriter) {
	f.mu.Lock()
	channelsJSON := f.channelsToJSONLocked()
	f.mu.Unlock()

	writeTestResponse(w, http.StatusOK, channelsJSON)
}

func (f *fakePlatform) handleGetRoles(w http.ResponseWriter) {
	f.mu.Lock()
	roles := f.roles
	if f.getRolesOverride != nil {
		roles = f.getRolesOverride
	}
	rolesJSON := f.rolesToJSON(roles)
	f.mu.Unlock()

	writeTestResponse(w, http.StatusOK, rolesJSON)
}

func (f *fakePlatform) handleCreateRole(w http.ResponseWriter, body []byte) {
	var req struct {
		Name        string  `json:"name"`
		Permissions *string `json:"permissions"`
		Hoist       bool    `json:"hoist"`
		Mentionable bool    `json:"mentionable"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	newID := f.nextID
	f.nextID++
	perms := "0"
	if req.Permissions != nil {
		perms = *req.Permissions
	}

	// A new role moves no other role, and the roles created in one run share
	// the places below the roles already there, read in the order of their
	// creation (measured on a live server on 2026-10-12). The local server puts
	// it on position 1; roles on one position sort by id, so it stands below
	// every role already on the server.
	role := fakeRole{
		id:          snowflake.ID(newID),
		name:        req.Name,
		position:    1,
		permissions: perms,
		hoist:       req.Hoist,
		mentionable: req.Mentionable,
	}
	f.roles = append(f.roles, role)
	f.mu.Unlock()

	writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":%q,"position":%d,"permissions":%q,"managed":false,"hoist":%t,"mentionable":%t}`,
		newID, req.Name, role.position, perms, req.Hoist, req.Mentionable))
}

func (f *fakePlatform) handleUpdateRole(w http.ResponseWriter, path string, body []byte) {
	parts := strings.Split(path, "/")
	roleIDStr := parts[len(parts)-1]
	roleID, err := strconv.ParseUint(roleIDStr, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req struct {
		Permissions *string `json:"permissions"`
		Hoist       *bool   `json:"hoist"`
		Mentionable *bool   `json:"mentionable"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	var updated *fakeRole
	for i := range f.roles {
		if f.roles[i].id == snowflake.ID(roleID) {
			if req.Permissions != nil {
				f.roles[i].permissions = *req.Permissions
			}
			if req.Hoist != nil {
				f.roles[i].hoist = *req.Hoist
			}
			if req.Mentionable != nil {
				f.roles[i].mentionable = *req.Mentionable
			}
			updated = &f.roles[i]
			break
		}
	}
	f.mu.Unlock()

	if updated == nil {
		http.NotFound(w, nil)
		return
	}
	perms := updated.permissions
	if perms == "" {
		perms = "0"
	}
	writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":%q,"position":%d,"permissions":%q,"managed":%t,"hoist":%t,"mentionable":%t}`,
		updated.id, updated.name, updated.position, perms, updated.managed, updated.hoist, updated.mentionable))
}

func (f *fakePlatform) handleUpdateRolePositions(w http.ResponseWriter, body []byte) {
	var updates []struct {
		ID       string `json:"id"`
		Position int    `json:"position"`
	}
	if err := json.Unmarshal(body, &updates); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	for _, u := range updates {
		id, err := strconv.ParseUint(u.ID, 10, 64)
		if err != nil {
			continue
		}
		for i := range f.roles {
			if f.roles[i].id == snowflake.ID(id) {
				f.roles[i].position = u.Position
				break
			}
		}
	}
	rolesJSON := f.rolesToJSONLocked()
	f.mu.Unlock()

	writeTestResponse(w, http.StatusOK, rolesJSON)
}

func (f *fakePlatform) handleCreateChannel(w http.ResponseWriter, body []byte) {
	var req struct {
		Name                 string  `json:"name"`
		Type                 int     `json:"type"`
		ParentID             *string `json:"parent_id"`
		PermissionOverwrites []struct {
			ID    string `json:"id"`
			Type  int    `json:"type"`
			Allow string `json:"allow"`
			Deny  string `json:"deny"`
		} `json:"permission_overwrites"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	newID := f.nextID
	f.nextID++

	var pID *snowflake.ID
	if req.ParentID != nil {
		val, err := strconv.ParseUint(*req.ParentID, 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sID := snowflake.ID(val)
		pID = &sID
	}

	var ows []fakeOverwrite
	for _, po := range req.PermissionOverwrites {
		idVal, err := strconv.ParseUint(po.ID, 10, 64)
		if err != nil {
			continue
		}
		ows = append(ows, fakeOverwrite{
			id:    snowflake.ID(idVal),
			t:     po.Type,
			allow: po.Allow,
			deny:  po.Deny,
		})
	}

	ch := fakeChannel{
		id:         snowflake.ID(newID),
		t:          req.Type,
		name:       req.Name,
		parentID:   pID,
		overwrites: ows,
	}
	f.channels = append(f.channels, ch)
	f.mu.Unlock()

	writeTestResponse(w, http.StatusOK, f.singleChannelToJSON(ch))
}

func (f *fakePlatform) handleUpdateChannel(w http.ResponseWriter, path string, body []byte) {
	parts := strings.Split(path, "/")
	chIDStr := parts[len(parts)-1]
	chID, err := strconv.ParseUint(chIDStr, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req struct {
		ParentID *string `json:"parent_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	var updated *fakeChannel
	for i := range f.channels {
		if f.channels[i].id == snowflake.ID(chID) {
			if req.ParentID != nil {
				val, parseErr := strconv.ParseUint(*req.ParentID, 10, 64)
				if parseErr != nil {
					f.mu.Unlock()
					http.Error(w, parseErr.Error(), http.StatusBadRequest)
					return
				}
				sID := snowflake.ID(val)
				f.channels[i].parentID = &sID
			} else {
				f.channels[i].parentID = nil
			}
			updated = &f.channels[i]
			break
		}
	}
	f.mu.Unlock()

	if updated == nil {
		http.NotFound(w, nil)
		return
	}
	writeTestResponse(w, http.StatusOK, f.singleChannelToJSON(*updated))
}

func (f *fakePlatform) handleUpdateOverwrite(w http.ResponseWriter, path string, body []byte) {
	// /channels/{c}/permissions/{role}
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		http.NotFound(w, nil)
		return
	}
	chIDStr := parts[2]
	roleIDStr := parts[4]
	chID, err := strconv.ParseUint(chIDStr, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	roleID, err := strconv.ParseUint(roleIDStr, 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req struct {
		Type  int    `json:"type"`
		Allow string `json:"allow"`
		Deny  string `json:"deny"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	for i := range f.channels {
		if f.channels[i].id == snowflake.ID(chID) {
			found := false
			for j := range f.channels[i].overwrites {
				if f.channels[i].overwrites[j].id == snowflake.ID(roleID) {
					f.channels[i].overwrites[j].allow = req.Allow
					f.channels[i].overwrites[j].deny = req.Deny
					f.channels[i].overwrites[j].t = req.Type
					found = true
					break
				}
			}
			if !found {
				f.channels[i].overwrites = append(f.channels[i].overwrites, fakeOverwrite{
					id:    snowflake.ID(roleID),
					t:     req.Type,
					allow: req.Allow,
					deny:  req.Deny,
				})
			}
			break
		}
	}
	f.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (f *fakePlatform) rolesToJSONLocked() string {
	return f.rolesToJSON(f.roles)
}

func (f *fakePlatform) rolesToJSON(roles []fakeRole) string {
	type apiRole struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Position    int    `json:"position"`
		Permissions string `json:"permissions"`
		Managed     bool   `json:"managed"`
		Hoist       bool   `json:"hoist"`
		Mentionable bool   `json:"mentionable"`
		Tags        *struct {
			BotID *string `json:"bot_id,omitempty"`
		} `json:"tags,omitempty"`
	}
	var res []apiRole
	for _, r := range roles {
		var tags *struct {
			BotID *string `json:"bot_id,omitempty"`
		}
		if r.botID != nil {
			bStr := r.botID.String()
			tags = &struct {
				BotID *string `json:"bot_id,omitempty"`
			}{BotID: &bStr}
		}
		perms := r.permissions
		if perms == "" {
			perms = "0"
		}
		res = append(res, apiRole{
			ID:          r.id.String(),
			Name:        r.name,
			Position:    r.position,
			Permissions: perms,
			Managed:     r.managed,
			Hoist:       r.hoist,
			Mentionable: r.mentionable,
			Tags:        tags,
		})
	}
	b, marshalErr := json.Marshal(res)
	if marshalErr != nil {
		f.t.Errorf("Marshal roles: %v", marshalErr)
	}
	return string(b)
}

func (f *fakePlatform) channelsToJSONLocked() string {
	var list []string
	for _, ch := range f.channels {
		list = append(list, f.singleChannelToJSON(ch))
	}
	return "[" + strings.Join(list, ",") + "]"
}

func (f *fakePlatform) singleChannelToJSON(ch fakeChannel) string {
	type apiOverwrite struct {
		ID    string `json:"id"`
		Type  int    `json:"type"`
		Allow string `json:"allow"`
		Deny  string `json:"deny"`
	}
	var ows []apiOverwrite
	for _, po := range ch.overwrites {
		ows = append(ows, apiOverwrite{
			ID:    po.id.String(),
			Type:  po.t,
			Allow: po.allow,
			Deny:  po.deny,
		})
	}
	owBytes, marshalErr := json.Marshal(ows)
	if marshalErr != nil {
		f.t.Errorf("Marshal overwrites: %v", marshalErr)
	}
	pIDStr := "null"
	if ch.parentID != nil {
		pIDStr = fmt.Sprintf("%q", ch.parentID.String())
	}
	return fmt.Sprintf(`{"id":%q,"type":%d,"name":%q,"parent_id":%s,"permission_overwrites":%s}`,
		ch.id.String(), ch.t, ch.name, pIDStr, string(owBytes))
}

func setupFreshServerFake(t *testing.T, guildID, botUserID, botRoleID uint64, adminBot bool) *fakePlatform {
	t.Helper()
	fake := newFakePlatform(t, guildID, botUserID, botRoleID)
	botPerms := "8" // ADMINISTRATOR
	if !adminBot {
		mask, maskErr := serverdef.PermissionMask([]string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "MANAGE_GUILD"})
		if maskErr != nil {
			t.Fatalf("PermissionMask: %v", maskErr)
		}
		botPerms = strconv.FormatUint(mask, 10)
	}
	bID := snowflake.ID(botUserID)
	fake.roles = []fakeRole{
		{
			id:          snowflake.ID(guildID),
			name:        "@everyone",
			position:    0,
			permissions: "1024",
			managed:     false,
		},
		{
			id:          snowflake.ID(botRoleID),
			name:        "Diallak",
			position:    1,
			permissions: botPerms,
			managed:     true,
			botID:       &bID,
		},
		{
			id:          snowflake.ID(4001),
			name:        "Operator",
			position:    2,
			permissions: "8",
			managed:     false,
		},
	}
	return fake
}

// addManagedRoles puts the three roles another party manages on the server,
// at the bottom, as on a server that had its boosters and its two funding
// integrations before the bot came.
func addManagedRoles(fake *fakePlatform) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for i := range fake.roles {
		if fake.roles[i].position > 0 {
			fake.roles[i].position += 3
		}
	}
	fake.roles = append(fake.roles,
		fakeRole{id: snowflake.ID(4002), name: "Server Booster", position: 1, permissions: "0", managed: true, hoist: true},
		fakeRole{id: snowflake.ID(4003), name: "Ko-fi Bot", position: 2, permissions: "0", managed: true},
		fakeRole{id: snowflake.ID(4004), name: "Patreon", position: 3, permissions: "0", managed: true},
	)
}
func assertRequestCounts(t *testing.T, recorded []recordedCall, wantReads, wantWrites int) {
	t.Helper()
	var reads, writes int
	for _, r := range recorded {
		if r.method == http.MethodGet {
			reads++
		} else {
			writes++
		}
	}
	if reads != wantReads || writes != wantWrites {
		t.Errorf("recorded %d reads and %d writes, want %d reads and %d writes", reads, writes, wantReads, wantWrites)
	}
}

func TestApplyServer_Negative_Gates(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	fakeToken := makeFakeToken(botUserID)
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	defDigest := server.DefinitionDigest()

	t.Run("empty PlanDigest or nil Definition", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		srv := fake.server()
		defer srv.Close()

		var out bytes.Buffer
		errNil := applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       nil,
			DefinitionDigest: defDigest,
			PlanDigest:       "sha256:abc",
		}, &out, clientOptions{restURL: srv.URL})
		if errNil == nil {
			t.Fatal("expected error with nil definition, got nil")
		}

		errEmpty := applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       "",
		}, &out, clientOptions{restURL: srv.URL})
		if errEmpty == nil {
			t.Fatal("expected error with empty plan digest, got nil")
		}

		if len(fake.recorded) != 0 {
			t.Errorf("recorded %d requests, want 0", len(fake.recorded))
		}
	})

	t.Run("wrong plan digest", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		srv := fake.server()
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read server: %v", err)
		}
		fake.recorded = nil
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		expectedDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		}, &out, clientOptions{restURL: srv.URL})
		if err == nil {
			t.Fatal("apply succeeded with wrong plan digest, want error")
		}
		if !strings.Contains(err.Error(), expectedDigest) {
			t.Errorf("error %q does not contain new digest %q", err.Error(), expectedDigest)
		}
		assertRequestCounts(t, fake.recorded, 4, 0)
	})

	t.Run("right plan digest with another DefinitionDigest", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		srv := fake.server()
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read server: %v", err)
		}
		fake.recorded = nil
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		validDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		const otherDefDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		otherExpectedDigest, err := serverplan.Digest(res, otherDefDigest)
		if err != nil {
			t.Fatalf("Digest other: %v", err)
		}

		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: otherDefDigest,
			PlanDigest:       validDigest,
		}, &out, clientOptions{restURL: srv.URL})
		if err == nil {
			t.Fatal("apply succeeded with different definition digest, want error")
		}
		if !strings.Contains(err.Error(), otherExpectedDigest) {
			t.Errorf("error %q does not contain new digest %q", err.Error(), otherExpectedDigest)
		}
		assertRequestCounts(t, fake.recorded, 4, 0)
	})

	t.Run("plan with a refusal", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, false)
		srv := fake.server()
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read server: %v", err)
		}
		fake.recorded = nil
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if len(res.Refused) == 0 {
			t.Fatal("expected refusals, got 0")
		}
		refusedDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       refusedDigest,
		}, &out, clientOptions{restURL: srv.URL})
		if err == nil {
			t.Fatal("apply succeeded on refused plan, want error")
		}
		wantRefusalErr := fmt.Sprintf("the plan holds %d refused changes; a plan with a refusal is not applied", len(res.Refused))
		if err.Error() != wantRefusalErr {
			t.Errorf("error = %q, want %q", err.Error(), wantRefusalErr)
		}
		assertRequestCounts(t, fake.recorded, 4, 0)
	})

	t.Run("server exactly as definition", func(t *testing.T) {
		appJSON, guildJSON, memberJSON, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)
		srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read server: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("apply failed on empty changes: %v", err)
		}
		if out.String() != "changes: 0\napplied: 0 of 0\n" {
			t.Errorf("output = %q, want 'changes: 0\\napplied: 0 of 0\\n'", out.String())
		}
	})
}
func TestApplyServer_Positive_FullRun(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	fakeToken := makeFakeToken(botUserID)
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	defDigest := server.DefinitionDigest()

	fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
	srv := fake.server()
	defer srv.Close()

	snapBefore, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("readServerWithOptions before: %v", err)
	}
	res, err := serverplan.Plan(def, snapBefore)
	if err != nil {
		t.Fatalf("Plan before: %v", err)
	}
	planDigest, err := serverplan.Digest(res, defDigest)
	if err != nil {
		t.Fatalf("Digest before: %v", err)
	}

	fake.recorded = nil
	var out bytes.Buffer
	err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
		Definition:       def,
		DefinitionDigest: defDigest,
		PlanDigest:       planDigest,
	}, &out, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("applyServerWithOptions: %v", err)
	}

	snapAfter, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("readServerWithOptions after: %v", err)
	}

	diffs, err := serverplan.Compare(def, snapAfter)
	if err != nil {
		t.Fatalf("Compare after: %v", err)
	}

	assertFullRunDifferences(t, diffs)

	resSecond, err := serverplan.Plan(def, snapAfter)
	if err != nil {
		t.Fatalf("Plan after: %v", err)
	}
	if len(resSecond.Changes) != 0 {
		t.Errorf("second plan holds %d changes, want 0: %+v", len(resSecond.Changes), resSecond.Changes)
	}

	assertRecordedRequestsContract(t, fake.recorded, res.Changes, planDigest, guildID, botRoleID)
}

func assertFullRunDifferences(t *testing.T, diffs []serverplan.Difference) {
	t.Helper()
	var botPermsDiff, roleOrderDiff bool
	for _, d := range diffs {
		switch d.Kind {
		case serverplan.KindRoleMissing:
			if d.Detail != "managed; the plan does not create it" {
				t.Errorf("unexpected diff of kind %s: %+v", d.Kind, d)
			}
		case serverplan.KindCategoryMissing, serverplan.KindChannelMissing,
			serverplan.KindChannelCategory, serverplan.KindChannelType, serverplan.KindAccessMissing,
			serverplan.KindAccessDiffers:
			t.Errorf("unexpected diff of kind %s: %+v", d.Kind, d)
		case serverplan.KindRolePermissions:
			if d.Object == "roles[bot]" {
				botPermsDiff = true
			} else {
				t.Errorf("unexpected role permissions diff: %+v", d)
			}
		case serverplan.KindRoleOrder:
			roleOrderDiff = true
		}
	}
	if !botPermsDiff {
		t.Error("expected role-permissions diff for bot role")
	}
	if !roleOrderDiff {
		t.Error("expected role-order diff caused by raised bot role")
	}
}

func assertRecordedRequestsContract(
	t *testing.T,
	recorded []recordedCall,
	planChanges []serverplan.Change,
	planDigest string,
	guildID, botRoleID uint64,
) {
	t.Helper()
	var writes []recordedCall
	wantReason := "diallak apply " + planDigest
	wantReasonEscaped := "diallak apply " + url.PathEscape(planDigest)
	createdRoles := make(map[string]string)
	createdCats := make(map[string]string)

	for _, call := range recorded {
		if call.method == http.MethodGet {
			if call.auditReason != "" {
				t.Errorf("GET %s carried audit reason %q", call.path, call.auditReason)
			}
			continue
		}
		if call.auditReason != wantReason && call.auditReason != wantReasonEscaped && !strings.HasPrefix(call.auditReason, "diallak") {
			t.Errorf("%s %s carried audit reason %q, want %q or %q", call.method, call.path, call.auditReason, wantReason, wantReasonEscaped)
		}
		writes = append(writes, call)
	}

	if len(writes) != len(planChanges) {
		t.Fatalf("recorded %d writes, want %d", len(writes), len(planChanges))
	}

	for i, c := range planChanges {
		w := writes[i]
		switch c.Action {
		case "create-role":
			assertCreatedRolePayload(t, w, createdRoles)
		case "create-category":
			assertCreatedCategoryPayload(t, w, createdCats)
		case "create-channel":
			assertCreatedChannelPayload(t, w, createdCats, createdRoles, guildID, botRoleID)
		}
	}
}

func assertCreatedRolePayload(t *testing.T, w recordedCall, createdRoles map[string]string) {
	t.Helper()
	var roleReq struct {
		Name        string `json:"name"`
		Permissions string `json:"permissions"`
	}
	if err := json.Unmarshal(w.body, &roleReq); err != nil {
		t.Errorf("Unmarshal role body: %v", err)
	}
	if roleReq.Name == "Language: Español" || roleReq.Name == "Interest: AI" {
		if roleReq.Permissions != "0" {
			t.Errorf("created role %q permissions = %q, want 0", roleReq.Name, roleReq.Permissions)
		}
	}
}

func assertCreatedCategoryPayload(t *testing.T, w recordedCall, createdCats map[string]string) {
	t.Helper()
	if !strings.HasSuffix(w.path, "/channels") || w.method != http.MethodPost {
		t.Errorf("create-category request = %s %s", w.method, w.path)
	}
}

func assertCreatedChannelPayload(
	t *testing.T,
	w recordedCall,
	createdCats map[string]string,
	createdRoles map[string]string,
	guildID, botRoleID uint64,
) {
	t.Helper()
	var chReq struct {
		Name                 string  `json:"name"`
		Type                 int     `json:"type"`
		ParentID             *string `json:"parent_id"`
		PermissionOverwrites []struct {
			ID    string `json:"id"`
			Type  int    `json:"type"`
			Allow string `json:"allow"`
			Deny  string `json:"deny"`
		} `json:"permission_overwrites"`
	}
	if err := json.Unmarshal(w.body, &chReq); err != nil {
		t.Errorf("Unmarshal channel body: %v", err)
	}
	for _, ow := range chReq.PermissionOverwrites {
		if ow.Type != 0 {
			t.Errorf("channel %s has overwrite with type %d, want 0 (role)", chReq.Name, ow.Type)
		}
	}
}
func TestApplyServer_Positive_PerActionFixtures(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	fakeToken := makeFakeToken(botUserID)
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	defDigest := server.DefinitionDigest()

	t.Run("set-role-permissions", func(t *testing.T) {
		appJSON, guildJSON, memberJSON, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)
		_ = appJSON
		_ = memberJSON
		var g struct {
			Roles []fakeRole `json:"roles"`
		}
		if err := json.Unmarshal([]byte(guildJSON), &g); err != nil {
			t.Fatalf("Unmarshal guild: %v", err)
		}
		for i := range g.Roles {
			if g.Roles[i].name == "Diallak" {
				g.Roles[i].permissions = "8"
			}
			if g.Roles[i].name == "Contributor" {
				g.Roles[i].permissions = "1024"
			}
		}
		var apiChs []fakeChannel
		if err := json.Unmarshal([]byte(channelsJSON), &apiChs); err != nil {
			t.Fatalf("Unmarshal channels: %v", err)
		}

		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.roles = g.Roles
		fake.channels = apiChs
		fakeSrv := fake.server()
		defer fakeSrv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		t.Logf("changes: %+v, refused: %+v", res.Changes, res.Refused)
		if len(res.Changes) != 1 || res.Changes[0].Action != "set-role-permissions" {
			t.Fatalf("expected 1 set-role-permissions change, got %+v", res.Changes)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		fake.recorded = nil
		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("apply: %v", err)
		}

		var patchReq *recordedCall
		for i := range fake.recorded {
			if fake.recorded[i].method == http.MethodPatch && strings.Contains(fake.recorded[i].path, "/roles/") {
				patchReq = &fake.recorded[i]
				break
			}
		}
		if patchReq == nil {
			t.Fatal("no PATCH role request recorded")
		}
		var bodyMap map[string]interface{}
		if err := json.Unmarshal(patchReq.body, &bodyMap); err != nil {
			t.Fatalf("Unmarshal patch body: %v", err)
		}
		if len(bodyMap) != 1 || bodyMap["permissions"] == nil {
			t.Errorf("set-role-permissions body = %s, want exactly permissions", string(patchReq.body))
		}
	})

	t.Run("move-channel", func(t *testing.T) {
		_, guildJSON, _, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)
		var g struct {
			Roles []fakeRole `json:"roles"`
		}
		if err := json.Unmarshal([]byte(guildJSON), &g); err != nil {
			t.Fatalf("Unmarshal guild: %v", err)
		}
		var chs []fakeChannel
		if err := json.Unmarshal([]byte(channelsJSON), &chs); err != nil {
			t.Fatalf("Unmarshal channels: %v", err)
		}

		for i := range g.Roles {
			if g.Roles[i].name == "Diallak" {
				g.Roles[i].permissions = "8"
			}
		}
		for i := range chs {
			if chs[i].name == "lounge" {
				chs[i].parentID = nil
			}
		}

		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.roles = g.Roles
		fake.channels = chs
		fakeSrv := fake.server()
		defer fakeSrv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if len(res.Changes) != 1 || res.Changes[0].Action != "move-channel" {
			t.Fatalf("expected 1 move-channel change, got %+v", res.Changes)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		fake.recorded = nil
		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("apply: %v", err)
		}

		var patchReq *recordedCall
		for i := range fake.recorded {
			if fake.recorded[i].method == http.MethodPatch && strings.HasPrefix(fake.recorded[i].path, "/channels/") {
				patchReq = &fake.recorded[i]
				break
			}
		}
		if patchReq == nil {
			t.Fatal("no PATCH channel request recorded")
		}
		var bodyMap map[string]interface{}
		if err := json.Unmarshal(patchReq.body, &bodyMap); err != nil {
			t.Fatalf("Unmarshal move body: %v", err)
		}
		if len(bodyMap) != 1 || bodyMap["parent_id"] == nil {
			t.Errorf("move-channel body = %s, want exactly parent_id", string(patchReq.body))
		}
	})

	t.Run("set-access", func(t *testing.T) {
		_, guildJSON, _, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)
		var g struct {
			Roles []fakeRole `json:"roles"`
		}
		if err := json.Unmarshal([]byte(guildJSON), &g); err != nil {
			t.Fatalf("Unmarshal guild: %v", err)
		}
		for i := range g.Roles {
			if g.Roles[i].name == "Diallak" {
				g.Roles[i].permissions = "8"
			}
		}
		var chs []fakeChannel
		if err := json.Unmarshal([]byte(channelsJSON), &chs); err != nil {
			t.Fatalf("Unmarshal channels: %v", err)
		}

		for i := range chs {
			if chs[i].name == "rules" && len(chs[i].overwrites) > 0 {
				chs[i].overwrites[0].deny = "0"
			}
		}

		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.roles = g.Roles
		fake.channels = chs
		fakeSrv := fake.server()
		defer fakeSrv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if len(res.Changes) != 1 || res.Changes[0].Action != "set-access" {
			t.Fatalf("expected 1 set-access change, got %+v", res.Changes)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		fake.recorded = nil
		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("apply: %v", err)
		}

		var putReq *recordedCall
		for i := range fake.recorded {
			if fake.recorded[i].method == http.MethodPut && strings.Contains(fake.recorded[i].path, "/permissions/") {
				putReq = &fake.recorded[i]
				break
			}
		}
		if putReq == nil {
			t.Fatal("no PUT overwrite request recorded")
		}
		var req struct {
			Type  int    `json:"type"`
			Allow string `json:"allow"`
			Deny  string `json:"deny"`
		}
		if err := json.Unmarshal(putReq.body, &req); err != nil {
			t.Fatalf("Unmarshal access body: %v", err)
		}
		if req.Type != 0 || req.Allow == "" || req.Deny == "" {
			t.Errorf("set-access body = %+v, want type 0 with non-empty allow and deny", req)
		}
	})
}
func TestApplyServer_Positive_And_Negative_RoleOrder(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	fakeToken := makeFakeToken(botUserID)
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	defDigest := server.DefinitionDigest()

	t.Run("roles already at wanted positions -> no PATCH", func(t *testing.T) {
		appJSON, guildJSON, _, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)
		_ = appJSON
		var g struct {
			Roles []fakeRole `json:"roles"`
		}
		if err := json.Unmarshal([]byte(guildJSON), &g); err != nil {
			t.Fatalf("Unmarshal guild: %v", err)
		}
		for i := range g.Roles {
			if g.Roles[i].name == "Diallak" {
				g.Roles[i].permissions = "8"
			}
		}
		var chs []fakeChannel
		if err := json.Unmarshal([]byte(channelsJSON), &chs); err != nil {
			t.Fatalf("Unmarshal channels: %v", err)
		}

		wantedRoles := make([]fakeRole, len(g.Roles))
		copy(wantedRoles, g.Roles)

		var idxContrib, idxSupport int
		for i, r := range g.Roles {
			if r.name == "Contributor" {
				idxContrib = i
			}
			if r.name == "Supporter" {
				idxSupport = i
			}
		}
		g.Roles[idxContrib].position, g.Roles[idxSupport].position = g.Roles[idxSupport].position, g.Roles[idxContrib].position

		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.roles = g.Roles
		fake.channels = chs
		fake.getRolesOverride = wantedRoles
		fakeSrv := fake.server()
		defer fakeSrv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		var out bytes.Buffer
		fake.recorded = nil
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("apply failed: %v", err)
		}
		for _, r := range fake.recorded {
			if r.method == http.MethodPatch && strings.HasSuffix(r.path, "/roles") {
				t.Errorf("unexpected PATCH roles call when positions match")
			}
		}
	})

	// The bot's role stands on position 1, and each created role lands there
	// too, below it: no position is free for an order step, and none is
	// needed, as the roles stand in the order of their creation.
	t.Run("fresh server -> no order step, the run goes through", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fakeSrv := fake.server()
		defer fakeSrv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		if len(res.Refused) != 0 {
			t.Fatalf("plan refused %+v, want no refusal", res.Refused)
		}
		for _, c := range res.Changes {
			if c.Action == serverplan.ActionSetRoleOrder {
				t.Errorf("plan holds %s %s, want no order step", c.Action, c.Object)
			}
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("Digest: %v", err)
		}

		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		for _, r := range fake.getRecorded() {
			if r.method == http.MethodPatch && strings.HasSuffix(r.path, "/roles") {
				t.Errorf("unexpected PATCH roles call on a fresh server")
			}
		}
	})

	t.Run("boundary n == caller position - 1 passes, n == caller position fails", func(t *testing.T) {
		const callerPos = 3
		fakePass := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		bID := snowflake.ID(botUserID)
		fakePass.roles = []fakeRole{
			{id: snowflake.ID(guildID), name: "@everyone", position: 0},
			{id: snowflake.ID(4001), name: "Operator", position: 4},
			{id: snowflake.ID(botRoleID), name: "Diallak", position: callerPos, botID: &bID, managed: true},
			{id: snowflake.ID(6001), name: "Moderator", position: 2},
			{id: snowflake.ID(6002), name: "Contributor", position: 1},
		}
		fakeSrvPass := fakePass.server()
		defer fakeSrvPass.Close()

		c := serverplan.Change{
			Step:   1,
			Action: "set-role-order",
			Object: "role_order",
			Detail: "operator, moderator, contributor",
		}
		clientPass, err := newRestClient(fakeToken, fakeSrvPass.URL)
		if err != nil {
			t.Fatalf("newRestClient: %v", err)
		}
		defer clientPass.Close(context.Background())

		idxPass := newServerIndex(guildID, snowflake.ID(botRoleID))
		for _, r := range fakePass.roles {
			idxPass.addRole(r.name, r.id)
		}

		err = executeSingleChange(context.Background(), clientPass, guildID, ApplyInput{Definition: def, PlanDigest: "sha256:abc"}, c, idxPass, 10*time.Second)
		if err != nil {
			t.Fatalf("pass case failed: %v", err)
		}

		// Fail case: 3 roles below caller position 3 (n == 3 == callerPos)
		fakeFail := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fakeFail.roles = []fakeRole{
			{id: snowflake.ID(guildID), name: "@everyone", position: 0},
			{id: snowflake.ID(4001), name: "Operator", position: 4},
			{id: snowflake.ID(botRoleID), name: "Diallak", position: callerPos, botID: &bID, managed: true},
			{id: snowflake.ID(6001), name: "Moderator", position: 2},
			{id: snowflake.ID(6002), name: "Contributor", position: 1},
			{id: snowflake.ID(6003), name: "Subject guide: coding", position: 1},
		}
		fakeSrvFail := fakeFail.server()
		defer fakeSrvFail.Close()

		cFail := serverplan.Change{
			Step:   1,
			Action: "set-role-order",
			Object: "role_order",
			Detail: "operator, moderator, contributor, guide-coding",
		}
		clientFail, err := newRestClient(fakeToken, fakeSrvFail.URL)
		if err != nil {
			t.Fatalf("newRestClient: %v", err)
		}
		defer clientFail.Close(context.Background())

		idxFail := newServerIndex(guildID, snowflake.ID(botRoleID))
		for _, r := range fakeFail.roles {
			idxFail.addRole(r.name, r.id)
		}

		err = executeSingleChange(context.Background(), clientFail, guildID, ApplyInput{Definition: def, PlanDigest: "sha256:abc"}, cFail, idxFail, 10*time.Second)
		if err == nil {
			t.Fatal("fail case unexpectedly succeeded")
		}
		if !strings.Contains(err.Error(), "not enough positions below the bot's role: 3 roles stand below position 3") {
			t.Errorf("error = %q, want 'not enough positions below the bot's role: 3 roles stand below position 3'", err.Error())
		}
	})

	t.Run("two roles swapped below caller", func(t *testing.T) {
		appJSON, guildJSON, _, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)
		_ = appJSON
		var g struct {
			Roles []fakeRole `json:"roles"`
		}
		if err := json.Unmarshal([]byte(guildJSON), &g); err != nil {
			t.Fatalf("Unmarshal guild: %v", err)
		}
		for i := range g.Roles {
			if g.Roles[i].name == "Diallak" {
				g.Roles[i].permissions = "8"
			}
		}
		var chs []fakeChannel
		if err := json.Unmarshal([]byte(channelsJSON), &chs); err != nil {
			t.Fatalf("Unmarshal channels: %v", err)
		}

		var idxContrib, idxSupport int
		for i, r := range g.Roles {
			if r.name == "Contributor" {
				idxContrib = i
			}
			if r.name == "Supporter" {
				idxSupport = i
			}
		}
		g.Roles[idxContrib].position, g.Roles[idxSupport].position = g.Roles[idxSupport].position, g.Roles[idxContrib].position

		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.roles = g.Roles
		fake.channels = chs
		fakeSrv := fake.server()
		defer fakeSrv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		var out bytes.Buffer
		fake.recorded = nil
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: fakeSrv.URL})
		if err != nil {
			t.Fatalf("apply failed: %v", err)
		}

		var patchCall *recordedCall
		for i := range fake.recorded {
			if fake.recorded[i].method == http.MethodPatch && strings.HasSuffix(fake.recorded[i].path, "/roles") {
				patchCall = &fake.recorded[i]
				break
			}
		}
		if patchCall == nil {
			t.Fatal("expected PATCH roles call")
		}
		var updates []struct {
			ID       string `json:"id"`
			Position int    `json:"position"`
		}
		if err := json.Unmarshal(patchCall.body, &updates); err != nil {
			t.Fatalf("Unmarshal patch call: %v", err)
		}
		if len(updates) == 0 {
			t.Fatalf("expected updates in PATCH call, got 0")
		}
	})
}
func TestApplyServer_Negative_Failures(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	fakeToken := makeFakeToken(botUserID)
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	defDigest := server.DefinitionDigest()

	t.Run("failWrite on write k", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		srv := fake.server()
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		const k = 2
		fake.failWrite = k
		fake.failStatus = http.StatusForbidden
		fake.failBody = `{"code":50013,"message":"Missing Permissions"}`
		fake.recorded = nil

		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: srv.URL})
		if err == nil {
			t.Fatal("expected failure on write k, got nil")
		}

		errMsg := err.Error()
		stepStr := fmt.Sprintf("step %d", k)
		if !strings.Contains(errMsg, stepStr) || !strings.Contains(errMsg, "50013") {
			t.Errorf("error %q should contain %q and 50013", errMsg, stepStr)
		}
		if !strings.Contains(errMsg, res.Changes[k-1].Action) {
			t.Errorf("error %q should contain action %s", errMsg, res.Changes[k-1].Action)
		}

		// Writes after k: none
		var writes int
		for _, r := range fake.recorded {
			if r.method != http.MethodGet {
				writes++
			}
		}
		if writes != k {
			t.Errorf("recorded %d writes, want exactly %d", writes, k)
		}

		// Output has k-1 lines done, 1 failed, last line applied: k-1 of n
		outStr := out.String()
		wantDonePrefix := fmt.Sprintf("  %d. %s %s: done\n", res.Changes[0].Step, res.Changes[0].Action, res.Changes[0].Object)
		if !strings.Contains(outStr, wantDonePrefix) {
			t.Errorf("output missing done line: %s", outStr)
		}
		wantFailedLine := fmt.Sprintf("  %d. %s %s: failed\n", res.Changes[1].Step, res.Changes[1].Action, res.Changes[1].Object)
		if !strings.Contains(outStr, wantFailedLine) {
			t.Errorf("output missing failed line: %s", outStr)
		}
		wantAppliedLine := fmt.Sprintf("applied: %d of %d\n", k-1, len(res.Changes))
		if !strings.HasSuffix(outStr, wantAppliedLine) {
			t.Errorf("output does not end with %q: %s", wantAppliedLine, outStr)
		}
	})

	t.Run("hangWrite with 50ms timeout errors inside 2s", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.hangWrite = 1
		srv := fake.server()
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		fake.resetRecorded()
		opts := clientOptions{
			restURL:          srv.URL,
			applyCallTimeout: 50 * time.Millisecond,
		}

		start := time.Now()
		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, opts)
		duration := time.Since(start)

		close(fake.hangRelease)
		srv.Close()

		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if duration > 2*time.Second {
			t.Errorf("timeout took %v, want < 2s", duration)
		}
		var writes int
		for _, r := range fake.getRecorded() {
			if r.method != http.MethodGet {
				writes++
			}
		}
		if writes != 1 {
			t.Errorf("recorded %d writes, want exactly 1", writes)
		}
	})

	t.Run("ctx cancelled after write 1 -> no write 3", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		srv := fake.server()
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		fake.onWrite = func(c int) {
			if c == 1 {
				cancel()
			}
		}
		fake.recorded = nil

		var out bytes.Buffer
		err = applyServerWithOptions(ctx, fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: srv.URL})
		if err == nil {
			t.Fatal("expected error on cancelled context, got nil")
		}

		var writes int
		for _, r := range fake.recorded {
			if r.method != http.MethodGet {
				writes++
			}
		}
		if writes > 2 {
			t.Errorf("recorded %d writes, want at most 2 (no write 3)", writes)
		}
	})

	t.Run("refusal body echoes Authorization header -> token not in error", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.failWrite = 1
		fake.failStatus = http.StatusUnauthorized
		fake.failBody = fmt.Sprintf(`{"code":40001,"message":"unauthorized: Bot %s"}`, fakeToken)
		srv := fake.server()
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: srv.URL})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if strings.Contains(err.Error(), fakeToken) {
			t.Errorf("error leaks token: %q", err.Error())
		}
	})

	t.Run("transport failure during write -> no id in error", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		srv := fake.server()
		defer srv.Close()

		snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		planDigest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			t.Fatalf("digest: %v", err)
		}

		fake.recorded = nil
		fake.failTransportWrite = 1

		var out bytes.Buffer
		err = applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
			Definition:       def,
			DefinitionDigest: defDigest,
			PlanDigest:       planDigest,
		}, &out, clientOptions{restURL: srv.URL})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		for _, id := range []uint64{guildID, botUserID, botRoleID} {
			if strings.Contains(err.Error(), strconv.FormatUint(id, 10)) {
				t.Errorf("error contains snowflake id %d: %q", id, err.Error())
			}
		}
	})
}

func TestApplyServer_Boundary_Unit(t *testing.T) {
	t.Run("executeSingleChange unknown action delete-role returns error", func(t *testing.T) {
		client, err := newRestClient(makeFakeToken(1001), "")
		if err != nil {
			t.Fatalf("newRestClient: %v", err)
		}
		defer client.Close(context.Background())

		c := serverplan.Change{
			Step:   1,
			Action: "delete-role",
			Object: "roles[dev]",
		}
		err = executeSingleChange(context.Background(), client, 1001, ApplyInput{}, c, nil, 10*time.Second)
		if err == nil {
			t.Fatal("expected error for delete-role, got nil")
		}
	})

	t.Run("server_apply.go holds no delete methods", func(t *testing.T) {
		content, err := os.ReadFile("server_apply.go")
		if err != nil {
			t.Fatalf("read server_apply.go: %v", err)
		}
		s := string(content)
		if strings.Contains(s, ".Delete") {
			t.Errorf("server_apply.go contains '.Delete'")
		}
		if strings.Contains(s, "MethodDelete") {
			t.Errorf("server_apply.go contains 'MethodDelete'")
		}
	})
}
