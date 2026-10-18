package discord

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
)

func newFakeRoleConnServer(appID uint64, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/applications/@me" {
			writeTestResponse(w, http.StatusOK, fmt.Sprintf(`{"id":"%d","name":"diallak-app"}`, appID))
			return
		}
		if r.URL.Path == fmt.Sprintf("/applications/%d/role-connections/metadata", appID) {
			handler(w, r)
			return
		}
		http.NotFound(w, r)
	}))
}

func sampleRecord(key string, typ discord.ApplicationRoleConnectionMetadataType) discord.ApplicationRoleConnectionMetadata {
	return discord.ApplicationRoleConnectionMetadata{
		Type:        typ,
		Key:         key,
		Name:        "Name " + key,
		Description: "Desc " + key,
	}
}

func TestPushRoleConnections_Positive_EchoesBody(t *testing.T) {
	const appID uint64 = 1001
	fakeToken := makeFakeToken(appID)
	recs := []discord.ApplicationRoleConnectionMetadata{
		sampleRecord("merged_contribution", discord.ApplicationRoleConnectionMetadataTypeBooleanEqual),
	}

	srv := newFakeRoleConnServer(appID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "bad method", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		writeTestResponse(w, http.StatusOK, string(body))
	})
	defer srv.Close()

	var out bytes.Buffer
	err := pushRoleConnections(context.Background(), fakeToken, recs, &out, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("pushRoleConnections error = %v, want nil", err)
	}

	wantOut := "pushed merged_contribution type=7\n"
	if got := out.String(); got != wantOut {
		t.Errorf("output = %q, want %q", got, wantOut)
	}
}

func TestPushRoleConnections_Negative_DifferentRecordReturned(t *testing.T) {
	const appID uint64 = 1001
	fakeToken := makeFakeToken(appID)
	recs := []discord.ApplicationRoleConnectionMetadata{
		sampleRecord("merged_contribution", discord.ApplicationRoleConnectionMetadataTypeBooleanEqual),
	}

	srv := newFakeRoleConnServer(appID, func(w http.ResponseWriter, r *http.Request) {
		writeTestResponse(w, http.StatusOK, `[{"type":1,"key":"other_key","name":"Other","description":"Desc"}]`)
	})
	defer srv.Close()

	var out bytes.Buffer
	err := pushRoleConnections(context.Background(), fakeToken, recs, &out, clientOptions{restURL: srv.URL})
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("error = %q, want to contain 'mismatch'", err.Error())
	}
}

func TestPushRoleConnections_Negative_UnauthorizedRefusalDoesNotContainToken(t *testing.T) {
	const appID uint64 = 1001
	fakeToken := makeFakeToken(appID)
	recs := []discord.ApplicationRoleConnectionMetadata{
		sampleRecord("merged_contribution", discord.ApplicationRoleConnectionMetadataTypeBooleanEqual),
	}

	srv := newFakeRoleConnServer(appID, func(w http.ResponseWriter, r *http.Request) {
		writeTestResponse(w, http.StatusUnauthorized, `{"message":"401: Unauthorized","code":0}`)
	})
	defer srv.Close()

	var out bytes.Buffer
	err := pushRoleConnections(context.Background(), fakeToken, recs, &out, clientOptions{restURL: srv.URL})
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
	if strings.Contains(err.Error(), fakeToken) {
		t.Errorf("error %q contains raw token %q", err.Error(), fakeToken)
	}
}

func TestPushRoleConnections_Boundary_FiveRecords(t *testing.T) {
	const appID uint64 = 1001
	fakeToken := makeFakeToken(appID)
	var recs []discord.ApplicationRoleConnectionMetadata
	for i := 1; i <= 5; i++ {
		recs = append(recs, sampleRecord(fmt.Sprintf("key_%d", i), discord.ApplicationRoleConnectionMetadataType(i)))
	}

	srv := newFakeRoleConnServer(appID, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		writeTestResponse(w, http.StatusOK, string(body))
	})
	defer srv.Close()

	var out bytes.Buffer
	err := pushRoleConnections(context.Background(), fakeToken, recs, &out, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("pushRoleConnections 5 records error = %v, want nil", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("output lines = %d, want 5: %q", len(lines), out.String())
	}
	for i, line := range lines {
		want := fmt.Sprintf("pushed key_%d type=%d", i+1, i+1)
		if line != want {
			t.Errorf("line %d = %q, want %q", i, line, want)
		}
	}
}

func TestPushRoleConnections_Negative_EmptyToken(t *testing.T) {
	var out bytes.Buffer
	err := PushRoleConnections(context.Background(), "", nil, &out)
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("error = %q, want to mention token", err.Error())
	}
}
