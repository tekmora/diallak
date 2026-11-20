package serverdef

import (
	"strings"
	"testing"
)

func TestInviteStrictDecoding(t *testing.T) {
	_, err := Load(strings.NewReader(`{"invites":[{"id":"hub","channel":"rules","purpose":"hub","max_age":0,"max_uses":0,"temporary":false}]}`))
	if err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	_, err = Load(strings.NewReader(`{"invites":[{"id":"hub","unknown":true}]}`))
	if err == nil {
		t.Fatal("unknown invite field accepted")
	}
}

func TestInviteValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Definition)
		code   string
		want   bool
	}{
		{"valid", func(*Definition) {}, CodeInvite, false},
		{"valid bot grant", func(*Definition) {}, CodeBotRights, false},
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
		{"max age boundary", func(d *Definition) { d.Invites[0].MaxAge = 604800 }, CodeInvite, false},
		{"max age over", func(d *Definition) { d.Invites[0].MaxAge = 604801 }, CodeInvite, true},
		{"max uses boundary", func(d *Definition) { d.Invites[0].MaxUses = 100 }, CodeInvite, false},
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
