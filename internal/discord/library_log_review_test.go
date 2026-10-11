package discord

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/rest"
)

func TestLibraryLogger_Positive_PartNameOfADerivedLoggerIsKept(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf)).With(slog.String("name", "rest_client"), slog.Int("shard_id", 0))
	logger.Warn("rate limit exceeded", slog.Int("retry_after", 2))

	data := parseJSONLog(t, &buf)
	if data["name"] != "rest_client" || data["shard_id"] != float64(0) || data["retry_after"] != float64(2) {
		t.Fatalf("derived logger lost a listed attribute: %s", buf.String())
	}
}

func TestLibraryLogger_Negative_ErrorOfADerivedLoggerIsReplaced(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf)).With(slog.Any("err", context.DeadlineExceeded))
	logger.Warn("derived")

	raw := buf.String()
	data := parseJSONLog(t, &buf)
	if _, ok := data["err"]; ok || strings.Contains(raw, "deadline exceeded") {
		t.Fatalf("error text of a derived logger reached the log: %s", raw)
	}
	if data["error_types"] != "context.deadlineExceededError" {
		t.Fatalf("error_types = %v, want the type name: %s", data["error_types"], raw)
	}
}

// The one-shot commands (registration, server read, server apply) build their own client; its
// library records take the same filter as those of the running bot.
func TestNewRestClient_Negative_LibraryRecordsOfOneShotCommandsAreFiltered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"body-marker-2209": not json}`)); err != nil {
			t.Errorf("write response body: %v", err)
		}
	}))
	defer server.Close()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(jsonLogger(&buf))
	t.Cleanup(func() { slog.SetDefault(previous) })

	token := makeFakeToken(2209)
	client, err := newRestClient(token, server.URL)
	if err != nil {
		t.Fatalf("newRestClient() failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Rest.GetCurrentApplication(rest.WithCtx(ctx)); err == nil {
		t.Fatal("GetCurrentApplication() = nil error, want the decode failure")
	}

	raw := buf.String()
	if !strings.Contains(raw, "error unmarshalling response body") {
		t.Fatalf("the library record did not reach the log: %s", raw)
	}
	if strings.Contains(raw, "body-marker-2209") || strings.Contains(raw, token) ||
		strings.Contains(raw, `"endpoint"`) || strings.Contains(raw, `"body"`) {
		t.Fatalf("a library record of a one-shot command holds content: %s", raw)
	}
}

type structuredValue struct {
	Text string
}

func TestLibraryLogger_Boundary_StructuredValueUnderAListedKeyIsDropped(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	logger.Error("structured", slog.Any("event_type", structuredValue{Text: "struct-marker-3310"}))

	raw := buf.String()
	data := parseJSONLog(t, &buf)
	if _, ok := data["event_type"]; ok || strings.Contains(raw, "struct-marker-3310") {
		t.Fatalf("a structured value under a listed key reached the log: %s", raw)
	}
	if data["dropped_attrs"] != float64(1) {
		t.Fatalf("dropped_attrs = %v, want 1", data["dropped_attrs"])
	}
}

// nilUnwrapError panics in Unwrap on a nil receiver, as an error type with a wrapped field does.
type nilUnwrapError struct {
	inner error
}

func (e *nilUnwrapError) Error() string { return "nil-unwrap" }

func (e *nilUnwrapError) Unwrap() error { return e.inner }

func TestLibraryLogger_Boundary_NilPointerErrorDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	var typedNil *nilUnwrapError
	logger.Error("typed nil", slog.Any("err", error(typedNil)))

	data := parseJSONLog(t, &buf)
	if data["error_types"] != "*discord.nilUnwrapError" {
		t.Fatalf("error_types = %v, want the type name of the nil pointer: %s", data["error_types"], buf.String())
	}
}
