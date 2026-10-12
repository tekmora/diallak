package discord

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/serverplan"
)

func TestApplyCreateRole_SendsHoistAndMentionable(t *testing.T) {
	def := mustDefinition(t)

	t.Run("helper sends hoist true and mentionable false", func(t *testing.T) {
		fx := newChangeFixture(t, def, "roles[helper]")
		c := serverplan.Change{Step: 1, Action: "create-role", Object: "roles[helper]"}
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
		if req.Name != "Helper" {
			t.Errorf("Name = %q, want 'Helper'", req.Name)
		}
		if !req.Hoist {
			t.Errorf("Hoist = %t, want true", req.Hoist)
		}
		if req.Mentionable {
			t.Errorf("Mentionable = %t, want false", req.Mentionable)
		}
	})

	t.Run("notify-releases sends mentionable true and hoist false", func(t *testing.T) {
		fx := newChangeFixture(t, def, "roles[notify-releases]")
		c := serverplan.Change{Step: 1, Action: "create-role", Object: "roles[notify-releases]"}
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
		if req.Name != "Notify: releases" {
			t.Errorf("Name = %q, want 'Notify: releases'", req.Name)
		}
		if req.Hoist {
			t.Errorf("Hoist = %t, want false", req.Hoist)
		}
		if !req.Mentionable {
			t.Errorf("Mentionable = %t, want true", req.Mentionable)
		}
	})
}

func TestApplySetRoleDisplay_SendsOnlyStatedFields(t *testing.T) {
	def := mustDefinition(t)

	t.Run("helper sends hoist only", func(t *testing.T) {
		fx := newChangeFixture(t, def)
		c := serverplan.Change{Step: 1, Action: "set-role-display", Object: "roles[helper]"}
		err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second)
		if err != nil {
			t.Fatalf("runChange: %v", err)
		}
		w := oneWrite(t, fx.fake)
		var raw map[string]any
		if err := json.Unmarshal(w.body, &raw); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		hoistVal, hasHoist := raw["hoist"]
		if !hasHoist || hoistVal != true {
			t.Errorf("hoist = %v (present: %t), want true", hoistVal, hasHoist)
		}
		if _, hasMentionable := raw["mentionable"]; hasMentionable {
			t.Errorf("mentionable present in request body %s, want absent", string(w.body))
		}
	})

	t.Run("notify-releases sends mentionable only", func(t *testing.T) {
		fx := newChangeFixture(t, def)
		c := serverplan.Change{Step: 1, Action: "set-role-display", Object: "roles[notify-releases]"}
		err := runChange(t, fx.fake, def, fx.idx, c, 10*time.Second)
		if err != nil {
			t.Fatalf("runChange: %v", err)
		}
		w := oneWrite(t, fx.fake)
		var raw map[string]any
		if err := json.Unmarshal(w.body, &raw); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		mentionableVal, hasMentionable := raw["mentionable"]
		if !hasMentionable || mentionableVal != true {
			t.Errorf("mentionable = %v (present: %t), want true", mentionableVal, hasMentionable)
		}
		if _, hasHoist := raw["hoist"]; hasHoist {
			t.Errorf("hoist present in request body %s, want absent", string(w.body))
		}
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
