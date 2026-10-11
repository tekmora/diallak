package server

import (
	"bytes"
	"os"
	"reflect"
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
