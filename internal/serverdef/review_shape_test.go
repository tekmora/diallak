package serverdef

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The cases in this file pin the shape rules: ids, texts, the fixed sets of values, references,
// permission names and lists, and who grants a role. They use the helpers of review_test.go.

func automationOf(t *testing.T, def *Definition, id string) *Automation {
	t.Helper()
	for i := range def.Automation {
		if def.Automation[i].ID == id {
			return &def.Automation[i]
		}
	}
	t.Fatalf("definition has no automation %q", id)
	return nil
}

func TestValidate_Negative_IDsOfEveryKind(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"metric", func(_ *testing.T, def *Definition) { def.Metrics[0].ID = "M" }, CodeID, "metrics[M]", "regex"},
		{"role", func(t *testing.T, def *Definition) { roleOf(t, def, "contributor").ID = "R" }, CodeID, "roles[R]", "regex"},
		{"category", func(_ *testing.T, def *Definition) { def.Categories[0].ID = "C" }, CodeID, "categories[C]", "regex"},
		{"channel", func(t *testing.T, def *Definition) { channelOf(t, def, "lounge").ID = "L" }, CodeID, "channels[L]", "regex"},
		{"area", func(t *testing.T, def *Definition) { areaOf(t, def, "games").ID = "A" }, CodeID, "areas[A]", "regex"},
		{"question", func(_ *testing.T, def *Definition) { def.Choices.Questions[0].ID = "Q" },
			CodeID, "choices.questions[Q]", "regex"},
		{"automation", func(t *testing.T, def *Definition) { automationOf(t, def, "news").ID = "N" },
			CodeID, "automation[N]", "regex"},
		{"two channels with one id", func(t *testing.T, def *Definition) {
			channelOf(t, def, "lounge").ID = "rules"
		}, CodeID, "channels[rules]", "duplicate"},
	})
}

func TestValidate_Negative_EmptyTexts(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"summary", func(_ *testing.T, def *Definition) { def.Summary = " " }, CodeText, "summary", "empty"},
		{"what a metric measures", func(_ *testing.T, def *Definition) { def.Metrics[0].Measures = " " },
			CodeText, "metrics[members-with-role].measures", "empty"},
		{"where a metric comes from", func(_ *testing.T, def *Definition) { def.Metrics[0].Source = "" },
			CodeText, "metrics[members-with-role].source", "empty"},
		{"role name", func(t *testing.T, def *Definition) { roleOf(t, def, "moderator").Name = " " },
			CodeText, "roles[moderator].name", "empty"},
		{"role purpose", func(t *testing.T, def *Definition) { roleOf(t, def, "moderator").Purpose = "" },
			CodeText, "roles[moderator].purpose", "empty"},
		{"category name", func(_ *testing.T, def *Definition) { def.Categories[0].Name = "" },
			CodeText, "categories[start].name", "empty"},
		{"category purpose", func(_ *testing.T, def *Definition) { def.Categories[0].Purpose = " " },
			CodeText, "categories[start].purpose", "empty"},
		{"channel name of blanks", func(t *testing.T, def *Definition) { channelOf(t, def, "lounge").Name = "   " },
			CodeText, "channels[lounge].name", "between 1 and 100"},
		{"channel purpose", func(t *testing.T, def *Definition) { channelOf(t, def, "lounge").Purpose = "" },
			CodeText, "channels[lounge].purpose", "empty"},
		{"tag name of blanks", func(t *testing.T, def *Definition) { channelOf(t, def, "support").Tags[1].Name = "  " },
			CodeText, "channels[support].tags[  ]", "between 1 and 20"},
		{"area name", func(t *testing.T, def *Definition) { areaOf(t, def, "games").Name = "" },
			CodeText, "areas[games].name", "empty"},
		{"reasoning of an opening rule", func(t *testing.T, def *Definition) {
			areaOf(t, def, "games").Opening.Reasoning = " "
		}, CodeText, "areas[games].opening.reasoning", "empty"},
		{"reasoning of a closing rule", func(t *testing.T, def *Definition) {
			areaOf(t, def, "games").Closing.Reasoning = ""
		}, CodeText, "areas[games].closing.reasoning", "empty"},
		{"question title", func(_ *testing.T, def *Definition) { def.Choices.Questions[0].Title = "" },
			CodeText, "choices.questions[interests].title", "empty"},
		{"question title of spaces", func(_ *testing.T, def *Definition) { def.Choices.Questions[1].Title = " \t" },
			CodeText, "choices.questions[languages].title", "empty"},
		{"option title", func(_ *testing.T, def *Definition) { def.Choices.Questions[0].Options[2].Title = " " },
			CodeText, "choices.questions[interests].options[interest-games].title", "empty"},
		{"what an automation does", func(t *testing.T, def *Definition) { automationOf(t, def, "news").What = "" },
			CodeText, "automation[news].what", "empty"},
	})
}

func TestValidate_Negative_ValuesOutsideTheFixedSets(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"role kind", func(t *testing.T, def *Definition) { roleOf(t, def, "contributor").Kind = "vip" },
			CodeEnum, "roles[contributor].kind", "vip"},
		{"who assigns a role", func(t *testing.T, def *Definition) { roleOf(t, def, "contributor").AssignedBy = "anyone" },
			CodeEnum, "roles[contributor].assigned_by", "anyone"},
		{"former assigned_by value", func(t *testing.T, def *Definition) {
			roleOf(t, def, "contributor").AssignedBy = "on" + "boarding"
		}, CodeEnum, "roles[contributor].assigned_by", "on" + "boarding"},
		{"basis of a role", func(t *testing.T, def *Definition) { roleOf(t, def, "contributor").Basis = "rumour" },
			CodeEnum, "roles[contributor].basis", "rumour"},
		{"channel type", func(t *testing.T, def *Definition) { channelOf(t, def, "lounge").Type = "stage" },
			CodeEnum, "channels[lounge].type", "stage"},
		{"when a channel opens", func(t *testing.T, def *Definition) { channelOf(t, def, "lounge").Opens = "never" },
			CodeEnum, "channels[lounge].opens", "never"},
		{"area kind", func(t *testing.T, def *Definition) { areaOf(t, def, "games").Kind = "club" },
			CodeEnum, "areas[games].kind", "club"},
		{"what closing does", func(t *testing.T, def *Definition) { areaOf(t, def, "games").OnClose = "delete" },
			CodeEnum, "areas[games].on_close", "delete"},
		{"comparison of a rule", func(t *testing.T, def *Definition) { areaOf(t, def, "games").Opening.Comparison = "at-most" },
			CodeEnum, "areas[games].opening.comparison", "at-most"},
		{"basis of a rule", func(t *testing.T, def *Definition) { areaOf(t, def, "games").Closing.Basis = "guess" },
			CodeEnum, "areas[games].closing.basis", "guess"},
		{"who does an automation", func(t *testing.T, def *Definition) { automationOf(t, def, "news").By = "nobody" },
			CodeEnum, "automation[news].by", "nobody"},
		{"state of an automation", func(t *testing.T, def *Definition) { automationOf(t, def, "news").State = "done" },
			CodeEnum, "automation[news].state", "done"},
	})
}

func TestValidate_Positive_MeasuredRuleIsAccepted(t *testing.T) {
	def := valid(t)
	areaOf(t, def, "games").Opening.Basis = "measured"
	if findings := Validate(def); len(findings) != 0 {
		t.Errorf("a rule marked as measured has findings: %+v", findings)
	}
}

func TestValidate_Negative_RolesThatMustExistOnce(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"no role for every member", func(t *testing.T, def *Definition) { roleOf(t, def, "everyone").Kind = "recognition" },
			CodeSingleton, "roles", "everyone"},
		{"no operator", func(t *testing.T, def *Definition) {
			op := roleOf(t, def, "operator")
			op.Kind = "moderator"
			op.Permissions = []string{}
		}, CodeSingleton, "roles", "operator"},
		{"no bot role", func(t *testing.T, def *Definition) { roleOf(t, def, "bot").Kind = "recognition" },
			CodeSingleton, "roles", "bot"},
		{"two operators", func(t *testing.T, def *Definition) { roleOf(t, def, "moderator").Kind = "operator" },
			CodeSingleton, "roles", "operator"},
	})
}

func TestValidate_Negative_ReferencesToNothing(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"category of a channel", func(t *testing.T, def *Definition) { channelOf(t, def, "lounge").Category = "nowhere" },
			CodeRef, "channels[lounge].category", "nowhere"},
		{"area of a channel", func(t *testing.T, def *Definition) { channelOf(t, def, "games").Area = "nowhere" },
			CodeRef, "channels[games].area", "nowhere"},
		{"role of an access entry", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "lounge"), "nobody").Deny = []string{"ADD_REACTIONS"}
		}, CodeRef, "channels[lounge].access[nobody]", "nobody"},
		{"role of an area", func(t *testing.T, def *Definition) { areaOf(t, def, "games").Role = "nobody" },
			CodeRef, "areas[games].role", "nobody"},
		{"metric of an opening rule", func(t *testing.T, def *Definition) { areaOf(t, def, "games").Opening.Metric = "mood" },
			CodeRef, "areas[games].opening.metric", "mood"},
		{"metric of a closing rule", func(t *testing.T, def *Definition) { areaOf(t, def, "games").Closing.Metric = "mood" },
			CodeRef, "areas[games].closing.metric", "mood"},
		{"role of a choice option", func(_ *testing.T, def *Definition) {
			def.Choices.Questions[0].Options[0].Role = "nobody"
		}, CodeRef, "choices.questions[interests].options[nobody]", "nobody"},
	})
}

func TestValidate_Negative_ChannelOwnerConditions(t *testing.T) {
	owner := func(id string) func(t *testing.T, def *Definition) {
		return func(t *testing.T, def *Definition) { channelOf(t, def, "lounge").Owner = id }
	}
	runPlanted(t, []plantedCase{
		{"no owner", owner(""), CodeChannelOwner, "channels[lounge].owner", "empty"},
		{"an owner that does not exist", owner("nobody"), CodeChannelOwner, "channels[lounge].owner", "not found"},
		{"the bot as owner", owner("bot"), CodeChannelOwner, "channels[lounge].owner", "operator or moderator"},
		{"a subject guide as owner", owner("guide-coding"), CodeChannelOwner, "channels[lounge].owner", "operator or moderator"},
		{"every member as owner", owner("everyone"), CodeChannelOwner, "channels[lounge].owner", "operator or moderator"},
	})
}

func TestValidate_Negative_PermissionNamesAndLists(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"unknown permission of a role", func(t *testing.T, def *Definition) {
			r := roleOf(t, def, "moderator")
			r.Permissions = append(r.Permissions, "FLY")
		}, CodePermissionName, "roles[moderator].permissions", "FLY"},
		{"unknown permission in an allow list", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "lounge"), "everyone").Allow = []string{"FLY"}
		}, CodePermissionName, "channels[lounge].access[everyone].allow", "FLY"},
		{"unknown permission in a deny list", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "lounge"), "everyone").Deny = []string{"FLY"}
		}, CodePermissionName, "channels[lounge].access[everyone].deny", "FLY"},
		{"a role lists a permission twice", func(t *testing.T, def *Definition) {
			r := roleOf(t, def, "everyone")
			r.Permissions = append(r.Permissions, "VIEW_CHANNEL")
		}, CodeAccessShape, "roles[everyone].permissions", "duplicate"},
		{"an allow list names a permission twice", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "support"), "bot")
			acc.Allow = append(acc.Allow, "VIEW_CHANNEL")
		}, CodeAccessShape, "channels[support].access[bot].allow", "duplicate"},
		{"a deny list names a permission twice", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "rules"), "everyone")
			acc.Deny = append(acc.Deny, "SEND_MESSAGES")
		}, CodeAccessShape, "channels[rules].access[everyone].deny", "duplicate"},
		{"a channel has two entries for one role", func(t *testing.T, def *Definition) {
			ch := channelOf(t, def, "rules")
			ch.Access = append(ch.Access, Access{Role: "everyone", Allow: []string{}, Deny: []string{}})
		}, CodeAccessShape, "channels[rules].access[everyone]", "duplicate access entry"},
		{"an entry allows and denies one permission", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "rules"), "everyone")
			acc.Allow = append(acc.Allow, "SEND_MESSAGES")
		}, CodeAccessShape, "channels[rules].access[everyone]", "both allow and deny"},
	})
}

func TestValidate_Negative_MentionEveryoneConditions(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"a moderator holds it as a role permission", func(t *testing.T, def *Definition) {
			r := roleOf(t, def, "moderator")
			r.Permissions = append(r.Permissions, "MENTION_EVERYONE")
		}, CodeMentionEveryone, "roles[moderator].permissions", "base"},
		{"a moderator is allowed it in one channel", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "lounge"), "moderator").Allow = []string{"MENTION_EVERYONE"}
		}, CodeMentionEveryone, "channels[lounge].access[moderator]", "allow list"},
		{"it takes effect for a moderator in that channel", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "lounge"), "moderator").Allow = []string{"MENTION_EVERYONE"}
		}, CodeMentionEveryone, "channels[lounge].access[moderator]", "effective"},
		{"every member holds it, so it takes effect in the lounge", func(t *testing.T, def *Definition) {
			r := roleOf(t, def, "everyone")
			r.Permissions = append(r.Permissions, "MENTION_EVERYONE")
		}, CodeMentionEveryone, "channels[lounge].access[everyone]", "effective"},
		{"every member is allowed it in the lounge", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "lounge"), "everyone").Allow = []string{"MENTION_EVERYONE"}
		}, CodeMentionEveryone, "channels[lounge].access[everyone]", "allow list"},
	})
}

func TestValidate_Negative_RightsOfTheOperatorAndOfModerators(t *testing.T) {
	grant := func(roleID, permission string) func(t *testing.T, def *Definition) {
		return func(t *testing.T, def *Definition) {
			r := roleOf(t, def, roleID)
			r.Permissions = append(r.Permissions, permission)
		}
	}
	cases := make([]plantedCase, 0, 16)
	for _, permission := range []string{
		"ADMINISTRATOR", "MANAGE_GUILD", "MANAGE_ROLES", "MANAGE_CHANNELS", "MANAGE_WEBHOOKS", "BAN_MEMBERS", "KICK_MEMBERS",
	} {
		cases = append(cases, plantedCase{
			"a moderator holds " + permission, grant("moderator", permission),
			CodeOperatorOnly, "channels[lounge].access[moderator]", "permission " + permission + " ",
		})
	}
	for _, permission := range []string{
		"MANAGE_THREADS", "MUTE_MEMBERS", "MOVE_MEMBERS", "VIEW_AUDIT_LOG",
	} {
		cases = append(cases, plantedCase{
			"the bot holds " + permission, grant("bot", permission),
			CodeModerationOnly, "channels[lounge].access[bot]", "permission " + permission + " ",
		})
	}
	cases = append(cases, plantedCase{
		"a contributor deletes messages", grant("contributor", "MANAGE_MESSAGES"),
		CodeModerationOnly, "channels[lounge].access[contributor]", "MANAGE_MESSAGES",
	})
	cases = append(cases, plantedCase{
		"a subject guide times members out", grant("guide-coding", "MODERATE_MEMBERS"),
		CodeModerationOnly, "channels[lounge].access[guide-coding]", "MODERATE_MEMBERS",
	})
	cases = append(cases, plantedCase{
		"a subject guide deletes messages", grant("guide-video", "MANAGE_MESSAGES"),
		CodeModerationOnly, "channels[lounge].access[guide-video]", "MANAGE_MESSAGES",
	})
	runPlanted(t, cases)
}

func TestValidate_Negative_NothingAttachesConditions(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"a supporter role carries a permission", func(t *testing.T, def *Definition) {
			roleOf(t, def, "supporter").Permissions = []string{"ADD_REACTIONS"}
		}, CodeNothingAttaches, "roles[supporter].permissions", "recognition"},
		{"an interest role carries a permission", func(t *testing.T, def *Definition) {
			roleOf(t, def, "interest-games").Permissions = []string{"ADD_REACTIONS"}
		}, CodeNothingAttaches, "roles[interest-games].permissions", "interest"},
		{"a language role carries a permission", func(t *testing.T, def *Definition) {
			roleOf(t, def, "lang-es").Permissions = []string{"ADD_REACTIONS"}
		}, CodeNothingAttaches, "roles[lang-es].permissions", "language"},
		{"a supporter is allowed something in a channel", func(t *testing.T, def *Definition) {
			accessOf(channelOf(t, def, "lounge"), "supporter").Allow = []string{"ATTACH_FILES"}
		}, CodeNothingAttaches, "channels[lounge].access[supporter]", "recognition"},
		{"an interest role is allowed more than seeing its area", func(t *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "games"), "interest-games")
			acc.Allow = append(acc.Allow, "ATTACH_FILES")
		}, CodeNothingAttaches, "channels[games].access[interest-games]", "VIEW_CHANNEL"},
	})
}

func TestValidate_Negative_WhoGrantsARole(t *testing.T) {
	by := func(roleID, who string) func(t *testing.T, def *Definition) {
		return func(t *testing.T, def *Definition) { roleOf(t, def, roleID).AssignedBy = who }
	}
	option := func(roleID string) func(t *testing.T, def *Definition) {
		return func(_ *testing.T, def *Definition) { def.Choices.Questions[0].Options[0].Role = roleID }
	}
	runPlanted(t, []plantedCase{
		{"choice makes a moderator", by("moderator", "choice"), CodeRoleGrant, "roles[moderator].assigned_by", "operator"},
		{"the bot makes an operator", by("operator", "bot"), CodeRoleGrant, "roles[operator].assigned_by", "operator"},
		{"the operator hands out an interest", by("interest-games", "operator"),
			CodeRoleGrant, "roles[interest-games].assigned_by", "choice"},
		{"the bot hands out a language", by("lang-es", "bot"), CodeRoleGrant, "roles[lang-es].assigned_by", "choice"},
		{"the operator assigns the bot's role", by("bot", "operator"), CodeRoleGrant, "roles[bot].assigned_by", "platform"},
		{"the operator assigns the role of every member", by("everyone", "operator"),
			CodeRoleGrant, "roles[everyone].assigned_by", "platform"},
		{"choice gives a recognition role", by("contributor", "choice"),
			CodeRoleGrant, "roles[contributor].assigned_by", "operator, bot or integration"},
		{"a choice option gives the contributor role", option("contributor"),
			CodeRoleGrant, "choices.questions[interests].options[contributor]", "neither interest nor language"},
		{"a choice option gives the moderator role", option("moderator"),
			CodeRoleGrant, "choices.questions[interests].options[moderator]", "neither interest nor language"},
	})
}

func committed(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../server/definition.json")
	if err != nil {
		t.Fatalf("read definition: %v", err)
	}
	return data
}

func TestLoad_Negative_SecondValueAfterTheDefinition(t *testing.T) {
	for _, tail := range []string{" {}", " []", " 1", ` "x"`, " null", " true"} {
		input := append(committed(t), []byte(tail)...)
		if _, err := Load(bytes.NewReader(input)); err == nil {
			t.Errorf("a definition followed by %q was accepted", tail)
		}
	}
}

func TestLoad_Boundary_SizeOfAValidDefinition(t *testing.T) {
	data := committed(t)
	pad := func(size int) []byte {
		return append(append([]byte{}, data...), bytes.Repeat([]byte{' '}, size-len(data))...)
	}
	if _, err := Load(bytes.NewReader(pad(MaxBytes))); err != nil {
		t.Errorf("a valid definition of exactly %d bytes was refused: %v", MaxBytes, err)
	}
	_, err := Load(bytes.NewReader(pad(MaxBytes + 1)))
	if err == nil {
		t.Fatalf("a valid definition of %d bytes was accepted", MaxBytes+1)
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("the error for an oversized definition does not name the limit: %v", err)
	}
}
