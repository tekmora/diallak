package config

import (
	"strings"
	"testing"
)

func modelEnv(t *testing.T) map[string]string {
	t.Helper()
	base := validBaseEnv(t)
	base[EnvModelBaseURL] = "http://127.0.0.1:8000"
	base[EnvModelAlias] = "router/test"
	base[EnvModelPolicy] = "example-policy"
	return base
}

func TestLoad_Positive_ModelGuardrail(t *testing.T) {
	base := modelEnv(t)
	base[EnvModelGuardrail] = "presidio-pii"
	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ModelGuardrail != "presidio-pii" {
		t.Errorf("ModelGuardrail = %q, want presidio-pii", cfg.ModelGuardrail)
	}
	if !strings.Contains(cfg.String(), "presidio-pii") {
		t.Errorf("String() = %q, want the guardrail name", cfg.String())
	}

	cfg, err = Load(env(modelEnv(t)))
	if err != nil || cfg.ModelGuardrail != "" {
		t.Errorf("Load() without the guardrail = %q, %v; want none", cfg.ModelGuardrail, err)
	}
}

func TestLoad_Negative_ModelGuardrail(t *testing.T) {
	cases := map[string]map[string]string{
		"comma":    {EnvModelGuardrail: "presidio,pii"},
		"space":    {EnvModelGuardrail: "presidio pii"},
		"129":      {EnvModelGuardrail: strings.Repeat("g", 129)},
		"no model": {EnvModelBaseURL: "", EnvModelAlias: "", EnvModelPolicy: "", EnvModelGuardrail: "presidio-pii"},
	}
	for name, over := range cases {
		t.Run(name, func(t *testing.T) {
			base := overEnv(modelEnv(t), over)
			cfg, err := Load(env(base))
			if err == nil || !strings.Contains(err.Error(), EnvModelGuardrail) {
				t.Fatalf("Load() error = %v, want one naming %s", err, EnvModelGuardrail)
			}
			if cfg != (Config{}) {
				t.Errorf("Load returned non-zero Config on error: %+v", cfg)
			}
			if v := over[EnvModelGuardrail]; name != "no model" && strings.Contains(err.Error(), v) {
				t.Errorf("error %q echoes the rejected value", err)
			}
		})
	}
}

// overEnv sets the values of over in base; an empty value removes the variable.
func overEnv(base, over map[string]string) map[string]string {
	for k, v := range over {
		if v == "" {
			delete(base, k)
			continue
		}
		base[k] = v
	}
	return base
}

func TestLoad_Boundary_ModelGuardrailLength(t *testing.T) {
	base := modelEnv(t)
	name := "a.b_c:d/e-" + strings.Repeat("g", 118)
	base[EnvModelGuardrail] = name
	cfg, err := Load(env(base))
	if err != nil || cfg.ModelGuardrail != name {
		t.Errorf("Load() with a 128-character guardrail = %q, %v", cfg.ModelGuardrail, err)
	}
}
