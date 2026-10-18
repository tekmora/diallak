package roleconn

import (
	"os"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
)

func validBaseRecordsAndValues() (*Records, *Values) {
	recs := &Records{
		Records: []discord.ApplicationRoleConnectionMetadata{
			{
				Type:        discord.ApplicationRoleConnectionMetadataTypeBooleanEqual,
				Key:         "test_key",
				Name:        "Test Name",
				Description: "Test Description",
			},
		},
	}
	vals := &Values{
		PlatformName: "GitHub",
		Values: []ValueRule{
			{
				Key:           "test_key",
				Rule:          "Rule description",
				Met:           "1",
				NotMet:        "0",
				Organisations: []string{"tekmora"},
				Visibility:    "public",
				Fork:          false,
			},
		},
	}
	return recs, vals
}

func TestValidate_Positive_ShippedFilesPass(t *testing.T) {
	recData, err := os.ReadFile("../../server/role-connections.json")
	if err != nil {
		t.Fatalf("read role-connections.json: %v", err)
	}
	valData, err := os.ReadFile("../../server/role-connection-values.json")
	if err != nil {
		t.Fatalf("read role-connection-values.json: %v", err)
	}

	recs, err := LoadRecords(strings.NewReader(string(recData)))
	if err != nil {
		t.Fatalf("LoadRecords: %v", err)
	}
	vals, err := LoadValues(strings.NewReader(string(valData)))
	if err != nil {
		t.Fatalf("LoadValues: %v", err)
	}

	findings := Validate(recs, vals)
	if len(findings) != 0 {
		t.Fatalf("Validate shipped files returned findings: %v", findings)
	}
}

func TestValidate_Boundary_RecordCount(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	recs.Records = nil
	vals.Values = nil
	for i := 1; i <= 6; i++ {
		key := strings.Repeat("a", i)
		recs.Records = append(recs.Records, discord.ApplicationRoleConnectionMetadata{
			Type:        discord.ApplicationRoleConnectionMetadataTypeBooleanEqual,
			Key:         key,
			Name:        "Name",
			Description: "Desc",
		})
		vals.Values = append(vals.Values, ValueRule{
			Key:           key,
			Rule:          "rule",
			Met:           "1",
			NotMet:        "0",
			Organisations: []string{"org"},
			Visibility:    "public",
			Fork:          false,
		})
	}
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted 6 records, want findings")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f, "between 1 and 5") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected finding mentioning 'between 1 and 5', got: %v", findings)
	}
}

func TestValidate_Boundary_TypeRange(t *testing.T) {
	tests := []struct {
		name       string
		recType    discord.ApplicationRoleConnectionMetadataType
		met        string
		notMet     string
		wantReject bool
	}{
		{"type 0 refused", 0, "1", "0", true},
		{"type 1 accepted", 1, "10", "5", false},
		{"type 8 accepted", 8, "1", "0", false},
		{"type 9 refused", 9, "1", "0", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recs, vals := validBaseRecordsAndValues()
			recs.Records[0].Type = tc.recType
			vals.Values[0].Met = tc.met
			vals.Values[0].NotMet = tc.notMet
			findings := Validate(recs, vals)
			if tc.wantReject && len(findings) == 0 {
				t.Fatalf("expected findings for %s, got none", tc.name)
			}
			if !tc.wantReject && len(findings) != 0 {
				t.Fatalf("unexpected findings for %s: %v", tc.name, findings)
			}
		})
	}
}

func TestValidate_Negative_InvalidKey(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	recs.Records[0].Key = "Bad-Key"
	vals.Values[0].Key = "Bad-Key"
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted 'Bad-Key', want finding")
	}
}

func TestValidate_Boundary_KeyLength(t *testing.T) {
	key50 := strings.Repeat("k", 50)
	key51 := strings.Repeat("k", 51)

	recs, vals := validBaseRecordsAndValues()
	recs.Records[0].Key = key50
	vals.Values[0].Key = key50
	if findings := Validate(recs, vals); len(findings) != 0 {
		t.Fatalf("Validate refused 50-char key: %v", findings)
	}

	recs.Records[0].Key = key51
	vals.Values[0].Key = key51
	if findings := Validate(recs, vals); len(findings) == 0 {
		t.Fatal("Validate accepted 51-char key, want finding")
	}
}

func TestValidate_Boundary_NameLength(t *testing.T) {
	name100 := strings.Repeat("n", 100)
	name101 := strings.Repeat("n", 101)

	recs, vals := validBaseRecordsAndValues()
	recs.Records[0].Name = name100
	if findings := Validate(recs, vals); len(findings) != 0 {
		t.Fatalf("Validate refused 100-rune name: %v", findings)
	}

	recs.Records[0].Name = name101
	if findings := Validate(recs, vals); len(findings) == 0 {
		t.Fatal("Validate accepted 101-rune name, want finding")
	}
}

func TestValidate_Negative_UnknownLocale(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	recs.Records[0].NameLocalizations = map[discord.Locale]string{
		discord.Locale("xx"): "Valid Name",
	}
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted unknown locale 'xx', want finding")
	}
}

func TestValidate_Positive_ValidLocale(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	recs.Records[0].NameLocalizations = map[discord.Locale]string{
		discord.LocaleGerman: "Gültiger Name",
	}
	findings := Validate(recs, vals)
	if len(findings) != 0 {
		t.Fatalf("Validate refused valid locale 'de': %v", findings)
	}
}

func TestValidate_Negative_ValueKeyWithNoRecord(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	vals.Values[0].Key = "other_key"
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted value key without record, want finding")
	}
}

func TestValidate_Negative_RecordWithNoValue(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	vals.Values = nil
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted record without value rule, want finding")
	}
}

func TestValidate_Negative_BooleanMetTrueRefused(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	vals.Values[0].Met = "true"
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted met=\"true\" for boolean record, want finding")
	}
}

func TestValidate_Negative_VisibilityPrivateRefused(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	vals.Values[0].Visibility = "private"
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted visibility=\"private\", want finding")
	}
}

func TestValidate_Negative_ForkTrueRefused(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	vals.Values[0].Fork = true
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted fork=true, want finding")
	}
}

func TestValidate_Negative_EmptyOrganisationsRefused(t *testing.T) {
	recs, vals := validBaseRecordsAndValues()
	vals.Values[0].Organisations = []string{}
	findings := Validate(recs, vals)
	if len(findings) == 0 {
		t.Fatal("Validate accepted empty organisations, want finding")
	}
}

func TestLoad_Negative_UnknownJSONFieldRefused(t *testing.T) {
	badRecJSON := `{"records":[],"unexpected_field":true}`
	if _, err := LoadRecords(strings.NewReader(badRecJSON)); err == nil {
		t.Fatal("LoadRecords accepted unknown JSON field, want error")
	}

	badValJSON := `{"platform_name":"GitHub","values":[],"unexpected_field":true}`
	if _, err := LoadValues(strings.NewReader(badValJSON)); err == nil {
		t.Fatal("LoadValues accepted unknown JSON field, want error")
	}
}
