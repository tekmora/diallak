package serverdef

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValidate_ChoicesBy(t *testing.T) {
	t.Run("bot is accepted", func(t *testing.T) {
		def := valid(t)
		def.Choices.By = "bot"
		findings := Validate(def)
		if hasFinding(findings, CodeEnum, "choices.by", "") {
			t.Errorf("choices.by = 'bot' produced CodeEnum: %+v", findings)
		}
	})

	t.Run("platform is refused", func(t *testing.T) {
		def := valid(t)
		def.Choices.By = "platform"
		findings := Validate(def)
		if !hasFinding(findings, CodeEnum, "choices.by", "platform") {
			t.Errorf("choices.by = 'platform' missing CodeEnum at choices.by: %+v", findings)
		}
	})

	t.Run("empty is refused", func(t *testing.T) {
		def := valid(t)
		def.Choices.By = ""
		findings := Validate(def)
		if !hasFinding(findings, CodeEnum, "choices.by", "") {
			t.Errorf("choices.by = '' missing CodeEnum at choices.by: %+v", findings)
		}
	})
}

// verifyExactlyOneValid verifies that a valid definition has no exactly-one findings.
func verifyExactlyOneValid(t *testing.T) {
	t.Helper()
	def := valid(t)
	for _, f := range Validate(def) {
		if f.Code == CodeRoleGrant && strings.Contains(f.Message, "is the option of") {
			t.Errorf("unexpected exactly-one finding on valid definition: %+v", f)
		}
	}
}

// verifyExactlyOneOptionRemoved verifies 0 choices finding when an option is removed.
func verifyExactlyOneOptionRemoved(t *testing.T) {
	t.Helper()
	def := valid(t)
	opts := make([]Option, 0, len(def.Choices.Questions[0].Options))
	for _, opt := range def.Choices.Questions[0].Options {
		if opt.Role != "interest-games" {
			opts = append(opts, opt)
		}
	}
	def.Choices.Questions[0].Options = opts
	findings := Validate(def)
	if !hasFinding(findings, CodeRoleGrant, "roles[interest-games]", "0 choices") {
		t.Errorf("missing 0 choices finding for interest-games: %+v", findings)
	}
}

// verifyExactlyOneOptionAddedLanguages verifies 2 choices finding when option is added to another question.
func verifyExactlyOneOptionAddedLanguages(t *testing.T) {
	t.Helper()
	def := valid(t)
	def.Choices.Questions[1].Options = append(def.Choices.Questions[1].Options, Option{
		Title: "Games in language",
		Role:  "interest-games",
	})
	findings := Validate(def)
	if !hasFinding(findings, CodeRoleGrant, "roles[interest-games]", "2 choices") {
		t.Errorf("missing 2 choices finding for interest-games in two questions: %+v", findings)
	}
}

// verifyExactlyOneDuplicateOption verifies 2 choices finding when same option is added twice in one question.
func verifyExactlyOneDuplicateOption(t *testing.T) {
	t.Helper()
	def := valid(t)
	def.Choices.Questions[0].Options = append(def.Choices.Questions[0].Options, Option{
		Title: "Games twice",
		Role:  "interest-games",
	})
	findings := Validate(def)
	if !hasFinding(findings, CodeRoleGrant, "roles[interest-games]", "2 choices") {
		t.Errorf("missing 2 choices finding for interest-games duplicate: %+v", findings)
	}
}

// verifyExactlyOneLanguageRemoved verifies 0 choices finding when language option is removed.
func verifyExactlyOneLanguageRemoved(t *testing.T) {
	t.Helper()
	def := valid(t)
	opts := make([]Option, 0, len(def.Choices.Questions[1].Options))
	for _, opt := range def.Choices.Questions[1].Options {
		if opt.Role != "lang-es" {
			opts = append(opts, opt)
		}
	}
	def.Choices.Questions[1].Options = opts
	findings := Validate(def)
	if !hasFinding(findings, CodeRoleGrant, "roles[lang-es]", "0 choices") {
		t.Errorf("missing 0 choices finding for lang-es: %+v", findings)
	}
}

// verifyExactlyOneRolesQuestionsRemoved verifies no finding when interest/language roles and questions are removed.
func verifyExactlyOneRolesQuestionsRemoved(t *testing.T) {
	t.Helper()
	def := valid(t)
	def.Choices.Questions = nil
	keptRoles := make([]Role, 0, len(def.Roles))
	for _, r := range def.Roles {
		if r.Kind != "interest" && r.Kind != "language" && r.Kind != "notification" {
			keptRoles = append(keptRoles, r)
		}
	}
	def.Roles = keptRoles
	for _, f := range Validate(def) {
		if f.Code == CodeRoleGrant && strings.Contains(f.Message, "is the option of") {
			t.Errorf("unexpected exactly-one finding when interest/language roles removed: %+v", f)
		}
	}
}

// verifyExactlyOneSupporterOption verifies findings when supporter role is used as an option.
func verifyExactlyOneSupporterOption(t *testing.T) {
	t.Helper()
	def := valid(t)
	def.Choices.Questions[0].Options = append(def.Choices.Questions[0].Options, Option{
		Title: "Supporter option",
		Role:  "supporter",
	})
	findings := Validate(def)
	if !hasFinding(findings, CodeRoleGrant, "choices.questions[interests].options[supporter]", "neither interest nor language") {
		t.Errorf("missing neither interest nor language finding: %+v", findings)
	}
	if hasFinding(findings, CodeRoleGrant, "roles[supporter]", "is the option of") {
		t.Errorf("supporter role got unexpected exactly-one finding: %+v", findings)
	}
}

func TestValidate_ExactlyOneOptionPerRole(t *testing.T) {
	t.Run("valid definition has no exactly-one findings", verifyExactlyOneValid)
	t.Run("option removed produces 0 choices finding", verifyExactlyOneOptionRemoved)
	t.Run("option added to languages question produces 2 choices finding", verifyExactlyOneOptionAddedLanguages)
	t.Run("same option twice in one question produces 2 choices finding", verifyExactlyOneDuplicateOption)
	t.Run("language option removed produces 0 choices finding", verifyExactlyOneLanguageRemoved)
	t.Run("boundary: interest and language roles and questions removed produces no finding", verifyExactlyOneRolesQuestionsRemoved)
	t.Run("supporter role as option produces existing finding and no exactly-one finding", verifyExactlyOneSupporterOption)
}

// verifyChoiceLimitOptionCounts verifies option count limits (25 passes, 26 fails).
func verifyChoiceLimitOptionCounts(t *testing.T) {
	t.Helper()
	def := valid(t)
	for i := 1; i <= 21; i++ {
		id := fmt.Sprintf("interest-extra-%02d", i)
		def.Roles = append(def.Roles, Role{
			ID:          id,
			Name:        "Extra Interest " + strconv.Itoa(i),
			Kind:        "interest",
			Purpose:     "Testing 25 options limit",
			AssignedBy:  "choice",
			Basis:       "decided",
			Permissions: []string{},
		})
		def.RoleOrder = append(def.RoleOrder, id)
		def.Choices.Questions[0].Options = append(def.Choices.Questions[0].Options, Option{
			Title: "Option " + strconv.Itoa(i),
			Role:  id,
		})
	}
	if len(def.Choices.Questions[0].Options) != 25 {
		t.Fatalf("expected 25 options, got %d", len(def.Choices.Questions[0].Options))
	}
	findings25 := Validate(def)
	if hasCode(findings25, CodeChoiceLimit) {
		t.Errorf("25 options produced CodeChoiceLimit: %+v", findings25)
	}
	if len(findings25) != 0 {
		t.Fatalf("expected 0 findings for valid 25 options definition, got %d: %+v", len(findings25), findings25)
	}

	id26 := "interest-extra-22"
	def.Roles = append(def.Roles, Role{
		ID:          id26,
		Name:        "Extra Interest 22",
		Kind:        "interest",
		Purpose:     "Testing 26 options limit",
		AssignedBy:  "choice",
		Basis:       "decided",
		Permissions: []string{},
	})
	def.RoleOrder = append(def.RoleOrder, id26)
	def.Choices.Questions[0].Options = append(def.Choices.Questions[0].Options, Option{
		Title: "Option 22",
		Role:  id26,
	})
	findings26 := Validate(def)
	if !hasFinding(findings26, CodeChoiceLimit, "choices.questions[interests].options", "26") {
		t.Errorf("26 options missing CodeChoiceLimit at choices.questions[interests].options with '26': %+v", findings26)
	}
}

// verifyChoiceLimitTitleRunes verifies title rune limits (100 runes passes, 101 fails).
func verifyChoiceLimitTitleRunes(t *testing.T) {
	t.Helper()
	def := valid(t)
	def.Choices.Questions[0].Options[0].Title = strings.Repeat("x", 100)
	findings100 := Validate(def)
	if hasCode(findings100, CodeChoiceLimit) {
		t.Errorf("100-rune title produced CodeChoiceLimit: %+v", findings100)
	}

	def = valid(t)
	def.Choices.Questions[0].Options[0].Title = strings.Repeat("x", 101)
	findings101 := Validate(def)
	path := fmt.Sprintf("choices.questions[interests].options[%s].title", def.Choices.Questions[0].Options[0].Role)
	if !hasFinding(findings101, CodeChoiceLimit, path, "101") {
		t.Errorf("101-rune title missing CodeChoiceLimit at %s with '101': %+v", path, findings101)
	}
}

// verifyChoiceLimitTwoByteTitle verifies that title of 100 2-byte runes passes.
func verifyChoiceLimitTwoByteTitle(t *testing.T) {
	t.Helper()
	def := valid(t)
	twoByteTitle := strings.Repeat("é", 100)
	if len(twoByteTitle) != 200 {
		t.Fatalf("expected 200 bytes, got %d", len(twoByteTitle))
	}
	if utf8.RuneCountInString(twoByteTitle) != 100 {
		t.Fatalf("expected 100 runes, got %d", utf8.RuneCountInString(twoByteTitle))
	}
	def.Choices.Questions[0].Options[0].Title = twoByteTitle
	findings := Validate(def)
	if hasCode(findings, CodeChoiceLimit) {
		t.Errorf("100-rune 2-byte title produced CodeChoiceLimit: %+v", findings)
	}
}

func TestValidate_ChoiceLimits(t *testing.T) {
	t.Run("question with 25 options has 0 findings; 26 produces CodeChoiceLimit", verifyChoiceLimitOptionCounts)
	t.Run("title with exactly 100 runes produces no finding; 101 produces CodeChoiceLimit", verifyChoiceLimitTitleRunes)
	t.Run("title of 100 runes of a two-byte character produces no finding", verifyChoiceLimitTwoByteTitle)
}
