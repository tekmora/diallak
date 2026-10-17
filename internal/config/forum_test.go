package config

import (
	"strings"
	"testing"
)

const (
	testForumChannel = "123456789012345678"
	testForumTag     = "234567890123456789"
)

// ADR-0009 "One place, one moment": without the channel setting the forum answer is off.
func TestLoad_Positive_ForumSettings(t *testing.T) {
	cfg, err := Load(env(validBaseEnv(t)))
	if err != nil || cfg.ForumChannelID != 0 || cfg.ForumOptInTagID != 0 || cfg.HasForum() {
		t.Fatalf("Load without the forum = %+v, %v; want both IDs zero and the forum off", cfg, err)
	}
	base := validBaseEnv(t)
	base[EnvForumChannelID] = testForumChannel
	base[EnvForumOptInTagID] = testForumTag
	cfg, err = Load(env(base))
	if err != nil {
		t.Fatalf("Load with the forum: %v", err)
	}
	if cfg.ForumChannelID != 123456789012345678 || cfg.ForumOptInTagID != 234567890123456789 || !cfg.HasForum() {
		t.Errorf("forum = %d, %d; want the two configured IDs and the forum on", cfg.ForumChannelID, cfg.ForumOptInTagID)
	}
	if s := cfg.String(); !strings.Contains(s, "ForumChannelID:"+testForumChannel) || !strings.Contains(s, "ForumOptInTagID:"+testForumTag) {
		t.Errorf("String() misses the forum settings: %s", s)
	}
}

// ADR-0009 case 18, as ADR-0020 amends it: a forum without the opt-in tag stops the start, and a
// tag without a forum is a setting that does nothing, so it stops the start too.
func TestLoad_Negative_ForumSettingsRefused(t *testing.T) {
	cases := []struct {
		name, channel, tag, wantVar string
	}{
		{"channel without tag", testForumChannel, "", EnvForumOptInTagID},
		{"tag without channel", "", testForumTag, EnvForumChannelID},
		{"channel not a number", "help", testForumTag, EnvForumChannelID},
		{"channel zero", "0", testForumTag, EnvForumChannelID},
		{"channel signed", "+1", testForumTag, EnvForumChannelID},
		{"tag not a number", testForumChannel, "tag", EnvForumOptInTagID},
		{"tag zero", testForumChannel, "0", EnvForumOptInTagID},
		{"tag over range", testForumChannel, "18446744073709551616", EnvForumOptInTagID},
	}
	for _, tc := range cases {
		base := validBaseEnv(t)
		base[EnvForumChannelID] = tc.channel
		base[EnvForumOptInTagID] = tc.tag
		if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), tc.wantVar) {
			t.Errorf("%s: Load = %v, want an error naming %s", tc.name, err, tc.wantVar)
		}
	}
}

func TestLoad_Boundary_ForumSnowflakeRange(t *testing.T) {
	for _, raw := range []string{"1", "18446744073709551615"} {
		base := validBaseEnv(t)
		base[EnvForumChannelID] = raw
		base[EnvForumOptInTagID] = raw
		if _, err := Load(env(base)); err != nil {
			t.Errorf("Load with IDs %q: %v, want accepted", raw, err)
		}
	}
}
