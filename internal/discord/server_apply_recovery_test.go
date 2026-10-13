package discord

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

const (
	recoveryGuildID   uint64 = 1001
	recoveryBotUserID uint64 = 2001
	recoveryBotRoleID uint64 = 3001
)

// recoveryRun is one server the tests fail an apply on and then recover, as
// docs/server-apply.md describes: the changes made before the failure stay,
// and a new plan shows what is left.
type recoveryRun struct {
	t         *testing.T
	fake      *fakePlatform
	url       string
	token     string
	def       *serverdef.Definition
	defDigest string
}

func newRecoveryRun(t *testing.T) *recoveryRun {
	t.Helper()
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	fake := setupFreshServerFake(t, recoveryGuildID, recoveryBotUserID, recoveryBotRoleID, true)
	srv := fake.server()
	t.Cleanup(srv.Close)
	return &recoveryRun{
		t: t, fake: fake, url: srv.URL, token: makeFakeToken(recoveryBotUserID),
		def: def, defDigest: server.DefinitionDigest(),
	}
}

func (r *recoveryRun) plan() (serverplan.Result, string) {
	r.t.Helper()
	snap, err := readServerWithOptions(context.Background(), r.token, recoveryGuildID, clientOptions{restURL: r.url})
	if err != nil {
		r.t.Fatalf("read: %v", err)
	}
	res, err := serverplan.Plan(r.def, snap)
	if err != nil {
		r.t.Fatalf("plan: %v", err)
	}
	digest, err := serverplan.Digest(res, r.defDigest)
	if err != nil {
		r.t.Fatalf("digest: %v", err)
	}
	return res, digest
}

func (r *recoveryRun) apply(planDigest string) (string, error) {
	r.t.Helper()
	var out bytes.Buffer
	err := applyServerWithOptions(context.Background(), r.token, recoveryGuildID, ApplyInput{
		Definition:       r.def,
		DefinitionDigest: r.defDigest,
		PlanDigest:       planDigest,
	}, &out, clientOptions{restURL: r.url})
	return out.String(), err
}

// failAt makes the k-th write from now fail with Missing Permissions.
func (r *recoveryRun) failAt(k int) {
	r.fake.mu.Lock()
	defer r.fake.mu.Unlock()
	r.fake.failWrite = r.fake.writeCount + k
	r.fake.failStatus = http.StatusForbidden
	r.fake.failBody = `{"code":50013,"message":"Missing Permissions"}`
}

func (r *recoveryRun) writes() int {
	r.fake.mu.Lock()
	defer r.fake.mu.Unlock()
	return r.fake.writeCount
}

func changeKeys(changes []serverplan.Change) []string {
	keys := make([]string, 0, len(changes))
	for i := 0; i < len(changes); i++ {
		keys = append(keys, changes[i].Action+" "+changes[i].Object)
	}
	return keys
}

// failThenRecover fails the k-th change of a fresh server's plan, checks that
// the new plan lists exactly the changes from k on, applies it and checks that
// a third plan has nothing left.
func failThenRecover(t *testing.T, k func(n int) int) {
	t.Helper()
	r := newRecoveryRun(t)
	first, firstDigest := r.plan()
	n := len(first.Changes)
	fail := k(n)
	r.failAt(fail)
	if _, err := r.apply(firstDigest); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("step %d", fail)) {
		t.Fatalf("first apply: error %v, want the failure at step %d", err, fail)
	}

	left, leftDigest := r.plan()
	want := strings.Join(changeKeys(first.Changes[fail-1:]), "\n")
	if got := strings.Join(changeKeys(left.Changes), "\n"); got != want {
		t.Fatalf("plan after a failure at step %d of %d lists:\n%s\nwant the changes from step %d on:\n%s", fail, n, got, fail, want)
	}

	before := r.writes()
	out, err := r.apply(leftDigest)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if got := r.writes() - before; got != len(left.Changes) {
		t.Errorf("second apply sent %d writes, want %d", got, len(left.Changes))
	}
	if wantLine := fmt.Sprintf("applied: %d of %d", len(left.Changes), len(left.Changes)); !strings.Contains(out, wantLine) {
		t.Errorf("second apply output lacks %q:\n%s", wantLine, out)
	}
	if done, _ := r.plan(); len(done.Changes) != 0 {
		t.Errorf("plan after recovery lists %d changes, want none: %v", len(done.Changes), changeKeys(done.Changes))
	}
}

func TestApplyServer_Positive_PlanAfterAFailureShowsWhatIsLeft(t *testing.T) {
	failThenRecover(t, func(n int) int { return n / 2 })
}

func TestApplyServer_Boundary_FailureOnTheFirstAndTheLastChange(t *testing.T) {
	t.Run("first change", func(t *testing.T) {
		failThenRecover(t, func(int) int { return 1 })
	})
	t.Run("last change", func(t *testing.T) {
		failThenRecover(t, func(n int) int { return n })
	})
}

func TestApplyServer_Negative_PlanReadBeforeAFailureIsRefused(t *testing.T) {
	r := newRecoveryRun(t)
	first, firstDigest := r.plan()
	r.failAt(len(first.Changes) / 2)
	if _, err := r.apply(firstDigest); err == nil {
		t.Fatal("first apply: want the planted failure, got nil")
	}

	before := r.writes()
	_, err := r.apply(firstDigest)
	if err == nil || !strings.Contains(err.Error(), "the plan is not the one that was shown") {
		t.Fatalf("apply with the plan read before the failure: error %v, want the stale plan refused", err)
	}
	if got := r.writes() - before; got != 0 {
		t.Errorf("refused apply sent %d writes, want none", got)
	}
}
