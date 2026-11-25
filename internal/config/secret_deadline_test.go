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
	"time"
)

const (
	fixtureReadTimeout = 50 * time.Millisecond
	fixtureDescription = "fixture"
	fixtureValue       = "example-value"
)

func TestSecretReader_Positive_ReturnsValue(t *testing.T) {
	reader := newSecretReader(fixtureReadTimeout)
	value, err := reader.read(t.Context(), fixtureDescription, readFixtureValue)
	if err != nil || value != fixtureValue {
		t.Fatalf("read did not return the fixture value: %v", err)
	}
}

func TestSecretReader_Negative_BlockedReadTimesOut(t *testing.T) {
	reader := newSecretReader(fixtureReadTimeout)
	_, result := startBlockedSecretRead(t, reader)
	err := waitSecretReadError(t, result)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), fixtureDescription) {
		t.Fatalf("read returned %v, want a deadline error naming the file description", err)
	}
}

func TestSecretReader_Negative_RejectsReadWhilePreviousIsStalled(t *testing.T) {
	reader := newSecretReader(fixtureReadTimeout)
	_, first := startBlockedSecretRead(t, reader)
	if err := waitSecretReadError(t, first); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first read returned %v, want a deadline error", err)
	}

	const nextDescription = "next fixture"
	var calls atomic.Int32
	result := make(chan error, 1)
	ctx := t.Context()
	go func() {
		_, err := reader.read(ctx, nextDescription, func() (string, error) {
			calls.Add(1)
			return readFixtureValue()
		})
		result <- err
	}()
	if err := waitSecretReadError(t, result); err == nil || !strings.Contains(err.Error(), nextDescription) ||
		!strings.Contains(err.Error(), "previous secret file read has not returned") {
		t.Errorf("second read returned %v, want a pending-read error naming the file description", err)
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("second raw read called %d times, want zero", got)
	}
}

func TestSecretReader_Boundary_CancelledContextSkipsRead(t *testing.T) {
	reader := newSecretReader(fixtureReadTimeout)
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

func TestSecretReader_Boundary_RecoversAfterStalledReadReturns(t *testing.T) {
	reader := newSecretReader(fixtureReadTimeout)
	release, result := startBlockedSecretRead(t, reader)
	if err := waitSecretReadError(t, result); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked read returned %v, want a deadline error", err)
	}
	release()
	for range 100 {
		value, err := reader.read(t.Context(), fixtureDescription, readFixtureValue)
		if err == nil {
			if value != fixtureValue {
				t.Fatal("read after release did not return the fixture value")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("read did not recover after 100 attempts following release")
}

func readFixtureValue() (string, error) {
	return fixtureValue, nil
}

func startBlockedSecretRead(t *testing.T, reader *secretReader) (func(), <-chan error) {
	t.Helper()
	started := make(chan struct{})
	blocked := make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)

	result := make(chan error, 1)
	ctx := t.Context()
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
