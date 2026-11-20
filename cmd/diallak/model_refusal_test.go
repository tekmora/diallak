package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/discord"
)

func envWithOverrides(base func(string) string, overrides map[string]string) func(string) string {
	return func(name string) string {
		if value, ok := overrides[name]; ok {
			return value
		}
		return base(name)
	}
}

func TestRun_Positive_UnsetModelVariablesStart(t *testing.T) {
	deps := noopDeps()
	deps.refuseModelPath = true
	if err := run(context.Background(), nil, validEnv(t), &bytes.Buffer{}, deps); err != nil {
		t.Fatalf("run with unset variables: %v", err)
	}
}

func TestRun_Negative_EachModelVariableIsRefused(t *testing.T) {
	for _, name := range config.ModelVariables() {
		t.Run(name, func(t *testing.T) {
			for _, args := range [][]string{nil, {"-register-commands"}} {
				called := false
				deps := noopDeps()
				deps.refuseModelPath = true
				deps.runDiscord = func(context.Context, discord.BotConfig) error { called = true; return nil }
				deps.registerCommands = func(context.Context, string, uint64, int) error { called = true; return nil }
				getenv := envWithOverrides(validEnv(t), map[string]string{name: "set"})
				err := run(context.Background(), args, getenv, &bytes.Buffer{}, deps)
				if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "ADR-0028") {
					t.Errorf("run(%v) = %v, want refusal naming %s and ADR-0028", args, err, name)
				}
				if called {
					t.Errorf("run(%v) called a mode after refusal", args)
				}
			}
		})
	}
}

func TestRun_Negative_TwoModelVariablesInListOrder(t *testing.T) {
	deps := noopDeps()
	deps.refuseModelPath = true
	getenv := envWithOverrides(validEnv(t), map[string]string{config.EnvModelAlias: "x", config.EnvEmbedAlias: "y"})
	err := run(context.Background(), nil, getenv, &bytes.Buffer{}, deps)
	if err == nil || !strings.Contains(err.Error(), config.EnvModelAlias+", "+config.EnvEmbedAlias) {
		t.Fatalf("run = %v, want both variables in list order", err)
	}
}

func TestRun_Boundary_WhitespaceModelVariableIsSet(t *testing.T) {
	deps := noopDeps()
	deps.refuseModelPath = true
	getenv := envWithOverrides(validEnv(t), map[string]string{config.EnvModelAlias: " \t"})
	err := run(context.Background(), nil, getenv, &bytes.Buffer{}, deps)
	if err == nil || !strings.Contains(err.Error(), config.EnvModelAlias) || !strings.Contains(err.Error(), "ADR-0028") {
		t.Fatalf("run = %v, want whitespace variable refused by the version 1 check", err)
	}
}

func TestRun_Boundary_ModelPathOnKeepsModelMode(t *testing.T) {
	getenv := envWithOverrides(validEnv(t), map[string]string{
		config.EnvModelBaseURL: "http://127.0.0.1:8000",
		config.EnvModelAlias:   "router/test", config.EnvModelPolicy: "example-policy",
	})
	var out bytes.Buffer
	if err := run(context.Background(), nil, getenv, &out, noopDeps()); err != nil {
		t.Fatalf("run with model path enabled: %v", err)
	}
	if !strings.Contains(out.String(), "model not ready") {
		t.Errorf("output = %q, want model path", out.String())
	}
}

func TestDefaultDependencies_Positive_ProductionRefusesModelVariables(t *testing.T) {
	if !defaultDependencies().refuseModelPath {
		t.Fatal("defaultDependencies does not refuse the model variables")
	}
}
