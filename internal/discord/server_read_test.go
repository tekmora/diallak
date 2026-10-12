package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

type recordedReq struct {
	method string
	path   string
}

// buildPlatformFixtureFromDef constructs Discord API JSON responses from server.Definition().
func buildPlatformFixtureFromDef(t *testing.T, guildID uint64, botUserID uint64, botRoleID uint64) (
	appJSON string,
	guildJSON string,
	memberJSON string,
	channelsJSON string,
) {
	t.Helper()
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}

	appJSON = fmt.Sprintf(`{"id":"9001","name":"diallak-app","bot":{"id":"%d","username":"diallak"}}`, botUserID)

	// Roles
	type roleTag struct {
		BotID *string `json:"bot_id,omitempty"`
	}
	type apiRole struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Position    int      `json:"position"`
		Permissions string   `json:"permissions"`
		Managed     bool     `json:"managed"`
		Hoist       bool     `json:"hoist"`
		Mentionable bool     `json:"mentionable"`
		Tags        *roleTag `json:"tags,omitempty"`
	}

	roleIDMap := make(map[string]uint64, len(def.Roles))
	var apiRoles []apiRole
	nextRoleID := uint64(5000)

	// Position assignments: roles in RoleOrder get descending positions above managed roles.
	// Everyone role gets position 0 and ID = guildID.
	for i, id := range def.RoleOrder {
		var defRole *serverdef.Role
		for rIdx := range def.Roles {
			if def.Roles[rIdx].ID == id {
				defRole = &def.Roles[rIdx]
				break
			}
		}
		if defRole == nil {
			t.Fatalf("role %s not found in def.Roles", id)
		}
		rID := nextRoleID
		nextRoleID++
		var tags *roleTag
		managed := false
		if defRole.Kind == "bot" {
			rID = botRoleID
			bIDStr := strconv.FormatUint(botUserID, 10)
			tags = &roleTag{BotID: &bIDStr}
			managed = true
		}
		roleIDMap[id] = rID

		mask, maskErr := serverdefPermissionMask(defRole.Permissions)
		if maskErr != nil {
			t.Fatalf("PermissionMask: %v", maskErr)
		}
		var hoist, mentionable bool
		if defRole.ShownApart != nil {
			hoist = *defRole.ShownApart
		}
		if defRole.Mentionable != nil {
			mentionable = *defRole.Mentionable
		}
		apiRoles = append(apiRoles, apiRole{
			ID:          strconv.FormatUint(rID, 10),
			Name:        defRole.Name,
			Position:    len(def.RoleOrder) + 3 - i,
			Permissions: strconv.FormatUint(mask, 10),
			Managed:     managed,
			Hoist:       hoist,
			Mentionable: mentionable,
			Tags:        tags,
		})
	}

	// Managed roles below moderator
	managedIDs := []string{"booster", "funding-ko-fi", "funding-patreon"}
	for mIdx, mID := range managedIDs {
		var mRole *serverdef.Role
		for rIdx := range def.Roles {
			if def.Roles[rIdx].ID == mID {
				mRole = &def.Roles[rIdx]
				break
			}
		}
		if mRole == nil {
			t.Fatalf("managed role %s not found in def.Roles", mID)
		}
		rID := nextRoleID
		nextRoleID++
		roleIDMap[mID] = rID
		var hoist, mentionable bool
		if mRole.ShownApart != nil {
			hoist = *mRole.ShownApart
		}
		if mRole.Mentionable != nil {
			mentionable = *mRole.Mentionable
		}
		apiRoles = append(apiRoles, apiRole{
			ID:          strconv.FormatUint(rID, 10),
			Name:        mRole.Name,
			Position:    len(managedIDs) - mIdx,
			Permissions: "0",
			Managed:     true,
			Hoist:       hoist,
			Mentionable: mentionable,
		})
	}

	// Everyone role
	for rIdx := range def.Roles {
		if def.Roles[rIdx].Kind == "everyone" {
			roleIDMap[def.Roles[rIdx].ID] = guildID
			mask, maskErr := serverdefPermissionMask(def.Roles[rIdx].Permissions)
			if maskErr != nil {
				t.Fatalf("PermissionMask everyone: %v", maskErr)
			}
			var hoist, mentionable bool
			if def.Roles[rIdx].ShownApart != nil {
				hoist = *def.Roles[rIdx].ShownApart
			}
			if def.Roles[rIdx].Mentionable != nil {
				mentionable = *def.Roles[rIdx].Mentionable
			}
			apiRoles = append(apiRoles, apiRole{
				ID:          strconv.FormatUint(guildID, 10),
				Name:        def.Roles[rIdx].Name,
				Position:    0,
				Permissions: strconv.FormatUint(mask, 10),
				Managed:     false,
				Hoist:       hoist,
				Mentionable: mentionable,
			})
			break
		}
	}

	// Guild JSON
	type apiGuild struct {
		ID       string    `json:"id"`
		Name     string    `json:"name"`
		Features []string  `json:"features"`
		Roles    []apiRole `json:"roles"`
	}
	g := apiGuild{
		ID:       strconv.FormatUint(guildID, 10),
		Name:     "Test Hub",
		Features: []string{"COMMUNITY", "NEWS"},
		Roles:    apiRoles,
	}
	gBytes, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal guild: %v", err)
	}
	guildJSON = string(gBytes)

	// Member JSON
	type apiMember struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		RoleIDs []string `json:"roles"`
	}
	m := apiMember{}
	m.User.ID = strconv.FormatUint(botUserID, 10)
	m.RoleIDs = []string{strconv.FormatUint(botRoleID, 10)}
	mBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal member: %v", err)
	}
	memberJSON = string(mBytes)

	// Channels
	type apiOverwrite struct {
		ID    string `json:"id"`
		Type  int    `json:"type"`
		Allow string `json:"allow"`
		Deny  string `json:"deny"`
	}
	type apiChannel struct {
		ID                   string         `json:"id"`
		Name                 string         `json:"name"`
		Type                 int            `json:"type"`
		ParentID             *string        `json:"parent_id,omitempty"`
		Position             int            `json:"position"`
		PermissionOverwrites []apiOverwrite `json:"permission_overwrites"`
	}

	var apiChannels []apiChannel
	nextChanID := uint64(7000)
	catIDMap := make(map[string]string, len(def.Categories))

	// Categories (type 4)
	for i, c := range def.Categories {
		cID := strconv.FormatUint(nextChanID, 10)
		nextChanID++
		catIDMap[c.ID] = cID
		apiChannels = append(apiChannels, apiChannel{
			ID:                   cID,
			Name:                 c.Name,
			Type:                 4,
			Position:             i,
			PermissionOverwrites: []apiOverwrite{},
		})
	}

	// Channels
	for i, ch := range def.Channels {
		chID := strconv.FormatUint(nextChanID, 10)
		nextChanID++
		var typeInt int
		switch ch.Type {
		case "text":
			typeInt = 0
		case "voice":
			typeInt = 2
		case "announcement":
			typeInt = 5
		case "forum":
			typeInt = 15
		default:
			typeInt = 0
		}

		parentIDStr := catIDMap[ch.Category]
		var parentIDPtr *string
		if parentIDStr != "" {
			parentIDPtr = &parentIDStr
		}

		var overwrites []apiOverwrite
		for _, acc := range ch.Access {
			rID := roleIDMap[acc.Role]
			allowMask, err := serverdefPermissionMask(acc.Allow)
			if err != nil {
				t.Fatalf("permission mask allow: %v", err)
			}
			denyMask, err := serverdefPermissionMask(acc.Deny)
			if err != nil {
				t.Fatalf("permission mask deny: %v", err)
			}
			overwrites = append(overwrites, apiOverwrite{
				ID:    strconv.FormatUint(rID, 10),
				Type:  0, // role
				Allow: strconv.FormatUint(allowMask, 10),
				Deny:  strconv.FormatUint(denyMask, 10),
			})
		}

		apiChannels = append(apiChannels, apiChannel{
			ID:                   chID,
			Name:                 ch.Name,
			Type:                 typeInt,
			ParentID:             parentIDPtr,
			Position:             i,
			PermissionOverwrites: overwrites,
		})
	}

	chBytes, err := json.Marshal(apiChannels)
	if err != nil {
		t.Fatalf("marshal channels: %v", err)
	}
	channelsJSON = string(chBytes)

	return appJSON, guildJSON, memberJSON, channelsJSON
}

func serverdefPermissionMask(names []string) (uint64, error) {
	return serverdef.PermissionMask(names)
}

func newStandardServer(guildID, botUserID uint64, appJSON, guildJSON, memberJSON, channelsJSON string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/applications/@me":
			writeTestResponse(w, http.StatusOK, appJSON)
		case fmt.Sprintf("/guilds/%d", guildID):
			writeTestResponse(w, http.StatusOK, guildJSON)
		case fmt.Sprintf("/guilds/%d/members/%d", guildID, botUserID):
			writeTestResponse(w, http.StatusOK, memberJSON)
		case fmt.Sprintf("/guilds/%d/channels", guildID):
			writeTestResponse(w, http.StatusOK, channelsJSON)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestReadServer_Positive_FixtureMatchesDefinition(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	appJSON, guildJSON, memberJSON, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition: %v", err)
	}

	diffs, err := serverplan.Compare(def, snap)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("Compare returned %d differences, want 0: %+v", len(diffs), diffs)
	}

	planResult, err := serverplan.Plan(def, snap)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(planResult.Changes) != 0 {
		t.Errorf("Plan returned %d changes, want 0: %+v", len(planResult.Changes), planResult.Changes)
	}
}

func TestReadServer_Positive_FourRequestsInOrderAndNothingElse(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	appJSON, guildJSON, memberJSON, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)

	var mu sync.Mutex
	var recorded []recordedReq

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		recorded = append(recorded, recordedReq{method: r.Method, path: r.URL.Path})
		mu.Unlock()

		switch r.URL.Path {
		case "/applications/@me":
			writeTestResponse(w, http.StatusOK, appJSON)
		case fmt.Sprintf("/guilds/%d", guildID):
			writeTestResponse(w, http.StatusOK, guildJSON)
		case fmt.Sprintf("/guilds/%d/members/%d", guildID, botUserID):
			writeTestResponse(w, http.StatusOK, memberJSON)
		case fmt.Sprintf("/guilds/%d/channels", guildID):
			writeTestResponse(w, http.StatusOK, channelsJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(recorded) != 4 {
		t.Fatalf("recorded %d requests, want exactly 4: %+v", len(recorded), recorded)
	}
	wantPaths := []string{
		"/applications/@me",
		fmt.Sprintf("/guilds/%d", guildID),
		fmt.Sprintf("/guilds/%d/members/%d", guildID, botUserID),
		fmt.Sprintf("/guilds/%d/channels", guildID),
	}
	for i, want := range wantPaths {
		if recorded[i].method != http.MethodGet {
			t.Errorf("request %d method = %q, want GET", i, recorded[i].method)
		}
		if recorded[i].path != want {
			t.Errorf("request %d path = %q, want %q", i, recorded[i].path, want)
		}
	}
}

func TestReadServer_Positive_RoleOrderPositionThenID(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5002", "name": "Alpha", "position": 5, "permissions": "1024"},
			{"id": "5001", "name": "Beta", "position": 5, "permissions": "1024"},
			{"id": "5003", "name": "BotRole", "position": 8, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5003"]}`, botUserID)
	channelsJSON := `[]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	// Expected order:
	// 1. BotRole (position 8)
	// 2. Beta (position 5, id 5001 < 5002)
	// 3. Alpha (position 5, id 5002)
	// 4. @everyone (everyone=true, moved to last place)
	if len(snap.Roles) != 4 {
		t.Fatalf("len(snap.Roles) = %d, want 4", len(snap.Roles))
	}
	wantOrder := []string{"BotRole", "Beta", "Alpha", "@everyone"}
	for i, want := range wantOrder {
		if snap.Roles[i].Name != want {
			t.Errorf("role %d = %q, want %q", i, snap.Roles[i].Name, want)
		}
	}
	if !snap.Roles[3].Everyone {
		t.Error("last role should have Everyone=true")
	}
}

func TestReadServer_Positive_EveryoneRoleMovedToLast(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	// @everyone has position 10, higher than MemberRole (position 1)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 10, "permissions": "1024"},
			{"id": "5001", "name": "MemberRole", "position": 1, "permissions": "1024"},
			{"id": "5002", "name": "BotRole", "position": 12, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5002"]}`, botUserID)
	channelsJSON := `[]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	if len(snap.Roles) != 3 {
		t.Fatalf("len(snap.Roles) = %d, want 3", len(snap.Roles))
	}
	if snap.Roles[2].Name != "@everyone" || !snap.Roles[2].Everyone {
		t.Errorf("last role = %+v, want @everyone with Everyone=true", snap.Roles[2])
	}
}

func TestReadServer_Positive_CallerRoleByBotID(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	// Two managed roles: one for an integration (no bot_id), one for another bot (9999), and one for our bot (2001)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "OtherIntegration", "position": 2, "permissions": "1024", "managed": true, "tags": {"integration_id": "8888"}},
			{"id": "5002", "name": "OtherBot", "position": 3, "permissions": "1024", "managed": true, "tags": {"bot_id": "9999"}},
			{"id": "5003", "name": "DiallakBot", "position": 4, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5003"]}`, botUserID)
	channelsJSON := `[]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	if snap.Caller.Role != "DiallakBot" {
		t.Errorf("Caller.Role = %q, want DiallakBot", snap.Caller.Role)
	}
}

func TestReadServer_Positive_CallerPermissionsUnion(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	// @everyone has VIEW_CHANNEL (1<<10 = 1024)
	// Role A (bot role) has SEND_MESSAGES (1<<11 = 2048)
	// Role B (extra role held by bot) has EMBED_LINKS (1<<14 = 16384)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 2, "permissions": "2048", "managed": true, "tags": {"bot_id": "%d"}},
			{"id": "5002", "name": "ExtraRole", "position": 3, "permissions": "16384"}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001","5002"]}`, botUserID)
	channelsJSON := `[]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	wantPerms := []string{"VIEW_CHANNEL", "SEND_MESSAGES", "EMBED_LINKS"}
	if len(snap.Caller.Permissions) != len(wantPerms) {
		t.Fatalf("Caller.Permissions = %v, want %v", snap.Caller.Permissions, wantPerms)
	}
	for i, want := range wantPerms {
		if snap.Caller.Permissions[i] != want {
			t.Errorf("Caller.Permissions[%d] = %q, want %q", i, snap.Caller.Permissions[i], want)
		}
	}
}

func TestReadServer_Positive_ChannelTypes(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001"]}`, botUserID)
	// Channel types 0, 2, 5, 15, 13 -> text, voice, announcement, forum, other
	channelsJSON := `[
		{"id": "7001", "name": "ch-text", "type": 0, "permission_overwrites": []},
		{"id": "7002", "name": "ch-voice", "type": 2, "permission_overwrites": []},
		{"id": "7003", "name": "ch-news", "type": 5, "permission_overwrites": []},
		{"id": "7004", "name": "ch-forum", "type": 15, "permission_overwrites": []},
		{"id": "7005", "name": "ch-stage", "type": 13, "permission_overwrites": []}
	]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	wantTypes := map[string]string{
		"ch-text":  "text",
		"ch-voice": "voice",
		"ch-news":  "announcement",
		"ch-forum": "forum",
		"ch-stage": "other",
	}

	for _, ch := range snap.Channels {
		want, ok := wantTypes[ch.Name]
		if !ok {
			t.Errorf("unexpected channel %q", ch.Name)
			continue
		}
		if ch.Type != want {
			t.Errorf("channel %q type = %q, want %q", ch.Name, ch.Type, want)
		}
	}
}

func TestReadServer_Positive_MemberOverwritesCounted(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001"]}`, botUserID)
	// 2 role overwrites, 3 member overwrites (type 1)
	channelsJSON := fmt.Sprintf(`[
		{
			"id": "7001",
			"name": "ch-general",
			"type": 0,
			"permission_overwrites": [
				{"id": "%d", "type": 0, "allow": "1024", "deny": "0"},
				{"id": "5001", "type": 0, "allow": "1024", "deny": "0"},
				{"id": "8001", "type": 1, "allow": "1024", "deny": "0"},
				{"id": "8002", "type": 1, "allow": "1024", "deny": "0"},
				{"id": "8003", "type": 1, "allow": "1024", "deny": "0"}
			]
		}
	]`, guildID)

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	if len(snap.Channels) != 1 {
		t.Fatalf("len(snap.Channels) = %d, want 1", len(snap.Channels))
	}
	if snap.Channels[0].MemberEntries != 3 {
		t.Errorf("MemberEntries = %d, want 3", snap.Channels[0].MemberEntries)
	}
	if len(snap.Channels[0].Access) != 2 {
		t.Errorf("len(Access) = %d, want 2 role overwrites", len(snap.Channels[0].Access))
	}
}

func TestReadServer_Positive_ChannelsComplete_CutoffAndAdmin(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	// Without admin: VIEW_CHANNEL (1024)
	noAdminGuild := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	// With admin: ADMINISTRATOR (8)
	adminGuild := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 1, "permissions": "8", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001"]}`, botUserID)
	channelsJSON := `[]`

	cutoff, err := time.Parse(time.RFC3339, "2026-11-16T00:00:00Z")
	if err != nil {
		t.Fatalf("parse cutoff: %v", err)
	}

	cases := []struct {
		name      string
		guildBody string
		nowTime   time.Time
		wantComp  bool
	}{
		{
			name:      "administrator after cutoff -> true",
			guildBody: adminGuild,
			nowTime:   cutoff.Add(24 * time.Hour),
			wantComp:  true,
		},
		{
			name:      "no administrator 1 nanosecond before cutoff -> true",
			guildBody: noAdminGuild,
			nowTime:   cutoff.Add(-1 * time.Nanosecond),
			wantComp:  true,
		},
		{
			name:      "no administrator exactly at cutoff -> false",
			guildBody: noAdminGuild,
			nowTime:   cutoff,
			wantComp:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newStandardServer(guildID, botUserID, appJSON, tc.guildBody, memberJSON, channelsJSON)
			defer srv.Close()

			opts := clientOptions{
				restURL: srv.URL,
				now:     func() time.Time { return tc.nowTime },
			}

			snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
			if err != nil {
				t.Fatalf("readServerWithOptions: %v", err)
			}
			if snap.ChannelsComplete != tc.wantComp {
				t.Errorf("ChannelsComplete = %v, want %v", snap.ChannelsComplete, tc.wantComp)
			}
		})
	}
}

func TestReadServer_Positive_UnknownBitOnRoleKept(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	// The role an integration manages holds bit 47 beside VIEW_CHANNEL, as
	// the first read of a live server found it: 1<<47 | 1<<10.
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "Server Booster", "position": 3, "permissions": "140737488356352", "managed": true},
			{"id": "5002", "name": "BotRole", "position": 2, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5002"]}`, botUserID)
	channelsJSON := `[]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions failed on a bit the table does not name: %v", err)
	}
	if err := serverplan.Check(snap); err != nil {
		t.Fatalf("snapshot does not pass its own check: %v", err)
	}
	if len(snap.Roles) != 3 {
		t.Fatalf("roles = %+v, want 3", snap.Roles)
	}
	want := serverplan.Role{Name: "Server Booster", Managed: true, Permissions: []string{"VIEW_CHANNEL"}, UnknownBits: []int{47}}
	if !reflect.DeepEqual(snap.Roles[0], want) {
		t.Errorf("role = %+v, want %+v", snap.Roles[0], want)
	}
	for _, r := range snap.Roles[1:] {
		if r.UnknownBits != nil {
			t.Errorf("role %q has unknown bits %v, want none", r.Name, r.UnknownBits)
		}
	}
}

func TestReadServer_Positive_UnknownBitOnEntryKept(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001"]}`, botUserID)
	// Channel overwrite has bit 47 in allow
	channelsJSON := fmt.Sprintf(`[
		{
			"id": "7001",
			"name": "ch-test",
			"type": 0,
			"permission_overwrites": [
				{"id": "%d", "type": 0, "allow": "140737488355328", "deny": "0"}
			]
		}
	]`, guildID)

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions failed on an entry bit the table does not name: %v", err)
	}
	if err := serverplan.Check(snap); err != nil {
		t.Fatalf("snapshot does not pass its own check: %v", err)
	}
	if len(snap.Channels) != 1 || len(snap.Channels[0].Access) != 1 {
		t.Fatalf("channels = %+v, want one channel with one entry", snap.Channels)
	}
	want := serverplan.Access{Role: "@everyone", Allow: []string{}, Deny: []string{}, UnknownAllow: []int{47}}
	if !reflect.DeepEqual(snap.Channels[0].Access[0], want) {
		t.Errorf("entry = %+v, want %+v", snap.Channels[0].Access[0], want)
	}
}

func TestReadServer_Negative_NoBotInApplication(t *testing.T) {
	const guildID uint64 = 1001
	appJSON := `{"id":"9001","name":"no-bot-app"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/applications/@me" {
			writeTestResponse(w, http.StatusOK, appJSON)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err == nil {
		t.Fatal("readServerWithOptions succeeded, want error")
	}
	const wantErr = "get application: the application has no bot user"
	if err.Error() != wantErr {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

func TestReadServer_Negative_NoManagedRoleOfBot(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	// No role has tags.bot_id == 2001
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "RegularRole", "position": 1, "permissions": "1024"}
		]
	}`, guildID, guildID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001"]}`, botUserID)
	channelsJSON := `[]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err == nil {
		t.Fatal("readServerWithOptions succeeded, want error")
	}
	const wantErr = "the bot has no role of its own on this server"
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

func TestReadServer_Negative_MemberLacksRole(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	// Bot role exists on guild (5002), but member does not have it in member.roles
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5002", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":[]}`, botUserID)
	channelsJSON := `[]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err == nil {
		t.Fatal("readServerWithOptions succeeded, want error")
	}
	const wantErr = "the bot has no role of its own on this server"
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

func TestReadServer_Negative_ParentCategoryMissing(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001"]}`, botUserID)
	// Channel names non-existent parent_id 9999
	channelsJSON := `[
		{"id": "7001", "name": "orphan-channel", "type": 0, "parent_id": "9999", "permission_overwrites": []}
	]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err == nil {
		t.Fatal("readServerWithOptions succeeded, want error")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "orphan-channel") {
		t.Errorf("error %q does not name channel orphan-channel", errStr)
	}
	if strings.Contains(errStr, "9999") {
		t.Errorf("error %q contains platform ID 9999", errStr)
	}
}

func TestReadServer_Negative_OverwriteUnknownRole(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001"]}`, botUserID)
	// Overwrite names role 9999 not in guild roles
	channelsJSON := `[
		{
			"id": "7001",
			"name": "ch-unknown-role",
			"type": 0,
			"permission_overwrites": [
				{"id": "9999", "type": 0, "allow": "1024", "deny": "0"}
			]
		}
	]`

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err == nil {
		t.Fatal("readServerWithOptions succeeded, want error")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "ch-unknown-role") {
		t.Errorf("error %q does not name channel ch-unknown-role", errStr)
	}
	if strings.Contains(errStr, "9999") {
		t.Errorf("error %q contains platform ID 9999", errStr)
	}
}

func TestReadServer_Negative_GuildError500StopsPipeline(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)

	var mu sync.Mutex
	var memberCalls, channelCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if strings.Contains(r.URL.Path, "/members/") {
			memberCalls++
		}
		if strings.Contains(r.URL.Path, "/channels") {
			channelCalls++
		}
		mu.Unlock()

		switch r.URL.Path {
		case "/applications/@me":
			writeTestResponse(w, http.StatusOK, appJSON)
		case fmt.Sprintf("/guilds/%d", guildID):
			writeTestResponse(w, http.StatusInternalServerError, `{"message":"Internal Server Error","code":0}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err == nil {
		t.Fatal("readServerWithOptions succeeded on guild 500, want error")
	}
	if !strings.Contains(err.Error(), "get guild") {
		t.Errorf("error %q does not name step 'get guild'", err.Error())
	}

	mu.Lock()
	mCalls := memberCalls
	cCalls := channelCalls
	mu.Unlock()

	if mCalls != 0 {
		t.Errorf("memberCalls = %d, want 0", mCalls)
	}
	if cCalls != 0 {
		t.Errorf("channelCalls = %d, want 0", cCalls)
	}
}

func TestReadServer_Negative_HangingRequestTimesOut(t *testing.T) {
	cleanupCh := make(chan struct{})
	t.Cleanup(func() { close(cleanupCh) })

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-cleanupCh:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	opts := clientOptions{
		restURL:           srv.URL,
		serverCallTimeout: 50 * time.Millisecond,
	}

	start := time.Now()
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), 1001, opts)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("readServerWithOptions succeeded on hanging server, want timeout error")
	}
	if duration > 500*time.Millisecond {
		t.Errorf("took %v, want timeout within ~50ms", duration)
	}
}

func TestReadServer_Negative_AuthorizationHeaderSanitized(t *testing.T) {
	secretToken := makeFakeToken(1001)

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		auth := r.Header.Get("Authorization")
		writeTestResponse(w, http.StatusUnauthorized, fmt.Sprintf(`{"message":"unauthorized: %s"}`, auth))
	}))
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	_, err := readServerWithOptions(context.Background(), secretToken, 1001, opts)
	if err == nil {
		t.Fatal("readServerWithOptions succeeded, want error")
	}
	if hits != 1 {
		t.Fatalf("requests = %d, want 1: the reply that repeats the token was never read", hits)
	}
	errStr := err.Error()
	if strings.Contains(errStr, secretToken) {
		t.Errorf("error contains token: %q", errStr)
	}
	if !strings.HasPrefix(errStr, "get application: ") || !strings.Contains(errStr, "unauthorized: Bot [redacted]") {
		t.Errorf("error = %q, want the step and the reply with the token replaced", errStr)
	}
}

func TestReadServer_Positive_SnapshotMarshalledHoldsNoIDs(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	appJSON, guildJSON, memberJSON, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
	defer srv.Close()

	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}

	snapJSON, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal(snap): %v", err)
	}
	snapStr := string(snapJSON)

	idsToCheck := []uint64{guildID, botUserID, botRoleID, 9001, 7001, 7002}
	for _, id := range idsToCheck {
		idStr := strconv.FormatUint(id, 10)
		if strings.Contains(snapStr, idStr) {
			t.Errorf("snapshot JSON contains platform ID %s: %s", idStr, snapStr)
		}
	}
}

// A transport failure comes back from the HTTP client with the request's address,
// and the address of the member request holds the server's id and the bot user's id.
func TestReadServer_Negative_TransportErrorNamesNoID(t *testing.T) {
	const (
		guildID   uint64 = 777000111
		botUserID uint64 = 888000222
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{"id":"%d","roles":[{"id":"%d","name":"@everyone","position":0,"permissions":"1024"}]}`, guildID, guildID)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/applications/@me":
			writeTestResponse(w, http.StatusOK, appJSON)
		case fmt.Sprintf("/guilds/%d", guildID):
			writeTestResponse(w, http.StatusOK, guildJSON)
		default:
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
	}))
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL, serverCallTimeout: 50 * time.Millisecond}
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, opts)
	if err == nil {
		t.Fatal("readServerWithOptions succeeded on a hanging member request, want error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %q, want it to wrap the passed deadline", err.Error())
	}
	if !strings.HasPrefix(err.Error(), "get member: ") {
		t.Errorf("error = %q, want the step \"get member\" first", err.Error())
	}
	for _, id := range []uint64{guildID, botUserID} {
		if strings.Contains(err.Error(), strconv.FormatUint(id, 10)) {
			t.Errorf("error holds the platform id %d: %q", id, err.Error())
		}
	}
	if strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error holds the request's address: %q", err.Error())
	}
}

// Between the role list and the member request a role can appear. A snapshot
// built from both would understate what the caller may do.
func TestReadServer_Negative_MemberRoleMissingFromRoleList(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{
		"id": "%d",
		"roles": [
			{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
			{"id": "5001", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}}
		]
	}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001","5777"]}`, botUserID)

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, `[]`)
	defer srv.Close()

	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, clientOptions{restURL: srv.URL})
	if err == nil {
		t.Fatal("readServerWithOptions succeeded, want error")
	}
	const wantErr = "the bot holds a role that the server's role list does not have"
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
	if strings.Contains(err.Error(), "5777") {
		t.Errorf("error holds the role's id: %q", err.Error())
	}
}

// The snapshot names the caller's role by name. With a second role of that name
// the plan could take the wrong one for the caller's place in the order, so the
// snapshot's own check refuses it.
func TestReadServer_Boundary_BotRoleNameHeldTwice(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001"]}`, botUserID)
	guild := func(secondName string) string {
		return fmt.Sprintf(`{
			"id": "%d",
			"roles": [
				{"id": "%d", "name": "@everyone", "position": 0, "permissions": "1024"},
				{"id": "5001", "name": "BotRole", "position": 1, "permissions": "1024", "managed": true, "tags": {"bot_id": "%d"}},
				{"id": "5002", "name": %q, "position": 2, "permissions": "1024"}
			]
		}`, guildID, guildID, botUserID, secondName)
	}

	twice := newStandardServer(guildID, botUserID, appJSON, guild("BotRole"), memberJSON, `[]`)
	defer twice.Close()
	_, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, clientOptions{restURL: twice.URL})
	const wantErr = `check snapshot: caller role "BotRole" names multiple roles`
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}

	once := newStandardServer(guildID, botUserID, appJSON, guild("Botrole"), memberJSON, `[]`)
	defer once.Close()
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, clientOptions{restURL: once.URL})
	if err != nil {
		t.Fatalf("a role whose name differs in case: %v", err)
	}
	if snap.Caller.Role != "BotRole" {
		t.Errorf("Caller.Role = %q, want BotRole", snap.Caller.Role)
	}
}

func TestReadServer_Boundary_UnknownBitsKeptByNumber(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	bit47 := uint64(1) << 47
	bits47and53 := bit47 | uint64(1)<<53
	// Every permission the table names, and with it the highest bit a signed
	// value can hold.
	documented, err := serverdef.PermissionMask(serverdef.Permissions())
	if err != nil {
		t.Fatalf("PermissionMask: %v", err)
	}
	cases := []struct {
		name          string
		everyonePerms uint64
		plainPerms    uint64
		entryAllow    uint64
		entryDeny     uint64
		wantEveryone  []int
		wantPlain     []int
		wantAllow     []int
		wantDeny      []int
		wantPlainLen  int
	}{
		{"no unknown bit", 1024, 1024, 1024, 0, nil, nil, nil, nil, 1},
		{"every documented bit", documented, documented, documented, 0, nil, nil, nil, nil, 52},
		{"everyone role", bit47 | 1024, 1024, 0, 0, []int{47}, nil, nil, nil, 1},
		{"plain role", 1024, bits47and53 | 1024, 0, 0, nil, []int{47, 53}, nil, nil, 1},
		{"entry allow", 1024, 1024, bit47, 0, nil, nil, []int{47}, nil, 1},
		{"entry deny", 1024, 1024, 0, bits47and53, nil, nil, nil, []int{47, 53}, 1},
		{"every side at once", bit47, bit47, bit47, bit47, []int{47}, []int{47}, []int{47}, []int{47}, 0},
		{"highest bit of the value", 1024, documented | uint64(1)<<62, 0, 0, nil, []int{62}, nil, nil, 52},
	}
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5002"]}`, botUserID)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guildJSON := fmt.Sprintf(`{"id":"%d","roles":[
				{"id":"%d","name":"@everyone","position":0,"permissions":"%d"},
				{"id":"5001","name":"Plain","position":1,"permissions":"%d"},
				{"id":"5002","name":"BotRole","position":2,"permissions":"1024","managed":true,"tags":{"bot_id":"%d"}}
			]}`, guildID, guildID, tc.everyonePerms, tc.plainPerms, botUserID)
			channelsJSON := fmt.Sprintf(`[{"id":"7001","name":"ch-test","type":0,"permission_overwrites":[
				{"id":"5001","type":0,"allow":"%d","deny":"%d"}
			]}]`, tc.entryAllow, tc.entryDeny)

			srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, channelsJSON)
			defer srv.Close()

			snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, clientOptions{restURL: srv.URL})
			if err != nil {
				t.Fatalf("readServerWithOptions failed: %v", err)
			}
			if err := serverplan.Check(snap); err != nil {
				t.Fatalf("snapshot does not pass its own check: %v", err)
			}
			// Roles from the highest: BotRole, Plain, @everyone.
			plain, everyone := snap.Roles[1], snap.Roles[2]
			entry := snap.Channels[0].Access[0]
			if !reflect.DeepEqual(everyone.UnknownBits, tc.wantEveryone) {
				t.Errorf("@everyone unknown bits = %v, want %v", everyone.UnknownBits, tc.wantEveryone)
			}
			if !reflect.DeepEqual(plain.UnknownBits, tc.wantPlain) {
				t.Errorf("Plain unknown bits = %v, want %v", plain.UnknownBits, tc.wantPlain)
			}
			if len(plain.Permissions) != tc.wantPlainLen {
				t.Errorf("Plain holds %d named permissions, want %d: %v", len(plain.Permissions), tc.wantPlainLen, plain.Permissions)
			}
			if !reflect.DeepEqual(entry.UnknownAllow, tc.wantAllow) {
				t.Errorf("entry unknown allow = %v, want %v", entry.UnknownAllow, tc.wantAllow)
			}
			if !reflect.DeepEqual(entry.UnknownDeny, tc.wantDeny) {
				t.Errorf("entry unknown deny = %v, want %v", entry.UnknownDeny, tc.wantDeny)
			}
		})
	}
}

func TestReadServer_Boundary_UnknownBitGivesTheCallerNoRight(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	// The bot's own role and a second role it holds both carry bit 47.
	guildJSON := fmt.Sprintf(`{"id":"%d","roles":[
		{"id":"%d","name":"@everyone","position":0,"permissions":"1024"},
		{"id":"5001","name":"Helper","position":1,"permissions":"140737488355328"},
		{"id":"5002","name":"BotRole","position":2,"permissions":"140737488356352","managed":true,"tags":{"bot_id":"%d"}}
	]}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5001","5002"]}`, botUserID)

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, `[]`)
	defer srv.Close()

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("readServerWithOptions failed: %v", err)
	}
	if want := []string{"VIEW_CHANNEL"}; !reflect.DeepEqual(snap.Caller.Permissions, want) {
		t.Errorf("caller permissions = %v, want %v", snap.Caller.Permissions, want)
	}
	if snap.Caller.Role != "BotRole" || !reflect.DeepEqual(snap.Roles[0].UnknownBits, []int{47}) {
		t.Errorf("caller role = %q with unknown bits %v, want BotRole with [47]", snap.Caller.Role, snap.Roles[0].UnknownBits)
	}
}

func TestReadServer_Positive_ManagedFlagKept(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
	)
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, botUserID)
	guildJSON := fmt.Sprintf(`{"id":"%d","roles":[
		{"id":"%d","name":"@everyone","position":0,"permissions":"1024"},
		{"id":"5001","name":"Plain","position":1,"permissions":"1024"},
		{"id":"5002","name":"BotRole","position":2,"permissions":"1024","managed":true,"tags":{"bot_id":"%d"}},
		{"id":"5003","name":"Booster","position":3,"permissions":"0","managed":true,"tags":{"premium_subscriber":null}}
	]}`, guildID, guildID, botUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5002"]}`, botUserID)

	srv := newStandardServer(guildID, botUserID, appJSON, guildJSON, memberJSON, `[]`)
	defer srv.Close()

	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), guildID, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}
	want := map[string]bool{"Booster": true, "BotRole": true, "Plain": false, "@everyone": false}
	if len(snap.Roles) != len(want) {
		t.Fatalf("roles = %d, want %d", len(snap.Roles), len(want))
	}
	for _, r := range snap.Roles {
		managed, known := want[r.Name]
		if !known {
			t.Errorf("unexpected role %q", r.Name)
			continue
		}
		if r.Managed != managed {
			t.Errorf("role %q managed = %v, want %v", r.Name, r.Managed, managed)
		}
	}
}
