package serverdef

import "slices"

// Limits of the platform's filter rules, read in Discord's developer documentation,
// repository discord/discord-api-docs at commit 37148620d232,
// developers/resources/auto-moderation.mdx.
const (
	// MaxKeywords is the upper bound of entries in a rule's keyword list.
	MaxKeywords = 1000

	// MaxKeywordRunes is the upper bound of characters in one keyword or allow-list entry.
	MaxKeywordRunes = 60

	// MaxPatterns is the upper bound of regular expressions in one rule.
	MaxPatterns = 10

	// MaxPatternRunes is the upper bound of characters in one regular expression.
	MaxPatternRunes = 260

	// MaxAllowList is the upper bound of allow-list entries in a keyword or member profile rule.
	MaxAllowList = 100

	// MaxPresetAllowList is the upper bound of allow-list entries in a keyword preset rule.
	MaxPresetAllowList = 1000

	// MaxMentionLimit is the upper bound of mentions a mention spam rule counts in one message.
	MaxMentionLimit = 50

	// MaxTimeoutSeconds is the upper bound of a timeout action, four weeks.
	MaxTimeoutSeconds = 2419200

	// MaxBlockMessageRunes is the upper bound of characters in the text a blocked member is shown.
	MaxBlockMessageRunes = 150

	// MaxExemptRoles is the upper bound of roles one rule exempts.
	MaxExemptRoles = 20

	// MaxExemptChannels is the upper bound of channels one rule exempts.
	MaxExemptChannels = 50
)

// filterRulesPerServer holds how many rules of each trigger one server may have.
var filterRulesPerServer = map[string]int{
	"keyword":        6,
	"spam":           1,
	"keyword_preset": 1,
	"mention_spam":   1,
	"member_profile": 1,
}

// Settings holds the server's settings the tool compares and applies besides roles and channels.
type Settings struct {
	// Verification is the server's verification level. Absent = not compared and not changed.
	Verification *Verification `json:"verification,omitempty"`
	FilterRules  []FilterRule  `json:"filter_rules"`
}

// Verification states the level a member must meet before they can write on the server.
type Verification struct {
	Level     string `json:"level"`
	Basis     string `json:"basis"`
	Reasoning string `json:"reasoning"`
}

// FilterRule describes one of the platform's filter rules.
type FilterRule struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Purpose   string `json:"purpose"`
	Basis     string `json:"basis"`
	Reasoning string `json:"reasoning"`
	Trigger   string `json:"trigger"`
	Enabled   bool   `json:"enabled"`
	// Keywords and Patterns are read by keyword and member profile rules only.
	Keywords []string `json:"keywords,omitempty"`
	Patterns []string `json:"patterns,omitempty"`
	// Presets are read by a keyword preset rule only.
	Presets []string `json:"presets,omitempty"`
	// AllowList is read by keyword, keyword preset and member profile rules.
	AllowList []string `json:"allow_list,omitempty"`
	// MentionLimit and RaidProtection are read by a mention spam rule only.
	MentionLimit   int            `json:"mention_limit,omitempty"`
	RaidProtection bool           `json:"raid_protection,omitempty"`
	Actions        []FilterAction `json:"actions"`
	ExemptRoles    []string       `json:"exempt_roles,omitempty"`
	ExemptChannels []string       `json:"exempt_channels,omitempty"`
}

// FilterAction is one thing a filter rule does when it matches.
type FilterAction struct {
	Type string `json:"type"`
	// Channel is the channel an alert is written to.
	Channel string `json:"channel,omitempty"`
	// TimeoutSeconds is the length of a timeout.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// Message is the text a member is shown when their message is blocked.
	Message string `json:"message,omitempty"`
}

// VerificationLevels returns the verification levels a definition may name, lowest first.
func VerificationLevels() []string { return slices.Clone(verificationLevels) }

// FilterTriggers returns the triggers a filter rule may name.
func FilterTriggers() []string { return slices.Clone(filterTriggers) }

// FilterActionTypes returns the actions a filter rule may name.
func FilterActionTypes() []string { return slices.Clone(filterActionTypes) }
