package discord

import (
	"context"
	"encoding/json"
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
)

// The platform's numbers, written out here apart from the reader's tables so a
// wrong entry there fails a test.
var (
	fixtureLevels   = map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3, "very_high": 4}
	fixtureTriggers = map[string]int{"keyword": 1, "spam": 3, "keyword_preset": 4, "mention_spam": 5, "member_profile": 6}
	fixturePresets  = map[string]int{"profanity": 1, "sexual_content": 2, "slurs": 3}
	fixtureActions  = map[string]int{"block_message": 1, "alert": 2, "timeout": 3, "block_interaction": 4}
)

func fixtureVerificationLevel(t *testing.T, def *serverdef.Definition) int {
	t.Helper()
	if def.Settings == nil || def.Settings.Verification == nil {
		return 0
	}
	n, ok := fixtureLevels[def.Settings.Verification.Level]
	if !ok {
		t.Fatalf("no number for level %q", def.Settings.Verification.Level)
	}
	return n
}

type fixtureAPIRuleAction struct {
	Type     int            `json:"type"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type fixtureAPIRule struct {
	ID              string                 `json:"id"`
	GuildID         string                 `json:"guild_id"`
	Name            string                 `json:"name"`
	EventType       int                    `json:"event_type"`
	TriggerType     int                    `json:"trigger_type"`
	TriggerMetadata map[string]any         `json:"trigger_metadata"`
	Actions         []fixtureAPIRuleAction `json:"actions"`
	Enabled         bool                   `json:"enabled"`
	ExemptRoles     []string               `json:"exempt_roles"`
	ExemptChannels  []string               `json:"exempt_channels"`
}

// fixtureChannelIDs gives each defined channel the id buildFixtureChannelsJSON
// gives it: categories first, from 7000 on, then the channels.
func fixtureChannelIDs(def *serverdef.Definition) map[string]string {
	ids := make(map[string]string, len(def.Channels))
	next := uint64(7000) + uint64(len(def.Categories))
	for _, ch := range def.Channels {
		ids[ch.ID] = strconv.FormatUint(next, 10)
		next++
	}
	return ids
}

// buildFixtureRulesJSON serializes the defined filter rules as the platform lists them.
func buildFixtureRulesJSON(t *testing.T, def *serverdef.Definition, guildID uint64, roleIDMap map[string]uint64) string {
	t.Helper()
	rules := []fixtureAPIRule{}
	if def.Settings != nil {
		channelIDs := fixtureChannelIDs(def)
		for i := range def.Settings.FilterRules {
			rules = append(rules, fixtureRuleOf(t, &def.Settings.FilterRules[i], guildID, uint64(8000+i), roleIDMap, channelIDs))
		}
	}
	b, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("marshal rules: %v", err)
	}
	return string(b)
}

func fixtureRuleOf(t *testing.T, r *serverdef.FilterRule, guildID, id uint64, roleIDMap map[string]uint64, channelIDs map[string]string) fixtureAPIRule {
	t.Helper()
	event := 1
	if r.Trigger == "member_profile" {
		event = 2
	}
	presets := []int{}
	for _, p := range r.Presets {
		presets = append(presets, fixturePresets[p])
	}
	out := fixtureAPIRule{
		ID: strconv.FormatUint(id, 10), GuildID: strconv.FormatUint(guildID, 10),
		Name: r.Name, EventType: event, TriggerType: fixtureTriggers[r.Trigger], Enabled: r.Enabled,
		TriggerMetadata: map[string]any{
			"keyword_filter": r.Keywords, "regex_patterns": r.Patterns, "presets": presets,
			"allow_list": r.AllowList, "mention_total_limit": r.MentionLimit,
			"mention_raid_protection_enabled": r.RaidProtection,
		},
		ExemptRoles: []string{}, ExemptChannels: []string{},
	}
	for _, a := range r.Actions {
		act := fixtureAPIRuleAction{Type: fixtureActions[a.Type]}
		switch a.Type {
		case "alert":
			act.Metadata = map[string]any{"channel_id": channelIDs[a.Channel]}
		case "timeout":
			act.Metadata = map[string]any{"duration_seconds": a.TimeoutSeconds}
		case "block_message":
			if a.Message != "" {
				act.Metadata = map[string]any{"custom_message": a.Message}
			}
		}
		out.Actions = append(out.Actions, act)
	}
	for _, role := range r.ExemptRoles {
		out.ExemptRoles = append(out.ExemptRoles, strconv.FormatUint(roleIDMap[role], 10))
	}
	for _, ch := range r.ExemptChannels {
		out.ExemptChannels = append(out.ExemptChannels, channelIDs[ch])
	}
	return out
}

const (
	settingsGuildID   uint64 = 1001
	settingsBotUserID uint64 = 2001
)

// settingsGuildJSON is a small server: the bot's role with the given rights, a
// Helper role and the verification level as a number.
func settingsGuildJSON(botPerms string, level int) string {
	return fmt.Sprintf(`{"id":"%d","verification_level":%d,"roles":[
		{"id":"%d","name":"@everyone","position":0,"permissions":"1024"},
		{"id":"5001","name":"Helper","position":1,"permissions":"0"},
		{"id":"5003","name":"BotRole","position":8,"permissions":"%s","managed":true,"tags":{"bot_id":"%d"}}]}`,
		settingsGuildID, level, settingsGuildID, botPerms, settingsBotUserID)
}

const settingsChannelsJSON = `[{"id":"7001","name":"mod-queue","type":0,"permission_overwrites":[]},
	{"id":"7002","name":"staff","type":0,"permission_overwrites":[]}]`

// readSettingsSnapshot reads the small server and records every request.
func readSettingsSnapshot(t *testing.T, guildJSON, rulesJSON string) (*serverplan.Snapshot, []recordedReq, error) {
	t.Helper()
	appJSON := fmt.Sprintf(`{"id":"9001","bot":{"id":"%d"}}`, settingsBotUserID)
	memberJSON := fmt.Sprintf(`{"user":{"id":"%d"},"roles":["5003"]}`, settingsBotUserID)
	std := newStandardServerWithRules(settingsGuildID, settingsBotUserID, appJSON, guildJSON, memberJSON, settingsChannelsJSON, rulesJSON)
	defer std.Close()
	var mu sync.Mutex
	var recorded []recordedReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		recorded = append(recorded, recordedReq{method: r.Method, path: r.URL.Path})
		mu.Unlock()
		std.Config.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	opts := clientOptions{
		restURL: srv.URL,
		now:     func() time.Time { return time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC) },
	}
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(1001), settingsGuildID, opts)
	mu.Lock()
	defer mu.Unlock()
	return snap, recorded, err
}

const manageGuildPerms = "32" // MANAGE_GUILD

// The level and every part of a rule are read by name.
func TestReadServer_Positive_SettingsReadByName(t *testing.T) {
	rules := `[{"id":"8001","name":"Words","event_type":1,"trigger_type":4,
		"trigger_metadata":{"presets":[3,2],"allow_list":["scunthorpe"]},
		"actions":[{"type":1,"metadata":{"custom_message":"Not here."}},
			{"type":2,"metadata":{"channel_id":"7001"}},
			{"type":3,"metadata":{"duration_seconds":60}},{"type":4}],
		"enabled":true,"exempt_roles":["5001"],"exempt_channels":["7002"]},
		{"id":"8002","name":"Mentions","event_type":1,"trigger_type":5,
		"trigger_metadata":{"mention_total_limit":10,"mention_raid_protection_enabled":true},
		"actions":[{"type":1}],"enabled":false,"exempt_roles":[],"exempt_channels":[]}]`
	snap, _, err := readSettingsSnapshot(t, settingsGuildJSON(manageGuildPerms, 2), rules)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if snap.VerificationLevel != "medium" || !snap.FilterRulesRead {
		t.Fatalf("level %q, rules read %v; want medium, true", snap.VerificationLevel, snap.FilterRulesRead)
	}
	want := []serverplan.FilterRule{
		{
			Name: "Words", Trigger: "keyword_preset", Enabled: true,
			Presets: []string{"slurs", "sexual_content"}, AllowList: []string{"scunthorpe"},
			Actions: []serverplan.FilterAction{
				{Type: "block_message", Message: "Not here."}, {Type: "alert", Channel: "mod-queue"},
				{Type: "timeout", TimeoutSeconds: 60}, {Type: "block_interaction"},
			},
			ExemptRoles: []string{"Helper"}, ExemptChannels: []string{"staff"},
		},
		{
			Name: "Mentions", Trigger: "mention_spam", MentionLimit: 10, RaidProtection: true,
			Actions: []serverplan.FilterAction{{Type: "block_message"}},
		},
	}
	if !reflect.DeepEqual(snap.FilterRules, want) {
		t.Errorf("rules = %+v\nwant %+v", snap.FilterRules, want)
	}
}

// Each platform number of the level reads as its name.
func TestReadServer_Positive_EveryVerificationLevel(t *testing.T) {
	for name, n := range fixtureLevels {
		snap, _, err := readSettingsSnapshot(t, settingsGuildJSON("0", n), `[]`)
		if err != nil {
			t.Fatalf("read level %d: %v", n, err)
		}
		if snap.VerificationLevel != name {
			t.Errorf("level %d read as %q, want %q", n, snap.VerificationLevel, name)
		}
	}
}

// Each platform number of a trigger, a preset and an action reads as its name.
func TestReadServer_Positive_EveryFilterNumber(t *testing.T) {
	for _, tc := range []struct {
		table map[string]int
		rule  func(n int) string
		got   func(r serverplan.FilterRule) string
	}{
		{fixtureTriggers, func(n int) string {
			return fmt.Sprintf(`{"trigger_type":%d,"trigger_metadata":{},"actions":[]}`, n)
		}, func(r serverplan.FilterRule) string { return r.Trigger }},
		{fixturePresets, func(n int) string {
			return fmt.Sprintf(`{"trigger_type":4,"trigger_metadata":{"presets":[%d]},"actions":[]}`, n)
		}, func(r serverplan.FilterRule) string { return strings.Join(r.Presets, ",") }},
		{fixtureActions, func(n int) string {
			return fmt.Sprintf(`{"trigger_type":3,"trigger_metadata":{},"actions":[{"type":%d}]}`, n)
		}, func(r serverplan.FilterRule) string { return r.Actions[0].Type }},
	} {
		for name, n := range tc.table {
			rules := "[" + strings.Replace(tc.rule(n), "{", `{"id":"8001","name":"R","event_type":1,"enabled":true,"exempt_roles":[],"exempt_channels":[],`, 1) + "]"
			snap, _, err := readSettingsSnapshot(t, settingsGuildJSON(manageGuildPerms, 0), rules)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if got := tc.got(snap.FilterRules[0]); got != name {
				t.Errorf("number %d read as %q, want %q", n, got, name)
			}
		}
	}
}

// Without the right to manage the server the rules are not asked for and are
// marked as not read; with ADMINISTRATOR they are.
func TestReadServer_Negative_RulesNotReadWithoutManageGuild(t *testing.T) {
	for _, tc := range []struct {
		perms    string
		wantRead bool
	}{{"1024", false}, {"8", true}, {manageGuildPerms, true}} {
		snap, recorded, err := readSettingsSnapshot(t, settingsGuildJSON(tc.perms, 0), `[]`)
		if err != nil {
			t.Fatalf("perms %s: read: %v", tc.perms, err)
		}
		asked := false
		for _, r := range recorded {
			asked = asked || strings.HasSuffix(r.path, "/auto-moderation/rules")
		}
		if snap.FilterRulesRead != tc.wantRead || asked != tc.wantRead {
			t.Errorf("perms %s: rules read %v, asked %v; want %v", tc.perms, snap.FilterRulesRead, asked, tc.wantRead)
		}
	}
}

// A rule that exempts a role the role list does not have stops the read; a
// refused read of the rules stops it too.
func TestReadServer_Negative_RulesReadFails(t *testing.T) {
	rules := `[{"id":"8001","name":"Spam","event_type":1,"trigger_type":3,"trigger_metadata":{},
		"actions":[{"type":1}],"enabled":true,"exempt_roles":["5999"],"exempt_channels":[]}]`
	_, _, err := readSettingsSnapshot(t, settingsGuildJSON(manageGuildPerms, 0), rules)
	if err == nil || !strings.Contains(err.Error(), `filter rule "Spam" exempts a role`) {
		t.Errorf("err = %v, want the exempt role named", err)
	}
	_, _, err = readSettingsSnapshot(t, settingsGuildJSON(manageGuildPerms, 0), `{"message":"Missing Access","code":50001}`)
	if err == nil || !strings.Contains(err.Error(), "get filter rules") {
		t.Errorf("err = %v, want the failed step named", err)
	}
}

// Numbers the table does not name read as "other", an unknown level as not
// read, and an alert channel the bot cannot see as no name.
func TestReadServer_Boundary_UnknownNumbers(t *testing.T) {
	rules := `[{"id":"8001","name":"Future","event_type":1,"trigger_type":99,
		"trigger_metadata":{"presets":[99]},
		"actions":[{"type":99},{"type":2,"metadata":{"channel_id":"7999"}}],
		"enabled":true,"exempt_roles":[],"exempt_channels":["7999"]}]`
	snap, _, err := readSettingsSnapshot(t, settingsGuildJSON(manageGuildPerms, 9), rules)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if snap.VerificationLevel != "" {
		t.Errorf("level = %q, want empty", snap.VerificationLevel)
	}
	want := []serverplan.FilterRule{{
		Name: "Future", Trigger: "other", Enabled: true, Presets: []string{"other"},
		Actions:        []serverplan.FilterAction{{Type: "other"}, {Type: "alert"}},
		ExemptChannels: []string{""},
	}}
	if !reflect.DeepEqual(snap.FilterRules, want) {
		t.Errorf("rules = %+v\nwant %+v", snap.FilterRules, want)
	}
}
