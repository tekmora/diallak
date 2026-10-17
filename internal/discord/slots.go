package discord

import (
	"context"
	"sync"
)

// slots bounds the work a handler runs at the same time. A caller that finds no free slot does not
// wait for one.
type slots struct {
	max int

	mu          sync.Mutex
	inFlight    int
	zeroWaiters []chan struct{}
}

func (s *slots) inFlightCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inFlight
}

// acquire takes a slot and reports whether one was free.
func (s *slots) acquire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight >= s.max {
		return false
	}
	s.inFlight++
	return true
}

func (s *slots) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight--
	if s.inFlight == 0 {
		for _, ch := range s.zeroWaiters {
			close(ch)
		}
		s.zeroWaiters = nil
	}
}

// wait blocks until no slot is taken or ctx is cancelled.
func (s *slots) wait(ctx context.Context) error {
	s.mu.Lock()
	if s.inFlight == 0 {
		s.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	s.zeroWaiters = append(s.zeroWaiters, ch)
	s.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		s.removeZeroWaiter(ch)
		s.mu.Unlock()
		return ctx.Err()
	}
}

func (s *slots) removeZeroWaiter(ch chan struct{}) {
	for i, w := range s.zeroWaiters {
		if w == ch {
			s.zeroWaiters = append(s.zeroWaiters[:i], s.zeroWaiters[i+1:]...)
			break
		}
	}
}
