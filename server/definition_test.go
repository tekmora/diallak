package server

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

func TestDefinition_Positive_LoadsAndValidatesEmbedded(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v, want nil", err)
	}
	if def == nil {
		t.Fatal("Definition() returned nil definition")
	}
	if def.Version != 1 {
		t.Errorf("def.Version = %d, want 1", def.Version)
	}
	if len(def.Roles) == 0 {
		t.Error("def.Roles is empty")
	}
}

func TestDefinition_Positive_EqualsDiskFile(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}

	data, err := os.ReadFile("definition.json")
	if err != nil {
		data, err = os.ReadFile("../server/definition.json")
		if err != nil {
			t.Fatalf("read definition.json from disk: %v", err)
		}
	}

	diskDef, err := serverdef.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load(disk definition): %v", err)
	}

	if !reflect.DeepEqual(def, diskDef) {
		t.Errorf("embedded definition does not equal disk definition")
	}
}

func TestDefinition_Negative_UnknownFieldsRefused(t *testing.T) {
	orig := definitionJSON
	withRequired := strings.Replace(orig, `"multiple": true`, `"required": false, "multiple": true`, 1)
	if withRequired == orig {
		t.Fatal("string replace for required did not change text")
	}
	_, err := serverdef.Load(strings.NewReader(withRequired))
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("Load with required = false error = %v, want error holding 'required'", err)
	}

	oldKey := "on" + "boarding"
	withOldKey := strings.Replace(orig, `"choices": {`, `"`+oldKey+`": {`, 1)
	if withOldKey == orig {
		t.Fatal("string replace for old key did not change text")
	}
	_, err = serverdef.Load(strings.NewReader(withOldKey))
	if err == nil || !strings.Contains(err.Error(), oldKey) {
		t.Errorf("Load with old key error = %v, want error holding %q", err, oldKey)
	}
}

func TestDefinition_Positive_AreaChoicesIsTheBotsDecidedAutomation(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v, want nil", err)
	}
	found := 0
	for _, a := range def.Automation {
		if strings.Contains(a.ID, "boarding") {
			t.Errorf("automation %q names the platform's former questions", a.ID)
		}
		if a.ID != "area-choices" {
			continue
		}
		found++
		if a.By != "bot" || a.State != "decided" ||
			a.Record != "docs/adr/0011-area-roles-from-member-choice.md" {
			t.Errorf("area-choices = %+v, want by bot, decided, record 0011", a)
		}
	}
	if found != 1 {
		t.Errorf("automation entries with id area-choices = %d, want 1", found)
	}
	if def.Choices.By != "bot" || len(def.Choices.Questions) != 3 {
		t.Errorf("choices = by %q with %d questions, want by bot with 3",
			def.Choices.By, len(def.Choices.Questions))
	}
}

func TestDefinitionDigest_Positive_ShapeAndEqualsDiskFile(t *testing.T) {
	digest := DefinitionDigest()
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("DefinitionDigest() = %q, want sha256:<64 hex>", digest)
	}
	data, err := os.ReadFile("definition.json")
	if err != nil {
		data, err = os.ReadFile("../server/definition.json")
		if err != nil {
			t.Fatalf("read definition.json from disk: %v", err)
		}
	}
	expected := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	if digest != expected {
		t.Errorf("DefinitionDigest() = %q, want %q", digest, expected)
	}
}

// rightsNamedAnywhere returns all rights named on any role's permissions or in any channel entry's allow list.
func rightsNamedAnywhere(def *serverdef.Definition) map[string]struct{} {
	named := make(map[string]struct{})
	for _, r := range def.Roles {
		for _, p := range r.Permissions {
			named[p] = struct{}{}
		}
	}
	for _, ch := range def.Channels {
		for _, acc := range ch.Access {
			for _, p := range acc.Allow {
				named[p] = struct{}{}
			}
		}
	}
	return named
}

func TestDefinition_Boundary_NoSendPolls(t *testing.T) {
	// members send no polls; a vote that counts runs through the wishes and polls path, ADR-0015
	t.Run("committed definition names no SEND_POLLS", func(t *testing.T) {
		def, err := Definition()
		if err != nil {
			t.Fatalf("Definition() error = %v", err)
		}
		named := rightsNamedAnywhere(def)
		if _, ok := named["SEND_POLLS"]; ok {
			t.Errorf("committed definition names SEND_POLLS in role permissions or channel allow")
		}
	})

	t.Run("helper reports SEND_POLLS when present on everyone", func(t *testing.T) {
		def, err := Definition()
		if err != nil {
			t.Fatalf("Definition() error = %v", err)
		}
		rolesCopy := make([]serverdef.Role, len(def.Roles))
		copy(rolesCopy, def.Roles)
		for i, r := range rolesCopy {
			if r.Kind == "everyone" {
				rolesCopy[i].Permissions = append(append([]string{}, r.Permissions...), "SEND_POLLS")
			}
		}
		copyDef := *def
		copyDef.Roles = rolesCopy
		named := rightsNamedAnywhere(&copyDef)
		if _, ok := named["SEND_POLLS"]; !ok {
			t.Errorf("rightsNamedAnywhere did not report SEND_POLLS when added to @everyone")
		}
	})

	t.Run("helper reports SEND_POLLS when an entry on a channel allows it", func(t *testing.T) {
		def, err := Definition()
		if err != nil {
			t.Fatalf("Definition() error = %v", err)
		}
		if len(def.Channels) == 0 || len(def.Channels[0].Access) == 0 {
			t.Fatal("first channel of the definition holds no entry")
		}
		channelsCopy := make([]serverdef.Channel, len(def.Channels))
		copy(channelsCopy, def.Channels)
		accessCopy := make([]serverdef.Access, len(channelsCopy[0].Access))
		copy(accessCopy, channelsCopy[0].Access)
		accessCopy[0].Allow = append(append([]string{}, accessCopy[0].Allow...), "SEND_POLLS")
		channelsCopy[0].Access = accessCopy
		copyDef := *def
		copyDef.Channels = channelsCopy
		if _, ok := rightsNamedAnywhere(&copyDef)["SEND_POLLS"]; !ok {
			t.Errorf("rightsNamedAnywhere did not report SEND_POLLS allowed by an entry on a channel")
		}
	})
}

func TestDefinition_Positive_ServerSettingsAutomation(t *testing.T) {
	def, err := Definition()
	if err != nil {
		t.Fatalf("Definition() error = %v", err)
	}
	var entry *serverdef.Automation
	for i := range def.Automation {
		if def.Automation[i].ID == "server-settings" {
			entry = &def.Automation[i]
			break
		}
	}
	if entry == nil {
		t.Fatal("automation entry server-settings not found")
	}
	if entry.By != "bot" {
		t.Errorf("server-settings.By = %q, want %q", entry.By, "bot")
	}
	if entry.State != "decided" {
		t.Errorf("server-settings.State = %q, want %q", entry.State, "decided")
	}
	// The record's path counts from the repository root; this package is one folder below it.
	if _, err := os.Stat(filepath.Join("..", entry.Record)); err != nil {
		t.Errorf("server-settings record file %q does not exist on disk: %v", entry.Record, err)
	}
}
