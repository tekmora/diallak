package discord

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/answer"
)

type fakeInteraction struct {
	guildID    uint64
	question   string
	deferErr   error
	replyErr   error
	deferBlock chan struct{}
	replyBlock chan struct{}

	mu         sync.Mutex
	deferCalls int
	replyCalls int
	replies    []string
}

func (f *fakeInteraction) GuildID() uint64 {
	return f.guildID
}

func (f *fakeInteraction) Question() string {
	return f.question
}

func (f *fakeInteraction) Defer(ctx context.Context) error {
	f.mu.Lock()
	f.deferCalls++
	block := f.deferBlock
	err := f.deferErr
	f.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (f *fakeInteraction) Reply(ctx context.Context, text string) error {
	f.mu.Lock()
	f.replyCalls++
	f.replies = append(f.replies, text)
	block := f.replyBlock
	err := f.replyErr
	f.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (f *fakeInteraction) ReplyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.replyCalls
}

func (f *fakeInteraction) Replies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.replies...)
}

type fakeAsker struct {
	askFunc func(ctx context.Context, question string) (answer.Answer, error)
	mu      sync.Mutex
	calls   int
}

func (f *fakeAsker) Ask(ctx context.Context, question string) (answer.Answer, error) {
	f.mu.Lock()
	f.calls++
	fn := f.askFunc
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, question)
	}
	return answer.Answer{
		Kind: answer.KindAnswer,
		Text: "Answer to: " + question,
	}, nil
}

func (f *fakeAsker) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestHandler_HandleAsk_Positive_BranchesAndReplies(t *testing.T) {
	const hubGuildID = 42

	cases := []struct {
		name        string
		interaction *fakeInteraction
		asker       *fakeAsker
		wantReply   string
		wantAsks    int
	}{
		{
			name: "wrong guild refused without ask",
			interaction: &fakeInteraction{
				guildID:  999,
				question: "What is this?",
			},
			asker:     &fakeAsker{},
			wantReply: answer.AppendFooter(TextWrongGuild, ""),
			wantAsks:  0,
		},
		{
			name: "missing guild zero refused without ask",
			interaction: &fakeInteraction{
				guildID:  0,
				question: "What is this?",
			},
			asker:     &fakeAsker{},
			wantReply: answer.AppendFooter(TextWrongGuild, ""),
			wantAsks:  0,
		},
		{
			name: "empty question refused without ask",
			interaction: &fakeInteraction{
				guildID:  hubGuildID,
				question: "   ",
			},
			asker:     &fakeAsker{},
			wantReply: answer.AppendFooter(TextBadQuestion, ""),
			wantAsks:  0,
		},
		{
			name: "oversized question refused without ask",
			interaction: &fakeInteraction{
				guildID:  hubGuildID,
				question: strings.Repeat("a", answer.MaxQuestionRunes+1),
			},
			asker:     &fakeAsker{},
			wantReply: answer.AppendFooter(TextBadQuestion, ""),
			wantAsks:  0,
		},
		{
			name: "invalid utf8 refused without ask",
			interaction: &fakeInteraction{
				guildID:  hubGuildID,
				question: string([]byte{0xff, 0xfe, 0xfd}),
			},
			asker:     &fakeAsker{},
			wantReply: answer.AppendFooter(TextBadQuestion, ""),
			wantAsks:  0,
		},
		{
			name: "asker failure replies with failure text",
			interaction: &fakeInteraction{
				guildID:  hubGuildID,
				question: "How does it work?",
			},
			asker: &fakeAsker{
				askFunc: func(_ context.Context, _ string) (answer.Answer, error) {
					return answer.Answer{}, errors.New("backend error")
				},
			},
			wantReply: answer.AppendFooter(TextFailure, ""),
			wantAsks:  1,
		},
		{
			name: "asker success replies with rendered answer",
			interaction: &fakeInteraction{
				guildID:  hubGuildID,
				question: "How does it work?",
			},
			asker: &fakeAsker{
				askFunc: func(_ context.Context, _ string) (answer.Answer, error) {
					return answer.Answer{
						Kind: answer.KindAnswer,
						Text: "Verified instructions.",
					}, nil
				},
			},
			wantReply: answer.Render(answer.Answer{
				Kind: answer.KindAnswer,
				Text: "Verified instructions.",
			}, answer.RenderOptions{}),
			wantAsks: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewHandler(HandlerConfig{
				Asker:         tc.asker,
				GuildID:       hubGuildID,
				MaxConcurrent: 4,
			})
			if err != nil {
				t.Fatalf("NewHandler() = %v, want nil", err)
			}

			h.HandleAsk(context.Background(), tc.interaction)

			if got := tc.asker.Calls(); got != tc.wantAsks {
				t.Errorf("asker calls = %d, want %d", got, tc.wantAsks)
			}
			if got := tc.interaction.ReplyCount(); got != 1 {
				t.Errorf("reply count = %d, want 1", got)
			}
			replies := tc.interaction.Replies()
			if len(replies) > 0 && replies[0] != tc.wantReply {
				t.Errorf("reply = %q, want %q", replies[0], tc.wantReply)
			}
			if inFlight := h.InFlight(); inFlight != 0 {
				t.Errorf("inFlight after execution = %d, want 0", inFlight)
			}
		})
	}
}

func TestHandleAsk_Negative_DeferErrorAbandonsAsk(t *testing.T) {
	asker := &fakeAsker{}
	h, err := NewHandler(HandlerConfig{
		Asker:   asker,
		GuildID: 42,
	})
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}

	in := &fakeInteraction{
		guildID:  42,
		question: "Will defer error abandon?",
		deferErr: errors.New("discord network error"),
	}

	h.HandleAsk(context.Background(), in)

	if asker.Calls() != 0 {
		t.Errorf("asker was called after defer error: %d", asker.Calls())
	}
	if in.ReplyCount() != 0 {
		t.Errorf("reply was called after defer error: %d", in.ReplyCount())
	}
	if h.InFlight() != 0 {
		t.Errorf("slot not released after defer error: %d", h.InFlight())
	}
}

func TestHandleAsk_Negative_BusyPathRefusesWhenSlotsFull(t *testing.T) {
	blockCh := make(chan struct{})
	var closeOnce sync.Once
	closeBlock := func() { closeOnce.Do(func() { close(blockCh) }) }
	defer closeBlock()

	asker := &fakeAsker{
		askFunc: func(ctx context.Context, _ string) (answer.Answer, error) {
			select {
			case <-blockCh:
				return answer.Answer{Kind: answer.KindRefusal}, nil
			case <-ctx.Done():
				return answer.Answer{}, ctx.Err()
			}
		},
	}

	h, err := NewHandler(HandlerConfig{
		Asker:         asker,
		GuildID:       42,
		MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}

	firstIn := &fakeInteraction{guildID: 42, question: "First slow inquiry"}
	secondIn := &fakeInteraction{guildID: 42, question: "Second inquiry while busy"}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.HandleAsk(context.Background(), firstIn)
	}()

	for i := 0; i < 50; i++ {
		if h.InFlight() == 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	h.HandleAsk(context.Background(), secondIn)

	if secondIn.ReplyCount() != 1 {
		t.Errorf("second reply count = %d, want 1", secondIn.ReplyCount())
	}
	wantBusy := answer.AppendFooter(TextBusy, "")
	if replies := secondIn.Replies(); len(replies) > 0 && replies[0] != wantBusy {
		t.Errorf("second reply = %q, want %q", replies[0], wantBusy)
	}

	closeBlock()
	wg.Wait()
	if h.InFlight() != 0 {
		t.Errorf("inFlight = %d, want 0", h.InFlight())
	}
}

func TestHandleAsk_Negative_LogCaptureNeverContainsQuestionOrError(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	const secretQuestion = "SECRET_USER_QUESTION_TOKEN_REVEAL_9999"
	const secretError = "SECRET_INTERNAL_DB_ERROR_MESSAGE_8888"

	asker := &fakeAsker{
		askFunc: func(_ context.Context, _ string) (answer.Answer, error) {
			return answer.Answer{}, errors.New(secretError)
		},
	}

	h, err := NewHandler(HandlerConfig{
		Asker:   asker,
		GuildID: 42,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}

	in := &fakeInteraction{
		guildID:  42,
		question: secretQuestion,
	}

	h.HandleAsk(context.Background(), in)

	logOutput := logBuf.String()
	if strings.Contains(logOutput, secretQuestion) {
		t.Errorf("log output leaks the question text: %s", logOutput)
	}
	if strings.Contains(logOutput, secretError) {
		t.Errorf("log output leaks the error text: %s", logOutput)
	}
	if !strings.Contains(logOutput, "failure") {
		t.Errorf("log output missing failure outcome: %s", logOutput)
	}
}

func TestHandleAsk_Boundary_ReplySentAfterAskDeadlineExpires(t *testing.T) {
	asker := &fakeAsker{
		askFunc: func(ctx context.Context, _ string) (answer.Answer, error) {
			<-ctx.Done()
			return answer.Answer{}, ctx.Err()
		},
	}

	h, err := NewHandler(HandlerConfig{
		Asker:        asker,
		GuildID:      42,
		AskTimeout:   10 * time.Millisecond,
		ReplyTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}

	in := &fakeInteraction{guildID: 42, question: "Will ask deadline timeout?"}
	h.HandleAsk(context.Background(), in)

	if in.ReplyCount() != 1 {
		t.Fatalf("reply count = %d, want 1", in.ReplyCount())
	}
	wantFailure := answer.AppendFooter(TextFailure, "")
	if replies := in.Replies(); len(replies) > 0 && replies[0] != wantFailure {
		t.Errorf("reply = %q, want %q", replies[0], wantFailure)
	}
	if h.InFlight() != 0 {
		t.Errorf("inFlight = %d, want 0", h.InFlight())
	}
}

func TestHandler_Wait_Positive_And_Negative(t *testing.T) {
	blockCh := make(chan struct{})
	asker := &fakeAsker{
		askFunc: func(ctx context.Context, _ string) (answer.Answer, error) {
			select {
			case <-blockCh:
				return answer.Answer{Kind: answer.KindRefusal}, nil
			case <-ctx.Done():
				return answer.Answer{}, ctx.Err()
			}
		},
	}

	h, err := NewHandler(HandlerConfig{
		Asker:         asker,
		GuildID:       42,
		MaxConcurrent: 2,
	})
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}

	in := &fakeInteraction{guildID: 42, question: "Waiting test"}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.HandleAsk(context.Background(), in)
	}()

	for i := 0; i < 50; i++ {
		if h.InFlight() == 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	timeoutCtx, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelTimeout()
	if err := h.Wait(timeoutCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait() before unblock = %v, want DeadlineExceeded", err)
	}

	close(blockCh)
	wg.Wait()

	if err := h.Wait(context.Background()); err != nil {
		t.Errorf("Wait() after completion = %v, want nil", err)
	}
}

func TestHandler_Positive_ConcurrentInteractionsRaceClean(t *testing.T) {
	const totalInteractions = 50
	const maxConcurrent = 4

	asker := &fakeAsker{
		askFunc: func(_ context.Context, q string) (answer.Answer, error) {
			time.Sleep(5 * time.Millisecond)
			return answer.Answer{
				Kind: answer.KindAnswer,
				Text: "Processed: " + q,
			}, nil
		},
	}

	h, err := NewHandler(HandlerConfig{
		Asker:         asker,
		GuildID:       42,
		MaxConcurrent: maxConcurrent,
	})
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}

	var wg sync.WaitGroup
	var answeredCount int64
	var busyCount int64
	var countMu sync.Mutex

	for i := 0; i < totalInteractions; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			in := &fakeInteraction{
				guildID:  42,
				question: fmt.Sprintf("Question %d", idx),
			}
			h.HandleAsk(context.Background(), in)

			replies := in.Replies()
			if len(replies) == 1 {
				countMu.Lock()
				if replies[0] == TextBusy {
					busyCount++
				} else {
					answeredCount++
				}
				countMu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	countMu.Lock()
	total := answeredCount + busyCount
	countMu.Unlock()

	if total != totalInteractions {
		t.Fatalf("total replied = %d (answered=%d, busy=%d), want %d", total, answeredCount, busyCount, totalInteractions)
	}
	if h.InFlight() != 0 {
		t.Errorf("inFlight = %d, want 0", h.InFlight())
	}
}

func TestCommandDescription_Positive_ExactStringPin(t *testing.T) {
	const wantDesc = "Ask the automated assistant a question about the project documentation"
	if CommandDescription != wantDesc {
		t.Errorf("CommandDescription = %q, want %q", CommandDescription, wantDesc)
	}
}

func TestHandler_Positive_FooterIsLastLineForAdapterTexts(t *testing.T) {
	const hubGuildID = 100

	places := []struct {
		desc       string
		channel    string
		wantFooter string
	}{
		{
			desc:       "default place",
			channel:    "",
			wantFooter: "Automated reply. A person can help in the support channel.",
		},
		{
			desc:       "configured place #help-desk",
			channel:    "#help-desk",
			wantFooter: "Automated reply. A person can help in #help-desk.",
		},
	}

	for _, p := range places {
		t.Run(p.desc, func(t *testing.T) {
			adapterTexts := []struct {
				name    string
				trigger func(h *Handler) string
			}{
				{
					name: "TextWrongGuild",
					trigger: func(h *Handler) string {
						in := &fakeInteraction{guildID: 999, question: "valid question"}
						h.HandleAsk(context.Background(), in)
						return in.Replies()[0]
					},
				},
				{
					name: "TextBadQuestion",
					trigger: func(h *Handler) string {
						in := &fakeInteraction{guildID: hubGuildID, question: "   "}
						h.HandleAsk(context.Background(), in)
						return in.Replies()[0]
					},
				},
				{
					name: "TextFailure",
					trigger: func(h *Handler) string {
						in := &fakeInteraction{guildID: hubGuildID, question: "valid question"}
						h.HandleAsk(context.Background(), in)
						return in.Replies()[0]
					},
				},
				{
					name: "TextBusy",
					trigger: func(h *Handler) string {
						block := make(chan struct{})
						firstIn := &fakeInteraction{
							guildID:    hubGuildID,
							question:   "Holding slot",
							replyBlock: block,
						}
						go h.HandleAsk(context.Background(), firstIn)
						for i := 0; i < 50; i++ {
							if h.InFlight() == 1 {
								break
							}
							time.Sleep(2 * time.Millisecond)
						}
						secondIn := &fakeInteraction{guildID: hubGuildID, question: "Second question"}
						h.HandleAsk(context.Background(), secondIn)
						close(block)
						return secondIn.Replies()[0]
					},
				},
			}

			for _, at := range adapterTexts {
				t.Run(at.name, func(t *testing.T) {
					asker := &fakeAsker{
						askFunc: func(_ context.Context, _ string) (answer.Answer, error) {
							return answer.Answer{}, errors.New("simulated error")
						},
					}
					h, err := NewHandler(HandlerConfig{
						Asker:          asker,
						GuildID:        hubGuildID,
						MaxConcurrent:  1,
						SupportChannel: p.channel,
					})
					if err != nil {
						t.Fatalf("NewHandler() = %v", err)
					}

					reply := at.trigger(h)
					if !strings.HasSuffix(reply, "\n\n"+p.wantFooter) {
						t.Fatalf("reply does not end with blank line and footer %q:\n%s", p.wantFooter, reply)
					}
					lines := strings.Split(reply, "\n")
					lastLine := lines[len(lines)-1]
					if lastLine != p.wantFooter {
						t.Errorf("last line = %q, want %q", lastLine, p.wantFooter)
					}
					if len(lines) < 3 || lines[len(lines)-2] != "" {
						t.Errorf("expected blank line before footer, got line: %q", lines[len(lines)-2])
					}
				})
			}
		})
	}
}
