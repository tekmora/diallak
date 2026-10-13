package discord

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/serverplan"
)

// assertCreateRoleDisplay verifies that create-role sends the expected role display fields.
func assertCreateRoleDisplay(t *testing.T, roleObject, wantName string, wantHoist, wantMentionable bool) {
	t.Helper()
	def := mustDefinition(t)
	fx := newChangeFixture(t, def, roleObject)
	c := serverplan.Change{Step: 1, Action: "create-role", Object: roleObject}
	err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second)
	if err != nil {
		t.Fatalf("runChange: %v", err)
	}
	w := oneWrite(t, fx.fake)
	var req struct {
		Name        string `json:"name"`
		Hoist       bool   `json:"hoist"`
		Mentionable bool   `json:"mentionable"`
	}
	if err := json.Unmarshal(w.body, &req); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if req.Name != wantName {
		t.Errorf("Name = %q, want '%s'", req.Name, wantName)
	}
	if req.Hoist != wantHoist {
		t.Errorf("Hoist = %t, want %t", req.Hoist, wantHoist)
	}
	if req.Mentionable != wantMentionable {
		t.Errorf("Mentionable = %t, want %t", req.Mentionable, wantMentionable)
	}
}

func TestApplyCreateRole_SendsHoistAndMentionable(t *testing.T) {
	t.Run("helper sends hoist true and mentionable false", func(t *testing.T) {
		assertCreateRoleDisplay(t, "roles[helper]", "Helper", true, false)
	})

	t.Run("notify-releases sends mentionable true and hoist false", func(t *testing.T) {
		assertCreateRoleDisplay(t, "roles[notify-releases]", "Notify: releases", false, true)
	})
}

// assertSetRoleDisplayField verifies that set-role-display sends only the stated field.
func assertSetRoleDisplayField(t *testing.T, roleObject, presentField, absentField string) {
	t.Helper()
	def := mustDefinition(t)
	fx := newChangeFixture(t, def)
	c := serverplan.Change{Step: 1, Action: "set-role-display", Object: roleObject}
	err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second)
	if err != nil {
		t.Fatalf("runChange: %v", err)
	}
	w := oneWrite(t, fx.fake)
	var raw map[string]any
	if err := json.Unmarshal(w.body, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	val, present := raw[presentField]
	if !present || val != true {
		t.Errorf("%s = %v (present: %t), want true", presentField, val, present)
	}
	if _, hasAbsent := raw[absentField]; hasAbsent {
		t.Errorf("%s present in request body %s, want absent", absentField, string(w.body))
	}
}

func TestApplySetRoleDisplay_SendsOnlyStatedFields(t *testing.T) {
	t.Run("helper sends hoist only", func(t *testing.T) {
		assertSetRoleDisplayField(t, "roles[helper]", "hoist", "mentionable")
	})

	t.Run("notify-releases sends mentionable only", func(t *testing.T) {
		assertSetRoleDisplayField(t, "roles[notify-releases]", "mentionable", "hoist")
	})
}

func TestApply_Defense_ManagedRoleRefusedWithoutRestCall(t *testing.T) {
	def := mustDefinition(t)
	managedRoles := []string{"booster", "funding-ko-fi", "funding-patreon"}
	actions := []string{"create-role", "set-role-permissions", "set-role-display"}

	for _, roleID := range managedRoles {
		for _, action := range actions {
			t.Run(action+"_"+roleID, func(t *testing.T) {
				fx := newChangeFixture(t, def)
				fx.fake.resetRecorded()
				c := serverplan.Change{Step: 1, Action: action, Object: "roles[" + roleID + "]"}
				err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second)
				if err == nil {
					t.Fatalf("%s on %s succeeded, want defense error", action, roleID)
				}
				wantMsg := "role \"" + roleID + "\" is managed by another party; the tool does not change it"
				if !strings.Contains(err.Error(), wantMsg) {
					t.Errorf("err = %q, want containing %q", err.Error(), wantMsg)
				}
				if n := len(fx.fake.getRecorded()); n != 0 {
					t.Errorf("recorded %d calls, want 0", n)
				}
			})
		}
	}
}
