package discord

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/model"
)

func maskedTrace(mask model.MaskTrace) model.Trace {
	tr := repliedTrace
	tr.Mask = &mask
	return tr
}

// The planted case of #69, receipt side: the receipt shows the masked text and the counts.
func TestReceiptText_Positive_MaskedTextAndCounts(t *testing.T) {
	mask := model.NewMaskTrace(
		"I am Jane Roe, write to jane.roe@example.com",
		"I am <PERSON>, write to <EMAIL_ADDRESS>")
	text := receiptText(receiptEntry{trace: maskedTrace(mask), shown: true, at: time.Unix(1792224000, 0)})
	for _, want := range []string{
		textReceiptMaskedIntro + "\n```\nI am <PERSON>, write to <EMAIL_ADDRESS>\n```\n",
		"Replaced by the masking: `EMAIL_ADDRESS` 1, `PERSON` 1.",
		textReceiptMaskCaveat, textReceiptMaskWhere, textReceiptMaskKept,
		"Guardrails that ran: `presidio-pii`, `prompt-injection`.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("receipt lacks %q:\n%s", want, text)
		}
	}
	for _, never := range []string{"Jane Roe", "jane.roe@example.com", textReceiptKept, textReceiptWhere, "no personal data"} {
		if strings.Contains(text, never) {
			t.Errorf("receipt holds %q:\n%s", never, text)
		}
	}
}

func TestReceiptText_Positive_UnknownAndUnchangedMasking(t *testing.T) {
	unknown := receiptText(receiptEntry{trace: maskedTrace(model.NewMaskTrace("call 555 0100", "call ***"))})
	if !strings.Contains(unknown, textReceiptMaskUnknown) || strings.Contains(unknown, "Replaced by") {
		t.Errorf("changed text without placeholders:\n%s", unknown)
	}
	q := "Why does the log show <EMAIL_ADDRESS>?"
	unchanged := receiptText(receiptEntry{trace: maskedTrace(model.NewMaskTrace(q, q))})
	if !strings.Contains(unchanged, textReceiptMaskUnchanged) || strings.Contains(unchanged, "Replaced by") {
		t.Errorf("quoted placeholder counted or unchanged text not said:\n%s", unchanged)
	}
	if !strings.Contains(unchanged, textReceiptMaskCaveat) {
		t.Errorf("unchanged receipt lacks the caveat:\n%s", unchanged)
	}
}

func TestReceiptText_Negative_FailedMaskSaysNothingWasSentToTheModel(t *testing.T) {
	tr := model.Trace{Sent: true, Mask: &model.MaskTrace{Failed: true}}
	text := receiptText(receiptEntry{trace: tr})
	if !strings.Contains(text, textReceiptMaskFailed) {
		t.Errorf("receipt lacks the failed text:\n%s", text)
	}
	for _, never := range []string{"Guardrails that ran", textReceiptNoReply, textReceiptMaskedIntro, "```"} {
		if strings.Contains(text, never) {
			t.Errorf("failed-mask receipt holds %q:\n%s", never, text)
		}
	}
}

func TestReceiptText_Negative_MaskedTextCannotLeaveItsCodeBlock(t *testing.T) {
	mask := model.MaskTrace{Text: "```\n**bold** @everyone\x00", Changed: true}
	text := receiptText(receiptEntry{trace: maskedTrace(mask)})
	if strings.Count(text, "```") != 2 {
		t.Errorf("masked text opened or closed a code block:\n%s", text)
	}
	if strings.Contains(text, "\x00") {
		t.Errorf("receipt holds a NUL:\n%s", text)
	}
}

// The worst receipt fits one Discord message, and the masked text it shows is cut and says so.
func TestReceiptText_Boundary_WorstReceiptFitsOneMessage(t *testing.T) {
	names := make([]string, 64)
	for i := range names {
		names[i] = fmt.Sprintf("%03d", i) + strings.Repeat("n", 125)
	}
	kinds := make([]model.KindCount, 32)
	for i := range kinds {
		kinds[i] = model.KindCount{Kind: fmt.Sprintf("K%02d", i) + strings.Repeat("A", 61), Count: 999}
	}
	tr := model.Trace{Sent: true, Replied: true, Guardrails: names, Policies: names, PolicyMissing: true,
		Mask: &model.MaskTrace{Text: strings.Repeat("é", model.MaxMaskedBytes), Changed: true, Kinds: kinds}}
	text := receiptText(receiptEntry{trace: tr, shown: true, at: time.Unix(1792224000, 0)})
	if n := utf8.RuneCountInString(text); n > maxReceiptMessageRunes {
		t.Errorf("worst receipt = %d runes, want at most %d", n, maxReceiptMessageRunes)
	}
	if !strings.Contains(text, textReceiptCut) || !strings.HasSuffix(text, textReceiptAutomated) {
		t.Errorf("worst receipt not cut in the masked text or lost its end:\n%s", text)
	}

	short := model.MaskTrace{Text: strings.Repeat("a", maxReceiptMaskedRunes), Changed: true}
	if got := receiptText(receiptEntry{trace: maskedTrace(short)}); strings.Contains(got, textReceiptCut) {
		t.Errorf("a masked text of exactly %d runes was cut", maxReceiptMaskedRunes)
	}
	long := model.MaskTrace{Text: strings.Repeat("a", maxReceiptMaskedRunes+1), Changed: true}
	if got := receiptText(receiptEntry{trace: maskedTrace(long)}); !strings.Contains(got, textReceiptCut) {
		t.Errorf("a masked text of %d runes was not cut", maxReceiptMaskedRunes+1)
	}
}

func TestReceiptNames_Boundary_ListRuneBudget(t *testing.T) {
	long := strings.Repeat("n", 128)
	got := receiptNames([]string{long, "b", long, "c"})
	if got != "`"+long+"`, `b` and 2 more" {
		t.Errorf("list = %q, want two names within the budget and 2 more", got)
	}
}
