package discord

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// The tests in this file press the receipt buttons over the gateway, as areas_gateway_test.go does
// with the areas menus, against the same fake Discord.

const receiptDirectChannelID uint64 = 870000000000000001

// receiptPressFrame is the frame of a press of the button with customID.
func receiptPressFrame(t *testing.T, seq int, id uint64, customID string) []byte {
	t.Helper()
	data := map[string]any{"custom_id": customID, "component_type": 2}
	message := map[string]any{
		"id": "760000000000000002", "channel_id": "740000000000000001", "type": 0, "content": "answer",
		"timestamp": "2026-10-17T00:00:00Z", "flags": 64,
		"author": map[string]any{"id": fmt.Sprint(gatewayAppID), "username": "diallak"},
	}
	return areasFrame(t, seq, id, 3, nil, data, map[string]any{"message": message})
}

// runReceiptsOverGateway holds one receipt, delivers the frames the test builds from its key and
// returns the requests the fake Discord received.
func runReceiptsOverGateway(t *testing.T, fake *areasFake, wantAnswers int, frames func(key string) [][]byte) []areasCall {
	t.Helper()
	receipts, err := NewReceipts(ReceiptsConfig{})
	if err != nil {
		t.Fatalf("NewReceipts: %v", err)
	}
	key, err := receipts.put(repliedTrace, true)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	handler, err := NewHandler(HandlerConfig{
		Asker: &fakeAsker{}, GuildID: areasGuildID, MaxConcurrent: 2, AskTimeout: 5 * time.Second,
		ReplyTimeout: 5 * time.Second, Logger: slog.New(slog.DiscardHandler), Receipts: receipts,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	fs := frames(key)
	fake.t, fake.answers = t, make(chan struct{}, 2*len(fs)+1)
	return runComponentsOverGateway(t, fake, BotConfig{Handler: handler}, wantAnswers, fs...)
}

func TestRunReceipt_Positive_PressShowsTheReceiptOnlyToTheMember(t *testing.T) {
	const id uint64 = 880000000000000001
	var key string
	calls := runReceiptsOverGateway(t, &areasFake{}, 1, func(k string) [][]byte {
		key = k
		return [][]byte{receiptPressFrame(t, 1, id, receiptCustomIDPrefix+k)}
	})
	got := areasInteractionCalls(calls, id, false)
	if len(got) != 1 {
		t.Fatalf("requests:\n%s\nwant one answer", requestLines(got))
	}
	b := got[0].body
	if b.Type != 4 || b.Data.Flags != 64 || !strings.Contains(b.Data.Content, "Guardrails that ran: `presidio-pii`") {
		t.Errorf("answer = %+v, want the receipt as a message only the member sees", b)
	}
	if len(b.Data.Components) != 1 || b.Data.Components[0].Components[0].CustomID != receiptDirectCustomIDPrefix+key {
		t.Errorf("components = %+v, want the direct message button for key %s", b.Data.Components, key)
	}
}

func TestRunReceipt_Positive_DirectPressSendsTheReceiptAndSaysSo(t *testing.T) {
	const id uint64 = 880000000000000002
	calls := runReceiptsOverGateway(t, &areasFake{}, 2, func(k string) [][]byte {
		return [][]byte{receiptPressFrame(t, 1, id, receiptDirectCustomIDPrefix+k)}
	})
	want := strings.Join([]string{
		fmt.Sprintf("POST /interactions/%d/interaction-token-%d/callback", id, id),
		"POST /users/@me/channels",
		fmt.Sprintf("POST /channels/%d/messages", receiptDirectChannelID),
		fmt.Sprintf("PATCH /webhooks/%d/interaction-token-%d/messages/@original", gatewayAppID, id),
	}, "\n")
	got := receiptRequests(calls, id)
	if requestLines(got) != want {
		t.Fatalf("requests:\n%s\nwant:\n%s", requestLines(got), want)
	}
	if got[0].body.Type != 5 || got[0].body.Data.Flags != 64 {
		t.Errorf("callback = %+v, want a deferred message only the member sees", got[0].body)
	}
	if !strings.Contains(got[2].body.Content, textReceiptAutomated) {
		t.Errorf("direct message = %q, want the receipt", got[2].body.Content)
	}
	if got[3].body.Content != TextReceiptDirectSent {
		t.Errorf("edit = %q, want %q", got[3].body.Content, TextReceiptDirectSent)
	}
}

func TestRunReceipt_Negative_ClosedDirectMessagesAreToldOnlyToTheMember(t *testing.T) {
	const id uint64 = 880000000000000003
	calls := runReceiptsOverGateway(t, &areasFake{directClosed: true}, 2, func(k string) [][]byte {
		return [][]byte{receiptPressFrame(t, 1, id, receiptDirectCustomIDPrefix+k)}
	})
	got := receiptRequests(calls, id)
	if len(got) != 4 || got[3].body.Content != TextReceiptDirectClosed {
		t.Fatalf("requests:\n%s\nwant the edit to say the direct messages are closed", requestLines(got))
	}
}

func TestRunReceipt_Boundary_UnknownKeySaysTheReceiptIsNoLongerHeld(t *testing.T) {
	const id uint64 = 880000000000000004
	calls := runReceiptsOverGateway(t, &areasFake{}, 1, func(string) [][]byte {
		return [][]byte{receiptPressFrame(t, 1, id, receiptCustomIDPrefix+"0123456789abcdef0123456789abcdef")}
	})
	got := areasInteractionCalls(calls, id, false)
	if len(got) != 1 || got[0].body.Data.Content != TextReceiptGone || len(got[0].body.Data.Components) != 0 {
		t.Errorf("requests:\n%s\nwant one answer saying the receipt is gone, without a button", requestLines(got))
	}
}

// receiptRequests returns the answers of one interaction and the direct message calls, in order.
func receiptRequests(calls []areasCall, id uint64) []areasCall {
	token := fmt.Sprintf("interaction-token-%d", id)
	var out []areasCall
	for _, c := range calls {
		if strings.Contains(c.path, token) || c.path == "/users/@me/channels" ||
			c.path == fmt.Sprintf("/channels/%d/messages", receiptDirectChannelID) {
			out = append(out, c)
		}
	}
	return out
}

func TestBuildReplyPayload_Boundary_ButtonOnlyWithAKey(t *testing.T) {
	with := buildReplyPayload("answer", "abc")
	if with.Components == nil || len(*with.Components) != 1 {
		t.Fatalf("reply with a key has components %v, want one row", with.Components)
	}
	if without := buildReplyPayload("answer", ""); without.Components != nil {
		t.Errorf("reply without a key has components %v, want none", *without.Components)
	}
	if _, _, ok := receiptButtonID("areas:interests"); ok {
		t.Error("an areas custom id was read as a receipt button")
	}
}
