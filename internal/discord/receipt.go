package discord

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/embed"
	"github.com/tekmora/diallak/internal/model"
)

const (
	// receiptCustomIDPrefix starts the custom id of the button under a reply that shows its receipt.
	receiptCustomIDPrefix = "receipt:"
	// receiptDirectCustomIDPrefix starts the custom id of the button under a receipt that sends it
	// as a direct message.
	receiptDirectCustomIDPrefix = "receipt-dm:"

	// ReceiptHold is how long the bot holds a receipt in memory (ADR-0013).
	ReceiptHold = 15 * time.Minute
	// DefaultMaxReceipts bounds the receipts held at once; the oldest goes first.
	// Source: chosen, not measured.
	DefaultMaxReceipts = 1000
	// maxReceiptNames bounds the names of one list the receipt shows, so it fits one message.
	maxReceiptNames = 10
	// maxReceiptListRunes bounds the runes of one list the receipt shows; the first entry is shown
	// whatever its length, so a list holds at most one 128-byte name past it.
	// Source: chosen, so that the worst receipt fits maxReceiptMessageRunes.
	maxReceiptListRunes = 160
	// maxMessageRunes is the longest content of one Discord message.
	maxMessageRunes = 2000
	// maxReceiptMessageRunes is the longest content of one Discord message.
	maxReceiptMessageRunes = maxMessageRunes
	// receiptKeyBytes is the length of the random key carried in the button.
	receiptKeyBytes = 16

	// ReceiptButtonLabel is the label of the button under a reply for which text left the bot.
	ReceiptButtonLabel = "What happened to my question"
	// ReceiptDirectButtonLabel is the label of the button under a receipt that sends it as a direct
	// message.
	ReceiptDirectButtonLabel = "Send me this as a direct message"

	// TextReceiptGone answers a press of a button whose receipt the bot no longer holds.
	TextReceiptGone = "This receipt is no longer held. The bot keeps a receipt in memory for 15 minutes " +
		"and loses it on a restart, so it holds nothing about this question any more."
	// TextReceiptDirectSent confirms a receipt sent as a direct message.
	TextReceiptDirectSent = "The receipt was sent to you as a direct message."
	// TextReceiptDirectClosed says that Discord refused the direct message.
	TextReceiptDirectClosed = "Your direct messages are closed, so the receipt could not be sent. " +
		"It stays here, visible to you only."
	// TextReceiptDirectFailed says that the direct message failed for another reason.
	TextReceiptDirectFailed = "The direct message could not be sent. The receipt stays here, visible to you only."

	textReceiptCaveat = "These guardrails can mask or change your text before the model sees it. " +
		"They do not find everything, and the bot does not see what they changed."
	textReceiptNoReply = "Your question was sent, but no reply came back, so the bot does not know " +
		"which guardrails ran."
	textReceiptPolicyMissing = "The reply did not name the bot's guardrail policy, so the model's answer " +
		"was discarded and you got excerpts only."
	textReceiptWhere = "Where your text went: your question, with the documentation passages found for it, " +
		"to the operator's gateway, and from there to the model behind it."
	textReceiptKept = "How long it is kept: the bot holds no text of your question and keeps this receipt " +
		"in memory for 15 minutes. " + textReceiptKeptGateway
	textReceiptKeptGateway = "The gateway writes your question to no storage and uses it for no " +
		"training. It holds an answer in its cache for at most 60 minutes, one log line with the matching " +
		"fragment of a blocked question for about 74 hours at most, and a record of the request without " +
		"question or answer for 30 days. The privacy notice has the details."
	textReceiptAutomated = "This receipt is automated."

	// The texts of step two of ADR-0013, when the bot sent its own masking request.
	textReceiptMaskFailed = "Your question was sent to the gateway's masking route, and no masked text came " +
		"back, so nothing was sent to the model."
	textReceiptMaskedIntro   = "The text sent on for the model, after masking:"
	textReceiptMaskUnchanged = "The masking changed nothing in your text."
	textReceiptMaskUnknown   = "The masking changed your text, and the kinds of what it replaced are not known."
	textReceiptMaskCaveat    = "The masking does not find everything: a passage it missed stands in the text " +
		"above as you wrote it."
	textReceiptMaskWhere = "Where your text went: your question to the gateway's masking route; the masked " +
		"text, with the documentation passages found for it, to the operator's gateway, and from there to " +
		"the model behind it."
	textReceiptMaskKept = "How long it is kept: the bot holds the masked text shown here, not your question as " +
		"you wrote it, and keeps it with this receipt in memory for 15 minutes. " + textReceiptKeptGateway
	// maxReceiptMaskedRunes bounds the masked text the receipt shows. Long lists shorten it further,
	// so the receipt fits one message.
	maxReceiptMaskedRunes = 500
	textReceiptCut        = " [cut here]"
	// receiptMaskedSlot stands for the masked text until the room left for it is known.
	receiptMaskedSlot = "\x00"
)

// ErrDirectClosed reports that Discord refused a direct message because the member's direct
// messages are closed.
var ErrDirectClosed = errors.New("the member's direct messages are closed")

// receiptEntry is what the bot holds for one reply with a button: no text of the member.
type receiptEntry struct {
	trace model.Trace
	embed embed.Trace
	shown bool
	at    time.Time
}

// ReceiptsConfig configures Receipts.
type ReceiptsConfig struct {
	// Max is the number of receipts held at once. Zero means DefaultMaxReceipts.
	Max int
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time
	// Random is the source of the keys. Nil means crypto/rand.
	Random io.Reader
}

// Receipts holds the receipts of recent replies in memory, for ReceiptHold and at most Max of them.
// Its key is random and not derived from the member. Receipts is safe for concurrent use.
type Receipts struct {
	max    int
	now    func() time.Time
	random io.Reader

	mu      sync.Mutex
	entries map[string]receiptEntry
	order   []string
}

// NewReceipts validates cfg and returns an empty Receipts.
func NewReceipts(cfg ReceiptsConfig) (*Receipts, error) {
	if cfg.Max < 0 {
		return nil, fmt.Errorf("max receipts %d is negative", cfg.Max)
	}
	maxHeld := cfg.Max
	if maxHeld == 0 {
		maxHeld = DefaultMaxReceipts
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	random := cfg.Random
	if random == nil {
		random = rand.Reader
	}
	return &Receipts{
		max:     maxHeld,
		now:     now,
		random:  random,
		entries: make(map[string]receiptEntry, maxHeld),
	}, nil
}

// put holds the receipt of a reply and returns the key its button carries.
func (r *Receipts) put(trace model.Trace, emb embed.Trace, shown bool) (string, error) {
	raw := make([]byte, receiptKeyBytes)
	if _, err := io.ReadFull(r.random, raw); err != nil {
		return "", fmt.Errorf("read receipt key: %w", err)
	}
	key := hex.EncodeToString(raw)

	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.dropLocked(now)
	if len(r.order) >= r.max {
		delete(r.entries, r.order[0])
		r.order = r.order[1:]
	}
	r.entries[key] = receiptEntry{trace: trace, embed: emb, shown: shown, at: now}
	r.order = append(r.order, key)
	return key, nil
}

// get returns the receipt held under key, if it is held and not older than ReceiptHold. A nil
// Receipts holds none.
func (r *Receipts) get(key string) (receiptEntry, bool) {
	if r == nil {
		return receiptEntry{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dropLocked(r.now())
	e, ok := r.entries[key]
	return e, ok
}

// dropLocked removes the receipts older than ReceiptHold. order holds the keys oldest first, so
// the loop ends at the first receipt still held; it runs at most len(order) times.
func (r *Receipts) dropLocked(now time.Time) {
	n := 0
	for n < len(r.order) && now.Sub(r.entries[r.order[n]].at) > ReceiptHold {
		delete(r.entries, r.order[n])
		n++
	}
	r.order = r.order[n:]
}

// receiptText is the receipt the member sees, built from what the bot holds only.
func receiptText(e receiptEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Receipt for your question** (sent <t:%d:f>)\n", e.at.Unix())
	mask := e.trace.Mask
	switch {
	case mask != nil && mask.Failed:
		b.WriteString(textReceiptMaskFailed + "\n")
	case mask != nil:
		writeMasked(&b, *mask)
	}
	switch {
	case mask != nil && mask.Failed:
	case e.trace.Replied:
		fmt.Fprintf(&b, "Guardrails that ran: %s.\n", receiptNames(e.trace.Guardrails))
		fmt.Fprintf(&b, "Policies that applied: %s.\n", receiptNames(e.trace.Policies))
		if e.trace.PolicyMissing {
			b.WriteString(textReceiptPolicyMissing + "\n")
		}
		b.WriteString(textReceiptCaveat + "\n")
	case e.trace.Sent:
		b.WriteString(textReceiptNoReply + "\n")
	}
	if e.shown {
		b.WriteString("The model's answer was shown to you.\n")
	} else {
		b.WriteString("No text of the model was shown to you.\n")
	}
	b.WriteString(receiptWhere(e) + "\n")
	if mask != nil {
		b.WriteString(textReceiptMaskKept + "\n")
	} else {
		b.WriteString(textReceiptKept + "\n")
	}
	b.WriteString(textReceiptAutomated)
	text := b.String()
	if mask == nil || mask.Failed {
		return text
	}
	room := maxReceiptMessageRunes - utf8.RuneCountInString(text) + utf8.RuneCountInString(receiptMaskedSlot)
	return strings.Replace(text, receiptMaskedSlot, fitMasked(mask.Text, room), 1)
}

func receiptWhere(e receiptEntry) string {
	mask := e.trace.Mask
	switch {
	case e.embed.Sent && e.trace.Sent && mask != nil:
		return fmt.Sprintf("Where your text went: your question to the embedding endpoint at %s; your question to the gateway's masking route; the masked text, with the documentation passages found for it, to the operator's gateway, and from there to the model behind it.", e.embed.Host)
	case e.embed.Sent && e.trace.Sent:
		return fmt.Sprintf("Where your text went: your question to the embedding endpoint at %s; your question, with the documentation passages found for it, to the operator's gateway, and from there to the model behind it.", e.embed.Host)
	case e.embed.Sent:
		return fmt.Sprintf("Where your text went: your question to the embedding endpoint at %s.", e.embed.Host)
	case mask != nil:
		return textReceiptMaskWhere
	default:
		return textReceiptWhere
	}
}

// fitMasked is the masked text as the receipt shows it: no backtick, so it cannot end its code
// block, and at most maxReceiptMaskedRunes or room runes, whichever is less.
func fitMasked(text string, room int) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "`", "'"), receiptMaskedSlot, "")
	limit := min(maxReceiptMaskedRunes, room)
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	keep := max(limit-utf8.RuneCountInString(textReceiptCut), 0)
	return string([]rune(text)[:keep]) + textReceiptCut
}

// writeMasked writes the masked text as a code block, so it cannot format the message, and what
// the masking replaced. It never says that the text holds no personal data.
func writeMasked(b *strings.Builder, mask model.MaskTrace) {
	b.WriteString(textReceiptMaskedIntro + "\n```\n" + receiptMaskedSlot + "\n```\n")
	switch {
	case len(mask.Kinds) > 0:
		parts := make([]string, len(mask.Kinds))
		for i, k := range mask.Kinds {
			parts[i] = fmt.Sprintf("`%s` %d", k.Kind, k.Count)
		}
		fmt.Fprintf(b, "Replaced by the masking: %s.\n", receiptList(parts))
	case mask.Changed:
		b.WriteString(textReceiptMaskUnknown + "\n")
	default:
		b.WriteString(textReceiptMaskUnchanged + "\n")
	}
	b.WriteString(textReceiptMaskCaveat + "\n")
}

// receiptNames lists names as code, so a name cannot format the message, or says "none".
func receiptNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "`" + strings.ReplaceAll(n, "`", "'") + "`"
	}
	return receiptList(quoted)
}

// receiptList joins at most maxReceiptNames entries within maxReceiptListRunes and says how many
// more there are. The first entry is always shown.
func receiptList(entries []string) string {
	shown, runes := 0, 0
	for _, e := range entries[:min(len(entries), maxReceiptNames)] {
		n := utf8.RuneCountInString(e) + 2
		if shown > 0 && runes+n > maxReceiptListRunes {
			break
		}
		shown++
		runes += n
	}
	list := strings.Join(entries[:shown], ", ")
	if more := len(entries) - shown; more > 0 {
		list += fmt.Sprintf(" and %d more", more)
	}
	return list
}

// ReceiptInteraction is a press of a receipt button, as the handler needs it.
type ReceiptInteraction interface {
	// Show answers the press with a message only the member sees. A non-empty directKey adds the
	// button that sends the message as a direct message.
	Show(ctx context.Context, text, directKey string) error
	// Defer acknowledges the press with a message only the member sees, which Edit fills.
	Defer(ctx context.Context) error
	Edit(ctx context.Context, text string) error
	// SendDirect sends text to the member who pressed as a direct message. It returns
	// ErrDirectClosed when Discord refuses it because the member's direct messages are closed.
	SendDirect(ctx context.Context, text string) error
}

// HandleReceipt answers a press of the button under a reply with its receipt. It makes no call
// but the answer, so it answers inside the three seconds Discord gives.
func (h *Handler) HandleReceipt(ctx context.Context, in ReceiptInteraction, key string) {
	defer h.recoverReceiptPanic("HandleReceipt")
	text, directKey, outcome := TextReceiptGone, "", "gone"
	if e, ok := h.receipts.get(key); ok {
		text, directKey, outcome = receiptText(e), key, "shown"
	}
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.replyTimeout)
	defer cancel()
	if err := in.Show(callCtx, text, directKey); err != nil {
		h.logger.Warn("receipt failed", slog.String("error_class", classifyError(err)))
		return
	}
	h.logger.Info("receipt answered", slog.String("outcome", outcome))
}

// HandleReceiptDirect sends a receipt as a direct message, because the member pressed for it.
// The bot opens a direct message for no other reason.
func (h *Handler) HandleReceiptDirect(ctx context.Context, in ReceiptInteraction, key string) {
	defer h.recoverReceiptPanic("HandleReceiptDirect")
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.replyTimeout)
	defer cancel()
	if err := in.Defer(callCtx); err != nil {
		h.logger.Warn("receipt failed", slog.String("error_class", classifyError(err)))
		return
	}
	status, outcome := TextReceiptGone, "gone"
	if e, ok := h.receipts.get(key); ok {
		status, outcome = h.sendDirect(callCtx, in, receiptText(e))
	}
	if err := in.Edit(callCtx, status); err != nil {
		h.logger.Warn("receipt failed", slog.String("error_class", classifyError(err)))
		return
	}
	h.logger.Info("receipt direct message", slog.String("outcome", outcome))
}

func (h *Handler) sendDirect(ctx context.Context, in ReceiptInteraction, text string) (string, string) {
	err := in.SendDirect(ctx, text)
	switch {
	case err == nil:
		return TextReceiptDirectSent, "sent"
	case errors.Is(err, ErrDirectClosed):
		return TextReceiptDirectClosed, "closed"
	default:
		h.logger.Warn("receipt direct message failed", slog.String("error_class", classifyError(err)))
		return TextReceiptDirectFailed, "failure"
	}
}

func (h *Handler) recoverReceiptPanic(where string) {
	if r := recover(); r != nil {
		h.logger.Error("recovered from panic in "+where, slog.String("panic_type", fmt.Sprintf("%T", r)))
	}
}

// receiptKeyFor holds the receipt of a reply and returns the key of its button, or "" when no text
// left the bot or no receipts are held.
func (h *Handler) receiptKeyFor(trace model.Trace, emb embed.Trace, shown bool) string {
	if h.receipts == nil || (!trace.Sent && !emb.Sent) {
		return ""
	}
	key, err := h.receipts.put(trace, emb, shown)
	if err != nil {
		h.logger.Warn("receipt not held", slog.String("error_class", "error"))
		return ""
	}
	return key
}
