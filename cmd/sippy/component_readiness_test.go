package main

import (
	"context"
	"testing"
	"time"
)

func TestStartMetricsRefreshUsesCallerContextAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	refreshed := make(chan context.Context, 16)
	stopped := startMetricsRefresh(ctx, time.Millisecond, func(refreshCtx context.Context) error {
		select {
		case refreshed <- refreshCtx:
		default:
		}
		return nil
	})

	for refreshNumber := 1; refreshNumber <= 2; refreshNumber++ {
		select {
		case refreshCtx := <-refreshed:
			if refreshCtx != ctx {
				t.Fatalf("refresh %d did not receive the caller context", refreshNumber)
			}
		case <-time.After(time.Second):
			t.Fatalf("refresh %d did not run", refreshNumber)
		}
	}

	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("periodic refresh did not stop after context cancellation")
	}
}
