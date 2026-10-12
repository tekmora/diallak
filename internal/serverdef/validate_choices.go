package serverdef

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// limits of a Discord select menu
const (
	maxChoiceOptions    = 25
	maxChoiceTitleRunes = 100
)

func (v *validator) checkChoiceEnums() {
	if !inSet(v.def.Choices.By, []string{"bot"}) {
		v.add(CodeEnum, "choices.by", fmt.Sprintf("invalid choices by %q", v.def.Choices.By))
	}
}

func (v *validator) checkChoiceTexts() {
	for _, q := range v.def.Choices.Questions {
		p := fmt.Sprintf("choices.questions[%s]", q.ID)
		if strings.TrimSpace(q.Title) == "" {
			v.add(CodeText, p+".title", "question title must not be empty")
		}
		for _, opt := range q.Options {
			if strings.TrimSpace(opt.Title) == "" {
				v.add(CodeText, fmt.Sprintf("%s.options[%s].title", p, opt.Role),
					"option title must not be empty")
			}
		}
	}
}

func (v *validator) checkChoiceRefs(roleMap map[string]Role) {
	for _, q := range v.def.Choices.Questions {
		p := fmt.Sprintf("choices.questions[%s]", q.ID)
		for _, opt := range q.Options {
			if _, ok := roleMap[opt.Role]; !ok {
				v.add(CodeRef, fmt.Sprintf("%s.options[%s]", p, opt.Role),
					fmt.Sprintf("unknown option role %q", opt.Role))
			}
		}
	}
}

func (v *validator) checkChoiceGrantRoles() {
	roleMap := make(map[string]Role, len(v.def.Roles))
	for _, r := range v.def.Roles {
		roleMap[r.ID] = r
	}
	for _, q := range v.def.Choices.Questions {
		p := fmt.Sprintf("choices.questions[%s]", q.ID)
		for _, opt := range q.Options {
			r, ok := roleMap[opt.Role]
			if !ok {
				continue
			}
			if r.Kind == "managed" {
				v.add(CodeRoleGrant, fmt.Sprintf("%s.options[%s]", p, opt.Role),
					fmt.Sprintf("role %q is managed", opt.Role))
				continue
			}
			if r.Kind != "interest" && r.Kind != "language" && r.Kind != "notification" {
				v.add(CodeRoleGrant, fmt.Sprintf("%s.options[%s]", p, opt.Role),
					"choice option grants role that is neither interest nor language nor notification")
			}
		}
	}
	v.checkChoiceRoleCoverage()
}

func (v *validator) checkChoiceRoleCoverage() {
	counts := make(map[string]int, len(v.def.Roles))
	for _, q := range v.def.Choices.Questions {
		for _, opt := range q.Options {
			counts[opt.Role]++
		}
	}
	for _, r := range v.def.Roles {
		if r.Kind == "interest" || r.Kind == "language" || r.Kind == "notification" {
			c := counts[r.ID]
			if c != 1 {
				v.add(CodeRoleGrant, fmt.Sprintf("roles[%s]", r.ID),
					fmt.Sprintf("role %q of kind %s is the option of %d choices; it must be the option of exactly one",
						r.ID, r.Kind, c))
			}
		}
	}
}

func (v *validator) checkChoiceLimits() {
	for _, q := range v.def.Choices.Questions {
		p := fmt.Sprintf("choices.questions[%s]", q.ID)
		if len(q.Options) > maxChoiceOptions {
			v.add(CodeChoiceLimit, p+".options",
				fmt.Sprintf("question holds %d options; a menu holds at most %d", len(q.Options), maxChoiceOptions))
		}
		for _, opt := range q.Options {
			runes := utf8.RuneCountInString(opt.Title)
			if runes > maxChoiceTitleRunes {
				v.add(CodeChoiceLimit, fmt.Sprintf("%s.options[%s].title", p, opt.Role),
					fmt.Sprintf("option title holds %d characters; a menu option holds at most %d",
						runes, maxChoiceTitleRunes))
			}
		}
	}
}
