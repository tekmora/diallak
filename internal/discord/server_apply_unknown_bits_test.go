package discord

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

// everyoneWithBit47 is VIEW_CHANNEL plus bit 47, which the permission table does not name.
const everyoneWithBit47 = "140737488356352"

// applyOnFake reads the fake server, plans against the definition and applies
// that plan. It returns the plan, the write requests the fake recorded during
// the apply and the error of the apply.
func applyOnFake(t *testing.T, fake *fakePlatform, guildID, botUserID uint64) (serverplan.Result, []recordedCall, error) {
	t.Helper()
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	defDigest := server.DefinitionDigest()
	fakeToken := makeFakeToken(botUserID)
	srv := fake.server()
	defer srv.Close()

	snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("read server: %v", err)
	}
	res, err := serverplan.Plan(def, snap)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	digest, err := serverplan.Digest(res, defDigest)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}

	fake.resetRecorded()
	var out bytes.Buffer
	applyErr := applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
		Definition:       def,
		DefinitionDigest: defDigest,
		PlanDigest:       digest,
	}, &out, clientOptions{restURL: srv.URL})

	var writes []recordedCall
	for _, call := range fake.getRecorded() {
		if call.method != http.MethodGet {
			writes = append(writes, call)
		}
	}
	return res, writes, applyErr
}

func TestApplyServer_Negative_UnknownBitStopsTheRun(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)

	t.Run("control: without the bit the run writes", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.createShifts = true

		res, writes, err := applyOnFake(t, fake, guildID, botUserID)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if len(res.Refused) != 0 {
			t.Fatalf("plan holds %d refusals, want 0: %+v", len(res.Refused), res.Refused)
		}
		if len(writes) == 0 {
			t.Fatal("apply recorded no write, want the changes of the plan")
		}
	})

	t.Run("bit 47 on the everyone role", func(t *testing.T) {
		fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
		fake.createShifts = true
		fake.roles[0].permissions = everyoneWithBit47

		res, writes, err := applyOnFake(t, fake, guildID, botUserID)

		var found bool
		for _, refusal := range res.Refused {
			if refusal.Reason != serverplan.ReasonUnknownBits {
				continue
			}
			found = true
			if refusal.Object != "roles[everyone]" || refusal.Detail != "bits 47" {
				t.Errorf("refusal = %+v, want object roles[everyone] with detail \"bits 47\"", refusal)
			}
		}
		if !found {
			t.Fatalf("plan holds no %s refusal: %+v", serverplan.ReasonUnknownBits, res.Refused)
		}

		if err == nil {
			t.Fatal("apply succeeded on a server role with an unnamed bit, want error")
		}
		want := fmt.Sprintf("the plan holds %d refused changes; a plan with a refusal is not applied", len(res.Refused))
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
		if len(writes) != 0 {
			t.Errorf("apply recorded %d writes, want 0: first %s %s", len(writes), writes[0].method, writes[0].path)
		}
		if got := fake.roles[0].permissions; got != everyoneWithBit47 {
			t.Errorf("everyone role permissions = %q after the run, want %q", got, everyoneWithBit47)
		}
	})
}
