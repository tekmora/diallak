// SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
// SPDX-License-Identifier: EUPL-1.2

package config

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

const (
	fixtureReadTimeout = 50 * time.Millisecond
	fixtureDescription = "fixture"
	fixtureValue       = "example-value"
)

func TestSecretReader_Positive_ReturnsValue(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(secretReadTimeout)
		value, err := reader.read(test.Context(), fixtureDescription, readFixtureValue)
		if err != nil || value != fixtureValue {
			test.Fatalf("read did not return the fixture value: %v", err)
		}
	})
}

func TestSecretReader_Positive_BackToBackReads(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(secretReadTimeout)
		ctx := test.Context()
		for attempt := range 3 {
			slotReleaseAllowed := make(chan struct{})
			value, err := reader.read(ctx, fixtureDescription, func() (string, error) {
				// Hold the slot token until the worker and caller are both blocked.
				<-reader.slot
				go func() {
					synctest.Wait()
					close(slotReleaseAllowed)
					reader.slot <- struct{}{}
				}()
				return readFixtureValue()
			})
			if err != nil || value != fixtureValue {
				test.Fatalf("read %d did not return the fixture value: %v", attempt+1, err)
			}
			select {
			case <-slotReleaseAllowed:
			default:
				test.Fatalf("read %d returned before the worker could release the slot", attempt+1)
			}
		}
	})
}

func TestSecretReader_Negative_BlockedReadTimesOut(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(fixtureReadTimeout)
		_, result := startBlockedSecretRead(test, test.Context(), reader)
		err := waitSecretReadError(test, result)
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), fixtureDescription) {
			test.Fatalf("read returned %v, want a deadline error naming the file description", err)
		}
	})
}

func TestSecretReader_Negative_BlockedReadIsCanceled(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(secretReadTimeout)
		ctx, cancel := context.WithCancel(test.Context())
		defer cancel()
		_, result := startBlockedSecretRead(test, ctx, reader)
		cancel()
		err := waitSecretReadError(test, result)
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), fixtureDescription) {
			test.Fatalf("read returned %v, want a cancellation error naming the file description", err)
		}
	})
}

func TestSecretReader_Negative_RejectsReadWhilePreviousIsStalled(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(fixtureReadTimeout)
		ctx := test.Context()
		_, first := startBlockedSecretRead(test, ctx, reader)
		if firstErr := waitSecretReadError(test, first); !errors.Is(firstErr, context.DeadlineExceeded) {
			test.Fatalf("first read returned %v, want a deadline error", firstErr)
		}

		const nextDescription = "next fixture"
		var calls atomic.Int32
		result := make(chan error, 1)
		go func() {
			_, readErr := reader.read(ctx, nextDescription, func() (string, error) {
				calls.Add(1)
				return readFixtureValue()
			})
			result <- readErr
		}()
		if err := waitSecretReadError(test, result); err == nil || !strings.Contains(err.Error(), nextDescription) ||
			!strings.Contains(err.Error(), "previous secret file read has not returned") {
			test.Errorf("second read returned %v, want a pending-read error naming the file description", err)
		}
		if got := calls.Load(); got != 0 {
			test.Errorf("second raw read called %d times, want zero", got)
		}
	})
}

func TestSecretReader_Boundary_CancelledContextSkipsRead(t *testing.T) {
	reader := newSecretReader(secretReadTimeout)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var calls atomic.Int32
	_, err := reader.read(ctx, fixtureDescription, func() (string, error) {
		calls.Add(1)
		return readFixtureValue()
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), fixtureDescription) {
		t.Errorf("read returned %v, want a cancellation error naming the file description", err)
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("raw read called %d times, want zero", got)
	}
}

func TestSecretReader_Boundary_CancellationWinsOverBusySlot(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(secretReadTimeout)
		reader.slot <- struct{}{}
		done := make(chan struct{})
		close(done)
		ctx := &cancelAfterEntryContext{Context: test.Context(), done: done}
		var calls int
		_, err := reader.read(ctx, fixtureDescription, func() (string, error) {
			calls++
			return readFixtureValue()
		})
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), fixtureDescription) {
			test.Errorf("read returned %v, want a cancellation error naming the file description", err)
		}
		synctest.Wait()
		if calls != 0 {
			test.Errorf("raw read called %d times, want zero", calls)
		}
		if got := len(reader.slot); got != 1 {
			test.Errorf("read slot holds %d tokens, want one", got)
		}
	})
}

func TestSecretReader_Boundary_CancellationWinsOverResult(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(secretReadTimeout)
		ctx, cancel := context.WithCancel(test.Context())
		defer cancel()
		value, err := reader.read(ctx, fixtureDescription, func() (string, error) {
			cancel()
			return readFixtureValue()
		})
		if value != "" || !errors.Is(err, context.Canceled) {
			test.Errorf("read returned %v, want a cancellation error and an empty value", err)
		}

		// Exercise the reader's selection with both channels ready before it starts.
		result := make(chan secretReadResult, 1)
		result <- secretReadResult{value: fixtureValue}
		readyValue, readyErr := waitSecretReadResult(ctx, fixtureDescription, result)
		if readyValue != "" || !errors.Is(readyErr, context.Canceled) {
			test.Errorf("read with both channels ready returned %v, want a cancellation error and an empty value", readyErr)
		}
	})
}

func TestSecretReader_Boundary_CancelledWorkerSkipsRead(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(secretReadTimeout)
		done := make(chan struct{})
		close(done)
		ctx := &cancelAfterEntryContext{Context: test.Context(), done: done}
		var calls int
		_, err := reader.read(ctx, fixtureDescription, func() (string, error) {
			calls++
			return readFixtureValue()
		})
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), fixtureDescription) {
			test.Errorf("read returned %v, want a cancellation error naming the file description", err)
		}
		synctest.Wait()
		if calls != 0 {
			test.Errorf("raw read called %d times, want zero", calls)
		}
		if len(reader.slot) != 0 {
			test.Error("cancelled worker did not release the read slot")
		}
	})
}

// cancelAfterEntryContext makes the entry check succeed and cancels the derived context synchronously.
type cancelAfterEntryContext struct {
	context.Context
	done    <-chan struct{}
	checked atomic.Bool
}

func (c *cancelAfterEntryContext) Done() <-chan struct{} {
	return c.done
}

func (c *cancelAfterEntryContext) Err() error {
	if !c.checked.Swap(true) {
		return nil
	}
	return context.Canceled
}

func TestSecretReader_Boundary_RecoversAfterStalledReadReturns(t *testing.T) {
	synctest.Test(t, func(test *testing.T) {
		reader := newSecretReader(fixtureReadTimeout)
		ctx := test.Context()
		release, result := startBlockedSecretRead(test, ctx, reader)
		if blockedErr := waitSecretReadError(test, result); !errors.Is(blockedErr, context.DeadlineExceeded) {
			test.Fatalf("blocked read returned %v, want a deadline error", blockedErr)
		}
		release()
		recoveryReader := &secretReader{slot: reader.slot, timeout: secretReadTimeout}
		for range 100 {
			value, err := recoveryReader.read(ctx, fixtureDescription, readFixtureValue)
			if err == nil {
				if value != fixtureValue {
					test.Fatal("read after release did not return the fixture value")
				}
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		test.Fatal("read did not recover after 100 attempts following release")
	})
}

func readFixtureValue() (string, error) {
	return fixtureValue, nil
}

func startBlockedSecretRead(t *testing.T, ctx context.Context, reader *secretReader) (func(), <-chan error) {
	t.Helper()
	started := make(chan struct{})
	blocked := make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)

	result := make(chan error, 1)
	go func() {
		_, err := reader.read(ctx, fixtureDescription, func() (string, error) {
			close(started)
			<-blocked
			return readFixtureValue()
		})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("raw read did not start within 5 seconds")
	}
	return release, result
}

func waitSecretReadError(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("read did not return within 5 seconds")
		return nil
	}
}
