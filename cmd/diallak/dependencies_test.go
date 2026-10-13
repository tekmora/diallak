package main

import (
	"reflect"
	"testing"

	"github.com/tekmora/diallak/internal/discord"
)

// The tests of run inject stand-ins for the four calls to Discord. This test
// pins what main injects instead: the exported entry points of internal/discord.
// ReadServer and ApplyServer run there over Discord's address against a fake
// (internal/discord/server_entry_test.go).
func TestDefaultDependencies_Positive_MainCallsTheEntryPointsOfDiscord(t *testing.T) {
	deps := defaultDependencies()
	cases := []struct {
		name      string
		got, want any
	}{
		{"registerCommands", deps.registerCommands, discord.RegisterCommands},
		{"runDiscord", deps.runDiscord, discord.Run},
		{"readServer", deps.readServer, discord.ReadServer},
		{"applyServer", deps.applyServer, discord.ApplyServer},
	}
	for _, tc := range cases {
		if !sameFunction(tc.got, tc.want) {
			t.Errorf("defaultDependencies().%s is not the exported entry point of internal/discord", tc.name)
		}
	}
}

func TestDefaultDependencies_Negative_AnotherFunctionIsNotTheEntryPoint(t *testing.T) {
	wrapped := func() {}
	if sameFunction(wrapped, discord.ReadServer) {
		t.Fatal("a function literal compares equal to discord.ReadServer")
	}
}

func TestDefaultDependencies_Boundary_EveryCallIsSet(t *testing.T) {
	deps := reflect.ValueOf(defaultDependencies())
	for i := range deps.NumField() {
		if deps.Field(i).IsNil() {
			t.Errorf("defaultDependencies().%s is nil", deps.Type().Field(i).Name)
		}
	}
}

// sameFunction reports whether two function values have the same code. A
// function literal that calls the other one has its own code.
func sameFunction(a, b any) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	return va.Kind() == reflect.Func && vb.Kind() == reflect.Func && !va.IsNil() && !vb.IsNil() &&
		va.Pointer() == vb.Pointer()
}
