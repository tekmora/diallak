//go:build semgrep_fixture

package fixture

import (
	"context"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

func each(items []string, work func(string)) {
	// ruleid: no-unbounded-goroutine-in-loop
	for _, it := range items {
		go work(it)
	}
}

func eachIndexed(items []string, work func(string)) {
	// ruleid: no-unbounded-goroutine-in-loop
	for i := 0; i < len(items); i++ {
		go work(items[i])
	}
}

func eachSendInside(items []string, out chan<- string) {
	// ruleid: no-unbounded-goroutine-in-loop
	for _, it := range items {
		go func() { out <- it }()
	}
}

func eachGroup(ctx context.Context, items []string, work func(string) error) error {
	g, _ := errgroup.WithContext(ctx)
	// ruleid: no-unbounded-goroutine-in-loop
	for _, it := range items {
		g.Go(func() error { return work(it) })
	}
	return g.Wait()
}

func eachSemaphoreChannel(items []string, work func(string)) {
	slots := make(chan struct{}, 4)
	// ok: no-unbounded-goroutine-in-loop
	for _, it := range items {
		slots <- struct{}{}
		go func() { defer func() { <-slots }(); work(it) }()
	}
}

func eachWeighted(ctx context.Context, items []string, work func(string)) error {
	sem := semaphore.NewWeighted(4)
	// ok: no-unbounded-goroutine-in-loop
	for _, it := range items {
		if err := sem.Acquire(ctx, 1); err != nil {
			return err
		}
		go func() { defer sem.Release(1); work(it) }()
	}
	return nil
}

func eachLimitedGroup(ctx context.Context, items []string, work func(string) error) error {
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(4)
	// ok: no-unbounded-goroutine-in-loop
	for _, it := range items {
		g.Go(func() error { return work(it) })
	}
	return g.Wait()
}

func workers(n int, work func()) {
	// ok: no-unbounded-goroutine-in-loop
	for i := 0; i < n; i++ {
		go work()
	}
}
