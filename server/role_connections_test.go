package server

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/roleconn"
)

func TestRoleConnections_Positive_LoadsAndValidatesEmbedded(t *testing.T) {
	recs, vals, err := RoleConnections()
	if err != nil {
		t.Fatalf("RoleConnections() error = %v, want nil", err)
	}
	if recs == nil || vals == nil {
		t.Fatal("RoleConnections() returned nil records or values")
	}
	if len(recs.Records) != 1 {
		t.Errorf("len(recs.Records) = %d, want 1", len(recs.Records))
	}
	if vals.PlatformName != "GitHub" {
		t.Errorf("vals.PlatformName = %q, want GitHub", vals.PlatformName)
	}

	digest := RoleConnectionsDigest()
	if !strings.HasPrefix(digest, "sha256:") {
		t.Errorf("RoleConnectionsDigest() = %q, want prefix sha256:", digest)
	}
	if len(digest) != 7+64 {
		t.Errorf("RoleConnectionsDigest() length = %d, want %d", len(digest), 7+64)
	}
}

func TestRoleConnections_Positive_EqualsDiskFiles(t *testing.T) {
	recs, vals, err := RoleConnections()
	if err != nil {
		t.Fatalf("RoleConnections() error = %v", err)
	}

	diskRecData, err := os.ReadFile("role-connections.json")
	if err != nil {
		diskRecData, err = os.ReadFile("../server/role-connections.json")
		if err != nil {
			t.Fatalf("read role-connections.json: %v", err)
		}
	}
	diskRecs, err := roleconn.LoadRecords(bytes.NewReader(diskRecData))
	if err != nil {
		t.Fatalf("LoadRecords(disk): %v", err)
	}
	if !reflect.DeepEqual(recs, diskRecs) {
		t.Errorf("embedded records != disk records")
	}

	diskValData, err := os.ReadFile("role-connection-values.json")
	if err != nil {
		diskValData, err = os.ReadFile("../server/role-connection-values.json")
		if err != nil {
			t.Fatalf("read role-connection-values.json: %v", err)
		}
	}
	diskVals, err := roleconn.LoadValues(bytes.NewReader(diskValData))
	if err != nil {
		t.Fatalf("LoadValues(disk): %v", err)
	}
	if !reflect.DeepEqual(vals, diskVals) {
		t.Errorf("embedded values != disk values")
	}
}

func TestRoleConnections_Positive_DigestMatchesDisk(t *testing.T) {
	diskRecData, err := os.ReadFile("role-connections.json")
	if err != nil {
		diskRecData, err = os.ReadFile("../server/role-connections.json")
		if err != nil {
			t.Fatalf("read role-connections.json: %v", err)
		}
	}
	sum := sha256.Sum256(diskRecData)
	want := fmt.Sprintf("sha256:%x", sum)
	got := RoleConnectionsDigest()
	if got != want {
		t.Errorf("RoleConnectionsDigest() = %q, want %q", got, want)
	}
}

// The merge scope decision (diallak-linked-roles-merge-scope) names these seven organisations,
// public repositories only and no forks.
func TestRoleConnections_Positive_ValueRuleNamesDecidedScope(t *testing.T) {
	_, vals, err := RoleConnections()
	if err != nil {
		t.Fatalf("RoleConnections() error = %v, want nil", err)
	}
	want := []string{"lusoris", "cordanaLLM", "golusoris", "VMAFx", "jellysin", "plurano", "tekmora"}
	rule := vals.Values[0]
	if !reflect.DeepEqual(rule.Organisations, want) {
		t.Errorf("organisations = %v, want %v", rule.Organisations, want)
	}
	if rule.Visibility != "public" || rule.Fork {
		t.Errorf("visibility = %q, fork = %v, want public and false", rule.Visibility, rule.Fork)
	}
}
