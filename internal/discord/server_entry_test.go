package discord

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

// The tests of reading and applying call readServerWithOptions and
// applyServerWithOptions with the address of a fake. The command calls the
// exported ReadServer and ApplyServer (cmd/diallak, defaultDependencies), which
// set no option and use the library's own address. The tests in this file call
// those and send that address to the same fake.

// discordAPI is the address the exported entry points use: the library's
// default, which this package never sets.
const discordAPI = "https://discord.com/api/v10"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// redirectDiscord sends every request for discordAPI to the fake at fakeURL
// until the test ends, and refuses a request for any other address. It replaces
// http.DefaultTransport, which the library's HTTP client uses, so the test must
// not run in parallel.
func redirectDiscord(t *testing.T, fakeURL string) {
	t.Helper()
	target, err := url.Parse(fakeURL)
	if err != nil {
		t.Fatalf("parse %q: %v", fakeURL, err)
	}
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		path, ok := strings.CutPrefix(r.URL.Path, "/api/v10/")
		if r.URL.Scheme != "https" || r.URL.Host != "discord.com" || !ok {
			return nil, fmt.Errorf("request for %s, want an address under %s", r.URL.Redacted(), discordAPI)
		}
		out := r.Clone(r.Context())
		out.URL.Scheme, out.URL.Host, out.URL.Path, out.URL.RawPath = target.Scheme, target.Host, "/"+path, ""
		out.Host = ""
		return orig.RoundTrip(out)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
}

// newEntryFixture is a fresh server on which the bot holds ADMINISTRATOR, so
// that the list of channels is complete on any day.
func newEntryFixture(t *testing.T) *fakePlatform {
	t.Helper()
	fake := setupFreshServerFake(t, orderGuildID, orderBotUserID, orderBotRoleID, true)
	srv := fake.server()
	t.Cleanup(srv.Close)
	redirectDiscord(t, srv.URL)
	return fake
}

// writes is the number of write requests the fake has received.
func writes(fake *fakePlatform) int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.writeCount
}

func TestReadServer_Positive_ExportedEntryReadsDiscordsAddress(t *testing.T) {
	fake := newEntryFixture(t)
	token := makeFakeToken(orderBotUserID)

	got, err := ReadServer(context.Background(), token, orderGuildID)
	if err != nil {
		t.Fatalf("ReadServer: %v", err)
	}
	if len(fake.getRecorded()) == 0 {
		t.Fatal("ReadServer sent no request to the fake")
	}
	want, err := readServerWithOptions(context.Background(), token, orderGuildID, clientOptions{restURL: discordAPI})
	if err != nil {
		t.Fatalf("readServerWithOptions: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadServer read\n%+v\nwant the snapshot of the unexported reader\n%+v", got, want)
	}
}

func TestApplyServer_Positive_ExportedEntryAppliesThePlanReadOverDiscordsAddress(t *testing.T) {
	fake := newEntryFixture(t)
	token := makeFakeToken(orderBotUserID)
	def := mustDefinition(t)

	before, err := ReadServer(context.Background(), token, orderGuildID)
	if err != nil {
		t.Fatalf("ReadServer before: %v", err)
	}
	result, err := serverplan.Plan(def, before)
	if err != nil {
		t.Fatalf("plan before: %v", err)
	}
	if len(result.Changes) == 0 {
		t.Fatal("the plan of a fresh server holds no change")
	}

	var out bytes.Buffer
	if err := ApplyServer(context.Background(), token, orderGuildID, gateInput(t, def, result), &out); err != nil {
		t.Fatalf("ApplyServer: %v\noutput:\n%s", err, out.String())
	}
	if out.Len() == 0 {
		t.Error("ApplyServer wrote no output")
	}

	after, err := ReadServer(context.Background(), token, orderGuildID)
	if err != nil {
		t.Fatalf("ReadServer after: %v", err)
	}
	second, err := serverplan.Plan(def, after)
	if err != nil {
		t.Fatalf("plan after: %v", err)
	}
	if len(second.Changes) != 0 {
		t.Errorf("the plan after the apply holds %d changes, want 0: %+v", len(second.Changes), second.Changes)
	}
	if writes(fake) == 0 {
		t.Error("ApplyServer wrote nothing to the fake")
	}
}

func TestApplyServer_Negative_ExportedEntryRefusesAnotherPlansDigest(t *testing.T) {
	fake := newEntryFixture(t)
	token := makeFakeToken(orderBotUserID)
	def := mustDefinition(t)

	in := ApplyInput{
		Definition:       def,
		DefinitionDigest: server.DefinitionDigest(),
		PlanDigest:       strings.Repeat("0", 64),
	}
	var out bytes.Buffer
	err := ApplyServer(context.Background(), token, orderGuildID, in, &out)
	if err == nil {
		t.Fatalf("ApplyServer applied a plan whose digest no person read; output:\n%s", out.String())
	}
	if len(fake.getRecorded()) == 0 {
		t.Error("ApplyServer refused without reading the server")
	}
	if n := writes(fake); n != 0 {
		t.Errorf("ApplyServer wrote %d times before refusing, want 0", n)
	}
}

func TestReadServer_Boundary_ExportedEntryChecksTheGuildBeforeAnyRequest(t *testing.T) {
	fake := newEntryFixture(t)

	if _, err := ReadServer(context.Background(), makeFakeToken(orderBotUserID), 0); err == nil {
		t.Fatal("ReadServer read guild 0")
	}
	if got := fake.getRecorded(); len(got) != 0 {
		t.Errorf("ReadServer sent %d requests for guild 0, want none: %+v", len(got), got)
	}
}
