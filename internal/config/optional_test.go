package config

import (
	"errors"
	"strings"
	"testing"
	"time"

	envparse "github.com/caarlos0/env/v11"
)

func TestLoad_Negative_InvalidModelTimeoutIsEnvParseError(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvModelTimeout] = "ten seconds"

	_, err := Load(env(base))
	if err == nil {
		t.Fatal("Load accepted an invalid duration")
	}
	var parseErr envparse.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("Load = %v, want an env.ParseError in the chain", err)
	}
	if !strings.Contains(err.Error(), EnvModelTimeout) || !strings.Contains(err.Error(), "ten seconds") {
		t.Errorf("Load = %v, want the variable name and the value", err)
	}
}

func TestLoad_Negative_InvalidIntegerIsEnvParseError(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvTopPassages] = "five"

	_, err := Load(env(base))
	var parseErr envparse.ParseError
	if !errors.As(err, &parseErr) || !strings.Contains(err.Error(), EnvTopPassages) {
		t.Fatalf("Load = %v, want an env.ParseError naming %s", err, EnvTopPassages)
	}
}

func TestLoad_Boundary_ExplicitZeroIsCheckedNotUnset(t *testing.T) {
	// A value of zero that is set must still meet the range; only an unset variable means "default".
	base := validBaseEnv(t)
	base[EnvTopPassages] = "0"

	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), "outside accepted range") {
		t.Fatalf("Load = %v, want the range error for %s=0", err, EnvTopPassages)
	}
}

func TestLoad_Positive_TrailRetentionParsed(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvTrailRetention] = "48h"

	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if cfg.TrailRetention != 48*time.Hour {
		t.Errorf("TrailRetention = %v, want 48h", cfg.TrailRetention)
	}
}

func TestReadOptional_Boundary_EveryFieldNamesADeclaredVariable(t *testing.T) {
	// The struct tags and the Env* constants must name the same variables.
	declared := map[string]bool{
		EnvTopPassages: true, EnvMinScore: true, EnvQuestionMaxChars: true, EnvExcerptChars: true,
		EnvReplyMaxChars: true, EnvMaxConcurrentAsks: true, EnvAskTimeout: true, EnvModelTimeout: true,
		EnvTrailRetention: true, EnvEmbedTimeout: true, EnvEmbedMinSimilarity: true, EnvSupporterGrace: true,
	}
	keys, err := optionalKeys()
	if err != nil {
		t.Fatalf("optionalKeys = %v", err)
	}
	if len(keys) != len(declared) {
		t.Fatalf("optionalKeys = %v, want %d keys", keys, len(declared))
	}
	for _, k := range keys {
		if !declared[k] {
			t.Errorf("struct tag %q names no declared variable", k)
		}
	}
}
