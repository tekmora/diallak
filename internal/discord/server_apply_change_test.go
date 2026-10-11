package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

const missingPermissionsBody = `{"code":50013,"message":"Missing Permissions"}`

func mustDefinition(t *testing.T) *serverdef.Definition {
	t.Helper()
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	return def
}

// orderAuditReason is the audit reason as the library puts it in the header.
func orderAuditReason() string {
	return strings.ReplaceAll(url.QueryEscape("diallak apply "+orderDigest), "+", " ")
}

// runChange runs one change against the fake, with the index a run's first
// read would have built.
func runChange(
	t *testing.T,
	fake *fakePlatform,
	def *serverdef.Definition,
	idx *serverIndex,
	c serverplan.Change,
	timeout time.Duration,
) error {
	t.Helper()
	srv := fake.server()
	defer srv.Close()

	client, err := newRestClient(makeFakeToken(orderBotUserID), srv.URL)
	if err != nil {
		t.Fatalf("newRestClient: %v", err)
	}
	defer client.Close(context.Background())

	in := ApplyInput{Definition: def, PlanDigest: orderDigest}
	return executeSingleChange(context.Background(), client, orderGuildID, in, c, idx, timeout)
}

func assertRefusedCall(t *testing.T, err error, step string) {
	t.Helper()
	if err == nil {
		t.Fatalf("the refused request returned no error, want one of the call step %s", step)
	}
	if !strings.HasPrefix(err.Error(), step+": ") || !strings.Contains(err.Error(), "50013") {
		t.Errorf("error = %q, want the prefix %q and the platform's code 50013", err.Error(), step+": ")
	}
}

// changeFixture is a server that holds the objects of the definition under ids
// the test knows, and the index a run's first read would build from it.
type changeFixture struct {
	fake       *fakePlatform
	idx        *serverIndex
	roles      map[string]snowflake.ID
	categories map[string]snowflake.ID
	channels   map[string]snowflake.ID
}

// newChangeFixture leaves out the objects named in without, written as a
// change names them: roles[id], categories[id] or channels[id].
func newChangeFixture(t *testing.T, def *serverdef.Definition, without ...string) changeFixture {
	t.Helper()
	skip := make(map[string]bool, len(without))
	for _, object := range without {
		skip[object] = true
	}
	fx := changeFixture{
		fake:       newFakePlatform(t, orderGuildID, orderBotUserID, orderBotRoleID),
		idx:        newServerIndex(orderGuildID, snowflake.ID(orderBotRoleID)),
		roles:      map[string]snowflake.ID{},
		categories: map[string]snowflake.ID{},
		channels:   map[string]snowflake.ID{},
	}
	for i, r := range def.Roles {
		if skip["roles["+r.ID+"]"] {
			continue
		}
		id := snowflake.ID(10001 + i)
		switch r.Kind {
		case "everyone":
			id = snowflake.ID(orderGuildID)
		case "bot":
			id = snowflake.ID(orderBotRoleID)
		default:
			fx.idx.addRole(r.Name, id)
		}
		fx.roles[r.ID] = id
		fx.fake.roles = append(fx.fake.roles, fakeRole{id: id, name: r.Name, position: len(def.Roles) - i})
	}
	for i, c := range def.Categories {
		if skip["categories["+c.ID+"]"] {
			continue
		}
		id := snowflake.ID(20001 + i)
		fx.categories[c.ID] = id
		fx.idx.addCategory(c.Name, id)
		fx.fake.channels = append(fx.fake.channels, fakeChannel{id: id, t: int(discord.ChannelTypeGuildCategory), name: c.Name})
	}
	for i, c := range def.Channels {
		if skip["channels["+c.ID+"]"] {
			continue
		}
		id := snowflake.ID(30001 + i)
		fx.channels[c.ID] = id
		fx.idx.addChannel(c.Name, id)
		fx.fake.channels = append(fx.fake.channels, fakeChannel{id: id, name: c.Name})
	}
	return fx
}

// oneWrite returns the single request of a change that makes no read.
func oneWrite(t *testing.T, fake *fakePlatform) recordedCall {
	t.Helper()
	recorded := fake.getRecorded()
	if len(recorded) != 1 || recorded[0].method == http.MethodGet {
		t.Fatalf("recorded requests = %+v, want one write", recorded)
	}
	return recorded[0]
}

func permissionText(p discord.Permissions) string {
	return strconv.FormatInt(int64(p), 10)
}

func TestExecuteSingleChange_Negative_RefusalNamesTheCallStep(t *testing.T) {
	cases := []struct {
		action string
		object string
		step   string
	}{
		{"create-role", "roles[contributor]", "create role"},
		{"set-role-permissions", "roles[moderator]", "set role permissions"},
		{"create-category", "categories[start]", "create category"},
		{"create-channel", "channels[lounge]", "create channel"},
		{"move-channel", "channels[room-es]", "move channel"},
		{"set-access", "channels[tickets].access[everyone]", "set access"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			def := mustDefinition(t)
			fx := newChangeFixture(t, def)
			fx.fake.failWrite = 1
			fx.fake.failStatus = http.StatusForbidden
			fx.fake.failBody = missingPermissionsBody
			c := serverplan.Change{Step: 1, Action: tc.action, Object: tc.object}
			err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second)
			assertRefusedCall(t, err, tc.step)
			assertRequestCounts(t, fx.fake.getRecorded(), 0, 1)
		})
	}
}

func TestExecuteSingleChange_Boundary_CreatedObjectAndItsID(t *testing.T) {
	cases := []struct {
		action string
		object string
		lookup func(idx *serverIndex, def *serverdef.Definition) (snowflake.ID, error)
		noID   string
	}{
		{
			"create-role", "roles[contributor]",
			func(idx *serverIndex, _ *serverdef.Definition) (snowflake.ID, error) {
				return idx.lookupRole("Contributor")
			},
			"create role: returned role id is 0",
		},
		{
			"create-category", "categories[start]",
			func(idx *serverIndex, _ *serverdef.Definition) (snowflake.ID, error) {
				return idx.lookupCategory("Start")
			},
			"create category: returned category id is 0",
		},
		{
			"create-channel", "channels[lounge]",
			func(idx *serverIndex, _ *serverdef.Definition) (snowflake.ID, error) {
				return idx.lookupChannel("lounge")
			},
			"create channel: returned channel id is 0",
		},
	}
	for _, tc := range cases {
		c := serverplan.Change{Step: 1, Action: tc.action, Object: tc.object}
		t.Run(tc.action+" is indexed under the returned id", func(t *testing.T) {
			def := mustDefinition(t)
			fx := newChangeFixture(t, def, tc.object)
			want := snowflake.ID(fx.fake.nextID)
			if err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second); err != nil {
				t.Fatalf("%s error = %v, want nil", tc.action, err)
			}
			got, err := tc.lookup(fx.idx, def)
			if err != nil || got != want {
				t.Errorf("index holds %d, %v, want %d", got, err, want)
			}
			if call := oneWrite(t, fx.fake); call.auditReason != orderAuditReason() {
				t.Errorf("audit reason = %q, want %q", call.auditReason, orderAuditReason())
			}
		})
		t.Run(tc.action+" with the id 0 fails", func(t *testing.T) {
			def := mustDefinition(t)
			fx := newChangeFixture(t, def, tc.object)
			fx.fake.nextID = 0
			err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second)
			if err == nil || err.Error() != tc.noID {
				t.Errorf("%s error = %v, want %q", tc.action, err, tc.noID)
			}
			if got, err := tc.lookup(fx.idx, def); err == nil {
				t.Errorf("index holds %d for the object without an id", got)
			}
		})
	}
}

type channelRequest struct {
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

func TestApplyCreateChannel_Positive_ForumCarriesNameCategoryAndAccess(t *testing.T) {
	def := mustDefinition(t)
	forum, ok := findChannelInDef(def, "support")
	if !ok || forum.Type != "forum" {
		t.Fatalf("the definition holds no forum channel with the id support: %+v", forum)
	}
	// In the definition the name equals the id; here they differ.
	forum.Name = "help-desk"
	fx := newChangeFixture(t, def, "channels[support]")

	c := serverplan.Change{Step: 1, Action: "create-channel", Object: "channels[support]"}
	if err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second); err != nil {
		t.Fatalf("create-channel error = %v, want nil", err)
	}
	call := oneWrite(t, fx.fake)
	if call.method != http.MethodPost || call.path != fmt.Sprintf("/guilds/%d/channels", orderGuildID) {
		t.Errorf("request = %s %s, want the channel creation of the server", call.method, call.path)
	}
	var req channelRequest
	if err := json.Unmarshal(call.body, &req); err != nil {
		t.Fatalf("decode the request: %v", err)
	}
	if req.Name != "help-desk" || req.Type != int(discord.ChannelTypeGuildForum) {
		t.Errorf("name and type = %q, %d, want help-desk, %d", req.Name, req.Type, discord.ChannelTypeGuildForum)
	}
	if req.ParentID == nil || *req.ParentID != fx.categories["community"].String() {
		t.Errorf("parent = %v, want the category Community %d", req.ParentID, fx.categories["community"])
	}
	if len(req.PermissionOverwrites) != 1 {
		t.Fatalf("access entries = %+v, want one for the bot's role", req.PermissionOverwrites)
	}
	wantAllow := discord.PermissionViewChannel | discord.PermissionReadMessageHistory | discord.PermissionSendMessagesInThreads
	got := req.PermissionOverwrites[0]
	if got.ID != strconv.FormatUint(orderBotRoleID, 10) || got.Type != 0 || got.Allow != permissionText(wantAllow) || got.Deny != "0" {
		t.Errorf("access entry = %+v, want role %d, allow %s, deny 0", got, orderBotRoleID, permissionText(wantAllow))
	}
}

func TestApplyMoveChannel_Positive_RequestNamesTheCategoryAndTheReason(t *testing.T) {
	def := mustDefinition(t)
	fx := newChangeFixture(t, def)
	c := serverplan.Change{Step: 1, Action: "move-channel", Object: "channels[room-es]"}
	if err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second); err != nil {
		t.Fatalf("move-channel error = %v, want nil", err)
	}
	call := oneWrite(t, fx.fake)
	if call.method != http.MethodPatch || call.path != fmt.Sprintf("/channels/%d", fx.channels["room-es"]) {
		t.Errorf("request = %s %s, want the update of channel %d", call.method, call.path, fx.channels["room-es"])
	}
	var req map[string]any
	if err := json.Unmarshal(call.body, &req); err != nil {
		t.Fatalf("decode the request: %v", err)
	}
	if len(req) != 1 || req["parent_id"] != fx.categories["languages"].String() {
		t.Errorf("request body = %s, want only the category Languages %d", call.body, fx.categories["languages"])
	}
	if call.auditReason != orderAuditReason() {
		t.Errorf("audit reason = %q, want %q", call.auditReason, orderAuditReason())
	}
}

func TestApplySetAccess_Positive_RequestCarriesAllowDenyAndTheReason(t *testing.T) {
	def := mustDefinition(t)
	fx := newChangeFixture(t, def)
	c := serverplan.Change{Step: 1, Action: "set-access", Object: "channels[tickets].access[everyone]"}
	if err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second); err != nil {
		t.Fatalf("set-access error = %v, want nil", err)
	}
	call := oneWrite(t, fx.fake)
	wantPath := fmt.Sprintf("/channels/%d/permissions/%d", fx.channels["tickets"], orderGuildID)
	if call.method != http.MethodPut || call.path != wantPath {
		t.Errorf("request = %s %s, want PUT %s", call.method, call.path, wantPath)
	}
	var req struct {
		Type  int    `json:"type"`
		Allow string `json:"allow"`
		Deny  string `json:"deny"`
	}
	if err := json.Unmarshal(call.body, &req); err != nil {
		t.Fatalf("decode the request: %v", err)
	}
	wantAllow := permissionText(discord.PermissionCreatePrivateThreads)
	wantDeny := permissionText(discord.PermissionSendMessages | discord.PermissionCreatePublicThreads)
	if req.Type != 0 || req.Allow != wantAllow || req.Deny != wantDeny {
		t.Errorf("request body = %+v, want a role entry with allow %s and deny %s", req, wantAllow, wantDeny)
	}
	if call.auditReason != orderAuditReason() {
		t.Errorf("audit reason = %q, want %q", call.auditReason, orderAuditReason())
	}
}

func TestServerIndex_Boundary_NameHeldNeverOnceOrTwice(t *testing.T) {
	idx := newServerIndex(orderGuildID, snowflake.ID(orderBotRoleID))
	kinds := []struct {
		kind   string
		add    func(name string, id snowflake.ID)
		lookup func(name string) (snowflake.ID, error)
	}{
		{"role", idx.addRole, idx.lookupRole},
		{"category", idx.addCategory, idx.lookupCategory},
		{"channel", idx.addChannel, idx.lookupChannel},
	}
	for _, k := range kinds {
		t.Run(k.kind, func(t *testing.T) {
			want := fmt.Sprintf("%s %q not found", k.kind, "lounge")
			if _, err := k.lookup("lounge"); err == nil || err.Error() != want {
				t.Errorf("never held: error = %v, want %q", err, want)
			}
			k.add("lounge", 7001)
			if got, err := k.lookup("lounge"); err != nil || got != 7001 {
				t.Errorf("held once: %d, %v, want 7001", got, err)
			}
			k.add("lounge", 7002)
			want = fmt.Sprintf("%s %q is held more than once", k.kind, "lounge")
			if _, err := k.lookup("lounge"); err == nil || err.Error() != want {
				t.Errorf("held twice: error = %v, want %q", err, want)
			}
		})
	}
}

func TestClientOptions_Boundary_ApplyCallTimeout(t *testing.T) {
	if got := (clientOptions{}).getApplyCallTimeout(); got != 60*time.Second {
		t.Errorf("default = %v, want 60s", got)
	}
	if got := (clientOptions{applyCallTimeout: time.Second}).getApplyCallTimeout(); got != time.Second {
		t.Errorf("set to one second: %v, want 1s", got)
	}
	if got := (clientOptions{applyCallTimeout: -time.Second}).getApplyCallTimeout(); got != 60*time.Second {
		t.Errorf("negative value: %v, want the default 60s", got)
	}
}
