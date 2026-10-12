package main

import (
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
)

// The page on applying the definition has a table that gives each right the label it carries in
// Discord's settings, because the operator switches rights on by label and the tool prints
// programming names. This test keeps the table in step with the definition: every right the
// definition names on a role or on a channel's entry, and MANAGE_CHANNELS, has a row with the label
// of the English and of the German app.
func TestServerApplyDoc_Negative_LabelsEveryRight(t *testing.T) {
	doc, err := os.ReadFile("../../docs/server-apply.md")
	if err != nil {
		t.Fatalf("read documentation page: %v", err)
	}
	def := loadDefinitionForDoc(t)

	named := map[string]struct{}{"MANAGE_CHANNELS": {}}
	for _, role := range def.Roles {
		for _, right := range role.Permissions {
			named[right] = struct{}{}
		}
	}
	for _, channel := range def.Channels {
		for _, entry := range channel.Access {
			for _, right := range entry.Allow {
				named[right] = struct{}{}
			}
			for _, right := range entry.Deny {
				named[right] = struct{}{}
			}
		}
	}
	if len(named) < 10 {
		t.Fatalf("the definition names %d rights; the walk of this test no longer fits", len(named))
	}
	for right := range named {
		if !labelRow(right).Match(doc) {
			t.Errorf("the documentation page has no row with both settings labels for the right %q", right)
		}
	}
}

// labelRow matches the table row of a right: its name, the label of the English app, the label of
// the German app and where both were read.
func labelRow(right string) *regexp.Regexp {
	const label = " \\| [^|`]*[A-Za-z][^|`]*"
	return regexp.MustCompile("(?m)^\\| `" + regexp.QuoteMeta(right) + "`" + label + label + " \\| [^|]+ \\|$")
}

// A row that holds one label only, or an empty cell in place of one, is no row of the table.
func TestServerApplyDoc_Boundary_LabelRowNeedsBothLabels(t *testing.T) {
	cases := []struct {
		name string
		row  string
		want bool
	}{
		{"both labels", "| `SPEAK` | Speak | Sprechen | the same two pages |", true},
		{"one label", "| `SPEAK` | Speak | the same two pages |", false},
		{"empty second label", "| `SPEAK` | Speak |  | the same two pages |", false},
		{"empty first label", "| `SPEAK` |  | Sprechen | the same two pages |", false},
		{"no source", "| `SPEAK` | Speak | Sprechen |", false},
		{"another right", "| `SPEAKER` | Speak | Sprechen | the same two pages |", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := labelRow("SPEAK").MatchString(tc.row); got != tc.want {
				t.Errorf("labelRow matches %q: %t, want %t", tc.row, got, tc.want)
			}
		})
	}
}

// The page gives the address that adds the bot to a new server with the rights of the first
// set-up as one number. This test computes that number from the definition and the plan: the
// rights the plan of a new server needs, without those every member keeps through @everyone.
// It fails when the page carries another number, and when the rights behind the number leave
// a refusal in the plan.
func TestServerApplyDoc_Negative_InvitationNumberIsThePlansRights(t *testing.T) {
	doc, err := os.ReadFile("../../docs/server-apply.md")
	if err != nil {
		t.Fatalf("read documentation page: %v", err)
	}
	def := loadDefinitionForDoc(t)
	bot, everyone := newServerRights(t, def)

	own, err := serverplan.Plan(def, newServerSnapshot(def, bot, everyone))
	if err != nil {
		t.Fatalf("plan with the bot's own rights: %v", err)
	}
	var setUp []string
	for _, right := range own.Rights() {
		if !slices.Contains(everyone, right) {
			setUp = append(setUp, right)
		}
	}
	if len(setUp) <= len(bot) || slices.Contains(setUp, "ADMINISTRATOR") {
		t.Fatalf("rights of the first set-up = %v; want more than the bot's own %v and no ADMINISTRATOR", setUp, bot)
	}

	raised, err := serverplan.Plan(def, newServerSnapshot(def, setUp, everyone))
	if err != nil {
		t.Fatalf("plan with the rights of the first set-up: %v", err)
	}
	if len(raised.Changes) == 0 || len(raised.Refused) != 0 {
		t.Fatalf("plan with %v on the bot's role: %d changes, %d refused; want changes and no refusal",
			setUp, len(raised.Changes), len(raised.Refused))
	}

	mask, err := serverdef.PermissionMask(setUp)
	if err != nil {
		t.Fatalf("number for %v: %v", setUp, err)
	}
	want := "scope=bot&permissions=" + strconv.FormatUint(mask, 10)
	numbers := regexp.MustCompile(`scope=bot&permissions=[0-9]+`).FindAllString(string(doc), -1)
	if len(numbers) == 0 {
		t.Fatalf("the documentation page has no address with %q", want)
	}
	for _, got := range numbers {
		if got != want {
			t.Errorf("the documentation page has an address with %q, want %q (rights %s)", got, want, strings.Join(setUp, ", "))
		}
	}
}

func loadDefinitionForDoc(t *testing.T) *serverdef.Definition {
	t.Helper()
	file, err := os.Open("../../server/definition.json")
	if err != nil {
		t.Fatalf("open server definition: %v", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Errorf("close server definition: %v", closeErr)
		}
	}()
	def, err := serverdef.Load(file)
	if err != nil {
		t.Fatalf("Load server definition: %v", err)
	}
	return def
}

// newServerRights returns the rights the definition gives the bot's role and @everyone.
func newServerRights(t *testing.T, def *serverdef.Definition) (bot, everyone []string) {
	t.Helper()
	for _, role := range def.Roles {
		switch role.Kind {
		case "bot":
			bot = role.Permissions
		case "everyone":
			everyone = role.Permissions
		}
	}
	if len(bot) == 0 || len(everyone) == 0 {
		t.Fatalf("the definition gives the bot's role %d rights and @everyone %d; want both named", len(bot), len(everyone))
	}
	return bot, everyone
}

// newServerSnapshot is a server before its first set-up: the Operator role, the bot's role with
// the given rights, @everyone with the rights the definition gives it, and nothing else.
func newServerSnapshot(def *serverdef.Definition, botRights, everyone []string) *serverplan.Snapshot {
	var roles []serverplan.Role
	callerRole := ""
	for _, role := range def.Roles {
		switch role.Kind {
		case "operator":
			roles = append(roles, serverplan.Role{Name: role.Name, Permissions: role.Permissions})
		case "bot":
			callerRole = role.Name
			roles = append(roles, serverplan.Role{Name: role.Name, Managed: true, Permissions: botRights})
		}
	}
	roles = append(roles, serverplan.Role{Name: "@everyone", Everyone: true, Permissions: everyone})
	return &serverplan.Snapshot{
		Features:         []string{"COMMUNITY", "NEWS"},
		Caller:           serverplan.Caller{Role: callerRole, Permissions: slices.Concat(botRights, everyone)},
		Roles:            roles,
		Categories:       []serverplan.Category{},
		Channels:         []serverplan.Channel{},
		ChannelsComplete: true,
	}
}
