// Package serverdef loads and validates declarative server definitions.
package serverdef

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	// MaxBytes is the upper bound on the number of bytes read from a definition reader.
	MaxBytes = 1 << 20

	// MaxDepth is the upper bound on nested JSON container depth.
	MaxDepth = 16

	// MaxFindings is the maximum number of validation findings returned.
	MaxFindings = 200

	// MaxChannelNameRunes is the maximum length of a channel name in runes.
	MaxChannelNameRunes = 100

	// MaxTagNameRunes is the maximum length of a forum tag name in runes.
	MaxTagNameRunes = 20

	// MaxForumTags is the maximum number of tags allowed in a forum channel.
	MaxForumTags = 20

	// MaxWindowDays is the upper bound of a rule evaluation window in days.
	MaxWindowDays = 365

	// MinWindowDays is the lower bound of a rule evaluation window in days.
	MinWindowDays = 1

	// MinRuleValue is the lower bound of a rule target value.
	MinRuleValue = 1
)

const (
	// CodeID flags an identifier that violates the naming regex or is duplicate.
	CodeID = "id"

	// CodeText flags a required text field that is empty after trimming or out of length bounds.
	CodeText = "text"

	// CodeEnum flags a value not present in the allowed enumerated set.
	CodeEnum = "enum"

	// CodeVersion flags an unsupported schema version or an applied flag set to true.
	CodeVersion = "version"

	// CodeSingleton flags a role kind that must have exactly one instance but does not.
	CodeSingleton = "singleton"

	// CodeRef flags an identifier reference that does not resolve to an existing entity.
	CodeRef = "ref"

	// CodeChannelOwner flags a channel whose owner is missing, unresolved, or of invalid kind.
	CodeChannelOwner = "channel-owner"

	// CodePermissionName flags a permission not present in the standard vocabulary.
	CodePermissionName = "permission-name"

	// CodeAccessShape flags duplicate roles, duplicate permissions, or overlapping allow and deny.
	CodeAccessShape = "access-shape"

	// CodeModeratorVisibility flags a channel that a moderator role cannot see.
	CodeModeratorVisibility = "moderator-visibility"

	// CodeMentionEveryone flags unauthorized permission to mention everyone.
	CodeMentionEveryone = "mention-everyone"

	// CodeOperatorOnly flags an operator-only permission effective for another role.
	CodeOperatorOnly = "operator-only"

	// CodeModerationOnly flags a moderation-only permission effective for an unauthorized role.
	CodeModerationOnly = "moderation-only"

	// CodeNothingAttaches flags permissions attached to roles that must remain empty.
	CodeNothingAttaches = "nothing-attaches"

	// CodeRoleGrant flags an invalid role assignment mechanism or unauthorized onboarding grant.
	CodeRoleGrant = "role-grant"

	// CodeTicketThreadReader flags a non-moderator role that can read private ticket threads.
	CodeTicketThreadReader = "ticket-thread-reader"

	// CodeTicketShape flags a private threads channel that deviates from expected ticket settings.
	CodeTicketShape = "ticket-shape"

	// CodeStaffVisibility flags visibility of staff channels granted to unauthorized roles.
	CodeStaffVisibility = "staff-visibility"

	// CodeFirstAnswer flags a first_answer channel deviating from forum and bot requirements.
	CodeFirstAnswer = "first-answer"

	// CodeVoiceClosed flags voice channels where @everyone can connect or use voice activity.
	CodeVoiceClosed = "voice-closed"

	// CodeAreaOpeningRule flags an area missing an opening rule.
	CodeAreaOpeningRule = "area-opening-rule"

	// CodeAreaClosingRule flags an area missing a closing rule.
	CodeAreaClosingRule = "area-closing-rule"

	// CodeRuleShape flags a metric rule with invalid values, invalid windows, or invalid subjects.
	CodeRuleShape = "rule-shape"

	// CodeAreaChannel flags channel-to-area linkage and permission inconsistencies.
	CodeAreaChannel = "area-channel"

	// CodeAreaCondition flags a language area lacking prerequisites or operational conditions.
	CodeAreaCondition = "area-condition"

	// CodeAutomationRecord flags an automation entry with an invalid or missing record path.
	CodeAutomationRecord = "automation-record"

	// CodeBotRights flags a bot role or bot channel entry violating the bot rights specification.
	CodeBotRights = "bot-rights"

	// CodeRoleOrder flags a role order definition that is incomplete, duplicated, or misordered.
	CodeRoleOrder = "role-order"
)

// Finding represents one declarative rule infraction in a server definition.
type Finding struct {
	Code    string
	Path    string
	Message string
}

// Definition represents the declarative target state of the community server.
type Definition struct {
	Version int      `json:"version"`
	Applied bool     `json:"applied"`
	Summary string   `json:"summary"`
	Metrics []Metric `json:"metrics"`
	Roles   []Role   `json:"roles"`
	// RoleOrder lists the roles from highest to lowest, without the role every member holds.
	RoleOrder  []string     `json:"role_order"`
	Categories []Category   `json:"categories"`
	Channels   []Channel    `json:"channels"`
	Areas      []Area       `json:"areas"`
	Onboarding Onboarding   `json:"onboarding"`
	Automation []Automation `json:"automation"`
}

// Metric describes a trackable quantitative measurement on the server.
type Metric struct {
	ID       string `json:"id"`
	Measures string `json:"measures"`
	Source   string `json:"source"`
	Verified bool   `json:"verified"`
}

// Role defines an identity, its kind, assignment origin, and default permissions.
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Purpose     string   `json:"purpose"`
	AssignedBy  string   `json:"assigned_by"`
	Basis       string   `json:"basis"`
	Permissions []string `json:"permissions"`
}

// Category defines an organizational grouping of channels.
type Category struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}

// Channel defines a communication venue, its type, lifecycle, and access rules.
type Channel struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Type           string   `json:"type"`
	Purpose        string   `json:"purpose"`
	Owner          string   `json:"owner"`
	Opens          string   `json:"opens"`
	Access         []Access `json:"access"`
	FirstAnswer    bool     `json:"first_answer,omitempty"`
	PrivateThreads bool     `json:"private_threads,omitempty"`
	Staff          bool     `json:"staff,omitempty"`
	Tags           []Tag    `json:"tags,omitempty"`
	Area           string   `json:"area,omitempty"`
}

// Tag defines a selectable tag within a forum channel.
type Tag struct {
	Name      string `json:"name"`
	OptOut    bool   `json:"opt_out"`
	Moderated bool   `json:"moderated"`
}

// Access specifies permission overwrites for a role in a channel.
type Access struct {
	Role  string   `json:"role"`
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

// Area defines a conditionally opened and closed topic or language sector.
type Area struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Role       string   `json:"role"`
	OnClose    string   `json:"on_close"`
	Conditions []string `json:"conditions"`
	Opening    *Rule    `json:"opening"`
	Closing    *Rule    `json:"closing"`
}

// Rule defines an automated threshold condition for area lifecycle transitions.
type Rule struct {
	Metric     string `json:"metric"`
	Subject    string `json:"subject"`
	Comparison string `json:"comparison"`
	Value      int    `json:"value"`
	WindowDays int    `json:"window_days"`
	Basis      string `json:"basis"`
	Reasoning  string `json:"reasoning"`
}

// Onboarding defines welcome questionnaire metadata and choices.
type Onboarding struct {
	By        string     `json:"by"`
	Questions []Question `json:"questions"`
}

// Question defines one questionnaire item presented to joining members.
type Question struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Required bool     `json:"required"`
	Multiple bool     `json:"multiple"`
	Options  []Option `json:"options"`
}

// Option defines a selectable questionnaire answer granting a specific role.
type Option struct {
	Title string `json:"title"`
	Role  string `json:"role"`
}

// Automation describes a programmatic task, its owner, state, and audit record.
type Automation struct {
	ID          string `json:"id"`
	What        string `json:"what"`
	By          string `json:"by"`
	State       string `json:"state"`
	Record      string `json:"record,omitempty"`
	NeedsRecord bool   `json:"needs_record,omitempty"`
}

var botRights = [...]string{
	"MANAGE_ROLES",
	"MANAGE_MESSAGES",
	"MODERATE_MEMBERS",
}

var vocabulary = [...]string{
	"VIEW_CHANNEL",
	"SEND_MESSAGES",
	"SEND_MESSAGES_IN_THREADS",
	"CREATE_PUBLIC_THREADS",
	"CREATE_PRIVATE_THREADS",
	"READ_MESSAGE_HISTORY",
	"ADD_REACTIONS",
	"EMBED_LINKS",
	"ATTACH_FILES",
	"USE_APPLICATION_COMMANDS",
	"MENTION_EVERYONE",
	"MANAGE_MESSAGES",
	"MANAGE_THREADS",
	"MODERATE_MEMBERS",
	"MANAGE_CHANNELS",
	"MANAGE_ROLES",
	"MANAGE_GUILD",
	"MANAGE_WEBHOOKS",
	"KICK_MEMBERS",
	"BAN_MEMBERS",
	"ADMINISTRATOR",
	"VIEW_AUDIT_LOG",
	"CONNECT",
	"SPEAK",
	"USE_VAD",
	"MUTE_MEMBERS",
	"MOVE_MEMBERS",
	"CREATE_EVENTS",
	"MANAGE_EVENTS",
	"SEND_POLLS",
}

// Permissions returns a fresh slice containing the fixed vocabulary of Discord permissions.
func Permissions() []string {
	res := make([]string, len(vocabulary))
	copy(res, vocabulary[:])
	return res
}

type jsonFrame struct {
	isObject  bool
	expectKey bool
	seenKeys  map[string]struct{}
}

func pushFrame(stack *[]jsonFrame, isObject bool) error {
	if len(*stack) >= MaxDepth {
		return fmt.Errorf("json depth exceeds maximum depth of %d", MaxDepth)
	}
	frame := jsonFrame{
		isObject:  isObject,
		expectKey: isObject,
	}
	if isObject {
		frame.seenKeys = make(map[string]struct{})
	}
	*stack = append(*stack, frame)
	return nil
}

func popFrame(stack *[]jsonFrame, isObject bool, rootClosed *bool) error {
	s := *stack
	if len(s) == 0 || s[len(s)-1].isObject != isObject {
		return errors.New("mismatched closing delimiter in JSON")
	}
	*stack = s[:len(s)-1]
	if len(*stack) == 0 {
		*rootClosed = true
		return nil
	}
	parent := &(*stack)[len(*stack)-1]
	if parent.isObject {
		parent.expectKey = true
	}
	return nil
}

func processDelim(d json.Delim, stack *[]jsonFrame, rootClosed *bool) error {
	switch d {
	case '{':
		return pushFrame(stack, true)
	case '[':
		return pushFrame(stack, false)
	case '}':
		return popFrame(stack, true, rootClosed)
	case ']':
		return popFrame(stack, false, rootClosed)
	default:
		return fmt.Errorf("unexpected delimiter %q", d)
	}
}

func processStringToken(val string, stack *[]jsonFrame, rootClosed *bool) error {
	s := *stack
	if len(s) == 0 {
		*rootClosed = true
		return nil
	}
	top := &s[len(s)-1]
	if top.isObject {
		if top.expectKey {
			if _, ok := top.seenKeys[val]; ok {
				return fmt.Errorf("duplicate key %q in JSON object", val)
			}
			top.seenKeys[val] = struct{}{}
			top.expectKey = false
			return nil
		}
		top.expectKey = true
	}
	return nil
}

func processScalarToken(stack *[]jsonFrame, rootClosed *bool) error {
	s := *stack
	if len(s) == 0 {
		*rootClosed = true
		return nil
	}
	top := &s[len(s)-1]
	if top.isObject {
		top.expectKey = true
	}
	return nil
}

func processToken(tok json.Token, stack *[]jsonFrame, rootClosed *bool) error {
	switch v := tok.(type) {
	case json.Delim:
		return processDelim(v, stack, rootClosed)
	case string:
		return processStringToken(v, stack, rootClosed)
	default:
		return processScalarToken(stack, rootClosed)
	}
}

func checkTokenEOF(stackLen int, rootClosed bool) error {
	if stackLen > 0 || !rootClosed {
		return errors.New("empty or incomplete JSON input")
	}
	return nil
}

func checkTokenError(err error, rootClosed bool) error {
	if rootClosed {
		return errors.New("trailing data after JSON value")
	}
	return fmt.Errorf("read JSON token: %w", err)
}

func checkJSONTokens(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	stack := make([]jsonFrame, 0, MaxDepth+1)
	rootClosed := false

	const maxSteps = 1 << 20
	for step := 0; step < maxSteps; step++ {
		tok, err := dec.Token()
		if err == io.EOF {
			return checkTokenEOF(len(stack), rootClosed)
		}
		if err != nil {
			return checkTokenError(err, rootClosed)
		}
		if rootClosed {
			return errors.New("trailing data after JSON value")
		}

		if procErr := processToken(tok, &stack, &rootClosed); procErr != nil {
			return procErr
		}
	}
	return errors.New("JSON token stream exceeded limit")
}

// DecodeStrict reads and parses JSON from r into target using strict JSON decoding.
func DecodeStrict(r io.Reader, what string, into any) error {
	limited := io.LimitReader(r, int64(MaxBytes+1))
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read %s: %w", what, err)
	}
	if len(data) > MaxBytes {
		return fmt.Errorf("%s size %d exceeds limit of %d bytes", what, len(data), MaxBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("empty JSON input")
	}

	if err := checkJSONTokens(data); err != nil {
		return fmt.Errorf("validate JSON structure: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("decode %s: %w", what, err)
	}

	return nil
}

// Load reads and parses a server definition from r using strict JSON decoding.
func Load(r io.Reader) (*Definition, error) {
	var def Definition
	if err := DecodeStrict(r, "definition", &def); err != nil {
		return nil, err
	}
	return &def, nil
}
