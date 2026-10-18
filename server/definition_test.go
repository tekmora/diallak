package server

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

func TestDefinition_Positive_LoadsAndValidatesEmbedded(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v, want nil", err)
	}
	if def == nil {
		t.Fatal("Definition() returned nil definition")
	}
	if def.Version != 1 {
		t.Errorf("def.Version = %d, want 1", def.Version)
	}
	if len(def.Roles) == 0 {
		t.Error("def.Roles is empty")
	}
}

func TestDefinition_Positive_EqualsDiskFile(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}

	data, err := os.ReadFile("definition.json")
	if err != nil {
		data, err = os.ReadFile("../server/definition.json")
		if err != nil {
			t.Fatalf("read definition.json from disk: %v", err)
		}
	}

	diskDef, err := serverdef.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load(disk definition): %v", err)
	}

	if !reflect.DeepEqual(def, diskDef) {
		t.Errorf("embedded definition does not equal disk definition")
	}
}

func TestDefinition_Negative_UnknownFieldsRefused(t *testing.T) {
	orig := definitionJSON
	withRequired := strings.Replace(orig, `"multiple": true`, `"required": false, "multiple": true`, 1)
	if withRequired == orig {
		t.Fatal("string replace for required did not change text")
	}
	_, err := serverdef.Load(strings.NewReader(withRequired))
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("Load with required = false error = %v, want error holding 'required'", err)
	}

	oldKey := "on" + "boarding"
	withOldKey := strings.Replace(orig, `"choices": {`, `"`+oldKey+`": {`, 1)
	if withOldKey == orig {
		t.Fatal("string replace for old key did not change text")
	}
	_, err = serverdef.Load(strings.NewReader(withOldKey))
	if err == nil || !strings.Contains(err.Error(), oldKey) {
		t.Errorf("Load with old key error = %v, want error holding %q", err, oldKey)
	}
}

func TestDefinition_Positive_AreaChoicesIsTheBotsDecidedAutomation(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v, want nil", err)
	}
	found := 0
	for _, a := range def.Automation {
		if strings.Contains(a.ID, "boarding") {
			t.Errorf("automation %q names the platform's former questions", a.ID)
		}
		if a.ID != "area-choices" {
			continue
		}
		found++
		if a.By != "bot" || a.State != "built" ||
			a.Record != "docs/adr/0011-area-roles-from-member-choice.md" {
			t.Errorf("area-choices = %+v, want by bot, built, record 0011", a)
		}
	}
	if found != 1 {
		t.Errorf("automation entries with id area-choices = %d, want 1", found)
	}
	if def.Choices.By != "bot" || len(def.Choices.Questions) != 3 {
		t.Errorf("choices = by %q with %d questions, want by bot with 3",
			def.Choices.By, len(def.Choices.Questions))
	}
}

func TestDefinitionDigest_Positive_ShapeAndEqualsDiskFile(t *testing.T) {
	digest := DefinitionDigest()
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("DefinitionDigest() = %q, want sha256:<64 hex>", digest)
	}
	data, err := os.ReadFile("definition.json")
	if err != nil {
		data, err = os.ReadFile("../server/definition.json")
		if err != nil {
			t.Fatalf("read definition.json from disk: %v", err)
		}
	}
	expected := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	if digest != expected {
		t.Errorf("DefinitionDigest() = %q, want %q", digest, expected)
	}
}

// rightsNamedAnywhere returns all rights named on any role's permissions or in any channel entry's allow list.
func rightsNamedAnywhere(def *serverdef.Definition) map[string]struct{} {
	named := make(map[string]struct{})
	for _, r := range def.Roles {
		for _, p := range r.Permissions {
			named[p] = struct{}{}
		}
	}
	for _, ch := range def.Channels {
		for _, acc := range ch.Access {
			for _, p := range acc.Allow {
				named[p] = struct{}{}
			}
		}
	}
	return named
}

// testDefinitionNoSendPollsCommitted verifies that committed definition has no SEND_POLLS.
func testDefinitionNoSendPollsCommitted(t *testing.T) {
	t.Helper()
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	named := rightsNamedAnywhere(def)
	if _, ok := named["SEND_POLLS"]; ok {
		t.Errorf("committed definition names SEND_POLLS in role permissions or channel allow")
	}
}

// testDefinitionNoSendPollsOnEveryone verifies helper detects SEND_POLLS on everyone role.
func testDefinitionNoSendPollsOnEveryone(t *testing.T) {
	t.Helper()
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	rolesCopy := make([]serverdef.Role, len(def.Roles))
	copy(rolesCopy, def.Roles)
	for i, r := range rolesCopy {
		if r.Kind == "everyone" {
			rolesCopy[i].Permissions = append(append([]string{}, r.Permissions...), "SEND_POLLS")
		}
	}
	copyDef := *def
	copyDef.Roles = rolesCopy
	named := rightsNamedAnywhere(&copyDef)
	if _, ok := named["SEND_POLLS"]; !ok {
		t.Errorf("rightsNamedAnywhere did not report SEND_POLLS when added to @everyone")
	}
}

// testDefinitionNoSendPollsOnChannel verifies helper detects SEND_POLLS when allowed on a channel.
func testDefinitionNoSendPollsOnChannel(t *testing.T) {
	t.Helper()
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	if len(def.Channels) == 0 || len(def.Channels[0].Access) == 0 {
		t.Fatal("first channel of the definition holds no entry")
	}
	channelsCopy := make([]serverdef.Channel, len(def.Channels))
	copy(channelsCopy, def.Channels)
	accessCopy := make([]serverdef.Access, len(channelsCopy[0].Access))
	copy(accessCopy, channelsCopy[0].Access)
	accessCopy[0].Allow = append(append([]string{}, accessCopy[0].Allow...), "SEND_POLLS")
	channelsCopy[0].Access = accessCopy
	copyDef := *def
	copyDef.Channels = channelsCopy
	if _, ok := rightsNamedAnywhere(&copyDef)["SEND_POLLS"]; !ok {
		t.Errorf("rightsNamedAnywhere did not report SEND_POLLS allowed by an entry on a channel")
	}
}

func TestDefinition_Boundary_NoSendPolls(t *testing.T) {
	// members send no polls; a vote that counts runs through the wishes and polls path, ADR-0015
	t.Run("committed definition names no SEND_POLLS", func(t *testing.T) {
		testDefinitionNoSendPollsCommitted(t)
	})

	t.Run("helper reports SEND_POLLS when present on everyone", func(t *testing.T) {
		testDefinitionNoSendPollsOnEveryone(t)
	})

	t.Run("helper reports SEND_POLLS when an entry on a channel allows it", func(t *testing.T) {
		testDefinitionNoSendPollsOnChannel(t)
	})
}

// builtCode names, for each bot automation that is built, the source file that carries it, counted
// from the repository root. A bot automation is built exactly when it has a row here and that file
// exists; a row added with the code while the state stays decided fails, and so does the reverse.
var builtCode = map[string]string{
	"ask-command":          "internal/discord/discord.go",
	"support-first-answer": "internal/discord/forum.go",
	"moderation-queue":     "internal/discord/moderation.go",
	"area-choices":         "internal/discord/areas.go",
	"server-settings":      "internal/discord/server_apply_settings.go",
	"discussions-bridge":   "internal/news/discussions.go",
	"news":                 "internal/news/run.go",
	"bug-report-link":      "internal/discord/bugreport.go",
	"supporter-roles":      "internal/discord/supporter.go",
}

func TestDefinition_Negative_BotAutomationStateMatchesCode(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	seen := map[string]bool{}
	for _, a := range def.Automation {
		if a.By != "bot" {
			continue
		}
		path, hasCode := builtCode[a.ID]
		seen[a.ID] = true
		if hasCode != (a.State == "built") {
			t.Errorf("automation %s: state %q, code file in builtCode %q; built exactly when a file is named",
				a.ID, a.State, path)
		}
		if !hasCode {
			continue
		}
		// The path counts from the repository root; this package is one folder below it.
		if _, statErr := os.Stat(filepath.Join("..", path)); statErr != nil {
			t.Errorf("automation %s: code file %s: %v", a.ID, path, statErr)
		}
	}
	for id := range builtCode {
		if !seen[id] {
			t.Errorf("builtCode names %s, which is no bot automation of the definition", id)
		}
	}
}

func TestDefinition_Positive_ServerSettingsAutomation(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	var entry *serverdef.Automation
	for i := range def.Automation {
		if def.Automation[i].ID == "server-settings" {
			entry = &def.Automation[i]
			break
		}
	}
	if entry == nil {
		t.Fatal("automation entry server-settings not found")
	}
	if entry.By != "bot" {
		t.Errorf("server-settings.By = %q, want %q", entry.By, "bot")
	}
	// The record's path counts from the repository root; this package is one folder below it.
	if _, err := os.Stat(filepath.Join("..", entry.Record)); err != nil {
		t.Errorf("server-settings record file %q does not exist on disk: %v", entry.Record, err)
	}
}

// roleByID returns the role of the given id, or nil.
func roleByID(def *serverdef.Definition, id string) *serverdef.Role {
	for i := range def.Roles {
		if def.Roles[i].ID == id {
			return &def.Roles[i]
		}
	}
	return nil
}

// entryFor returns the entry the channel holds for the role, or nil.
func entryFor(ch serverdef.Channel, role string) *serverdef.Access {
	for i := range ch.Access {
		if ch.Access[i].Role == role {
			return &ch.Access[i]
		}
	}
	return nil
}

// holdsAll reports whether have names every right of want.
func holdsAll(have, want []string) bool {
	held := make(map[string]struct{}, len(have))
	for _, p := range have {
		held[p] = struct{}{}
	}
	for _, p := range want {
		if _, ok := held[p]; !ok {
			return false
		}
	}
	return true
}

// holdsAny reports whether have names at least one right of want.
func holdsAny(have, want []string) bool {
	for _, p := range want {
		if holdsAll(have, []string{p}) {
			return true
		}
	}
	return false
}

// supporterChannelGaps returns gaps where a channel allows rights to id that honorary supporter does not hold.
func supporterChannelGaps(def *serverdef.Definition, id string) []string {
	var gaps []string
	for _, ch := range def.Channels {
		theirs := entryFor(ch, id)
		if theirs == nil {
			continue
		}
		ours := entryFor(ch, "honorary-supporter")
		if ours == nil {
			ours = &serverdef.Access{}
		}
		if !holdsAll(ours.Allow, theirs.Allow) || holdsAny(ours.Deny, theirs.Allow) {
			gaps = append(gaps, "channels["+ch.ID+"].access")
		}
	}
	return gaps
}

// supporterStandingGaps lists every place where the definition gives a supporter role something the
// Honorary supporter role does not get: a right on the role, or what an entry on a channel allows. ADR-0016.
func supporterStandingGaps(def *serverdef.Definition) []string {
	honorary := roleByID(def, "honorary-supporter")
	if honorary == nil {
		return []string{"roles[honorary-supporter]"}
	}
	var gaps []string
	for _, id := range []string{"supporter", "supporter-12-months"} {
		given := roleByID(def, id)
		if given == nil {
			gaps = append(gaps, "roles["+id+"]")
			continue
		}
		if !holdsAll(honorary.Permissions, given.Permissions) {
			gaps = append(gaps, "roles[honorary-supporter].permissions")
		}
		gaps = append(gaps, supporterChannelGaps(def, id)...)
	}
	return gaps
}

// withEntries returns a copy of the definition whose first channel holds the given entries in addition.
func withEntries(t *testing.T, entries ...serverdef.Access) *serverdef.Definition {
	t.Helper()
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	if len(def.Channels) == 0 {
		t.Fatal("the definition holds no channel")
	}
	channels := make([]serverdef.Channel, len(def.Channels))
	copy(channels, def.Channels)
	channels[0].Access = append(append([]serverdef.Access{}, channels[0].Access...), entries...)
	copyDef := *def
	copyDef.Channels = channels
	return &copyDef
}

func TestDefinition_Boundary_SupporterStandingHeldByHonorary(t *testing.T) {
	// whatever is attached to a supporter role is attached to the Honorary supporter role, ADR-0016
	view := []string{"VIEW_CHANNEL"}
	both := []string{"VIEW_CHANNEL", "SEND_MESSAGES"}
	cases := []struct {
		name    string
		entries []serverdef.Access
		gaps    int
	}{
		{"committed definition", nil, 0},
		{"entry for honorary only", []serverdef.Access{{Role: "honorary-supporter", Allow: view}}, 0},
		{"supporter entry without one for honorary", []serverdef.Access{{Role: "supporter", Allow: view}}, 1},
		{"year role entry without one for honorary", []serverdef.Access{{Role: "supporter-12-months", Allow: view}}, 1},
		{"both entries, equal", []serverdef.Access{
			{Role: "supporter", Allow: view}, {Role: "honorary-supporter", Allow: view},
		}, 0},
		{"both entries, honorary allows more", []serverdef.Access{
			{Role: "supporter", Allow: view}, {Role: "honorary-supporter", Allow: both},
		}, 0},
		{"both entries, honorary allows less", []serverdef.Access{
			{Role: "supporter", Allow: both}, {Role: "honorary-supporter", Allow: view},
		}, 1},
		{"both entries, honorary denies what supporter is allowed", []serverdef.Access{
			{Role: "supporter", Allow: view}, {Role: "honorary-supporter", Allow: view, Deny: view},
		}, 1},
		{"supporter entry that allows nothing", []serverdef.Access{{Role: "supporter", Deny: view}}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gaps := supporterStandingGaps(withEntries(t, tc.entries...))
			if len(gaps) != tc.gaps {
				t.Errorf("supporterStandingGaps = %v, want %d", gaps, tc.gaps)
			}
		})
	}
}

// withRoles returns a copy of the definition whose roles went through edit.
func withRoles(t *testing.T, edit func([]serverdef.Role) []serverdef.Role) *serverdef.Definition {
	t.Helper()
	def := withEntries(t)
	roles := make([]serverdef.Role, len(def.Roles))
	copy(roles, def.Roles)
	def.Roles = edit(roles)
	return def
}

// withoutRole drops the role of the given id.
func withoutRole(id string) func([]serverdef.Role) []serverdef.Role {
	return func(roles []serverdef.Role) []serverdef.Role {
		kept := roles[:0]
		for _, r := range roles {
			if r.ID != id {
				kept = append(kept, r)
			}
		}
		return kept
	}
}

// withRight gives the role of the given id one right.
func withRight(id string) func([]serverdef.Role) []serverdef.Role {
	return func(roles []serverdef.Role) []serverdef.Role {
		for i := range roles {
			if roles[i].ID == id {
				roles[i].Permissions = []string{"USE_EXTERNAL_EMOJIS"}
			}
		}
		return roles
	}
}

func TestDefinition_Negative_SupporterStandingRolesAndRights(t *testing.T) {
	cases := []struct {
		name string
		edit func([]serverdef.Role) []serverdef.Role
		want string
	}{
		{"no honorary role", withoutRole("honorary-supporter"), "roles[honorary-supporter]"},
		{"no supporter role", withoutRole("supporter"), "roles[supporter]"},
		{"no year role", withoutRole("supporter-12-months"), "roles[supporter-12-months]"},
		{"a right on the supporter role only", withRight("supporter"), "roles[honorary-supporter].permissions"},
		{"a right on the year role only", withRight("supporter-12-months"), "roles[honorary-supporter].permissions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gaps := supporterStandingGaps(withRoles(t, tc.edit))
			if len(gaps) != 1 || gaps[0] != tc.want {
				t.Errorf("supporterStandingGaps = %v, want [%s]", gaps, tc.want)
			}
		})
	}
	t.Run("the same right on the honorary role closes the gap", func(t *testing.T) {
		def := withRoles(t, func(roles []serverdef.Role) []serverdef.Role {
			return withRight("honorary-supporter")(withRight("supporter")(roles))
		})
		if gaps := supporterStandingGaps(def); len(gaps) != 0 {
			t.Errorf("supporterStandingGaps = %v, want none", gaps)
		}
	})
}

// verifyManualRoleAttributes checks name, kind, assignment, and permissions for honorary and guide roles.
func verifyManualRoleAttributes(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	names := map[string]string{
		"honorary-supporter":         "Honorary supporter",
		"guide-software-engineering": "Subject guide: software engineering",
	}
	for id, name := range names {
		role := roleByID(def, id)
		if role == nil {
			t.Fatalf("the definition holds no role %s", id)
		}
		if role.Name != name || role.Kind != "recognition" || role.AssignedBy != "operator" || role.Basis != "decided" {
			t.Errorf("role %s = name %q, kind %q, by %q, basis %q; want %q, recognition, operator, decided",
				id, role.Name, role.Kind, role.AssignedBy, role.Basis, name)
		}
		if len(role.Permissions) != 0 || role.ShownApart != nil || role.Mentionable != nil {
			t.Errorf("role %s holds a right or states how it is shown", id)
		}
	}
}

// verifyManualRoleOrdering checks that honorary supporter and subject guide roles stand in the required positions.
func verifyManualRoleOrdering(t *testing.T, def *serverdef.Definition) {
	t.Helper()
	place := make(map[string]int, len(def.RoleOrder))
	for i, id := range def.RoleOrder {
		place[id] = i
	}
	if place["bot"] != place["honorary-supporter"]+1 {
		t.Errorf("honorary-supporter stands at %d and the bot's role at %d, want directly above",
			place["honorary-supporter"], place["bot"])
	}
	if place["guide-software-engineering"] != place["guide-video"]+1 || place["guide-video"] != place["guide-coding"]+1 {
		t.Errorf("the three subject guide roles do not stand together: coding %d, video %d, software engineering %d",
			place["guide-coding"], place["guide-video"], place["guide-software-engineering"])
	}
}

func TestDefinition_Positive_HonoraryAndThirdGuideRole(t *testing.T) {
	// two roles the operator gives by hand, both above the bot's role, ADR-0016
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	verifyManualRoleAttributes(t, def)
	verifyManualRoleOrdering(t, def)
}

// ADR-0020: the support forum carries one opt-in tag, which every member can set; the opt-out tag
// of ADR-0009 is gone.
func TestDefinition_Positive_SupportForumHasTheOptInTag(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v, want nil", err)
	}
	var optIn []serverdef.Tag
	for _, ch := range def.Channels {
		tags, findings := channelOptInTags(ch)
		for _, f := range findings {
			t.Error(f)
		}
		optIn = append(optIn, tags...)
	}
	if len(optIn) != 1 || optIn[0].Name != "Automated answer" || optIn[0].Moderated {
		t.Errorf("opt-in tags = %+v, want one 'Automated answer' that every member can set", optIn)
	}
}

// channelOptInTags returns the opt-in tags of ch, with a finding for an opt-in tag outside the
// support forum and for the opt-out tag of ADR-0009.
func channelOptInTags(ch serverdef.Channel) ([]serverdef.Tag, []string) {
	var optIn []serverdef.Tag
	var findings []string
	for _, tag := range ch.Tags {
		if tag.Name == "No automated reply" {
			findings = append(findings, fmt.Sprintf("channels[%s] still carries the opt-out tag of ADR-0009", ch.ID))
		}
		if !tag.OptIn {
			continue
		}
		if ch.ID != "support" {
			findings = append(findings, fmt.Sprintf("channels[%s] carries an opt-in tag, want only the support forum", ch.ID))
		}
		optIn = append(optIn, tag)
	}
	return optIn, findings
}

// The check above refuses an opt-in tag outside the support forum and the old opt-out tag.
func TestDefinition_Negative_OptInTagOutsideSupportReported(t *testing.T) {
	ch := serverdef.Channel{ID: "general", Tags: []serverdef.Tag{
		{Name: "Automated answer", OptIn: true},
		{Name: "No automated reply"},
	}}
	tags, findings := channelOptInTags(ch)
	if len(tags) != 1 || len(findings) != 2 {
		t.Errorf("channelOptInTags(general) = %d tags, findings %q; want 1 tag and 2 findings", len(tags), findings)
	}
}

func TestDefinition_Negative_OptOutKeyRefused(t *testing.T) {
	withOptOut := strings.Replace(definitionJSON, `"opt_in": true`, `"opt_out": true`, 1)
	if withOptOut == definitionJSON {
		t.Fatal("string replace for opt_in did not change text")
	}
	if _, err := serverdef.Load(strings.NewReader(withOptOut)); err == nil || !strings.Contains(err.Error(), "opt_out") {
		t.Errorf("Load with the opt_out key error = %v, want error holding 'opt_out'", err)
	}
}
