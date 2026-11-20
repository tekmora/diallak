package serverdef

import (
	"strings"
	"testing"
)

func TestInviteStrictDecoding_Positive_KnownFields(t *testing.T) {
	_, err := Load(strings.NewReader(`{"invites":[{"id":"hub","channel":"rules","purpose":"hub","max_age":0,"max_uses":0,"temporary":false}]}`))
	if err != nil {
		t.Fatalf("decode invite: %v", err)
	}
}
func TestInviteStrictDecoding_Negative_UnknownField(t *testing.T) {
	_, err := Load(strings.NewReader(`{"invites":[{"id":"hub","unknown":true}]}`))
	if err == nil {
		t.Fatal("unknown invite field accepted")
	}
}

func TestInviteValidation_Positive_Valid(t *testing.T) {
	d := valid(t)
	for _, code := range []string{CodeInvite, CodeBotRights} {
		if hasCode(Validate(d), code) {
			t.Errorf("unexpected %s finding", code)
		}
	}
}
func TestInviteValidation_Boundary_MaximumLimits(t *testing.T) {
	d := valid(t)
	d.Invites[0].MaxAge = 604800
	d.Invites[0].MaxUses = 100
	if hasCode(Validate(d), CodeInvite) {
		t.Fatal("maximum limits rejected")
	}
}

func TestInviteValidation_Negative_InvalidFields(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Definition)
		code   string
		want   bool
	}{
		{"unknown channel", func(d *Definition) { d.Invites[0].Channel = "missing" }, CodeInvite, true},
		{"voice", func(d *Definition) {
			for i := range d.Channels {
				if d.Channels[i].ID == "rules" {
					d.Channels[i].Type = "voice"
				}
			}
		}, CodeInvite, true},
		{"forum", func(d *Definition) {
			for i := range d.Channels {
				if d.Channels[i].ID == "rules" {
					d.Channels[i].Type = "forum"
				}
			}
		}, CodeInvite, true},
		{"blank", func(d *Definition) { d.Invites[0].Purpose = " " }, CodeInvite, true},
		{"duplicate", func(d *Definition) { d.Invites = append(d.Invites, d.Invites[0]) }, CodeInvite, true},
		{"bad id", func(d *Definition) { d.Invites[0].ID = "!" }, CodeInvite, true},
		{"max age over", func(d *Definition) { d.Invites[0].MaxAge = 604801 }, CodeInvite, true},
		{"max uses over", func(d *Definition) { d.Invites[0].MaxUses = 101 }, CodeInvite, true},
		{"bad code", func(d *Definition) { d.Invites[0].Code = "bad_" }, CodeInvite, true},
		{"no bot grant", func(d *Definition) {
			findChannel(d.Channels, "rules").Access = findChannel(d.Channels, "rules").Access[:1]
		}, CodeBotRights, true},
		{"grant outside invite", func(d *Definition) { d.Invites = nil }, CodeBotRights, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := valid(t)
			tc.change(d)
			if got := hasCode(Validate(d), tc.code); got != tc.want {
				t.Errorf("finding %s = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestInviteValidation_Negative_DuplicateCode(t *testing.T) {
	d := valid(t)
	d.Invites[0].Code = "abc"
	second := d.Invites[0]
	second.ID = "second"
	d.Invites = append(d.Invites, second)
	if !hasCode(Validate(d), CodeInvite) {
		t.Fatal("duplicate invite code accepted")
	}
}
func TestInviteValidation_Boundary_CodeLength(t *testing.T) {
	for _, tc := range []struct {
		length int
		want   bool
	}{{32, false}, {33, true}} {
		d := valid(t)
		d.Invites[0].Code = strings.Repeat("a", tc.length)
		if got := hasCode(Validate(d), CodeInvite); got != tc.want {
			t.Errorf("length %d: got %v", tc.length, got)
		}
	}
}
