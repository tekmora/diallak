package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	// MaskTimeout is the deadline of one masking request, inside the deadline of the whole
	// question (ADR-0013, step two).
	MaskTimeout = 5 * time.Second
	// MaxMaskedBytes bounds the masked text the bot takes from the guardrail route. A longer
	// reply is not understood: a masked question of at most 500 runes stays far below it.
	// Source: chosen, not measured.
	MaxMaskedBytes = 8 << 10
	// MaskProbeAddress is the made-up e-mail address in the fixed text the start sends to the
	// guardrail route. Its domain is reserved for documentation (RFC 2606, section 3).
	MaskProbeAddress = "jane.roe@example.com"
	// MaskProbeText is the fixed text the start and every trial send to the guardrail route
	// before a member's question is masked.
	MaskProbeText = "Please write to " + MaskProbeAddress + " about the setup."
	// guardrailPath is the gateway's route that applies one guardrail to a text. The gateway serves
	// it at the root, not under "/v1" (LiteLLM 1.104.0,
	// litellm/proxy/guardrails/guardrail_endpoints.py).
	guardrailPath = "/guardrails/apply_guardrail"
)

// ErrMaskEmpty reports a reply of the guardrail route that holds no text, or no text the bot
// understands. The model is not called.
var ErrMaskEmpty = errors.New("guardrail reply holds no masked text")

// ErrMaskProbeUnmasked reports that the fixed text came back with its e-mail address unmasked.
var ErrMaskProbeUnmasked = errors.New("guardrail returned the planted e-mail address unmasked")

// MaskTrace is what the bot's own masking request leaves for the member's receipt (ADR-0013,
// step two). It holds the masked text, never the question as written.
type MaskTrace struct {
	// Failed reports that no masked text came back, so the model was not called.
	Failed bool
	// Text is the masked text as the bot sent it on for the model.
	Text string
	// Changed reports that Text differs from the question as written.
	Changed bool
	// Kinds are the kinds of masked data with how many of each, sorted by kind.
	Kinds []KindCount
}

// KindCount is one kind of masked data and how many placeholders of it the masking added.
type KindCount struct {
	Kind  string
	Count int
}

type guardrailRequest struct {
	GuardrailName string `json:"guardrail_name"`
	Text          string `json:"text"`
}

type guardrailResponse struct {
	ResponseText *string `json:"response_text"`
}

// guardrailURL is the address of the guardrail route for a chat base URL. The chat routes are
// served with and without "/v1"; the guardrail route only without, so a trailing "/v1" is dropped.
func guardrailURL(baseURL string) string {
	return strings.TrimSuffix(baseURL, "/v1") + guardrailPath
}

// Masking reports whether the client has a guardrail for its own masking request.
func (c *Client) Masking() bool {
	return c.guardrail != ""
}

// Mask sends text to the guardrail route and returns the masked text. The request has its own
// deadline of MaskTimeout. A reply without text returns ErrMaskEmpty.
func (c *Client) Mask(ctx context.Context, text string) (_ string, err error) {
	if c.guardrail == "" {
		return "", errors.New("no guardrail configured")
	}
	body, err := json.Marshal(guardrailRequest{GuardrailName: c.guardrail, Text: text})
	if err != nil {
		return "", fmt.Errorf("marshal guardrail request: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, MaskTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, guardrailURL(c.baseURL), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create guardrail request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do guardrail request: %w", &TransportError{Err: err})
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close guardrail response body: %w", closeErr))
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return "", drainWith(resp.Body, &StatusError{Code: resp.StatusCode})
	}
	return readMasked(resp.Body)
}

// readMasked reads the masked text from the body of a guardrail reply.
func readMasked(body io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(body, int64(MaxResponseBytes+1)))
	if err != nil {
		return "", fmt.Errorf("read guardrail response body: %w", &TransportError{Err: err})
	}
	var reply guardrailResponse
	if err := json.Unmarshal(data, &reply); err != nil || reply.ResponseText == nil {
		return "", ErrMaskEmpty
	}
	masked := *reply.ResponseText
	if strings.TrimSpace(masked) == "" || len(masked) > MaxMaskedBytes {
		return "", ErrMaskEmpty
	}
	return masked, nil
}

// ProbeMask sends MaskProbeText to the guardrail route. A reply that still holds
// MaskProbeAddress returns ErrMaskProbeUnmasked.
func (c *Client) ProbeMask(ctx context.Context) error {
	masked, err := c.Mask(ctx, MaskProbeText)
	if err != nil {
		return err
	}
	if strings.Contains(masked, MaskProbeAddress) {
		return ErrMaskProbeUnmasked
	}
	return nil
}

var (
	anglePlaceholder    = regexp.MustCompile(`<([A-Z][A-Z0-9_]{0,63})>`)
	redactedPlaceholder = regexp.MustCompile(`\[([A-Z][A-Z0-9_]{0,63})_REDACTED\]`)
)

// maxMaskKinds bounds the kinds a receipt lists.
const maxMaskKinds = 32

// NewMaskTrace compares the question as written with its masked text. It counts the
// placeholders of both default forms, "<KIND>" and "[KIND_REDACTED]", per kind in the masked text
// and subtracts those the question already held, so a quoted placeholder is not counted.
func NewMaskTrace(question, masked string) MaskTrace {
	added := placeholderCounts(masked)
	for kind, n := range placeholderCounts(question) {
		added[kind] -= n
	}
	var kinds []KindCount
	for kind, n := range added {
		if n > 0 {
			kinds = append(kinds, KindCount{Kind: kind, Count: n})
		}
	}
	slices.SortFunc(kinds, func(a, b KindCount) int { return strings.Compare(a.Kind, b.Kind) })
	if len(kinds) > maxMaskKinds {
		kinds = kinds[:maxMaskKinds]
	}
	return MaskTrace{Text: masked, Changed: masked != question, Kinds: kinds}
}

// placeholderCounts counts the placeholders of both forms in text, per kind.
func placeholderCounts(text string) map[string]int {
	counts := make(map[string]int)
	for _, re := range []*regexp.Regexp{anglePlaceholder, redactedPlaceholder} {
		for _, m := range re.FindAllStringSubmatch(text, MaxMaskedBytes) {
			counts[m[1]]++
		}
	}
	return counts
}
