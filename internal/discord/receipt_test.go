package discord

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/model"
)

// testClock is a clock the test moves by hand.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestReceipts(t *testing.T, maxHeld int) (*Receipts, *testClock) {
	t.Helper()
	clock := &testClock{now: time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)}
	r, err := NewReceipts(ReceiptsConfig{Max: maxHeld, Now: clock.Now})
	if err != nil {
		t.Fatalf("NewReceipts: %v", err)
	}
	return r, clock
}

var repliedTrace = model.Trace{
	Sent: true, Replied: true,
	Guardrails: []string{"presidio-pii", "prompt-injection"},
	Policies:   []string{"diallak"},
}

func TestReceipts_Positive_HeldReceiptIsReturnedUnderItsKey(t *testing.T) {
	r, _ := newTestReceipts(t, 0)
	key, err := r.put(repliedTrace, true)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if len(key) != 2*receiptKeyBytes {
		t.Errorf("key %q has %d characters, want %d", key, len(key), 2*receiptKeyBytes)
	}
	e, ok := r.get(key)
	if !ok || !e.shown || len(e.trace.Guardrails) != 2 {
		t.Errorf("get = %+v, %v; want the held receipt", e, ok)
	}
}

func TestReceipts_Negative_UnknownKeyAndNilStoreHoldNothing(t *testing.T) {
	r, _ := newTestReceipts(t, 0)
	if _, err := r.put(repliedTrace, true); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, ok := r.get("0123456789abcdef0123456789abcdef"); ok {
		t.Error("an unknown key returned a receipt")
	}
	var none *Receipts
	if _, ok := none.get("any"); ok {
		t.Error("a nil Receipts returned a receipt")
	}
}

func TestReceipts_Negative_FailingRandomSourceHoldsNothing(t *testing.T) {
	r, err := NewReceipts(ReceiptsConfig{Random: bytes.NewReader([]byte{1, 2, 3})})
	if err != nil {
		t.Fatalf("NewReceipts: %v", err)
	}
	if key, err := r.put(repliedTrace, true); err == nil || key != "" {
		t.Errorf("put = %q, %v; want an error and no key", key, err)
	}
	if _, err := NewReceipts(ReceiptsConfig{Max: -1}); err == nil {
		t.Error("NewReceipts accepted a negative maximum")
	}
}

func TestReceipts_Boundary_ReceiptIsHeldForExactlyFifteenMinutes(t *testing.T) {
	r, clock := newTestReceipts(t, 0)
	key, err := r.put(repliedTrace, true)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	clock.advance(ReceiptHold)
	if _, ok := r.get(key); !ok {
		t.Fatal("the receipt was gone at exactly 15 minutes")
	}
	clock.advance(time.Nanosecond)
	if _, ok := r.get(key); ok {
		t.Fatal("the receipt was still held after 15 minutes")
	}
}

func TestReceipts_Boundary_OldestGoesFirstAtTheMaximum(t *testing.T) {
	r, clock := newTestReceipts(t, 2)
	keys := make([]string, 3)
	for i := range keys {
		key, err := r.put(repliedTrace, true)
		if err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
		keys[i] = key
		clock.advance(time.Second)
	}
	if _, ok := r.get(keys[0]); ok {
		t.Error("the oldest receipt was kept beyond the maximum")
	}
	for _, k := range keys[1:] {
		if _, ok := r.get(k); !ok {
			t.Errorf("receipt %s was dropped although it is among the newest two", k)
		}
	}
	if len(r.entries) != 2 || len(r.order) != 2 {
		t.Errorf("holding %d entries and %d keys, want 2 and 2", len(r.entries), len(r.order))
	}
}

func TestReceiptText_Positive_RepliedReceiptNamesGuardrailsPoliciesAndRetention(t *testing.T) {
	text := receiptText(receiptEntry{trace: repliedTrace, shown: true, at: time.Unix(1792224000, 0)})
	for _, want := range []string{
		"<t:1792224000:f>",
		"Guardrails that ran: `presidio-pii`, `prompt-injection`.",
		"Policies that applied: `diallak`.",
		textReceiptCaveat, "The model's answer was shown to you.",
		textReceiptWhere, "60 minutes", "74 hours", "30 days", "15 minutes", textReceiptAutomated,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("receipt lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, textReceiptNoReply) || strings.Contains(text, textReceiptPolicyMissing) {
		t.Errorf("receipt of a reply with the policy says no reply came or the policy was missing:\n%s", text)
	}
}

func TestReceiptText_Negative_NoReplyAndMissingPolicyClaimNoGuardrails(t *testing.T) {
	noReply := receiptText(receiptEntry{trace: model.Trace{Sent: true}})
	if !strings.Contains(noReply, textReceiptNoReply) || strings.Contains(noReply, "Guardrails that ran") {
		t.Errorf("receipt without a reply names guardrails or lacks the sentence:\n%s", noReply)
	}
	if !strings.Contains(noReply, "No text of the model was shown to you.") {
		t.Errorf("receipt without a reply claims an answer was shown:\n%s", noReply)
	}

	missing := receiptText(receiptEntry{trace: model.Trace{
		Sent: true, Replied: true, Guardrails: []string{"presidio-pii"}, PolicyMissing: true,
	}})
	for _, want := range []string{"Policies that applied: none.", textReceiptPolicyMissing} {
		if !strings.Contains(missing, want) {
			t.Errorf("receipt with the policy missing lacks %q:\n%s", want, missing)
		}
	}
	for _, text := range []string{noReply, missing} {
		if strings.Contains(text, "was masked") || strings.Contains(text, "were masked") {
			t.Errorf("receipt claims that data was masked:\n%s", text)
		}
	}
}

func TestReceiptText_Boundary_NameListIsCappedAndCannotFormatTheMessage(t *testing.T) {
	names := make([]string, maxReceiptNames+3)
	for i := range names {
		names[i] = fmt.Sprintf("g%d", i)
	}
	names[0] = "evil`**bold**"
	got := receiptNames(names)
	if !strings.HasPrefix(got, "`evil'**bold**`") {
		t.Errorf("a backtick in a name was kept: %s", got)
	}
	if !strings.HasSuffix(got, " and 3 more") || strings.Contains(got, "g10") {
		t.Errorf("list of %d names = %s, want %d shown and 3 more", len(names), got, maxReceiptNames)
	}
	if got := receiptNames(names[:maxReceiptNames]); strings.Contains(got, "more") {
		t.Errorf("list of exactly %d names says more: %s", maxReceiptNames, got)
	}
}

// fakeReceiptInteraction records the answers to a press of a receipt button.
type fakeReceiptInteraction struct {
	shown, directKeys, edits, direct []string
	deferred                         int
	deferErr, directErr              error
}

func (f *fakeReceiptInteraction) Show(_ context.Context, text, directKey string) error {
	f.shown = append(f.shown, text)
	f.directKeys = append(f.directKeys, directKey)
	return nil
}

func (f *fakeReceiptInteraction) Defer(context.Context) error {
	f.deferred++
	return f.deferErr
}

func (f *fakeReceiptInteraction) Edit(_ context.Context, text string) error {
	f.edits = append(f.edits, text)
	return nil
}

func (f *fakeReceiptInteraction) SendDirect(_ context.Context, text string) error {
	f.direct = append(f.direct, text)
	return f.directErr
}

func newReceiptHandler(t *testing.T, receipts *Receipts, asker Asker) *Handler {
	t.Helper()
	h, err := NewHandler(HandlerConfig{
		Asker: asker, GuildID: 123, MaxConcurrent: 1,
		AskTimeout: time.Second, ReplyTimeout: time.Second, Receipts: receipts,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func TestHandleReceipt_Positive_HeldReceiptIsShownWithTheDirectButton(t *testing.T) {
	r, _ := newTestReceipts(t, 0)
	key, err := r.put(repliedTrace, true)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	in := &fakeReceiptInteraction{}
	newReceiptHandler(t, r, &fakeAsker{}).HandleReceipt(context.Background(), in, key)
	if len(in.shown) != 1 || !strings.Contains(in.shown[0], "Guardrails that ran") || in.directKeys[0] != key {
		t.Errorf("shown %q with direct key %q, want the receipt with key %q", in.shown, in.directKeys, key)
	}
}

func TestHandleReceipt_Negative_ExpiredReceiptSaysItIsNoLongerHeld(t *testing.T) {
	r, clock := newTestReceipts(t, 0)
	key, err := r.put(repliedTrace, true)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	clock.advance(ReceiptHold + time.Second)
	in := &fakeReceiptInteraction{}
	newReceiptHandler(t, r, &fakeAsker{}).HandleReceipt(context.Background(), in, key)
	if len(in.shown) != 1 || in.shown[0] != TextReceiptGone || in.directKeys[0] != "" {
		t.Errorf("shown %q with direct key %q, want TextReceiptGone without a button", in.shown, in.directKeys)
	}
}

func TestHandleReceiptDirect_Positive_SentClosedAndFailedEachSayWhatHappened(t *testing.T) {
	cases := []struct {
		name      string
		directErr error
		want      string
	}{
		{"sent", nil, TextReceiptDirectSent},
		{"direct messages closed", ErrDirectClosed, TextReceiptDirectClosed},
		{"other failure", errors.New("boom"), TextReceiptDirectFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newTestReceipts(t, 0)
			key, err := r.put(repliedTrace, false)
			if err != nil {
				t.Fatalf("put: %v", err)
			}
			in := &fakeReceiptInteraction{directErr: tc.directErr}
			newReceiptHandler(t, r, &fakeAsker{}).HandleReceiptDirect(context.Background(), in, key)
			if in.deferred != 1 || len(in.direct) != 1 || !strings.Contains(in.direct[0], textReceiptAutomated) {
				t.Errorf("deferred %d times, sent %q; want one deferral and the receipt", in.deferred, in.direct)
			}
			if len(in.edits) != 1 || in.edits[0] != tc.want {
				t.Errorf("edits = %q, want %q", in.edits, tc.want)
			}
		})
	}
}

func TestHandleReceiptDirect_Negative_GoneReceiptOpensNoDirectMessage(t *testing.T) {
	in := &fakeReceiptInteraction{}
	newReceiptHandler(t, nil, &fakeAsker{}).HandleReceiptDirect(context.Background(), in, "unknown")
	if len(in.direct) != 0 || len(in.edits) != 1 || in.edits[0] != TextReceiptGone {
		t.Errorf("direct %q, edits %q; want no direct message and TextReceiptGone", in.direct, in.edits)
	}

	failing := &fakeReceiptInteraction{deferErr: errors.New("boom")}
	newReceiptHandler(t, nil, &fakeAsker{}).HandleReceiptDirect(context.Background(), failing, "unknown")
	if len(failing.edits) != 0 || len(failing.direct) != 0 {
		t.Errorf("after a failed deferral: edits %q, direct %q; want none", failing.edits, failing.direct)
	}
}

// askReceiptKey runs one ask whose answer carries trace and returns the receipt key of the reply.
func askReceiptKey(t *testing.T, receipts *Receipts, kind answer.Kind, trace model.Trace) string {
	t.Helper()
	asker := &fakeAsker{askFunc: func(context.Context, string) (answer.Answer, error) {
		return answer.Answer{Kind: kind, Text: "text", Trace: trace}, nil
	}}
	in := &fakeInteraction{guildID: 123, question: "How do I install it?"}
	newReceiptHandler(t, receipts, asker).HandleAsk(context.Background(), in)
	if len(in.receiptKeys) != 1 {
		t.Fatalf("%d replies, want one", len(in.receiptKeys))
	}
	return in.receiptKeys[0]
}

func TestHandleAsk_Positive_ReplyAfterTextLeftTheBotCarriesTheButton(t *testing.T) {
	r, _ := newTestReceipts(t, 0)
	for _, kind := range []answer.Kind{answer.KindAnswer, answer.KindExcerpts, answer.KindRefusal} {
		key := askReceiptKey(t, r, kind, model.Trace{Sent: true})
		e, ok := r.get(key)
		if key == "" || !ok {
			t.Errorf("%s reply after text left the bot carries key %q, held %v; want a held receipt", kind, key, ok)
		}
		if e.shown != (kind == answer.KindAnswer) {
			t.Errorf("%s reply holds shown=%v", kind, e.shown)
		}
	}
}

func TestHandleAsk_Negative_ExcerptModeAndNoStoreCarryNoButton(t *testing.T) {
	r, _ := newTestReceipts(t, 0)
	if key := askReceiptKey(t, r, answer.KindExcerpts, model.Trace{}); key != "" {
		t.Errorf("excerpt reply where no text left the bot carries key %q", key)
	}
	if key := askReceiptKey(t, nil, answer.KindAnswer, repliedTrace); key != "" {
		t.Errorf("reply without a receipt store carries key %q", key)
	}
}
