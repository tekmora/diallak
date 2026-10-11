package serverplan

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failWriter struct{}

func (f *failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write disk full")
}

func TestRender_Positive_Golden(t *testing.T) {
	def := loadDef(t)

	t.Run("own-mode area entry plan", func(t *testing.T) {
		s := snapshotOf(t, def)
		for i := range s.Channels {
			if s.Channels[i].Name == "home-lab" {
				var newAccess []Access
				for _, acc := range s.Channels[i].Access {
					if acc.Role != "Interest: home lab and self-hosting" {
						newAccess = append(newAccess, acc)
					}
				}
				s.Channels[i].Access = newAccess
			}
		}

		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}

		var buf bytes.Buffer
		if err := Render(&buf, res); err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		want := "mode: own\n" +
			"rights needed: MANAGE_ROLES, VIEW_CHANNEL\n" +
			"changes: 1\n" +
			"  1. set-access channels[home-lab].access[interest-home-lab] needs MANAGE_ROLES, VIEW_CHANNEL\n" +
			"refused: 0\n"

		if buf.String() != want {
			t.Errorf("Render mismatch:\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
		}
	})

	t.Run("one refused plan", func(t *testing.T) {
		s := snapshotOf(t, def)
		for i := range s.Channels {
			if s.Channels[i].Name == "home-lab" {
				var newAccess []Access
				for _, acc := range s.Channels[i].Access {
					if acc.Role != "Moderator" {
						newAccess = append(newAccess, acc)
					}
				}
				s.Channels[i].Access = newAccess
			}
		}

		res, err := Plan(def, s)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}

		var buf bytes.Buffer
		if err := Render(&buf, res); err != nil {
			t.Fatalf("Render failed: %v", err)
		}

		want := "mode: own\n" +
			"rights needed: MANAGE_ROLES, VIEW_CHANNEL\n" +
			"changes: 0\n" +
			"refused: 1\n" +
			"  - set-access channels[home-lab].access[moderator]: outside-own-changes\n"

		if buf.String() != want {
			t.Errorf("Render mismatch:\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
		}
	})
}

func TestRender_Negative_WriterFails(t *testing.T) {
	fw := &failWriter{}
	r := Result{
		Raised: false,
	}
	err := Render(fw, r)
	if err == nil {
		t.Fatal("expected error on failing writer, got nil")
	}
	if !strings.Contains(err.Error(), "write disk full") {
		t.Errorf("error %q should wrap original writer error", err.Error())
	}
}

func TestRender_Boundary_EmptyPlan(t *testing.T) {
	var buf bytes.Buffer
	r := Result{
		Raised: false,
	}
	if err := Render(&buf, r); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	want := "mode: own\n" +
		"rights needed: none\n" +
		"changes: 0\n" +
		"refused: 0\n"

	if buf.String() != want {
		t.Errorf("Render mismatch:\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}

// TestRender_Boundary_DetailAndRaised renders a hand-built plan so that every
// part of a line is pinned: the mode of a raised caller, a change with and
// without detail, and a refusal with and without detail.
func TestRender_Boundary_DetailAndRaised(t *testing.T) {
	r := Result{
		Raised: true,
		Changes: []Change{
			{Step: 1, Action: ActionCreateRole, Object: "roles[contributor]", Needs: []string{"MANAGE_ROLES"}},
			{
				Step: 2, Action: ActionSetRoleOrder, Object: "role_order",
				Needs: []string{"MANAGE_ROLES"}, Detail: "operator, bot, contributor",
			},
		},
		Refused: []Refusal{
			{
				Action: ActionCreateChannel, Object: "channels[announcements]",
				Needs: []string{"MANAGE_CHANNELS"}, Reason: ReasonMissingFeature, Detail: "NEWS",
			},
			{
				Action: ActionSetAccess, Object: "channels[rules].access[everyone]",
				Needs: []string{"MANAGE_ROLES"}, Reason: ReasonLacksRight,
			},
		},
	}

	var buf bytes.Buffer
	if err := Render(&buf, r); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	want := "mode: raised\n" +
		"rights needed: MANAGE_CHANNELS, MANAGE_ROLES\n" +
		"changes: 2\n" +
		"  1. " + ActionCreateRole + " roles[contributor] needs MANAGE_ROLES\n" +
		"  2. " + ActionSetRoleOrder + " role_order needs MANAGE_ROLES; operator, bot, contributor\n" +
		"refused: 2\n" +
		"  - " + ActionCreateChannel + " channels[announcements]: " + ReasonMissingFeature + "; NEWS\n" +
		"  - " + ActionSetAccess + " channels[rules].access[everyone]: " + ReasonLacksRight + "\n"

	if buf.String() != want {
		t.Errorf("Render mismatch:\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}
