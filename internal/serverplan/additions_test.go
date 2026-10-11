package serverplan

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
)

func sampleDefWithBot() *serverdef.Definition {
	return &serverdef.Definition{
		Version: 1,
		Roles: []serverdef.Role{
			{
				ID:          "everyone",
				Name:        "@everyone",
				Kind:        "everyone",
				Permissions: []string{"VIEW_CHANNEL"},
			},
			{
				ID:          "bot",
				Name:        "Diallak",
				Kind:        "bot",
				Permissions: []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS"},
			},
		},
		Channels: []serverdef.Channel{
			{
				ID:       "general",
				Name:     "general",
				Type:     "text",
				Category: "General",
				Access: []serverdef.Access{
					{
						Role:  "everyone",
						Allow: []string{"VIEW_CHANNEL"},
					},
				},
			},
		},
		Categories: []serverdef.Category{
			{
				ID:   "general-cat",
				Name: "General",
			},
		},
	}
}

func TestCheck_Negative_NegativeMemberEntries(t *testing.T) {
	snap := &Snapshot{
		Caller: Caller{Role: "Diallak", Permissions: []string{"VIEW_CHANNEL"}},
		Roles: []Role{
			{Name: "Diallak", Everyone: false, Permissions: []string{"VIEW_CHANNEL"}},
			{Name: "@everyone", Everyone: true, Permissions: []string{"VIEW_CHANNEL"}},
		},
		Categories: []Category{{Name: "General"}},
		Channels: []Channel{
			{
				Name:          "general",
				Type:          "text",
				Category:      "General",
				MemberEntries: -1,
			},
		},
		ChannelsComplete: true,
	}

	err := Check(snap)
	if err == nil {
		t.Fatal("Check with negative MemberEntries succeeded, want error")
	}
	if !strings.Contains(err.Error(), "general") {
		t.Errorf("error %q does not name channel general", err.Error())
	}
}

func TestCompare_Positive_AccessMemberMatchedAndExtra(t *testing.T) {
	def := loadDef(t)
	snap := snapshotOf(t, def)
	// Add MemberEntries to a matched channel (lounge)
	var matchedChannelID string
	for i := range snap.Channels {
		if snap.Channels[i].Name == "lounge" {
			snap.Channels[i].MemberEntries = 3
			matchedChannelID = "lounge"
			break
		}
	}

	// Add an extra channel with MemberEntries = 2
	snap.Channels = append(snap.Channels, Channel{
		Name:          "extra-channel",
		Type:          "text",
		Category:      "Community",
		MemberEntries: 2,
	})

	diffs, err := Compare(def, snap)
	if err != nil {
		t.Fatalf("Compare error: %v", err)
	}

	var foundMatched, foundExtra, foundZero bool
	var matchedIdx, extraIdx int
	for i, d := range diffs {
		if d.Kind == KindAccessMember && d.Object == fmt.Sprintf("channels[%s]", matchedChannelID) {
			foundMatched = true
			matchedIdx = i
			if d.Detail != "entries for single members: 3" {
				t.Errorf("matched detail = %q, want 'entries for single members: 3'", d.Detail)
			}
		}
		if d.Kind == KindAccessMember && d.Object == "server.channels[extra-channel]" {
			foundExtra = true
			extraIdx = i
			if d.Detail != "entries for single members: 2" {
				t.Errorf("extra detail = %q, want 'entries for single members: 2'", d.Detail)
			}
		}
		if d.Kind == KindAccessMember && strings.Contains(d.Object, "rules") {
			foundZero = true
		}
	}

	if !foundMatched {
		t.Error("missing KindAccessMember for matched channel lounge")
	}
	if !foundExtra {
		t.Error("missing KindAccessMember for extra channel extra-channel")
	}
	if foundZero {
		t.Error("found KindAccessMember for zero-member channel, want absent at 0")
	}

	// Verify order: extraIdx should be directly after server.channels[extra-channel] channel-extra
	if foundExtra && extraIdx > 0 {
		prev := diffs[extraIdx-1]
		if prev.Kind != KindChannelExtra || prev.Object != "server.channels[extra-channel]" {
			t.Errorf("diff before access-member extra is %+v, want channel-extra server.channels[extra-channel]", prev)
		}
	}
	_ = matchedIdx
}

func TestPlan_Positive_AccessMemberIgnored(t *testing.T) {
	def := loadDef(t)
	snap0 := snapshotOf(t, def)

	snap5 := snapshotOf(t, def)
	for i := range snap5.Channels {
		if snap5.Channels[i].Name == "lounge" {
			snap5.Channels[i].MemberEntries = 5
			break
		}
	}

	r0, err := Plan(def, snap0)
	if err != nil {
		t.Fatalf("Plan(snap0): %v", err)
	}
	r5, err := Plan(def, snap5)
	if err != nil {
		t.Fatalf("Plan(snap5): %v", err)
	}

	if len(r0.Changes) != len(r5.Changes) || len(r0.Refused) != len(r5.Refused) {
		t.Errorf("Plan results differ between MemberEntries 0 and 5: r0=%+v, r5=%+v", r0, r5)
	}
}

func TestRenderDifferences_Positive_Formats(t *testing.T) {
	t.Run("zero differences own mode", func(t *testing.T) {
		var buf bytes.Buffer
		if err := RenderDifferences(&buf, false, nil); err != nil {
			t.Fatalf("RenderDifferences: %v", err)
		}
		want := "mode: own\ndifferences: 0\n"
		if buf.String() != want {
			t.Errorf("got %q, want %q", buf.String(), want)
		}
	})

	t.Run("one difference without detail raised mode", func(t *testing.T) {
		var buf bytes.Buffer
		diffs := []Difference{
			{Kind: KindRoleMissing, Object: "roles[operator]"},
		}
		if err := RenderDifferences(&buf, true, diffs); err != nil {
			t.Fatalf("RenderDifferences: %v", err)
		}
		want := "mode: raised\ndifferences: 1\n  role-missing roles[operator]\n"
		if buf.String() != want {
			t.Errorf("got %q, want %q", buf.String(), want)
		}
	})

	t.Run("two differences with detail own mode", func(t *testing.T) {
		var buf bytes.Buffer
		diffs := []Difference{
			{Kind: KindRoleMissing, Object: "roles[operator]"},
			{Kind: KindAccessMember, Object: "channels[general]", Detail: "entries for single members: 3"},
		}
		if err := RenderDifferences(&buf, false, diffs); err != nil {
			t.Fatalf("RenderDifferences: %v", err)
		}
		want := "mode: own\ndifferences: 2\n  role-missing roles[operator]\n  access-member channels[general]; entries for single members: 3\n"
		if buf.String() != want {
			t.Errorf("got %q, want %q", buf.String(), want)
		}
	})
}

func TestDigest_Positive_Sha256Format(t *testing.T) {
	r1 := Result{
		Raised: false,
		Changes: []Change{
			{Step: 1, Action: "create-role", Object: "roles[dev]", Needs: []string{"MANAGE_ROLES"}, Detail: "info"},
		},
	}
	r2 := Result{
		Raised: false,
		Changes: []Change{
			{Step: 1, Action: "create-role", Object: "roles[dev]", Needs: []string{"MANAGE_ROLES"}, Detail: "infx"},
		},
	}
	const defA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const defB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	d1, err := Digest(r1, defA)
	if err != nil {
		t.Fatalf("Digest(r1, defA): %v", err)
	}
	d1Repeat, err := Digest(r1, defA)
	if err != nil {
		t.Fatalf("Digest(r1Repeat, defA): %v", err)
	}
	if d1 != d1Repeat {
		t.Errorf("Digest not stable: %q vs %q", d1, d1Repeat)
	}

	dDefDiff, err := Digest(r1, defB)
	if err != nil {
		t.Fatalf("Digest(r1, defB): %v", err)
	}
	if d1 == dDefDiff {
		t.Errorf("Digest equal when only definition differs: %q", d1)
	}

	d2, err := Digest(r2, defA)
	if err != nil {
		t.Fatalf("Digest(r2, defA): %v", err)
	}
	if d1 == d2 {
		t.Errorf("Digest equal for differing detail: %q", d1)
	}

	if !strings.HasPrefix(d1, "sha256:") || len(d1) != len("sha256:")+64 {
		t.Errorf("Digest format invalid: %q (len %d)", d1, len(d1))
	}

	var buf bytes.Buffer
	if err := Render(&buf, r1); err != nil {
		t.Fatalf("Render: %v", err)
	}
	buf.WriteString("definition: " + defA + "\n")
	expectedHex := fmt.Sprintf("sha256:%x", sha256.Sum256(buf.Bytes()))
	if d1 != expectedHex {
		t.Errorf("Digest = %q, want %q", d1, expectedHex)
	}
}

func TestRaised_Boundary_ThreeRightsVsOneMore(t *testing.T) {
	def := sampleDefWithBot() // Bot permissions: MANAGE_ROLES, MANAGE_MESSAGES, MODERATE_MEMBERS

	snapThree := &Snapshot{
		Caller: Caller{
			Role:        "Diallak",
			Permissions: []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS"},
		},
		Roles: []Role{
			{
				Name:        "Diallak",
				Everyone:    false,
				Permissions: []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS"},
			},
			{
				Name:        "@everyone",
				Everyone:    true,
				Permissions: []string{"VIEW_CHANNEL"},
			},
		},
		Categories:       []Category{{Name: "General"}},
		ChannelsComplete: true,
	}

	if Raised(def, snapThree) {
		t.Errorf("Raised(def, snapThree) = true, want false for exact 3 rights")
	}

	snapFour := &Snapshot{
		Caller: Caller{
			Role:        "Diallak",
			Permissions: []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "ADMINISTRATOR"},
		},
		Roles: []Role{
			{
				Name:        "Diallak",
				Everyone:    false,
				Permissions: []string{"MANAGE_ROLES", "MANAGE_MESSAGES", "MODERATE_MEMBERS", "ADMINISTRATOR"},
			},
			{
				Name:        "@everyone",
				Everyone:    true,
				Permissions: []string{"VIEW_CHANNEL"},
			},
		},
		Categories:       []Category{{Name: "General"}},
		ChannelsComplete: true,
	}

	if !Raised(def, snapFour) {
		t.Errorf("Raised(def, snapFour) = false, want true for 3 rights + 1 more")
	}
}

func TestCompare_Boundary_AccessMemberAtOneEntry(t *testing.T) {
	def := loadDef(t)
	snap := snapshotOf(t, def)
	for i := range snap.Channels {
		if snap.Channels[i].Name == "lounge" {
			snap.Channels[i].MemberEntries = 1
		}
	}
	snap.Channels = append(snap.Channels, Channel{
		Name:          "extra-channel",
		Type:          "text",
		Category:      "Community",
		MemberEntries: 1,
	})

	diffs, err := Compare(def, snap)
	if err != nil {
		t.Fatalf("Compare error: %v", err)
	}
	found := make(map[string]string, 2)
	for _, d := range diffs {
		if d.Kind == KindAccessMember {
			found[d.Object] = d.Detail
		}
	}
	want := map[string]string{
		"channels[lounge]":               "entries for single members: 1",
		"server.channels[extra-channel]": "entries for single members: 1",
	}
	if len(found) != len(want) {
		t.Fatalf("access-member differences = %v, want %v", found, want)
	}
	for object, detail := range want {
		if found[object] != detail {
			t.Errorf("access-member detail of %s = %q, want %q", object, found[object], detail)
		}
	}
}
