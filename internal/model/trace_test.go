package model

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
)

func completeTraced(t *testing.T, headers http.Header, status int) (string, Trace, error) {
	t.Helper()
	server := runPolicyServer(headers, status, reviewBody)
	defer server.Close()
	c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: "example-policy"}, server.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c.CompleteTraced(context.Background(), "sys", "usr")
}

func TestCompleteTraced_Positive_TraceNamesTheGuardrailsAndPoliciesSorted(t *testing.T) {
	out, trace, err := completeTraced(t, http.Header{
		GuardrailHeader: []string{"prompt-injection, presidio-pii"},
		PolicyHeader:    []string{"other,example-policy"},
	}, http.StatusOK)
	if err != nil || out != "model-text" {
		t.Fatalf("CompleteTraced = %q, %v; want the reply", out, err)
	}
	if !trace.Sent || !trace.Replied || trace.PolicyMissing {
		t.Errorf("trace = %+v, want sent and replied with the policy named", trace)
	}
	if !slices.Equal(trace.Guardrails, []string{"presidio-pii", "prompt-injection"}) {
		t.Errorf("guardrails = %q, want both sorted by name", trace.Guardrails)
	}
	if !slices.Equal(trace.Policies, []string{"example-policy", "other"}) {
		t.Errorf("policies = %q, want both sorted by name", trace.Policies)
	}
}

func TestCompleteTraced_Negative_ReplyWithoutThePolicyIsMarkedAndDiscarded(t *testing.T) {
	out, trace, err := completeTraced(t, http.Header{GuardrailHeader: []string{"presidio-pii"}}, http.StatusOK)
	var polErr *PolicyError
	if out != "" || !errors.As(err, &polErr) {
		t.Fatalf("CompleteTraced = %q, %v; want no text and a PolicyError", out, err)
	}
	if !trace.Replied || !trace.PolicyMissing || len(trace.Policies) != 0 || len(trace.Guardrails) != 1 {
		t.Errorf("trace = %+v, want a reply with the policy missing and the guardrail named", trace)
	}
}

func TestCompleteTraced_Negative_NoReplyLeavesTheGuardrailsUnknown(t *testing.T) {
	server := runPolicyServer(nil, http.StatusOK, reviewBody)
	c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: "example-policy"}, server.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	server.Close()
	_, trace, err := c.CompleteTraced(context.Background(), "sys", "usr")
	if err == nil || !trace.Sent || trace.Replied || trace.Guardrails != nil {
		t.Errorf("trace = %+v, err = %v; want sent, no reply and no guardrails", trace, err)
	}
}

func TestCompleteTraced_Boundary_ErrorStatusStillNamesTheGuardrails(t *testing.T) {
	_, trace, err := completeTraced(t, http.Header{
		GuardrailHeader: []string{"presidio-pii"},
		PolicyHeader:    []string{"example-policy"},
	}, http.StatusBadRequest)
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err = %v, want a StatusError", err)
	}
	if !trace.Replied || trace.PolicyMissing || !slices.Equal(trace.Guardrails, []string{"presidio-pii"}) {
		t.Errorf("trace = %+v, want a reply naming the guardrail and no missing policy", trace)
	}
}
