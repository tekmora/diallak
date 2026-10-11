package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

type customTypeString string

type stubLogValuer struct {
	val string
}

func (v stubLogValuer) LogValue() slog.Value {
	return slog.StringValue(v.val)
}

func jsonLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func parseJSONLog(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("unmarshal json log: %v (raw: %s)", err, buf.String())
	}
	return data
}

func testDisallowedKeyDropped(t *testing.T, key string) {
	t.Helper()
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	marker := "marker-" + key + "-value"
	logger.Error("test error", slog.String(key, marker))

	raw := buf.String()
	if strings.Contains(raw, marker) {
		t.Fatalf("log contains marker %q for key %s: %s", marker, key, raw)
	}
	data := parseJSONLog(t, &buf)
	if _, ok := data[key]; ok {
		t.Fatalf("log contains disallowed key %s: %s", key, raw)
	}
	dropped, ok := data["dropped_attrs"].(float64)
	if !ok || int(dropped) != 1 {
		t.Fatalf("dropped_attrs = %v, want 1", data["dropped_attrs"])
	}
}

func TestLibraryLogger_Negative_BodyDropped(t *testing.T) {
	testDisallowedKeyDropped(t, "body")
}

func TestLibraryLogger_Negative_EndpointDropped(t *testing.T) {
	testDisallowedKeyDropped(t, "endpoint")
}

func TestLibraryLogger_Negative_ArgDropped(t *testing.T) {
	testDisallowedKeyDropped(t, "arg")
}

func TestLibraryLogger_Negative_StackDropped(t *testing.T) {
	testDisallowedKeyDropped(t, "stack")
}

func TestLibraryLogger_Negative_ErrorDropped(t *testing.T) {
	testDisallowedKeyDropped(t, "error")
}

func TestLibraryLogger_Negative_ErrWrappedErrorReplaced(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	wrappedErr := fmt.Errorf("outer marker-0815: %w", io.ErrUnexpectedEOF)
	logger.Error("test error", slog.Any("err", wrappedErr))

	raw := buf.String()
	if strings.Contains(raw, "marker-0815") {
		t.Fatalf("log contains marker: %s", raw)
	}
	data := parseJSONLog(t, &buf)
	if _, ok := data["err"]; ok {
		t.Fatalf("log contains 'err' key: %s", raw)
	}
	wantTypes := "*fmt.wrapError>*errors.errorString"
	if gotTypes, ok := data["error_types"].(string); !ok || gotTypes != wantTypes {
		t.Fatalf("error_types = %v, want %q", data["error_types"], wantTypes)
	}
	dropped, ok := data["dropped_attrs"].(float64)
	if !ok || int(dropped) != 1 {
		t.Fatalf("dropped_attrs = %v, want 1", data["dropped_attrs"])
	}
}

func TestLibraryLogger_Negative_ErrStringValueDropped(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	logger.Error("test error", slog.String("err", "plain string error"))

	data := parseJSONLog(t, &buf)
	if _, ok := data["err"]; ok {
		t.Fatalf("log contains 'err' key: %s", buf.String())
	}
	if _, ok := data["error_types"]; ok {
		t.Fatalf("log contains 'error_types' key: %s", buf.String())
	}
	dropped, ok := data["dropped_attrs"].(float64)
	if !ok || int(dropped) != 1 {
		t.Fatalf("dropped_attrs = %v, want 1", data["dropped_attrs"])
	}
}

func TestLibraryLogger_Positive_AllowedKeysPassed(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	logger.Warn("test warning",
		slog.Int("retry_after", 5),
		slog.Int("try", 2),
		slog.Duration("delay", 100*time.Millisecond),
		slog.Duration("last_heartbeat_ago", 2*time.Second),
		slog.Bool("reconnect", true),
		slog.Bool("can_resume", false),
		slog.String("code", "200"),
		slog.String("type", "READY"),
		slog.String("event_type", "RESUMED"),
		slog.Any("data", customTypeString("sample-data")),
	)

	data := parseJSONLog(t, &buf)
	if _, ok := data["dropped_attrs"]; ok {
		t.Fatalf("unexpected dropped_attrs: %v", data["dropped_attrs"])
	}
	keys := []string{
		"retry_after", "try", "delay", "last_heartbeat_ago",
		"reconnect", "can_resume", "code", "type", "event_type", "data",
	}
	for i := 0; i < len(keys); i++ {
		k := keys[i]
		if _, ok := data[k]; !ok {
			t.Fatalf("missing allowed key %q in log: %s", k, buf.String())
		}
	}
}

func TestLibraryLogger_Boundary_StringLengthCut(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	s65 := strings.Repeat("x", 65)
	s64 := strings.Repeat("y", 64)
	logger.Warn("cut test",
		slog.String("type", s65),
		slog.String("data", s64),
		slog.Any("code", customTypeString(strings.Repeat("z", 65))),
	)

	data := parseJSONLog(t, &buf)
	if got, ok := data["type"].(string); !ok || got != strings.Repeat("x", 64) {
		t.Fatalf("type = %q, want 64 x's", got)
	}
	if got, ok := data["data"].(string); !ok || got != s64 {
		t.Fatalf("data = %q, want %q", got, s64)
	}
	if got, ok := data["code"].(string); !ok || got != strings.Repeat("z", 64) {
		t.Fatalf("code = %q, want 64 z's", got)
	}
}

func TestLibraryLogger_Boundary_GroupAndTimeDropped(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	logger.Warn("group test", slog.Group("code", slog.String("inner", "val")))

	data1 := parseJSONLog(t, &buf)
	if _, ok := data1["code"]; ok {
		t.Fatalf("group code was not dropped: %s", buf.String())
	}
	if d, ok := data1["dropped_attrs"].(float64); !ok || int(d) != 1 {
		t.Fatalf("dropped_attrs = %v, want 1", data1["dropped_attrs"])
	}

	buf.Reset()
	logger.Warn("time test", slog.Time("code", time.Now()))
	data2 := parseJSONLog(t, &buf)
	if _, ok := data2["code"]; ok {
		t.Fatalf("time code was not dropped: %s", buf.String())
	}
	if d, ok := data2["dropped_attrs"].(float64); !ok || int(d) != 1 {
		t.Fatalf("dropped_attrs = %v, want 1", data2["dropped_attrs"])
	}
}

func TestLibraryLogger_Boundary_LogValuerResolvedOrDropped(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))
	markerType := strings.Repeat("t", 70)
	logger.Warn("valuer test",
		slog.Any("type", stubLogValuer{val: markerType}),
		slog.Any("body", stubLogValuer{val: "secret-body"}),
	)

	raw := buf.String()
	if strings.Contains(raw, "secret-body") {
		t.Fatalf("body marker found in log: %s", raw)
	}
	data := parseJSONLog(t, &buf)
	if _, ok := data["body"]; ok {
		t.Fatalf("body key found in log: %s", raw)
	}
	if got, ok := data["type"].(string); !ok || got != strings.Repeat("t", 64) {
		t.Fatalf("type = %q, want 64 t's", got)
	}
	if d, ok := data["dropped_attrs"].(float64); !ok || int(d) != 1 {
		t.Fatalf("dropped_attrs = %v, want 1", data["dropped_attrs"])
	}
}

func TestLibraryLogger_Boundary_ErrorChainMaxEightLinks(t *testing.T) {
	var buf bytes.Buffer
	logger := libraryLogger(jsonLogger(&buf))

	errChain := errors.New("base error")
	for i := 0; i < 9; i++ {
		errChain = fmt.Errorf("wrap %d: %w", i, errChain)
	}

	logger.Error("chain test", slog.Any("err", errChain))
	data := parseJSONLog(t, &buf)
	errorTypes, ok := data["error_types"].(string)
	if !ok {
		t.Fatalf("missing error_types: %s", buf.String())
	}
	links := strings.Split(errorTypes, ">")
	if len(links) != 8 {
		t.Fatalf("got %d links in error_types (%q), want 8", len(links), errorTypes)
	}
}

func TestLibraryLogger_Boundary_MaxThirtyTwoAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := floorHandler{inner: jsonLogger(&buf).Handler()}

	record := slog.NewRecord(time.Now(), slog.LevelWarn, "limit test", 0)
	attrs := make([]slog.Attr, 40)
	for i := 0; i < 40; i++ {
		attrs[i] = slog.Int("try", i)
	}
	record.AddAttrs(attrs...)

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	raw := buf.String()
	data := parseJSONLog(t, &buf)
	if d, ok := data["dropped_attrs"].(float64); !ok || int(d) != 8 {
		t.Fatalf("dropped_attrs = %v, want 8", data["dropped_attrs"])
	}
	if got := strings.Count(raw, `"try":`); got != 32 {
		t.Fatalf("found %d occurrences of 'try', want 32", got)
	}
}

func TestLibraryLogger_Negative_WithAttrsAndWithGroup(t *testing.T) {
	var buf bytes.Buffer
	base := jsonLogger(&buf)

	logger := libraryLogger(base).With("body", "marker-with-attrs")
	logger.Warn("with warn")
	if strings.Contains(buf.String(), "marker-with-attrs") {
		t.Fatalf("marker found after With: %s", buf.String())
	}
	data1 := parseJSONLog(t, &buf)
	if _, ok := data1["body"]; ok {
		t.Fatalf("body found after With: %s", buf.String())
	}

	buf.Reset()
	logger2 := libraryLogger(base).WithGroup("g")
	logger2.Warn("group warn", slog.String("body", "marker-with-group"), slog.String("code", "200"))
	raw2 := buf.String()
	if strings.Contains(raw2, "marker-with-group") {
		t.Fatalf("marker found after WithGroup: %s", raw2)
	}
	data2 := parseJSONLog(t, &buf)
	if _, ok := data2["body"]; ok {
		t.Fatalf("body found after WithGroup: %s", raw2)
	}
	if _, ok := data2["g"]; ok {
		t.Fatalf("group 'g' found in output: %s", raw2)
	}
	if code, ok := data2["code"].(string); !ok || code != "200" {
		t.Fatalf("code = %v, want '200' at top level", data2["code"])
	}
}

func TestLibraryLogger_Positive_EndToEndRestUnmarshallingError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/messages/@original") {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"answer-marker-4711": invalid_json}`)); err != nil {
				t.Errorf("write response body: %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	var buf bytes.Buffer
	base := jsonLogger(&buf)
	client := rest.NewClient("", rest.WithURL(server.URL), rest.WithLogger(libraryLogger(base)))
	interactions := rest.NewInteractions(client, discord.AllowedMentions{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := interactions.UpdateInteractionResponse(
		snowflake.ID(12345),
		"token-marker-4711",
		discord.MessageUpdate{},
		rest.WithCtx(ctx),
	)
	if err == nil {
		t.Fatalf("expected error from unmarshalling failure, got nil")
	}

	raw := buf.String()
	if strings.Contains(raw, "answer-marker-4711") {
		t.Fatalf("captured log holds answer marker: %s", raw)
	}
	if strings.Contains(raw, "token-marker-4711") {
		t.Fatalf("captured log holds token marker: %s", raw)
	}
	if !strings.Contains(raw, "error unmarshalling response body") {
		t.Fatalf("captured log missing expected message: %s", raw)
	}
}
