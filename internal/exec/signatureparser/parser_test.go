//go:build cgo && (linux || darwin)

package signatureparser

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestNativeStructuralEvidence(t *testing.T) {
	for _, dialect := range []string{"mysql", "tsql", "bigquery", "snowflake", "databricks"} {
		root, err := Inspect(context.Background(), "SELECT round(sum(amount),2) AS total FROM analytics.sales", dialect, 1000, 32)
		if err != nil || root["select"] == nil {
			t.Fatal(dialect, err)
		}
	}
	for _, sql := range []string{"SELECT 1; SELECT 2", "SELECT " + strings.Repeat("(", 1000) + "1" + strings.Repeat(")", 1000), "SELECT '\x00'"} {
		if _, err := Inspect(context.Background(), sql, "mysql", 1000, 32); err == nil {
			t.Fatal("unsafe input accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Inspect(ctx, "SELECT 1", "mysql", 1000, 32); err != context.Canceled {
		t.Fatal("cancel ignored", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			if _, err := Inspect(context.Background(), "SELECT count(*) AS n FROM analytics.sales", "mysql", 1000, 32); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}

// Cancel immediately after the final admission check to exercise the bounded-native-work
// race deterministically, rather than relying on a wall-clock sleep.
type cancelAfterCheck struct {
	context.Context
	cancel context.CancelFunc
	calls  int
}

func (c *cancelAfterCheck) Err() error {
	err := c.Context.Err()
	c.calls++
	if c.calls == 2 {
		c.cancel()
	}
	return err
}
func TestNativeStructuralCancellationDuringCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := Inspect(&cancelAfterCheck{Context: ctx, cancel: cancel}, "SELECT sum(1)", "mysql", 1000, 32); err != context.Canceled {
		t.Fatal("cancellation after native call was lost", err)
	}
}

type signalFirstCheck struct {
	context.Context
	checked chan struct{}
	once    sync.Once
}

func (c *signalFirstCheck) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}
func TestNativeStructuralAdmissionCancellation(t *testing.T) {
	// Occupy the exact shared stack budget; no caller can start another parser
	// thread while queued, and cancellation releases the waiter without a lease.
	for i := 0; i < cap(nativeSlots); i++ {
		nativeSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(nativeSlots); i++ {
			<-nativeSlots
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := &signalFirstCheck{Context: ctx, checked: make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, err := Inspect(observed, "SELECT sum(1)", "mysql", 1000, 32); done <- err }()
	<-observed.checked
	select {
	case err := <-done:
		t.Fatalf("work escaped full native budget: %v", err)
	default:
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatal("queued cancellation lost", err)
	}
	if len(nativeSlots) != cap(nativeSlots) {
		t.Fatal("waiter consumed another call's lease")
	}
}

func TestNativeStructuralRejectsStaleArchive(t *testing.T) {
	// Simulate a rebuilt Go consumer beside an older cached native archive. The
	// real FFI contract remains unchanged; no parser or native response is mocked.
	original := nativeSource
	nativeSource = original + "\n// changed consumer source\n"
	defer func() { nativeSource = original }()
	if _, err := Inspect(context.Background(), "SELECT sum(1)", "mysql", 1000, 32); err != ErrSyntax {
		t.Fatal("stale archive accepted", err)
	}
}
