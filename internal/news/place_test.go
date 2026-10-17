package news

import (
	"testing"
	"time"
)

// An open area holds the new items of its feed for the day's digest.
func TestHoldForPlace_Positive_OpenAreaHolds(t *testing.T) {
	s, _ := openTestStore(t)
	s.Record("ai", Result{}, storeNow)
	fresh := s.Record("ai", Result{Items: []Item{item("a", storeNow)}}, storeNow)
	if n := HoldForPlace(s, "ai-news-tools", true, fresh, storeNow); n != 1 {
		t.Fatalf("HoldForPlace() = %d, want 1", n)
	}
	if got := s.Held("ai-news-tools"); len(got) != 1 || got[0].Link != "https://example.org/a" {
		t.Errorf("Held() = %+v", got)
	}
}

// For an area that is not open the items are dropped and stay seen, so the area starts with the
// next day's news when it opens (ADR-0019, point 3).
func TestHoldForPlace_Negative_ClosedAreaDrops(t *testing.T) {
	s, _ := openTestStore(t)
	s.Record("ai", Result{}, storeNow)
	fresh := s.Record("ai", Result{Items: []Item{item("a", storeNow)}}, storeNow)
	if n := HoldForPlace(s, "ai-news-tools", false, fresh, storeNow); n != 0 {
		t.Fatalf("HoldForPlace() for a closed area = %d, want 0", n)
	}
	if got := s.HeldChannels(); len(got) != 0 {
		t.Fatalf("a closed area holds %v", got)
	}

	later := storeNow.Add(24 * time.Hour)
	again := s.Record("ai", Result{Items: []Item{item("a", storeNow), item("b", later)}}, later)
	if n := HoldForPlace(s, "ai-news-tools", true, again, later); n != 1 {
		t.Fatalf("after opening HoldForPlace() = %d, want 1", n)
	}
	if got := s.Held("ai-news-tools"); len(got) != 1 || got[0].Link != "https://example.org/b" {
		t.Errorf("after opening Held() = %+v, want only the next day's item", got)
	}
}

// A place without a channel holds nothing even when it is called open.
func TestHoldForPlace_Boundary_NoChannelNoItems(t *testing.T) {
	s, _ := openTestStore(t)
	if n := HoldForPlace(s, "", true, []Item{item("a", storeNow)}, storeNow); n != 0 {
		t.Errorf("HoldForPlace() without a channel = %d, want 0", n)
	}
	if n := HoldForPlace(s, "ai-news-tools", true, nil, storeNow); n != 0 {
		t.Errorf("HoldForPlace() without items = %d, want 0", n)
	}
	if got := s.HeldChannels(); len(got) != 0 {
		t.Errorf("held %v", got)
	}
}
