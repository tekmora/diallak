package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

// settingsActions are the plan's actions on the server settings.
var settingsActions = []string{
	serverplan.ActionSetVerificationLevel, serverplan.ActionCreateFilterRule, serverplan.ActionSetFilterRule,
}

func (f *fakePlatform) handleGetFilterRules(w http.ResponseWriter) {
	f.mu.Lock()
	rules := append([]discord.AutoModerationRule{}, f.filterRules...)
	f.mu.Unlock()
	b, err := json.Marshal(rules)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTestResponse(w, http.StatusOK, string(b))
}

func (f *fakePlatform) handleUpdateGuild(w http.ResponseWriter, body []byte) {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req) != 1 || req["verification_level"] == nil {
		f.t.Errorf("guild update %s, want the verification level alone", body)
		http.Error(w, "unexpected fields", http.StatusBadRequest)
		return
	}
	var level int
	if err := json.Unmarshal(req["verification_level"], &level); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.verificationLevel = level
	f.mu.Unlock()
	f.handleGetGuild(w)
}

func (f *fakePlatform) handleCreateFilterRule(w http.ResponseWriter, body []byte) {
	var req discord.AutoModerationRuleCreate
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	rule := discord.AutoModerationRule{
		ID: snowflake.ID(f.nextID), GuildID: snowflake.ID(f.guildID), Name: req.Name,
		EventType: req.EventType, TriggerType: req.TriggerType, Actions: req.Actions,
		Enabled: req.Enabled != nil && *req.Enabled, ExemptRoles: req.ExemptRoles, ExemptChannels: req.ExemptChannels,
	}
	f.nextID++
	if req.TriggerMetadata != nil {
		rule.TriggerMetadata = *req.TriggerMetadata
	}
	f.filterRules = append(f.filterRules, rule)
	f.mu.Unlock()
	f.writeRule(w, rule)
}

func (f *fakePlatform) handleUpdateFilterRule(w http.ResponseWriter, path string, body []byte) {
	ruleID, err := strconv.ParseUint(path[strings.LastIndex(path, "/")+1:], 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req discord.AutoModerationRuleUpdate
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	var found *discord.AutoModerationRule
	for i := range f.filterRules {
		if f.filterRules[i].ID == snowflake.ID(ruleID) {
			found = &f.filterRules[i]
		}
	}
	if found == nil {
		f.mu.Unlock()
		http.NotFound(w, nil)
		return
	}
	applyRuleUpdate(found, req)
	rule := *found
	f.mu.Unlock()
	f.writeRule(w, rule)
}

func applyRuleUpdate(r *discord.AutoModerationRule, req discord.AutoModerationRuleUpdate) {
	if req.Name != nil {
		r.Name = *req.Name
	}
	if req.EventType != nil {
		r.EventType = *req.EventType
	}
	if req.TriggerMetadata != nil {
		r.TriggerMetadata = *req.TriggerMetadata
	}
	if req.Actions != nil {
		r.Actions = *req.Actions
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	if req.ExemptRoles != nil {
		r.ExemptRoles = *req.ExemptRoles
	}
	if req.ExemptChannels != nil {
		r.ExemptChannels = *req.ExemptChannels
	}
}

func (f *fakePlatform) writeRule(w http.ResponseWriter, rule discord.AutoModerationRule) {
	b, err := json.Marshal(rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTestResponse(w, http.StatusOK, string(b))
}

// holdFixtureSettings gives the fake the verification level and the filter
// rules of a fixture built as the definition says.
func (f *fakePlatform) holdFixtureSettings(t *testing.T, pf platformFixture) {
	t.Helper()
	var guild fixtureAPIGuild
	if err := json.Unmarshal([]byte(pf.guild), &guild); err != nil {
		t.Fatalf("unmarshal fixture guild: %v", err)
	}
	var rules []discord.AutoModerationRule
	if err := json.Unmarshal([]byte(pf.rules), &rules); err != nil {
		t.Fatalf("unmarshal fixture rules: %v", err)
	}
	f.verificationLevel = guild.VerificationLevel
	f.filterRules = rules
}

// settingsApplyFixture is a server built as the definition says, except for
// the settings: the level is low, the spam rule is missing and the mention
// rule counts 20 mentions.
func settingsApplyFixture(t *testing.T) (*fakePlatform, *serverdef.Definition, string) {
	t.Helper()
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	// The spam rule exempts a role and a channel, so that sending them is checked.
	for i := range def.Settings.FilterRules {
		if def.Settings.FilterRules[i].Name == "Spam" {
			def.Settings.FilterRules[i].ExemptRoles = []string{"moderator"}
			def.Settings.FilterRules[i].ExemptChannels = []string{"mod-queue"}
		}
	}
	fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
	srv := fake.server()
	t.Cleanup(srv.Close)
	applyFullPlan(t, fake, srv.URL, def)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.verificationLevel = 1
	kept := fake.filterRules[:0]
	for _, r := range fake.filterRules {
		switch r.Name {
		case "Spam":
			continue
		case "Mass mentions":
			r.TriggerMetadata.MentionTotalLimit = 20
		}
		kept = append(kept, r)
	}
	fake.filterRules = kept
	fake.recorded = nil
	return fake, def, srv.URL
}

func applyFullPlan(t *testing.T, fake *fakePlatform, restURL string, def *serverdef.Definition) {
	t.Helper()
	token := makeFakeToken(fake.botUserID)
	_, planDigest := planServerSnapshot(t, token, fake.guildID, restURL, def, server.DefinitionDigest())
	var out bytes.Buffer
	err := applyServerWithOptions(context.Background(), token, fake.guildID, ApplyInput{
		Definition: def, DefinitionDigest: server.DefinitionDigest(), PlanDigest: planDigest,
	}, &out, clientOptions{restURL: restURL})
	if err != nil {
		t.Fatalf("first apply: %v\n%s", err, out.String())
	}
}

// The plan sets the level, creates the missing rule and sets the differing one;
// a second read finds nothing left to do.
func TestApplyServer_Positive_SettingsApplied(t *testing.T) {
	fake, def, restURL := settingsApplyFixture(t)
	token := makeFakeToken(fake.botUserID)
	res, planDigest := planServerSnapshot(t, token, fake.guildID, restURL, def, server.DefinitionDigest())
	if len(res.Changes) != 3 || len(res.Refused) != 0 {
		t.Fatalf("plan %+v refused %+v, want 3 changes", res.Changes, res.Refused)
	}
	fake.resetRecorded()
	var out bytes.Buffer
	err := applyServerWithOptions(context.Background(), token, fake.guildID, ApplyInput{
		Definition: def, DefinitionDigest: server.DefinitionDigest(), PlanDigest: planDigest,
	}, &out, clientOptions{restURL: restURL})
	if err != nil {
		t.Fatalf("apply: %v\n%s", err, out.String())
	}
	var writes []string
	for _, c := range fake.getRecorded() {
		if c.method != http.MethodGet {
			writes = append(writes, c.method+" "+c.path)
			if reason, err := url.QueryUnescape(c.auditReason); err != nil || reason != "diallak apply "+planDigest {
				t.Errorf("%s %s audit reason %q", c.method, c.path, c.auditReason)
			}
		}
	}
	rulesPath := fmt.Sprintf("/guilds/%d/auto-moderation/rules", fake.guildID)
	mentionID := ruleIDByName(t, fake, "Mass mentions")
	want := []string{
		fmt.Sprintf("PATCH /guilds/%d", fake.guildID),
		"POST " + rulesPath,
		fmt.Sprintf("PATCH %s/%d", rulesPath, mentionID),
	}
	if strings.Join(writes, "\n") != strings.Join(want, "\n") {
		t.Errorf("writes\n%s\nwant\n%s", strings.Join(writes, "\n"), strings.Join(want, "\n"))
	}
	after, _ := planServerSnapshot(t, token, fake.guildID, restURL, def, server.DefinitionDigest())
	if len(after.Changes) != 0 || len(after.Refused) != 0 {
		t.Errorf("second plan %+v refused %+v, want none", after.Changes, after.Refused)
	}
}

func ruleIDByName(t *testing.T, fake *fakePlatform, name string) snowflake.ID {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, r := range fake.filterRules {
		if r.Name == name {
			return r.ID
		}
	}
	t.Fatalf("no rule %q", name)
	return 0
}

// A settings change whose object names nothing the definition holds is refused
// before any request.
func TestApplyServer_Negative_SettingsObjectNotInDefinition(t *testing.T) {
	def := mustDefinition(t)
	for _, c := range []serverplan.Change{
		{Step: 1, Action: serverplan.ActionSetVerificationLevel, Object: "settings.other"},
		{Step: 1, Action: serverplan.ActionCreateFilterRule, Object: "settings.filter_rules[nope]"},
		{Step: 1, Action: serverplan.ActionSetFilterRule, Object: "filter_rules[spam]"},
	} {
		if err := validateChangeObject(def, c); err == nil {
			t.Errorf("%s %s accepted", c.Action, c.Object)
		}
	}
	noLevel := *def
	noLevel.Settings = &serverdef.Settings{FilterRules: def.Settings.FilterRules}
	c := serverplan.Change{Step: 1, Action: serverplan.ActionSetVerificationLevel, Object: "settings.verification"}
	if err := validateChangeObject(&noLevel, c); err == nil {
		t.Error("level change accepted for a definition without a level")
	}
}

// Every settings action of the plan is one the apply carries out; a converted
// trigger is never one of them.
func TestApplyServer_Boundary_SettingsActionsAccepted(t *testing.T) {
	def := mustDefinition(t)
	for _, c := range []serverplan.Change{
		{Step: 1, Action: serverplan.ActionSetVerificationLevel, Object: "settings.verification"},
		{Step: 1, Action: serverplan.ActionCreateFilterRule, Object: "settings.filter_rules[spam]"},
		{Step: 1, Action: serverplan.ActionSetFilterRule, Object: "settings.filter_rules[spam]"},
	} {
		if err := validateChangeObject(def, c); err != nil {
			t.Errorf("%s %s refused: %v", c.Action, c.Object, err)
		}
	}
	c := serverplan.Change{Step: 1, Action: serverplan.ActionConvertFilterRule, Object: "settings.filter_rules[spam]"}
	if err := validateChangeObject(def, c); err == nil {
		t.Error("convert-filter-rule accepted")
	}
}
