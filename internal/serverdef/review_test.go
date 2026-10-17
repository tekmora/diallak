package serverdef

import (
	"fmt"
	"strings"
	"testing"
)

// The tests in this file pin single conditions of the rules. Each case plants one defect into the
// committed definition and expects one finding, named by its code, its path and a word of its
// message, so that a rule which loses one of its conditions fails a case here.

func channelOf(t *testing.T, def *Definition, id string) *Channel {
	t.Helper()
	for i := range def.Channels {
		if def.Channels[i].ID == id {
			return &def.Channels[i]
		}
	}
	t.Fatalf("definition has no channel %q", id)
	return nil
}

func roleOf(t *testing.T, def *Definition, id string) *Role {
	t.Helper()
	for i := range def.Roles {
		if def.Roles[i].ID == id {
			return &def.Roles[i]
		}
	}
	t.Fatalf("definition has no role %q", id)
	return nil
}

func areaOf(t *testing.T, def *Definition, id string) *Area {
	t.Helper()
	for i := range def.Areas {
		if def.Areas[i].ID == id {
			return &def.Areas[i]
		}
	}
	t.Fatalf("definition has no area %q", id)
	return nil
}

// accessOf returns the access entry of a role in a channel and adds an empty one when the channel
// has none for that role.
func accessOf(ch *Channel, roleID string) *Access {
	for i := range ch.Access {
		if ch.Access[i].Role == roleID {
			return &ch.Access[i]
		}
	}
	ch.Access = append(ch.Access, Access{Role: roleID, Allow: []string{}, Deny: []string{}})
	return &ch.Access[len(ch.Access)-1]
}

func without(list []string, drop string) []string {
	kept := make([]string, 0, len(list))
	for _, item := range list {
		if item != drop {
			kept = append(kept, item)
		}
	}
	return kept
}

func hasFinding(findings []Finding, code, path, word string) bool {
	for _, f := range findings {
		if f.Code == code && f.Path == path && strings.Contains(f.Message, word) {
			return true
		}
	}
	return false
}

type plantedCase struct {
	name  string
	plant func(t *testing.T, def *Definition)
	code  string
	path  string
	word  string
}

func runPlanted(t *testing.T, cases []plantedCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := valid(t)
			if hasFinding(Validate(def), tc.code, tc.path, tc.word) {
				t.Fatalf("the committed definition already has the finding %s at %s", tc.code, tc.path)
			}
			tc.plant(t, def)
			findings := Validate(def)
			if !hasFinding(findings, tc.code, tc.path, tc.word) {
				t.Errorf("no finding %s at %s with %q; got %+v", tc.code, tc.path, tc.word, findings)
			}
		})
	}
}

func TestValidate_Negative_TicketChannelConditions(t *testing.T) {
	everyone := "channels[tickets].access[everyone]"
	runPlanted(t, []plantedCase{
		{"opens at a threshold", func(t *testing.T, def *Definition) {
			channelOf(t, def, "tickets").Opens = "threshold"
		}, CodeTicketShape, "channels[tickets].opens", "start"},
		{"is a forum", func(t *testing.T, def *Definition) {
			channelOf(t, def, "tickets").Type = "forum"
		}, CodeTicketShape, "channels[tickets].type", "text"},
		{"members cannot open a private thread", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "tickets"), "everyone")
			acc.Allow = without(acc.Allow, "CREATE_PRIVATE_THREADS")
		}, CodeTicketShape, everyone, "CREATE_PRIVATE_THREADS"},
		{"members cannot write in a thread", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "tickets"), "everyone")
			acc.Deny = append(acc.Deny, "SEND_MESSAGES_IN_THREADS")
		}, CodeTicketShape, everyone, "SEND_MESSAGES_IN_THREADS"},
		{"members cannot see the channel", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "tickets"), "everyone")
			acc.Deny = append(acc.Deny, "VIEW_CHANNEL")
		}, CodeTicketShape, everyone, "VIEW_CHANNEL"},
		{"members can write in the channel itself", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "tickets"), "everyone")
			acc.Deny = without(acc.Deny, "SEND_MESSAGES")
		}, CodeTicketShape, everyone, "must not have SEND_MESSAGES"},
		{"the bot reads the threads of others", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "tickets"), "bot")
			acc.Deny = []string{}
			acc.Allow = []string{"MANAGE_THREADS"}
		}, CodeTicketThreadReader, "channels[tickets].access[bot]", "MANAGE_THREADS"},
		{"a subject guide reads the threads of others", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "tickets"), "guide-coding")
			acc.Allow = []string{"MANAGE_THREADS"}
		}, CodeTicketThreadReader, "channels[tickets].access[guide-coding]", "MANAGE_THREADS"},
	})
}

func TestValidate_Negative_StaffChannelConditions(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"a contributor sees the queue", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "mod-queue"), "contributor").Allow = []string{"VIEW_CHANNEL"}
		}, CodeStaffVisibility, "channels[mod-queue].access[contributor]", "visible"},
		{"every member sees the trail", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "mod-log"), "everyone")
			acc.Deny = without(acc.Deny, "VIEW_CHANNEL")
		}, CodeStaffVisibility, "channels[mod-log].access[everyone]", "visible"},
	})
}

func TestValidate_Negative_FirstAnswerConditions(t *testing.T) {
	bot := "channels[support].access[bot]"
	denyBot := func(permission string) func(t *testing.T, def *Definition) {
		return func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "support"), "bot")
			acc.Allow = without(acc.Allow, permission)
			acc.Deny = append(acc.Deny, permission)
		}
	}
	runPlanted(t, []plantedCase{
		{"the forum is a text channel", func(t *testing.T, def *Definition) {
			channelOf(t, def, "support").Type = "text"
		}, CodeFirstAnswer, "channels[support].type", "forum"},
		{"only moderators can set the opt-in tag", func(t *testing.T, def *Definition) {
			channelOf(t, def, "support").Tags[0].Moderated = true
		}, CodeFirstAnswer, "channels[support].tags[Automated answer]", "moderated"},
		{"two opt-in tags", func(t *testing.T, def *Definition) {
			channelOf(t, def, "support").Tags[1].OptIn = true
		}, CodeFirstAnswer, "channels[support].tags", "exactly one opt_in"},
		{"no opt-in tag", func(t *testing.T, def *Definition) {
			channelOf(t, def, "support").Tags[0].OptIn = false
		}, CodeFirstAnswer, "channels[support].tags", "exactly one opt_in"},
		{"the bot cannot see the forum", denyBot("VIEW_CHANNEL"), CodeFirstAnswer, bot, "VIEW_CHANNEL"},
		{"the bot cannot read the first message", denyBot("READ_MESSAGE_HISTORY"),
			CodeFirstAnswer, bot, "READ_MESSAGE_HISTORY"},
		{"the bot cannot reply in a post", denyBot("SEND_MESSAGES_IN_THREADS"),
			CodeFirstAnswer, bot, "SEND_MESSAGES_IN_THREADS"},
		{"a text channel carries tags", func(t *testing.T, def *Definition) {
			channelOf(t, def, "lounge").Tags = []Tag{{Name: "Topic"}}
		}, CodeFirstAnswer, "channels[lounge].tags", "non-forum"},
	})
}

func TestValidate_Boundary_ForumTagCount(t *testing.T) {
	withTags := func(t *testing.T, count int) []Finding {
		t.Helper()
		def := valid(t)
		ch := channelOf(t, def, "support")
		for i := len(ch.Tags); i < count; i++ {
			ch.Tags = append(ch.Tags, Tag{Name: fmt.Sprintf("Tag %d", i)})
		}
		if len(ch.Tags) != count {
			t.Fatalf("forum has %d tags, want %d", len(ch.Tags), count)
		}
		return Validate(def)
	}
	if findings := withTags(t, MaxForumTags); len(findings) != 0 {
		t.Errorf("%d tags refused: %+v", MaxForumTags, findings)
	}
	if findings := withTags(t, MaxForumTags+1); !hasFinding(findings, CodeFirstAnswer, "channels[support].tags", "limit") {
		t.Errorf("%d tags accepted: %+v", MaxForumTags+1, findings)
	}
}

func TestValidate_Negative_VoiceChannelConditions(t *testing.T) {
	everyone := "channels[events-voice].access[everyone]"
	runPlanted(t, []plantedCase{
		{"the room is open between events", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "events-voice"), "everyone")
			acc.Deny = without(acc.Deny, "CONNECT")
			acc.Allow = append(acc.Allow, "CONNECT")
		}, CodeVoiceClosed, everyone, "CONNECT"},
		{"members speak without push-to-talk", func(t *testing.T, def *Definition) {
			r := roleOf(t, def, "everyone")
			r.Permissions = append(r.Permissions, "USE_VAD")
		}, CodeVoiceClosed, everyone, "USE_VAD"},
		{"the room is hidden from members and still open to them", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "events-voice"), "everyone")
			acc.Deny = []string{"VIEW_CHANNEL"}
			acc.Allow = []string{"CONNECT"}
		}, CodeVoiceClosed, everyone, "CONNECT"},
	})
}

func TestValidate_Negative_AreaConditions(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"no closing rule", func(t *testing.T, def *Definition) {
			areaOf(t, def, "games").Closing = nil
		}, CodeAreaClosingRule, "areas[games].closing", "closing"},
		{"no opening rule", func(t *testing.T, def *Definition) {
			areaOf(t, def, "games").Opening = nil
		}, CodeAreaOpeningRule, "areas[games].opening", "opening"},
		{"the closing rule measures a channel that does not exist", func(t *testing.T, def *Definition) {
			areaOf(t, def, "home-lab").Closing.Subject = "no-such-channel"
		}, CodeRuleShape, "areas[home-lab].closing.subject", "not a channel"},
		{"the opening rule counts a role that does not exist", func(t *testing.T, def *Definition) {
			areaOf(t, def, "home-lab").Opening.Subject = "no-such-role"
		}, CodeRuleShape, "areas[home-lab].opening.subject", "not a role"},
		{"a channel opens at a threshold without an area", func(t *testing.T, def *Definition) {
			channelOf(t, def, "lounge").Opens = "threshold"
		}, CodeAreaChannel, "channels[lounge].area", "name an area"},
		{"an area has no channel", func(t *testing.T, def *Definition) {
			ch := channelOf(t, def, "games")
			ch.Area = ""
			ch.Opens = "start"
		}, CodeAreaChannel, "areas[games]", "not named"},
		{"an area channel is open from the start", func(t *testing.T, def *Definition) {
			channelOf(t, def, "games").Opens = "start"
		}, CodeAreaChannel, "channels[games].opens", "threshold"},
		{"every member sees an area channel", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "home-lab"), "everyone")
			acc.Deny = without(acc.Deny, "VIEW_CHANNEL")
		}, CodeAreaChannel, "channels[home-lab].access[everyone]", "everyone"},
		{"the members who chose the area do not see it", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "home-lab"), "interest-home-lab")
			acc.Allow = without(acc.Allow, "VIEW_CHANNEL")
		}, CodeAreaChannel, "channels[home-lab].access[interest-home-lab]", "area role"},
		{"a language room has no condition", func(t *testing.T, def *Definition) {
			areaOf(t, def, "room-es").Conditions = []string{}
		}, CodeAreaCondition, "areas[room-es].conditions", "at least one"},
		{"a language room has blank conditions only", func(t *testing.T, def *Definition) {
			areaOf(t, def, "room-es").Conditions = []string{" ", ""}
		}, CodeAreaCondition, "areas[room-es].conditions", "empty"},
	})
}

func TestValidate_Negative_AutomationRecordConditions(t *testing.T) {
	record := func(id, value string) func(t *testing.T, def *Definition) {
		return func(t *testing.T, def *Definition) {
			for i := range def.Automation {
				if def.Automation[i].ID == id {
					def.Automation[i].Record = value
					return
				}
			}
			t.Fatalf("definition has no automation %q", id)
		}
	}
	runPlanted(t, []plantedCase{
		{"what the platform does has no record", record("spam-filter", ""),
			CodeAutomationRecord, "automation[spam-filter].record", "relative path"},
		{"what the bot does has no record", record("ask-command", ""),
			CodeAutomationRecord, "automation[ask-command].record", "relative path"},
		{"the record is an absolute path", record("ask-command", "/etc/passwd"),
			CodeAutomationRecord, "automation[ask-command].record", "relative path"},
		{"the record leaves the repository", record("ask-command", "docs/../../outside.md"),
			CodeAutomationRecord, "automation[ask-command].record", "relative path"},
	})
}

func TestValidate_Positive_ObservedPlatformState(t *testing.T) {
	def := valid(t)
	findings := Validate(def)
	if len(findings) != 0 {
		t.Fatalf("committed definition has findings: %+v", findings)
	}
	auto := automationOf(t, def, "membership-screening")
	if auto.By != "platform" {
		t.Errorf("automation[membership-screening].by = %q, want %q", auto.By, "platform")
	}
	if auto.State != "observed" {
		t.Errorf("automation[membership-screening].state = %q, want %q", auto.State, "observed")
	}
	if auto.Record != "" {
		t.Errorf("automation[membership-screening].record = %q, want empty", auto.Record)
	}
}

func TestValidate_Negative_ObservedStateConditions(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"observed automation by bot", func(t *testing.T, def *Definition) {
			automationOf(t, def, "membership-screening").By = "bot"
		}, CodeAutomationRecord, "automation[membership-screening].state", "platform"},
		{"observed automation by operator", func(t *testing.T, def *Definition) {
			automationOf(t, def, "membership-screening").By = "operator"
		}, CodeAutomationRecord, "automation[membership-screening].state", "platform"},
		{"observed automation by moderator", func(t *testing.T, def *Definition) {
			automationOf(t, def, "membership-screening").By = "moderator"
		}, CodeAutomationRecord, "automation[membership-screening].state", "platform"},
		{"observed automation with record", func(t *testing.T, def *Definition) {
			automationOf(t, def, "membership-screening").Record = "docs/adr/0005-moderation-and-escalation.md"
		}, CodeAutomationRecord, "automation[membership-screening].record", "observed"},
	})
}

func TestValidate_Boundary_ObservedNeedsNoRecord(t *testing.T) {
	def := valid(t)
	auto := automationOf(t, def, "membership-screening")
	if auto.Record != "" {
		t.Fatalf("membership-screening has non-empty record: %q", auto.Record)
	}
	findings := Validate(def)
	for _, f := range findings {
		if strings.HasPrefix(f.Path, "automation[membership-screening]") {
			t.Errorf("membership-screening has finding: %+v", f)
		}
	}
	runPlanted(t, []plantedCase{
		{"platform decided automation with empty record is refused", func(t *testing.T, def *Definition) {
			automationOf(t, def, "spam-filter").Record = ""
		}, CodeAutomationRecord, "automation[spam-filter].record", "relative path"},
		{"state observe one letter short", func(t *testing.T, def *Definition) {
			automationOf(t, def, "membership-screening").State = "observe"
		}, CodeEnum, "automation[membership-screening].state", "observe"},
	})
}

// The role of every member is found by its kind. A definition that gives it another id is read
// the same way.
func TestEffective_Positive_EveryoneIsFoundByKind(t *testing.T) {
	def := valid(t)
	const renamed = "all-members"
	roleOf(t, def, "everyone").ID = renamed
	for i := range def.Channels {
		for j := range def.Channels[i].Access {
			if def.Channels[i].Access[j].Role == "everyone" {
				def.Channels[i].Access[j].Role = renamed
			}
		}
	}
	if findings := Validate(def); len(findings) != 0 {
		t.Fatalf("definition with a renamed everyone role has findings: %+v", findings)
	}
	inTickets := Effective(def, renamed, "tickets")
	if _, ok := inTickets["CREATE_PRIVATE_THREADS"]; !ok {
		t.Errorf("every member cannot open a private thread in the ticket channel: %v", inTickets)
	}
	if _, ok := inTickets["SEND_MESSAGES"]; ok {
		t.Errorf("every member can write in the ticket channel itself: %v", inTickets)
	}
	if len(Effective(def, "contributor", "mod-queue")) != 0 {
		t.Errorf("a contributor has permissions in the moderators' queue")
	}
}

// Discord removes what an entry denies and then adds what it allows, so a permission that one
// entry both denies and allows is allowed. The shape rule refuses such an entry; the computation
// still follows the documented order.
func TestEffective_Boundary_AllowAfterDeny(t *testing.T) {
	def := valid(t)
	acc := accessOf(channelOf(t, def, "lounge"), "contributor")
	acc.Deny = []string{"ADD_REACTIONS"}
	acc.Allow = []string{"ADD_REACTIONS"}
	if _, ok := Effective(def, "contributor", "lounge")["ADD_REACTIONS"]; !ok {
		t.Errorf("a permission that an entry denies and allows is not allowed")
	}
}

func TestEffective_Negative_OverwritesApply(t *testing.T) {
	def := valid(t)
	if _, ok := Effective(def, "everyone", "rules")["SEND_MESSAGES"]; ok {
		t.Errorf("every member can write in the rules channel: the entry for every member is not applied")
	}
	if _, ok := Effective(def, "contributor", "rules")["SEND_MESSAGES"]; ok {
		t.Errorf("a contributor can write in the rules channel: the entry for every member is not applied to a role")
	}
	if _, ok := Effective(def, "interest-games", "games")["VIEW_CHANNEL"]; !ok {
		t.Errorf("a member who chose games cannot see the area: the entry of the role is not applied")
	}
	if len(Effective(def, "bot", "tickets")) != 0 {
		t.Errorf("the bot has permissions in the ticket channel: the entry of the role is not applied")
	}
}

// A member can hold several roles, and one of them can make a channel visible that another one
// cannot see. So a rule that refuses a right reads what a role is granted in a channel whether
// or not that role sees the channel.
func TestValidate_Negative_RightsInAChannelTheRoleCannotSee(t *testing.T) {
	inTickets := func(permission string) func(t *testing.T, def *Definition) {
		return func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "tickets"), "bot")
			if len(acc.Deny) != 1 || acc.Deny[0] != "VIEW_CHANNEL" {
				t.Fatalf("the bot is expected not to see the ticket channel, entry: %+v", acc)
			}
			acc.Allow = []string{permission}
		}
	}
	runPlanted(t, []plantedCase{
		{"the bot may read all ticket threads", inTickets("MANAGE_THREADS"),
			CodeTicketThreadReader, "channels[tickets].access[bot]", "MANAGE_THREADS"},
		{"the bot may manage the ticket channel", inTickets("MANAGE_CHANNELS"),
			CodeOperatorOnly, "channels[tickets].access[bot]", "MANAGE_CHANNELS"},
		{"the bot may delete messages in the ticket channel", inTickets("MANAGE_MESSAGES"),
			CodeBotRights, "channels[tickets].access[bot]", "MANAGE_MESSAGES"},
		{"the bot may view the audit log in the ticket channel", inTickets("VIEW_AUDIT_LOG"),
			CodeModerationOnly, "channels[tickets].access[bot]", "VIEW_AUDIT_LOG"},
		{"the bot may mention everyone in the ticket channel", inTickets("MENTION_EVERYONE"),
			CodeMentionEveryone, "channels[tickets].access[bot]", "effective"},
	})
}

// Moderators cannot moderate what they cannot see, and they own most channels.
func TestValidate_Negative_ModeratorCannotSeeAChannel(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"an area channel", func(t *testing.T, def *Definition) {
			ch := channelOf(t, def, "games")
			kept := make([]Access, 0, len(ch.Access))
			for _, acc := range ch.Access {
				if acc.Role != "moderator" {
					kept = append(kept, acc)
				}
			}
			ch.Access = kept
		}, CodeModeratorVisibility, "channels[games].access[moderator]", "see"},
		{"the lounge", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "lounge"), "moderator").Deny = []string{"VIEW_CHANNEL"}
		}, CodeModeratorVisibility, "channels[lounge].access[moderator]", "see"},
	})
}

func TestValidate_Positive_ModeratorsSeeEveryChannel(t *testing.T) {
	def := valid(t)
	for _, ch := range def.Channels {
		if _, ok := Effective(def, "moderator", ch.ID)["VIEW_CHANNEL"]; !ok {
			t.Errorf("a moderator cannot see the channel %s", ch.ID)
		}
	}
}
