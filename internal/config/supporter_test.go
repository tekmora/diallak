package config

import (
	"strings"
	"testing"
	"time"
)

// ADR-0024, ADR-0027: the supporter function is off unless switched on, and its grace period is
// read only with it.
func TestLoad_Positive_SupporterRoles(t *testing.T) {
	cfg, err := Load(env(validBaseEnv(t)))
	if err != nil || cfg.HasSupporterRoles() || cfg.SupporterGrace != 0 {
		t.Fatalf("Load without %s = %v, %v, %v; want off", EnvSupporterRoles, cfg.SupporterRoles, cfg.SupporterGrace, err)
	}
	base := validBaseEnv(t)
	base[EnvSupporterRoles] = "on"
	cfg, err = Load(env(base))
	if err != nil || !cfg.HasSupporterRoles() || cfg.SupporterGrace != 0 {
		t.Fatalf("Load with on = %v, %v, %v; want on with the default grace", cfg.SupporterRoles, cfg.SupporterGrace, err)
	}
	base[EnvSupporterGrace] = "96h"
	cfg, err = Load(env(base))
	if err != nil || cfg.SupporterGrace != 96*time.Hour {
		t.Fatalf("Load with 96h = %v, %v; want 96h", cfg.SupporterGrace, err)
	}
	for _, want := range []string{"SupporterRoles:true", "SupporterGrace:96h0m0s"} {
		if !strings.Contains(cfg.String(), want) {
			t.Errorf("String() misses %s: %s", want, cfg.String())
		}
	}
}

func TestLoad_Negative_SupporterRolesRefused(t *testing.T) {
	cases := map[string]map[string]string{
		"switch not on":        {EnvSupporterRoles: "true"},
		"switch upper case":    {EnvSupporterRoles: "ON"},
		"grace without switch": {EnvSupporterGrace: "72h"},
		"grace not a duration": {EnvSupporterRoles: "on", EnvSupporterGrace: "three days"},
		"grace negative":       {EnvSupporterRoles: "on", EnvSupporterGrace: "-72h"},
	}
	for name, vars := range cases {
		base := validBaseEnv(t)
		for k, v := range vars {
			base[k] = v
		}
		if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), "DIALLAK_SUPPORTER_") {
			t.Errorf("%s: Load = %v, want an error naming the supporter variable", name, err)
		}
	}
}

func TestLoad_Boundary_SupporterGraceBounds(t *testing.T) {
	for raw, ok := range map[string]bool{"24h": true, "2160h": true, "23h59m59s": false, "2160h0m1s": false} {
		base := validBaseEnv(t)
		base[EnvSupporterRoles], base[EnvSupporterGrace] = "on", raw
		_, err := Load(env(base))
		if (err == nil) != ok {
			t.Errorf("Load with grace %q = %v, want accepted %v", raw, err, ok)
		}
	}
}
