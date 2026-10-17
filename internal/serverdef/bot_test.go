package serverdef

import (
	"strings"
	"testing"
)

func TestBotRights_Positive_CommittedSet(t *testing.T) {
	def := valid(t)
	for _, f := range Validate(def) {
		if f.Code == CodeBotRights {
			t.Errorf("unexpected bot-rights finding on committed definition: %+v", f)
		}
	}
}

func TestBotRights_Negative_LacksEachRight(t *testing.T) {
	rights := []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "MANAGE_GUILD"}
	for _, perm := range rights {
		p := perm
		t.Run("lacks "+p, func(t *testing.T) {
			def := valid(t)
			bot := roleOf(t, def, "bot")
			bot.Permissions = without(bot.Permissions, p)
			findings := Validate(def)
			if !hasFinding(findings, CodeBotRights, "roles[bot].permissions", "lacks "+p) {
				t.Errorf("expected finding bot-rights at roles[bot].permissions with 'lacks %s', got %+v", p, findings)
			}
		})
	}
}

func TestBotRights_Negative_RightBeyondTheSet(t *testing.T) {
	beyond := []string{
		"ADMINISTRATOR", "MANAGE_CHANNELS", "MANAGE_WEBHOOKS",
		"KICK_MEMBERS", "BAN_MEMBERS", "MANAGE_THREADS", "VIEW_AUDIT_LOG",
		"MENTION_EVERYONE", "ADD_REACTIONS",
	}
	for _, perm := range beyond {
		p := perm
		t.Run("bot holds "+p, func(t *testing.T) {
			def := valid(t)
			bot := roleOf(t, def, "bot")
			bot.Permissions = append(bot.Permissions, p)
			findings := Validate(def)
			if !hasFinding(findings, CodeBotRights, "roles[bot].permissions", "beyond") {
				t.Errorf("expected finding bot-rights at roles[bot].permissions with 'beyond' for %s, got %+v", p, findings)
			}
			if !hasFinding(findings, CodeBotRights, "roles[bot].permissions", p) {
				t.Errorf("expected finding bot-rights at roles[bot].permissions with %q for %s, got %+v", p, p, findings)
			}
		})
	}
}

func TestBotRights_Boundary_EmptyPermissions(t *testing.T) {
	def := valid(t)
	bot := roleOf(t, def, "bot")
	bot.Permissions = []string{}
	var botFindings []Finding
	for _, f := range Validate(def) {
		if f.Code == CodeBotRights && f.Path == "roles[bot].permissions" {
			botFindings = append(botFindings, f)
		}
	}
	if len(botFindings) != 4 {
		t.Fatalf("expected exactly 4 bot-rights findings at roles[bot].permissions, got %d: %+v", len(botFindings), botFindings)
	}
}

func TestBotRights_Negative_EntryBeyondTheChannelKind(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"SEND_MESSAGES on area channel home-lab", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "home-lab"), "bot")
			acc.Allow = append(acc.Allow, "SEND_MESSAGES")
		}, CodeBotRights, "channels[home-lab].access[bot]", "beyond"},
		// ADR-0019, point 4: the bot crossposts only its own posts, so MANAGE_MESSAGES is not given.
		{"MANAGE_MESSAGES added to announcements", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "announcements"), "bot")
			acc.Allow = append(acc.Allow, "MANAGE_MESSAGES")
		}, CodeBotRights, "channels[announcements].access[bot]", "beyond"},
		{"new bot entry on lounge allowing VIEW_CHANNEL", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "lounge"), "bot")
			acc.Allow = []string{"VIEW_CHANNEL"}
		}, CodeBotRights, "channels[lounge].access[bot]", "beyond"},
		{"SEND_MESSAGES added to support", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "support"), "bot")
			acc.Allow = append(acc.Allow, "SEND_MESSAGES")
		}, CodeBotRights, "channels[support].access[bot]", "SEND_MESSAGES"},
		{"MANAGE_MESSAGES added to support", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "support"), "bot")
			acc.Allow = append(acc.Allow, "MANAGE_MESSAGES")
		}, CodeBotRights, "channels[support].access[bot]", "beyond"},
		{"READ_MESSAGE_HISTORY added to mod-queue", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "mod-queue"), "bot")
			acc.Allow = append(acc.Allow, "READ_MESSAGE_HISTORY")
		}, CodeBotRights, "channels[mod-queue].access[bot]", "beyond"},
	})
}

func TestBotRights_Boundary_EntryAtTheCeiling(t *testing.T) {
	t.Run("committed entries = 0 bot-rights findings", func(t *testing.T) {
		def := valid(t)
		for _, f := range Validate(def) {
			if f.Code == CodeBotRights {
				t.Errorf("unexpected bot-rights finding on committed: %+v", f)
			}
		}
	})

	t.Run("bot entry with empty allow on lounge = 0 bot-rights", func(t *testing.T) {
		def := valid(t)
		acc := accessOf(channelOf(t, def, "lounge"), "bot")
		acc.Allow = []string{}
		acc.Deny = []string{}
		for _, f := range Validate(def) {
			if f.Code == CodeBotRights {
				t.Errorf("unexpected bot-rights finding with empty allow: %+v", f)
			}
		}
	})

	t.Run("deny-only bot entry on lounge = 0 bot-rights", func(t *testing.T) {
		def := valid(t)
		acc := accessOf(channelOf(t, def, "lounge"), "bot")
		acc.Allow = []string{}
		acc.Deny = []string{"VIEW_CHANNEL"}
		for _, f := range Validate(def) {
			if f.Code == CodeBotRights {
				t.Errorf("unexpected bot-rights finding with deny-only: %+v", f)
			}
		}
	})
}

func TestBotRights_Negative_StaffChannelConditions(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"mod-log VIEW_CHANNEL dropped from allow", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "mod-log"), "bot")
			acc.Allow = without(acc.Allow, "VIEW_CHANNEL")
		}, CodeBotRights, "channels[mod-log].access[bot]", "lacks VIEW_CHANNEL"},
		{"mod-queue SEND_MESSAGES moved from allow to deny", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "mod-queue"), "bot")
			acc.Allow = without(acc.Allow, "SEND_MESSAGES")
			acc.Deny = append(acc.Deny, "SEND_MESSAGES")
		}, CodeBotRights, "channels[mod-queue].access[bot]", "lacks SEND_MESSAGES"},
	})
}

func TestBotRights_Negative_BotSeesTheTicketChannel(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"bot entry of tickets removed", func(_ *testing.T, def *Definition) {
			ch := channelOf(t, def, "tickets")
			kept := make([]Access, 0, len(ch.Access))
			for _, acc := range ch.Access {
				if acc.Role != "bot" {
					kept = append(kept, acc)
				}
			}
			ch.Access = kept
		}, CodeBotRights, "channels[tickets].access[bot]", "ticket"},
	})
}

// verifyBotPassesCommitted verifies that committed definition has no operator-only or moderation-only finding for bot.
func verifyBotPassesCommitted(t *testing.T) {
	t.Helper()
	def := valid(t)
	for _, f := range Validate(def) {
		if strings.Contains(f.Path, "bot") && (f.Code == CodeOperatorOnly || f.Code == CodeModerationOnly) {
			t.Errorf("committed definition produced finding %s for bot: %+v", f.Code, f)
		}
	}
}

// verifyBotManageChannels verifies findings when bot has MANAGE_CHANNELS.
func verifyBotManageChannels(t *testing.T) {
	t.Helper()
	def := valid(t)
	bot := roleOf(t, def, "bot")
	bot.Permissions = append(bot.Permissions, "MANAGE_CHANNELS")
	findings := Validate(def)
	foundChannels := false
	for _, f := range findings {
		if f.Code == CodeOperatorOnly && strings.Contains(f.Path, "bot") {
			if strings.Contains(f.Message, "MANAGE_CHANNELS") {
				foundChannels = true
			}
			if strings.Contains(f.Message, "MANAGE_ROLES") {
				t.Errorf("unexpected finding naming MANAGE_ROLES: %+v", f)
			}
			if strings.Contains(f.Message, "MANAGE_GUILD") {
				t.Errorf("unexpected finding naming MANAGE_GUILD: %+v", f)
			}
		}
	}
	if !foundChannels {
		t.Errorf("missing operator-only finding naming MANAGE_CHANNELS for bot: %+v", findings)
	}
}

// verifyBotManageThreads verifies findings when bot has MANAGE_THREADS.
func verifyBotManageThreads(t *testing.T) {
	t.Helper()
	def := valid(t)
	bot := roleOf(t, def, "bot")
	bot.Permissions = append(bot.Permissions, "MANAGE_THREADS")
	findings := Validate(def)
	foundThreads := false
	for _, f := range findings {
		if f.Code == CodeModerationOnly && strings.Contains(f.Path, "bot") {
			if strings.Contains(f.Message, "MANAGE_THREADS") {
				foundThreads = true
			}
			if strings.Contains(f.Message, "MANAGE_MESSAGES") {
				t.Errorf("unexpected finding naming MANAGE_MESSAGES: %+v", f)
			}
			if strings.Contains(f.Message, "MODERATE_MEMBERS") {
				t.Errorf("unexpected finding naming MODERATE_MEMBERS: %+v", f)
			}
		}
	}
	if !foundThreads {
		t.Errorf("missing moderation-only finding naming MANAGE_THREADS for bot: %+v", findings)
	}
}

// verifyModeratorManageRoles verifies operator-only finding when moderator has MANAGE_ROLES.
func verifyModeratorManageRoles(t *testing.T) {
	t.Helper()
	def := valid(t)
	mod := roleOf(t, def, "moderator")
	mod.Permissions = append(mod.Permissions, "MANAGE_ROLES")
	findings := Validate(def)
	if !hasFinding(findings, CodeOperatorOnly, "channels[lounge].access[moderator]", "MANAGE_ROLES") {
		t.Errorf("missing operator-only finding for moderator with MANAGE_ROLES: %+v", findings)
	}
}

// verifyContributorManageMessages verifies moderation-only finding when contributor has MANAGE_MESSAGES.
func verifyContributorManageMessages(t *testing.T) {
	t.Helper()
	def := valid(t)
	contrib := roleOf(t, def, "contributor")
	contrib.Permissions = append(contrib.Permissions, "MANAGE_MESSAGES")
	findings := Validate(def)
	if !hasFinding(findings, CodeModerationOnly, "channels[lounge].access[contributor]", "MANAGE_MESSAGES") {
		t.Errorf("missing moderation-only finding for contributor with MANAGE_MESSAGES: %+v", findings)
	}
}

// verifyGuideCodingModerateMembers verifies moderation-only finding when guide-coding has MODERATE_MEMBERS.
func verifyGuideCodingModerateMembers(t *testing.T) {
	t.Helper()
	def := valid(t)
	guide := roleOf(t, def, "guide-coding")
	guide.Permissions = append(guide.Permissions, "MODERATE_MEMBERS")
	findings := Validate(def)
	if !hasFinding(findings, CodeModerationOnly, "channels[lounge].access[guide-coding]", "MODERATE_MEMBERS") {
		t.Errorf("missing moderation-only finding for guide-coding with MODERATE_MEMBERS: %+v", findings)
	}
}

func TestValidate_Boundary_BotPassesForItsFourRightsOnly(t *testing.T) {
	t.Run("committed definition has no operator-only or moderation-only finding for bot", verifyBotPassesCommitted)
	t.Run("bot + MANAGE_CHANNELS produces operator-only naming MANAGE_CHANNELS and none naming MANAGE_ROLES or MANAGE_GUILD", verifyBotManageChannels)
	t.Run("bot + MANAGE_THREADS produces moderation-only naming MANAGE_THREADS and none naming MANAGE_MESSAGES or MODERATE_MEMBERS", verifyBotManageThreads)
	t.Run("moderator + MANAGE_ROLES produces operator-only", verifyModeratorManageRoles)
	t.Run("contributor + MANAGE_MESSAGES produces moderation-only", verifyContributorManageMessages)
	t.Run("guide-coding + MODERATE_MEMBERS produces moderation-only", verifyGuideCodingModerateMembers)
}

func TestValidate_Negative_AreaChannelBotCannotSee(t *testing.T) {
	removeBot := func(channelID string) func(t *testing.T, def *Definition) {
		return func(t *testing.T, def *Definition) {
			ch := channelOf(t, def, channelID)
			kept := make([]Access, 0, len(ch.Access))
			for _, acc := range ch.Access {
				if acc.Role != "bot" {
					kept = append(kept, acc)
				}
			}
			ch.Access = kept
		}
	}
	runPlanted(t, []plantedCase{
		{"bot entry removed from home-lab", removeBot("home-lab"),
			CodeAreaChannel, "channels[home-lab].access[bot]", "bot"},
		{"bot entry removed from room-de", removeBot("room-de"),
			CodeAreaChannel, "channels[room-de].access[bot]", "bot"},
	})
}
