// Package roleconn provides types, decoding, and validation for Discord application
// role-connection metadata and verification values.
package roleconn

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/tekmora/diallak/internal/serverdef"
)

var (
	keyPattern = regexp.MustCompile(`^[a-z0-9_]{1,50}$`)
	orgPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)
)

// Records holds the collection of Discord application role connection metadata records.
type Records struct {
	Records []discord.ApplicationRoleConnectionMetadata `json:"records"`
}

// Values holds the platform configuration and value rules for role connections.
type Values struct {
	PlatformName string      `json:"platform_name"`
	Values       []ValueRule `json:"values"`
}

// ValueRule defines verification and display rules for a specific metadata record key.
type ValueRule struct {
	Key           string   `json:"key"`
	Rule          string   `json:"rule"`
	Met           string   `json:"met"`
	NotMet        string   `json:"not_met"`
	Organisations []string `json:"organisations"`
	Visibility    string   `json:"visibility"`
	Fork          bool     `json:"fork"`
}

// LoadRecords reads and parses role connection records from r using strict JSON decoding.
func LoadRecords(r io.Reader) (*Records, error) {
	var rec Records
	if err := serverdef.DecodeStrict(r, "records", &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// LoadValues reads and parses role connection values from r using strict JSON decoding.
func LoadValues(r io.Reader) (*Values, error) {
	var val Values
	if err := serverdef.DecodeStrict(r, "values", &val); err != nil {
		return nil, err
	}
	return &val, nil
}

// Validate checks role connection records and values against Discord metadata specifications
// and project value rules. It returns a slice of findings, which is empty when validation passes.
func Validate(rec *Records, val *Values) []string {
	if rec == nil {
		return []string{"records is nil"}
	}
	if val == nil {
		return []string{"values is nil"}
	}

	var findings []string
	if n := len(rec.Records); n < 1 || n > 5 {
		findings = append(findings, fmt.Sprintf("records count %d must be between 1 and 5", n))
	}

	recFindings, recKeys := validateRecords(rec.Records)
	findings = append(findings, recFindings...)

	valFindings, valKeys := validateValues(val, recKeys)
	findings = append(findings, valFindings...)

	for _, r := range rec.Records {
		if !valKeys[r.Key] {
			findings = append(findings, fmt.Sprintf("record %q has no corresponding value rule", r.Key))
		}
	}
	return findings
}

func validateRecords(records []discord.ApplicationRoleConnectionMetadata) ([]string, map[string]discord.ApplicationRoleConnectionMetadataType) {
	var findings []string
	recKeys := make(map[string]discord.ApplicationRoleConnectionMetadataType, len(records))
	seenKeys := make(map[string]bool, len(records))

	for _, r := range records {
		recFindings := validateRecord(r, seenKeys)
		findings = append(findings, recFindings...)
		recKeys[r.Key] = r.Type
	}
	return findings, recKeys
}

func validateRecord(r discord.ApplicationRoleConnectionMetadata, seenKeys map[string]bool) []string {
	var findings []string
	if int(r.Type) < 1 || int(r.Type) > 8 {
		findings = append(findings, fmt.Sprintf("record %q type %d must be between 1 and 8", r.Key, r.Type))
	}
	if !keyPattern.MatchString(r.Key) {
		findings = append(findings, fmt.Sprintf("record key %q must match ^[a-z0-9_]{1,50}$", r.Key))
	}
	if seenKeys[r.Key] {
		findings = append(findings, fmt.Sprintf("duplicate record key %q", r.Key))
	}
	seenKeys[r.Key] = true

	if n := utf8.RuneCountInString(r.Name); n < 1 || n > 100 {
		findings = append(findings, fmt.Sprintf("record %q name length %d must be between 1 and 100 runes", r.Key, n))
	}
	if n := utf8.RuneCountInString(r.Description); n < 1 || n > 200 {
		findings = append(findings, fmt.Sprintf("record %q description length %d must be between 1 and 200 runes", r.Key, n))
	}

	findings = append(findings, validateLocalizations(r.Key, r.NameLocalizations, r.DescriptionLocalizations)...)
	return findings
}

func sortedLocales(m map[discord.Locale]string) []discord.Locale {
	locs := make([]discord.Locale, 0, len(m))
	for loc := range m {
		locs = append(locs, loc)
	}
	slices.Sort(locs)
	return locs
}

func validateLocalizations(key string, names, descs map[discord.Locale]string) []string {
	var findings []string
	for _, loc := range sortedLocales(names) {
		if _, ok := discord.Locales[loc]; !ok || loc == discord.LocaleUnknown {
			findings = append(findings, fmt.Sprintf("record %q name localization has invalid locale %q", key, loc))
		}
		if n := utf8.RuneCountInString(names[loc]); n < 1 || n > 100 {
			findings = append(findings, fmt.Sprintf("record %q name localization %q length %d must be between 1 and 100 runes", key, loc, n))
		}
	}
	for _, loc := range sortedLocales(descs) {
		if _, ok := discord.Locales[loc]; !ok || loc == discord.LocaleUnknown {
			findings = append(findings, fmt.Sprintf("record %q description localization has invalid locale %q", key, loc))
		}
		if n := utf8.RuneCountInString(descs[loc]); n < 1 || n > 200 {
			findings = append(findings, fmt.Sprintf("record %q description localization %q length %d must be between 1 and 200 runes", key, loc, n))
		}
	}
	return findings
}

func validateValues(val *Values, recKeys map[string]discord.ApplicationRoleConnectionMetadataType) ([]string, map[string]bool) {
	var findings []string
	valKeys := make(map[string]bool, len(val.Values))

	if n := utf8.RuneCountInString(val.PlatformName); n > 50 {
		findings = append(findings, fmt.Sprintf("platform_name length %d exceeds maximum of 50 runes", n))
	}

	for _, rule := range val.Values {
		findings = append(findings, validateValueRule(rule, valKeys, recKeys)...)
	}
	return findings, valKeys
}

func validateValueRule(
	rule ValueRule,
	valKeys map[string]bool,
	recKeys map[string]discord.ApplicationRoleConnectionMetadataType,
) []string {
	var findings []string
	if valKeys[rule.Key] {
		findings = append(findings, fmt.Sprintf("duplicate value rule key %q", rule.Key))
	}
	valKeys[rule.Key] = true

	recType, hasRec := recKeys[rule.Key]
	if !hasRec {
		findings = append(findings, fmt.Sprintf("value rule key %q does not match any record", rule.Key))
	}

	if len(strings.TrimSpace(rule.Rule)) == 0 {
		findings = append(findings, fmt.Sprintf("value rule %q rule text must not be empty", rule.Key))
	}
	if rule.Visibility != "public" {
		findings = append(findings, fmt.Sprintf("value rule %q visibility %q must equal %q", rule.Key, rule.Visibility, "public"))
	}
	if rule.Fork {
		findings = append(findings, fmt.Sprintf("value rule %q fork must be false", rule.Key))
	}

	findings = append(findings, validateMetNotMet(rule.Key, recType, hasRec, rule.Met, rule.NotMet)...)
	findings = append(findings, validateOrganisations(rule.Key, rule.Organisations)...)
	return findings
}

func validateMetNotMet(
	key string,
	recType discord.ApplicationRoleConnectionMetadataType,
	hasRec bool,
	met, notMet string,
) []string {
	var findings []string
	if utf8.RuneCountInString(met) > 100 {
		findings = append(findings, fmt.Sprintf("value rule %q met length exceeds 100 characters", key))
	}
	if utf8.RuneCountInString(notMet) > 100 {
		findings = append(findings, fmt.Sprintf("value rule %q not_met length exceeds 100 characters", key))
	}

	if !hasRec {
		return findings
	}

	if recType == 7 || recType == 8 {
		if met != "1" {
			findings = append(findings, fmt.Sprintf("value rule %q boolean met %q must equal \"1\"", key, met))
		}
		if notMet != "0" {
			findings = append(findings, fmt.Sprintf("value rule %q boolean not_met %q must equal \"0\"", key, notMet))
		}
		return findings
	}

	if recType >= 1 && recType <= 6 {
		if _, err := strconv.ParseInt(met, 10, 64); err != nil {
			findings = append(findings, fmt.Sprintf("value rule %q integer/datetime met %q must parse as int64: %v", key, met, err))
		}
		if _, err := strconv.ParseInt(notMet, 10, 64); err != nil {
			findings = append(findings, fmt.Sprintf("value rule %q integer/datetime not_met %q must parse as int64: %v", key, notMet, err))
		}
	}
	return findings
}

func validateOrganisations(key string, orgs []string) []string {
	var findings []string
	if len(orgs) == 0 {
		return []string{fmt.Sprintf("value rule %q organisations must not be empty", key)}
	}

	seenOrgs := make(map[string]bool, len(orgs))
	for _, org := range orgs {
		if seenOrgs[org] {
			findings = append(findings, fmt.Sprintf("value rule %q duplicate organisation %q", key, org))
		}
		seenOrgs[org] = true
		if !orgPattern.MatchString(org) {
			findings = append(findings, fmt.Sprintf("value rule %q organisation %q must match ^[A-Za-z0-9-]{1,39}$", key, org))
		}
	}
	return findings
}
