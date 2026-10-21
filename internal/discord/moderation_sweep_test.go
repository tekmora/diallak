package discord

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"
)

// fakeTrail is a trail channel: messages newest first, as Discord lists them.
type fakeTrail struct {
	messages  []TrailMessage
	deleteErr error
	listErr   error

	lists   []uint64
	deleted []uint64
}

func (f *fakeTrail) ListLines(_ context.Context, channelID, before uint64) ([]TrailMessage, error) {
	if channelID != modTrailID {
		return nil, errors.New("wrong channel")
	}
	f.lists = append(f.lists, before)
	if f.listErr != nil {
		return nil, f.listErr
	}
	var page []TrailMessage
	for _, m := range f.messages {
		if (before == 0 || m.ID < before) && len(page) < trailPageSize {
			page = append(page, m)
		}
	}
	return page, nil
}

func (f *fakeTrail) DeleteLine(_ context.Context, channelID, messageID uint64) error {
	if channelID != modTrailID {
		return errors.New("wrong channel")
	}
	f.deleted = append(f.deleted, messageID)
	return f.deleteErr
}

// trailOf returns n lines of the bot, newest first, one hour apart from modNow back.
func trailOf(n int) *fakeTrail {
	f := &fakeTrail{}
	for i := range n {
		f.messages = append(f.messages, TrailMessage{
			ID: uint64(1_000_000 - i), AuthorID: modBotUserID, Sent: modNow.Add(-time.Duration(i) * time.Hour),
		})
	}
	return f
}

func newSweepHandler(t *testing.T, retention time.Duration) *ModerationHandler {
	t.Helper()
	h, err := NewModerationHandler(ModerationConfig{
		Definition: modTestDefinition(), GuildID: modGuildID, Logger: slog.New(slog.DiscardHandler),
		Retention: retention, Now: func() time.Time { return modNow },
	})
	if err != nil {
		t.Fatalf("NewModerationHandler: %v", err)
	}
	h.SetServer(modTestRoles(), modTestChannels(), modBotUserID)
	return h
}

func TestModerationSweep_Positive_DeletesOnlyTheBotsOwnOldLines(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	trail := &fakeTrail{messages: []TrailMessage{
		{ID: 50, AuthorID: modBotUserID, Sent: modNow.Add(-time.Hour)},
		{ID: 40, AuthorID: modModeratorID, Sent: modNow.Add(-48 * time.Hour)},
		{ID: 30, AuthorID: modBotUserID, Sent: modNow.Add(-48 * time.Hour)},
		{ID: 20, AuthorID: modBotUserID, Sent: modNow.Add(-400 * 24 * time.Hour)},
	}}
	n, err := h.Sweep(context.Background(), trail)
	if err != nil || n != 2 || !slices.Equal(trail.deleted, []uint64{30, 20}) {
		t.Errorf("Sweep = %d, %v; deleted %v; want 2, nil, [30 20]", n, err, trail.deleted)
	}
	if !slices.Equal(trail.lists, []uint64{0}) {
		t.Errorf("lists = %v, want one page from the newest", trail.lists)
	}
}

func TestModerationSweep_Positive_PagesFromTheOldestIDOfEachPage(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	trail := trailOf(250)
	n, err := h.Sweep(context.Background(), trail)
	// Lines 24 hours and older are past the retention: hours 25 to 249 = 225 lines.
	if err != nil || n != 225 {
		t.Errorf("Sweep = %d, %v; want 225, nil", n, err)
	}
	want := []uint64{0, 1_000_000 - 99, 1_000_000 - 199}
	if !slices.Equal(trail.lists, want) {
		t.Errorf("lists = %v, want %v", trail.lists, want)
	}
}

func TestModerationSweep_Boundary_LineAtTheRetentionStays(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	trail := &fakeTrail{messages: []TrailMessage{
		{ID: 30, AuthorID: modBotUserID, Sent: modNow.Add(-MinTrailRetention)},
		{ID: 20, AuthorID: modBotUserID, Sent: modNow.Add(-MinTrailRetention - time.Nanosecond)},
	}}
	n, err := h.Sweep(context.Background(), trail)
	if err != nil || n != 1 || !slices.Equal(trail.deleted, []uint64{20}) {
		t.Errorf("Sweep = %d, %v; deleted %v; want only the line past the retention", n, err, trail.deleted)
	}
}

func TestModerationSweep_Boundary_StopsAtThePageBound(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	trail := trailOf(trailPageSize*maxTrailPages + 1)
	if _, err := h.Sweep(context.Background(), trail); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(trail.lists) != maxTrailPages {
		t.Errorf("%d pages read, want %d", len(trail.lists), maxTrailPages)
	}
}

func TestModerationSweep_Boundary_FullLastPageReadsOneEmptyPage(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	trail := trailOf(trailPageSize)
	if _, err := h.Sweep(context.Background(), trail); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(trail.lists) != 2 {
		t.Errorf("%d pages read, want 2", len(trail.lists))
	}
}

func TestModerationSweep_Negative_ErrorsStopTheSweep(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	listFails := trailOf(30)
	listFails.listErr = errors.New("connection reset")
	if _, err := h.Sweep(context.Background(), listFails); err == nil {
		t.Error("Sweep with a failing list succeeded, want an error")
	}

	deleteFails := trailOf(30)
	deleteFails.deleteErr = ErrDiscordRefused
	if n, err := h.Sweep(context.Background(), deleteFails); err == nil || n != 0 || len(deleteFails.deleted) != 1 {
		t.Errorf("Sweep = %d, %v after %d deletes; want an error after the first", n, err, len(deleteFails.deleted))
	}

	gone := trailOf(30)
	gone.deleteErr = ErrDiscordGone
	if n, err := h.Sweep(context.Background(), gone); err != nil || n != 5 {
		t.Errorf("Sweep = %d, %v; want a line already gone counted, 5, nil", n, err)
	}

	unresolved, err := NewModerationHandler(ModerationConfig{Definition: modTestDefinition(), GuildID: modGuildID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unresolved.Sweep(context.Background(), trailOf(3)); err == nil {
		t.Error("Sweep before the server was read succeeded, want an error")
	}
}

func TestModerationSweep_Boundary_ShortPageEndsTheSweep(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	trail := trailOf(trailPageSize - 1)
	if _, err := h.Sweep(context.Background(), trail); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(trail.lists) != 1 {
		t.Errorf("%d pages read, want 1", len(trail.lists))
	}
}

type cursorStuckTrail struct {
	*fakeTrail
}

func (c *cursorStuckTrail) ListLines(ctx context.Context, channelID, before uint64) ([]TrailMessage, error) {
	if before == 0 {
		return c.fakeTrail.ListLines(ctx, channelID, before)
	}
	c.lists = append(c.lists, before)
	page := make([]TrailMessage, trailPageSize)
	for i := range page {
		page[i] = TrailMessage{ID: before, AuthorID: modBotUserID, Sent: modNow.Add(-48 * time.Hour)}
	}
	return page, nil
}

func TestModerationSweep_Positive_DecreasingOldestPagesAcrossMultiplePages(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	trail := trailOf(200)
	n, err := h.Sweep(context.Background(), trail)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(trail.lists) != 3 { // 100, 100, 0
		t.Errorf("lists = %d, want 3", len(trail.lists))
	}
	_ = n
}

func TestModerationSweep_Negative_FullPageWithZeroIDStops(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	trail := &fakeTrail{}
	for range trailPageSize {
		trail.messages = append(trail.messages, TrailMessage{
			ID: 0, AuthorID: modBotUserID, Sent: modNow.Add(-48 * time.Hour),
		})
	}
	_, err := h.Sweep(context.Background(), trail)
	if err == nil {
		t.Fatal("Sweep on full page of ID 0 succeeded, want error")
	}
	if len(trail.lists) != 1 {
		t.Errorf("lists = %d, want 1", len(trail.lists))
	}
}

func TestModerationSweep_Boundary_OldestNotMovingStopsSweep(t *testing.T) {
	h := newSweepHandler(t, MinTrailRetention)
	base := trailOf(trailPageSize)
	stuck := &cursorStuckTrail{fakeTrail: base}
	_, err := h.Sweep(context.Background(), stuck)
	if err == nil {
		t.Fatal("Sweep on stuck cursor succeeded, want error")
	}
	if len(stuck.lists) != 2 {
		t.Errorf("lists = %d, want 2", len(stuck.lists))
	}
}
