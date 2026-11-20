package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/serverplan"
)

func TestApplyCreateInvite_Positive_BodyAndOutput(t *testing.T) {
	def := mustDefinition(t)
	fx := newChangeFixture(t, def)
	srv := fx.fake.server()
	defer srv.Close()
	client, err := newRestClient(makeFakeToken(orderBotUserID), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close(context.Background())
	change := serverplan.Change{Step: 1, Action: serverplan.ActionCreateInvite, Object: "invites[hub]"}
	if err := validateChangeObject(def, change); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = executePlan(context.Background(), client, orderGuildID, ApplyInput{Definition: def, PlanDigest: orderDigest}, []serverplan.Change{change}, fx.idx, &output, clientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "invite code: new-code") {
		t.Fatalf("output: %s", output.String())
	}
	call := oneWrite(t, fx.fake)
	var body map[string]any
	if err := json.Unmarshal(call.body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 2 || body["max_age"] != float64(0) || body["max_uses"] != float64(0) {
		t.Fatalf("body: %s", call.body)
	}
	if outside := callsOutsideList(fx.fake.getRecorded()); len(outside) != 0 {
		t.Fatalf("unexpected calls: %v", outside)
	}
}

func TestApplyCreateInvite_Negative_InvalidObjects(t *testing.T) {
	for _, object := range []string{"invites[missing]", "invites[hub]"} {
		t.Run(object, func(t *testing.T) {
			def := mustDefinition(t)
			if object == "invites[hub]" {
				def.Invites[0].Code = "published"
			}
			fx := newChangeFixture(t, def)
			c := serverplan.Change{Action: serverplan.ActionCreateInvite, Object: object}
			if err := validateChangeObject(def, c); err == nil {
				t.Fatal("invalid invite accepted")
			}
			if err := runChange(t, fx.fake, def, fx.idx, c, time.Second); err == nil {
				t.Fatal("invalid invite executed")
			}
			if calls := fx.fake.getRecorded(); len(calls) != 0 {
				t.Fatalf("unexpected calls: %+v", calls)
			}
		})
	}
}
