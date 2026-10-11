package serverdef

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func valid(t *testing.T) *Definition {
	t.Helper()
	data, err := os.ReadFile("../../server/definition.json")
	if err != nil {
		t.Fatalf("read definition fixture: %v", err)
	}
	def, err := Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load valid fixture: %v", err)
	}
	return def
}

func hasCode(findings []Finding, code string) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestValidate_Positive_CommittedDefinitionHasNoFindings(t *testing.T) {
	def := valid(t)
	findings := Validate(def)
	if len(findings) != 0 {
		t.Fatalf("Validate on committed definition returned %d findings: %+v", len(findings), findings)
	}
}

func TestValidate_Positive_ModeratorMayManageThreadsInTickets(t *testing.T) {
	def := valid(t)
	// Add an access overwrite granting MANAGE_THREADS to the moderator in tickets.
	for i, ch := range def.Channels {
		if ch.ID == "tickets" {
			def.Channels[i].Access = append(def.Channels[i].Access, Access{
				Role:  "moderator",
				Allow: []string{"MANAGE_THREADS"},
				Deny:  []string{},
			})
			break
		}
	}
	findings := Validate(def)
	if hasCode(findings, CodeTicketThreadReader) {
		t.Errorf("moderator granted MANAGE_THREADS produced %s finding", CodeTicketThreadReader)
	}
}

func TestValidate_Negative_ChannelWithoutOwnerFails(t *testing.T) {
	t.Run("owner empty", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Owner = ""
		findings := Validate(def)
		if !hasCode(findings, CodeChannelOwner) {
			t.Errorf("channel with empty owner missing code %s", CodeChannelOwner)
		}
	})

	t.Run("owner is recognition role", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Owner = "contributor"
		findings := Validate(def)
		if !hasCode(findings, CodeChannelOwner) {
			t.Errorf("channel with recognition owner missing code %s", CodeChannelOwner)
		}
	})

	t.Run("owner unknown id", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Owner = "nonexistent-role"
		findings := Validate(def)
		if !hasCode(findings, CodeChannelOwner) {
			t.Errorf("channel with unknown owner missing code %s", CodeChannelOwner)
		}
	})
}

func TestValidate_Negative_EveryoneMayMentionAllFails(t *testing.T) {
	t.Run("everyone base permission", func(t *testing.T) {
		def := valid(t)
		for i, r := range def.Roles {
			if r.ID == "everyone" {
				def.Roles[i].Permissions = append(def.Roles[i].Permissions, "MENTION_EVERYONE")
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeMentionEveryone) {
			t.Errorf("everyone with base MENTION_EVERYONE missing code %s", CodeMentionEveryone)
		}
	})

	t.Run("everyone allow in one channel", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "lounge" {
				def.Channels[i].Access = append(def.Channels[i].Access, Access{
					Role:  "everyone",
					Allow: []string{"MENTION_EVERYONE"},
					Deny:  []string{},
				})
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeMentionEveryone) {
			t.Errorf("everyone allowed MENTION_EVERYONE in lounge missing code %s", CodeMentionEveryone)
		}
	})

	t.Run("recognition role allow", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "lounge" {
				def.Channels[i].Access = append(def.Channels[i].Access, Access{
					Role:  "contributor",
					Allow: []string{"MENTION_EVERYONE"},
					Deny:  []string{},
				})
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeMentionEveryone) {
			t.Errorf("contributor allowed MENTION_EVERYONE missing code %s", CodeMentionEveryone)
		}
	})
}

func TestValidate_Negative_AreaWithoutClosingRuleFails(t *testing.T) {
	t.Run("closing rule removed", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Closing = nil
		findings := Validate(def)
		if !hasCode(findings, CodeAreaClosingRule) {
			t.Errorf("area without closing rule missing code %s", CodeAreaClosingRule)
		}
	})

	t.Run("opening rule removed", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening = nil
		findings := Validate(def)
		if !hasCode(findings, CodeAreaOpeningRule) {
			t.Errorf("area without opening rule missing code %s", CodeAreaOpeningRule)
		}
	})
}

func TestValidate_Negative_NonModeratorReadsTicketThreadsFails(t *testing.T) {
	t.Run("bot allow MANAGE_THREADS in tickets", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "tickets" {
				for j, acc := range ch.Access {
					if acc.Role == "bot" {
						def.Channels[i].Access[j].Allow = append(def.Channels[i].Access[j].Allow, "VIEW_CHANNEL", "MANAGE_THREADS")
						def.Channels[i].Access[j].Deny = []string{}
					}
				}
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeTicketThreadReader) {
			t.Errorf("bot allowed MANAGE_THREADS in tickets missing code %s", CodeTicketThreadReader)
		}
	})

	t.Run("everyone allow MANAGE_THREADS in tickets", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "tickets" {
				for j, acc := range ch.Access {
					if acc.Role == "everyone" {
						def.Channels[i].Access[j].Allow = append(def.Channels[i].Access[j].Allow, "MANAGE_THREADS")
					}
				}
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeTicketThreadReader) {
			t.Errorf("everyone allowed MANAGE_THREADS in tickets missing code %s", CodeTicketThreadReader)
		}
	})

	t.Run("interest role base permission MANAGE_THREADS", func(t *testing.T) {
		def := valid(t)
		for i, r := range def.Roles {
			if r.ID == "interest-home-lab" {
				def.Roles[i].Permissions = append(def.Roles[i].Permissions, "MANAGE_THREADS")
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeTicketThreadReader) {
			t.Errorf("interest role with base MANAGE_THREADS missing code %s in tickets", CodeTicketThreadReader)
		}
	})
}

func TestValidate_Negative_IDFormatAndUniquenessFails(t *testing.T) {
	t.Run("invalid id characters", func(t *testing.T) {
		def := valid(t)
		def.Roles[0].ID = "Invalid_ID!"
		findings := Validate(def)
		if !hasCode(findings, CodeID) {
			t.Errorf("invalid role id missing code %s", CodeID)
		}
	})

	t.Run("duplicate id within same kind", func(t *testing.T) {
		def := valid(t)
		def.Roles[1].ID = def.Roles[0].ID
		findings := Validate(def)
		if !hasCode(findings, CodeID) {
			t.Errorf("duplicate role id missing code %s", CodeID)
		}
	})
}

func TestValidate_Negative_TextFieldsFail(t *testing.T) {
	t.Run("empty summary", func(t *testing.T) {
		def := valid(t)
		def.Summary = "   "
		findings := Validate(def)
		if !hasCode(findings, CodeText) {
			t.Errorf("whitespace summary missing code %s", CodeText)
		}
	})

	t.Run("channel name over 100 runes", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Name = strings.Repeat("c", 101)
		findings := Validate(def)
		if !hasCode(findings, CodeText) {
			t.Errorf("long channel name missing code %s", CodeText)
		}
	})

	t.Run("tag name over 20 runes", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if len(ch.Tags) > 0 {
				def.Channels[i].Tags[0].Name = strings.Repeat("t", 21)
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeText) {
			t.Errorf("long tag name missing code %s", CodeText)
		}
	})
}

func TestValidate_Negative_EnumFailures(t *testing.T) {
	t.Run("unknown role kind", func(t *testing.T) {
		def := valid(t)
		def.Roles[0].Kind = "superuser"
		findings := Validate(def)
		if !hasCode(findings, CodeEnum) {
			t.Errorf("unknown role kind missing code %s", CodeEnum)
		}
	})

	t.Run("unknown channel type", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Type = "stage"
		findings := Validate(def)
		if !hasCode(findings, CodeEnum) {
			t.Errorf("unknown channel type missing code %s", CodeEnum)
		}
	})
}

func TestValidate_Negative_VersionAndAppliedFail(t *testing.T) {
	t.Run("version not 1", func(t *testing.T) {
		def := valid(t)
		def.Version = 2
		findings := Validate(def)
		if !hasCode(findings, CodeVersion) {
			t.Errorf("version 2 missing code %s", CodeVersion)
		}
	})

	t.Run("applied true", func(t *testing.T) {
		def := valid(t)
		def.Applied = true
		findings := Validate(def)
		if !hasCode(findings, CodeVersion) {
			t.Errorf("applied true missing code %s", CodeVersion)
		}
	})
}

func TestValidate_Negative_SingletonRolesFail(t *testing.T) {
	t.Run("two operator roles", func(t *testing.T) {
		def := valid(t)
		def.Roles = append(def.Roles, Role{
			ID:          "second-op",
			Name:        "Second Op",
			Kind:        "operator",
			Purpose:     "Second operator",
			AssignedBy:  "operator",
			Basis:       "decided",
			Permissions: []string{"ADMINISTRATOR"},
		})
		findings := Validate(def)
		if !hasCode(findings, CodeSingleton) {
			t.Errorf("two operator roles missing code %s", CodeSingleton)
		}
	})

	t.Run("zero bot roles", func(t *testing.T) {
		def := valid(t)
		var filtered []Role
		for _, r := range def.Roles {
			if r.Kind != "bot" {
				filtered = append(filtered, r)
			}
		}
		def.Roles = filtered
		findings := Validate(def)
		if !hasCode(findings, CodeSingleton) {
			t.Errorf("zero bot roles missing code %s", CodeSingleton)
		}
	})
}

func TestValidate_Negative_BrokenReferencesFail(t *testing.T) {
	t.Run("channel category non-existent", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Category = "nonexistent-category"
		findings := Validate(def)
		if !hasCode(findings, CodeRef) {
			t.Errorf("unresolved category missing code %s", CodeRef)
		}
	})

	t.Run("access role non-existent", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Access = append(def.Channels[0].Access, Access{
			Role:  "phantom-role",
			Allow: []string{"VIEW_CHANNEL"},
			Deny:  []string{},
		})
		findings := Validate(def)
		if !hasCode(findings, CodeRef) {
			t.Errorf("unresolved access role missing code %s", CodeRef)
		}
	})
}

func TestValidate_Negative_UnknownPermissionFails(t *testing.T) {
	def := valid(t)
	def.Roles[0].Permissions = append(def.Roles[0].Permissions, "SUPER_ADMIN_POWERS")
	findings := Validate(def)
	if !hasCode(findings, CodePermissionName) {
		t.Errorf("unknown permission missing code %s", CodePermissionName)
	}
}

func TestValidate_Negative_AccessShapeFails(t *testing.T) {
	t.Run("duplicate role in channel access", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Access = append(def.Channels[0].Access, Access{
			Role:  "everyone",
			Allow: []string{},
			Deny:  []string{},
		})
		findings := Validate(def)
		if !hasCode(findings, CodeAccessShape) {
			t.Errorf("duplicate role in access list missing code %s", CodeAccessShape)
		}
	})

	t.Run("overlap between allow and deny", func(t *testing.T) {
		def := valid(t)
		def.Channels[0].Access = append(def.Channels[0].Access, Access{
			Role:  "operator",
			Allow: []string{"VIEW_CHANNEL"},
			Deny:  []string{"VIEW_CHANNEL"},
		})
		findings := Validate(def)
		if !hasCode(findings, CodeAccessShape) {
			t.Errorf("allow/deny overlap missing code %s", CodeAccessShape)
		}
	})
}

func TestValidate_Negative_OperatorOnlyPermissionsFail(t *testing.T) {
	def := valid(t)
	// Give moderator role BAN_MEMBERS permission.
	for i, r := range def.Roles {
		if r.ID == "moderator" {
			def.Roles[i].Permissions = append(def.Roles[i].Permissions, "BAN_MEMBERS")
			break
		}
	}
	findings := Validate(def)
	if !hasCode(findings, CodeOperatorOnly) {
		t.Errorf("moderator with BAN_MEMBERS missing code %s", CodeOperatorOnly)
	}
}

func TestValidate_Negative_ModerationOnlyPermissionsFail(t *testing.T) {
	def := valid(t)
	// Give contributor recognition role MODERATE_MEMBERS in channel.
	for i, ch := range def.Channels {
		if ch.ID == "lounge" {
			def.Channels[i].Access = append(def.Channels[i].Access, Access{
				Role:  "contributor",
				Allow: []string{"MODERATE_MEMBERS"},
				Deny:  []string{},
			})
			break
		}
	}
	findings := Validate(def)
	if !hasCode(findings, CodeModerationOnly) {
		t.Errorf("contributor with MODERATE_MEMBERS missing code %s", CodeModerationOnly)
	}
}

func TestValidate_Negative_NothingAttachesFails(t *testing.T) {
	t.Run("recognition role permissions not empty", func(t *testing.T) {
		def := valid(t)
		for i, r := range def.Roles {
			if r.Kind == "recognition" {
				def.Roles[i].Permissions = []string{"VIEW_CHANNEL"}
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeNothingAttaches) {
			t.Errorf("recognition role with permissions missing code %s", CodeNothingAttaches)
		}
	})

	t.Run("recognition role allowed permission in channel", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "lounge" {
				def.Channels[i].Access = append(def.Channels[i].Access, Access{
					Role:  "contributor",
					Allow: []string{"VIEW_CHANNEL"},
					Deny:  []string{},
				})
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeNothingAttaches) {
			t.Errorf("recognition role allowed in channel missing code %s", CodeNothingAttaches)
		}
	})
}

func TestValidate_Negative_RoleGrantFails(t *testing.T) {
	t.Run("operator assigned by onboarding", func(t *testing.T) {
		def := valid(t)
		for i, r := range def.Roles {
			if r.Kind == "operator" {
				def.Roles[i].AssignedBy = "onboarding"
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeRoleGrant) {
			t.Errorf("operator assigned by onboarding missing code %s", CodeRoleGrant)
		}
	})

	t.Run("onboarding option grants moderator", func(t *testing.T) {
		def := valid(t)
		def.Onboarding.Questions[0].Options[0].Role = "moderator"
		findings := Validate(def)
		if !hasCode(findings, CodeRoleGrant) {
			t.Errorf("onboarding granting moderator missing code %s", CodeRoleGrant)
		}
	})
}

func TestValidate_Negative_TicketShapeFails(t *testing.T) {
	t.Run("tickets type voice", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "tickets" {
				def.Channels[i].Type = "voice"
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeTicketShape) {
			t.Errorf("tickets channel with voice type missing code %s", CodeTicketShape)
		}
	})

	t.Run("tickets everyone allowed SEND_MESSAGES", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "tickets" {
				for j, acc := range ch.Access {
					if acc.Role == "everyone" {
						def.Channels[i].Access[j].Deny = []string{}
						def.Channels[i].Access[j].Allow = append(def.Channels[i].Access[j].Allow, "SEND_MESSAGES")
					}
				}
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeTicketShape) {
			t.Errorf("tickets everyone having SEND_MESSAGES missing code %s", CodeTicketShape)
		}
	})
}

func TestValidate_Negative_StaffVisibilityFails(t *testing.T) {
	def := valid(t)
	// Allow everyone in staff channel mod-queue.
	for i, ch := range def.Channels {
		if ch.ID == "mod-queue" {
			for j, acc := range ch.Access {
				if acc.Role == "everyone" {
					def.Channels[i].Access[j].Deny = []string{}
					def.Channels[i].Access[j].Allow = []string{"VIEW_CHANNEL"}
				}
			}
			break
		}
	}
	findings := Validate(def)
	if !hasCode(findings, CodeStaffVisibility) {
		t.Errorf("everyone having VIEW_CHANNEL in staff channel missing code %s", CodeStaffVisibility)
	}
}

func TestValidate_Negative_FirstAnswerFails(t *testing.T) {
	t.Run("first_answer channel type text", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "support" {
				def.Channels[i].Type = "text"
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeFirstAnswer) {
			t.Errorf("first_answer text channel missing code %s", CodeFirstAnswer)
		}
	})

	t.Run("tags on non-forum channel", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "lounge" {
				def.Channels[i].Tags = []Tag{{Name: "Tag", OptOut: false, Moderated: false}}
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeFirstAnswer) {
			t.Errorf("tags on text channel missing code %s", CodeFirstAnswer)
		}
	})
}

func TestValidate_Negative_VoiceClosedFails(t *testing.T) {
	def := valid(t)
	for i, ch := range def.Channels {
		if ch.ID == "events-voice" {
			def.Channels[i].Access = []Access{
				{
					Role:  "everyone",
					Allow: []string{"CONNECT"},
					Deny:  []string{},
				},
			}
			break
		}
	}
	findings := Validate(def)
	if !hasCode(findings, CodeVoiceClosed) {
		t.Errorf("voice channel with CONNECT for everyone missing code %s", CodeVoiceClosed)
	}
}

func TestValidate_Negative_RuleShapeFails(t *testing.T) {
	t.Run("value less than 1", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.Value = 0
		findings := Validate(def)
		if !hasCode(findings, CodeRuleShape) {
			t.Errorf("rule value 0 missing code %s", CodeRuleShape)
		}
	})

	t.Run("window_days 0", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.WindowDays = 0
		findings := Validate(def)
		if !hasCode(findings, CodeRuleShape) {
			t.Errorf("window_days 0 missing code %s", CodeRuleShape)
		}
	})

	t.Run("window_days 366", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.WindowDays = 366
		findings := Validate(def)
		if !hasCode(findings, CodeRuleShape) {
			t.Errorf("window_days 366 missing code %s", CodeRuleShape)
		}
	})

	t.Run("metric members-with-role subject is channel", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.Subject = "lounge"
		findings := Validate(def)
		if !hasCode(findings, CodeRuleShape) {
			t.Errorf("members-with-role with channel subject missing code %s", CodeRuleShape)
		}
	})
}

func TestValidate_Negative_AreaChannelFails(t *testing.T) {
	t.Run("threshold channel without area", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "home-lab" {
				def.Channels[i].Area = ""
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeAreaChannel) {
			t.Errorf("threshold channel without area missing code %s", CodeAreaChannel)
		}
	})

	t.Run("area channel without opens threshold", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if ch.ID == "home-lab" {
				def.Channels[i].Opens = "start"
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeAreaChannel) {
			t.Errorf("area channel with opens start missing code %s", CodeAreaChannel)
		}
	})
}

func TestValidate_Negative_AreaConditionFails(t *testing.T) {
	def := valid(t)
	for i, a := range def.Areas {
		if a.Kind == "language" {
			def.Areas[i].Conditions = []string{"   "}
			break
		}
	}
	findings := Validate(def)
	if !hasCode(findings, CodeAreaCondition) {
		t.Errorf("language area with empty condition missing code %s", CodeAreaCondition)
	}
}

func TestValidate_Negative_AutomationRecordFails(t *testing.T) {
	t.Run("built automation without record", func(t *testing.T) {
		def := valid(t)
		for i, a := range def.Automation {
			if a.ID == "ask-command" {
				def.Automation[i].Record = ""
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeAutomationRecord) {
			t.Errorf("built bot automation without record missing code %s", CodeAutomationRecord)
		}
	})

	t.Run("record path with parent traversal", func(t *testing.T) {
		def := valid(t)
		for i, a := range def.Automation {
			if a.ID == "ask-command" {
				def.Automation[i].Record = "../docs/adr/0003.md"
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeAutomationRecord) {
			t.Errorf("record path with '..' missing code %s", CodeAutomationRecord)
		}
	})

	t.Run("planned bot automation with needs_record false", func(t *testing.T) {
		def := valid(t)
		for i, a := range def.Automation {
			if a.ID == "area-lifecycle" {
				def.Automation[i].NeedsRecord = false
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeAutomationRecord) {
			t.Errorf("planned bot automation without needs_record missing code %s", CodeAutomationRecord)
		}
	})
}

func TestValidate_Boundary_IDLengths(t *testing.T) {
	// ID regex: ^[a-z][a-z0-9-]{1,31}$
	// Len 1: refused
	// Len 2: accepted
	// Len 32: accepted
	// Len 33: refused
	t.Run("id length 2 accepted", func(t *testing.T) {
		def := valid(t)
		def.Categories[0].ID = "ab"
		for i := range def.Channels {
			if def.Channels[i].Category == "start" {
				def.Channels[i].Category = "ab"
			}
		}
		findings := Validate(def)
		if hasCode(findings, CodeID) {
			t.Errorf("id length 2 refused with %s", CodeID)
		}
	})

	t.Run("id length 32 accepted", func(t *testing.T) {
		def := valid(t)
		longID := "a" + strings.Repeat("b", 31)
		def.Categories[0].ID = longID
		for i := range def.Channels {
			if def.Channels[i].Category == "start" {
				def.Channels[i].Category = longID
			}
		}
		findings := Validate(def)
		if hasCode(findings, CodeID) {
			t.Errorf("id length 32 refused with %s", CodeID)
		}
	})

	t.Run("id length 1 refused", func(t *testing.T) {
		def := valid(t)
		def.Categories[0].ID = "a"
		findings := Validate(def)
		if !hasCode(findings, CodeID) {
			t.Errorf("id length 1 accepted, want %s", CodeID)
		}
	})

	t.Run("id length 33 refused", func(t *testing.T) {
		def := valid(t)
		def.Categories[0].ID = "a" + strings.Repeat("b", 32)
		findings := Validate(def)
		if !hasCode(findings, CodeID) {
			t.Errorf("id length 33 accepted, want %s", CodeID)
		}
	})
}

func TestValidate_Boundary_TagNameLengths(t *testing.T) {
	// 20 runes accepted, 21 refused
	t.Run("tag name 20 runes accepted", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if len(ch.Tags) > 0 {
				def.Channels[i].Tags[1].Name = strings.Repeat("x", 20)
				break
			}
		}
		findings := Validate(def)
		if hasCode(findings, CodeText) {
			t.Errorf("tag name 20 runes produced %s", CodeText)
		}
	})

	t.Run("tag name 21 runes refused", func(t *testing.T) {
		def := valid(t)
		for i, ch := range def.Channels {
			if len(ch.Tags) > 0 {
				def.Channels[i].Tags[1].Name = strings.Repeat("x", 21)
				break
			}
		}
		findings := Validate(def)
		if !hasCode(findings, CodeText) {
			t.Errorf("tag name 21 runes accepted, want %s", CodeText)
		}
	})
}

func TestValidate_Boundary_WindowDaysRange(t *testing.T) {
	// 1 and 365 accepted; 0 and 366 refused
	t.Run("window_days 1 accepted", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.WindowDays = 1
		findings := Validate(def)
		if hasCode(findings, CodeRuleShape) {
			t.Errorf("window_days 1 produced %s", CodeRuleShape)
		}
	})

	t.Run("window_days 365 accepted", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.WindowDays = 365
		findings := Validate(def)
		if hasCode(findings, CodeRuleShape) {
			t.Errorf("window_days 365 produced %s", CodeRuleShape)
		}
	})

	t.Run("window_days 0 refused", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.WindowDays = 0
		findings := Validate(def)
		if !hasCode(findings, CodeRuleShape) {
			t.Errorf("window_days 0 accepted, want %s", CodeRuleShape)
		}
	})

	t.Run("window_days 366 refused", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.WindowDays = 366
		findings := Validate(def)
		if !hasCode(findings, CodeRuleShape) {
			t.Errorf("window_days 366 accepted, want %s", CodeRuleShape)
		}
	})
}

func TestValidate_Boundary_RuleValue(t *testing.T) {
	// 1 accepted, 0 refused
	t.Run("value 1 accepted", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.Value = 1
		findings := Validate(def)
		if hasCode(findings, CodeRuleShape) {
			t.Errorf("value 1 produced %s", CodeRuleShape)
		}
	})

	t.Run("value 0 refused", func(t *testing.T) {
		def := valid(t)
		def.Areas[0].Opening.Value = 0
		findings := Validate(def)
		if !hasCode(findings, CodeRuleShape) {
			t.Errorf("value 0 accepted, want %s", CodeRuleShape)
		}
	})
}

func TestValidate_Boundary_MaxFindingsCap(t *testing.T) {
	def := valid(t)
	// Generate > 200 findings by adding invalid roles.
	for i := 0; i < 250; i++ {
		def.Roles = append(def.Roles, Role{
			ID:          "INVALID_ID!",
			Kind:        "invalid_kind",
			Permissions: []string{"INVALID_PERM"},
		})
	}
	findings := Validate(def)
	if len(findings) != MaxFindings {
		t.Fatalf("len(findings) = %d, want MaxFindings (%d)", len(findings), MaxFindings)
	}
}

func TestValidate_Boundary_NilAndZeroInput(t *testing.T) {
	if findings := Validate(nil); len(findings) != 0 {
		t.Errorf("Validate(nil) = %+v, want empty", findings)
	}
	emptyDef := &Definition{}
	findings := Validate(emptyDef)
	if len(findings) == 0 {
		t.Errorf("Validate on empty struct should return findings without panic")
	}
}
