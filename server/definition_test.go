package server

import (
	"bytes"
	"os"
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
