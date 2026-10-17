package serverdef

import "testing"

// The trail channel holds the lines of the moderation commands (ADR-0021). The bot lists its own
// lines there to delete them after the retention, so it has to read the history of that channel.
// It does through the server permissions of @everyone; the check keeps it that way.

func TestTrail_Positive_ModLogIsTheTrailAndTheBotReadsIt(t *testing.T) {
	def := valid(t)
	if !channelOf(t, def, "mod-log").Trail {
		t.Fatal("mod-log is not marked as the trail channel")
	}
	eff := Effective(def, "bot", "mod-log")
	for _, perm := range []string{"VIEW_CHANNEL", "SEND_MESSAGES", "READ_MESSAGE_HISTORY"} {
		if _, ok := eff[perm]; !ok {
			t.Errorf("the bot lacks %s in mod-log", perm)
		}
	}
	for _, f := range Validate(def) {
		if f.Code == CodeTrail {
			t.Errorf("unexpected trail finding on the committed definition: %+v", f)
		}
	}
}

func TestTrail_Negative_ShapeFails(t *testing.T) {
	runPlanted(t, []plantedCase{
		{"second trail channel, reported on the first", func(_ *testing.T, def *Definition) {
			channelOf(t, def, "mod-queue").Trail = true
		}, CodeTrail, "channels[mod-log].trail", "more than one trail channel"},
		{"second trail channel, reported on the second", func(_ *testing.T, def *Definition) {
			channelOf(t, def, "mod-queue").Trail = true
		}, CodeTrail, "channels[mod-queue].trail", "more than one trail channel"},
		{"trail on a channel that is not staff", func(_ *testing.T, def *Definition) {
			channelOf(t, def, "mod-log").Staff = false
		}, CodeTrail, "channels[mod-log].trail", "staff"},
		{"bot entry denies history in the trail", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "mod-log"), "bot")
			acc.Deny = append(acc.Deny, "READ_MESSAGE_HISTORY")
		}, CodeTrail, "channels[mod-log].access[bot]", "READ_MESSAGE_HISTORY"},
		{"@everyone loses history on the server", func(_ *testing.T, def *Definition) {
			everyone := roleOf(t, def, "everyone")
			everyone.Permissions = without(everyone.Permissions, "READ_MESSAGE_HISTORY")
		}, CodeTrail, "channels[mod-log].access[bot]", "READ_MESSAGE_HISTORY"},
		{"bot cannot see the trail", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "mod-log"), "bot")
			acc.Allow = without(acc.Allow, "VIEW_CHANNEL")
		}, CodeTrail, "channels[mod-log].access[bot]", "READ_MESSAGE_HISTORY"},
		{"READ_MESSAGE_HISTORY allowed in the trail stays beyond the staff ceiling", func(_ *testing.T, def *Definition) {
			acc := accessOf(channelOf(t, def, "mod-log"), "bot")
			acc.Allow = append(acc.Allow, "READ_MESSAGE_HISTORY")
		}, CodeBotRights, "channels[mod-log].access[bot]", "beyond"},
	})
}

func TestTrail_Boundary_NoTrailChannel(t *testing.T) {
	def := valid(t)
	channelOf(t, def, "mod-log").Trail = false
	for _, f := range Validate(def) {
		if f.Code == CodeTrail {
			t.Errorf("a definition without a trail channel is valid, got %+v", f)
		}
	}
}

func TestTrail_Boundary_HistoryGivenBackByTheBotEntry(t *testing.T) {
	def := valid(t)
	everyone := roleOf(t, def, "everyone")
	everyone.Permissions = without(everyone.Permissions, "READ_MESSAGE_HISTORY")
	ch := channelOf(t, def, "mod-log")
	ch.Staff = true
	acc := accessOf(ch, "bot")
	acc.Allow = append(acc.Allow, "READ_MESSAGE_HISTORY")
	for _, f := range Validate(def) {
		if f.Code == CodeTrail {
			t.Errorf("history allowed in the bot's entry is enough for the trail, got %+v", f)
		}
	}
}
