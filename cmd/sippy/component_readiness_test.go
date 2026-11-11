package main

import (
	"context"
	"testing"
	"time"
)

func TestRefreshMetricsPeriodicallyUsesCallerContextAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	refreshed := make(chan context.Context, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		refreshMetricsPeriodically(ctx, time.Millisecond, func(refreshCtx context.Context) error {
			select {
			case refreshed <- refreshCtx:
			default:
			}
			return nil
		})
	}()

	select {
	case refreshCtx := <-refreshed:
		if refreshCtx != ctx {
			t.Fatal("periodic refresh did not receive the caller context")
		}
	case <-time.After(time.Second):
		t.Fatal("periodic refresh did not run")
	}

	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("periodic refresh did not stop after context cancellation")
	}
}
