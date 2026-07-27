package jobrunscan

import (
	"sync"
	"testing"
	"time"
)

func TestTaskStoreCreateAndGet(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	created := store.Create(10)
	if created.ID == "" {
		t.Fatal("expected non-empty task ID")
	}
	if created.Status != ReEvalTaskPending {
		t.Errorf("expected status %q, got %q", ReEvalTaskPending, created.Status)
	}
	if created.Total != 10 {
		t.Errorf("expected total 10, got %d", created.Total)
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}

	// Get returns a copy
	got := store.Get(created.ID)
	if got == nil {
		t.Fatal("expected to find task")
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %q, got %q", created.ID, got.ID)
	}
	if got.Status != ReEvalTaskPending {
		t.Errorf("expected status %q, got %q", ReEvalTaskPending, got.Status)
	}
	if got.Processed != 0 {
		t.Errorf("expected processed 0, got %d", got.Processed)
	}
	if len(got.Results) != 0 {
		t.Errorf("expected empty results, got %d", len(got.Results))
	}
	if got.CompletedAt != nil {
		t.Error("expected nil CompletedAt")
	}

	// Get unknown ID returns nil
	if store.Get("nonexistent-id") != nil {
		t.Error("expected nil for unknown task ID")
	}
}

func TestTaskStoreGetReturnsCopy(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	created := store.Create(5)
	got := store.Get(created.ID)

	// Mutating the returned copy should not affect the store
	got.Status = ReEvalTaskCompleted
	got.Processed = 99

	original := store.Get(created.ID)
	if original.Status != ReEvalTaskPending {
		t.Errorf("store mutation leaked: expected status %q, got %q", ReEvalTaskPending, original.Status)
	}
	if original.Processed != 0 {
		t.Errorf("store mutation leaked: expected processed 0, got %d", original.Processed)
	}
}

func TestTaskStoreGetDeepCopiesCompletedAt(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	created := store.Create(1)
	store.Complete(created.ID, nil)

	got := store.Get(created.ID)
	if got.CompletedAt == nil {
		t.Fatal("expected non-nil CompletedAt")
	}

	// Mutating CompletedAt on the copy must not affect the store.
	modified := got.CompletedAt.Add(1 * time.Hour)
	got.CompletedAt = &modified

	original := store.Get(created.ID)
	if original.CompletedAt.Equal(modified) {
		t.Error("CompletedAt pointer was shared between store and copy")
	}
}

func TestTaskStoreProgressTracking(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	created := store.Create(3)

	store.SetRunning(created.ID)
	got := store.Get(created.ID)
	if got.Status != ReEvalTaskRunning {
		t.Errorf("expected status %q, got %q", ReEvalTaskRunning, got.Status)
	}

	// Append results one at a time
	store.AppendResult(created.ID, ReEvaluationResult{
		ProwJobBuildID: "111",
		Status:         ReEvalSuccess,
	})
	got = store.Get(created.ID)
	if got.Processed != 1 {
		t.Errorf("expected processed 1, got %d", got.Processed)
	}
	if len(got.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(got.Results))
	}

	store.AppendResult(created.ID, ReEvaluationResult{
		ProwJobBuildID: "222",
		Status:         ReEvalMissingError,
		Error:          "not found",
	})
	store.AppendResult(created.ID, ReEvaluationResult{
		ProwJobBuildID: "333",
		Status:         ReEvalSuccess,
	})

	got = store.Get(created.ID)
	if got.Processed != 3 {
		t.Errorf("expected processed 3, got %d", got.Processed)
	}
	if len(got.Results) != 3 {
		t.Errorf("expected 3 results, got %d", len(got.Results))
	}

	// Verify results are in order
	if got.Results[0].ProwJobBuildID != "111" {
		t.Errorf("expected first result build ID '111', got %q", got.Results[0].ProwJobBuildID)
	}
	if got.Results[1].Status != ReEvalMissingError {
		t.Errorf("expected second result status %q, got %q", ReEvalMissingError, got.Results[1].Status)
	}

	// Complete the task
	store.Complete(created.ID, nil)
	got = store.Get(created.ID)
	if got.Status != ReEvalTaskCompleted {
		t.Errorf("expected status %q, got %q", ReEvalTaskCompleted, got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected non-nil CompletedAt")
	}
	if got.Error != "" {
		t.Errorf("expected empty error, got %q", got.Error)
	}
}

func TestTaskStoreCompleteWithError(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	created := store.Create(5)
	store.SetRunning(created.ID)

	store.Complete(created.ID, errForTest("symptom load failed"))
	got := store.Get(created.ID)
	if got.Status != ReEvalTaskFailed {
		t.Errorf("expected status %q, got %q", ReEvalTaskFailed, got.Status)
	}
	if got.Error != "symptom load failed" {
		t.Errorf("expected error 'symptom load failed', got %q", got.Error)
	}
	if got.CompletedAt == nil {
		t.Error("expected non-nil CompletedAt on failure")
	}
}

func TestTaskStoreConcurrentAccess(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	created := store.Create(100)
	store.SetRunning(created.ID)

	var wg sync.WaitGroup

	// Concurrently append results from multiple goroutines
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.AppendResult(created.ID, ReEvaluationResult{
				ProwJobBuildID: makeIDs(1)[0],
				Status:         ReEvalSuccess,
			})
		}(i)
	}

	// Concurrently read while writing
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := store.Get(created.ID)
			if got == nil {
				t.Error("expected to find task during concurrent read")
			}
		}()
	}

	wg.Wait()

	got := store.Get(created.ID)
	if got.Processed != 100 {
		t.Errorf("expected processed 100 after concurrent writes, got %d", got.Processed)
	}
}

func TestTaskStoreCleanupExpired(t *testing.T) {
	// Use a very short TTL for testing
	store := NewTaskStore(50 * time.Millisecond)
	defer store.Stop()

	created1 := store.Create(1)
	created2 := store.Create(1)

	// Complete created1 but leave created2 pending
	store.Complete(created1.ID, nil)

	// Wait for the TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Manually trigger cleanup (don't wait for the 5-minute ticker)
	store.removeExpired()

	// created1 should be cleaned up (completed + expired)
	if store.Get(created1.ID) != nil {
		t.Error("expected completed+expired task to be cleaned up")
	}

	// created2 should also be cleaned up (stuck pending past TTL based on CreatedAt)
	if store.Get(created2.ID) != nil {
		t.Error("expected stuck pending task to be cleaned up after TTL based on CreatedAt")
	}

	// Verify that a newly created task is NOT cleaned up
	created3 := store.Create(1)
	store.removeExpired()
	if store.Get(created3.ID) == nil {
		t.Error("expected fresh pending task to survive cleanup")
	}
}

func TestTaskStoreCleanupKeepsRecent(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	created := store.Create(1)
	store.Complete(created.ID, nil)

	// Cleanup should not remove recently completed tasks
	store.removeExpired()

	if store.Get(created.ID) == nil {
		t.Error("expected recently completed task to survive cleanup")
	}
}

func TestTaskStoreMultipleTasks(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	created1 := store.Create(5)
	created2 := store.Create(10)

	if created1.ID == created2.ID {
		t.Error("expected unique task IDs")
	}

	store.SetRunning(created1.ID)
	store.AppendResult(created1.ID, ReEvaluationResult{
		ProwJobBuildID: "111",
		Status:         ReEvalSuccess,
	})

	// created2 should be unaffected
	got2 := store.Get(created2.ID)
	if got2.Status != ReEvalTaskPending {
		t.Errorf("expected task2 status %q, got %q", ReEvalTaskPending, got2.Status)
	}
	if got2.Processed != 0 {
		t.Errorf("expected task2 processed 0, got %d", got2.Processed)
	}

	got1 := store.Get(created1.ID)
	if got1.Status != ReEvalTaskRunning {
		t.Errorf("expected task1 status %q, got %q", ReEvalTaskRunning, got1.Status)
	}
	if got1.Processed != 1 {
		t.Errorf("expected task1 processed 1, got %d", got1.Processed)
	}
}

func TestInjectTaskHATEOASLinks(t *testing.T) {
	resp := &ReEvalTaskResponse{
		ID:     "test-uuid-123",
		Status: ReEvalTaskRunning,
	}
	InjectTaskHATEOASLinks(resp, "http://localhost:8080")

	if resp.Links["self"] != "http://localhost:8080/api/jobs/runs/reevaluate/test-uuid-123" {
		t.Errorf("unexpected self link: %q", resp.Links["self"])
	}
	if resp.Links["reevaluate"] != "http://localhost:8080/api/jobs/runs/reevaluate" {
		t.Errorf("unexpected reevaluate link: %q", resp.Links["reevaluate"])
	}
}

func TestTaskStoreCleanupStuckByCreatedAt(t *testing.T) {
	store := NewTaskStore(50 * time.Millisecond)
	defer store.Stop()

	// Create a task but never complete it (simulates a stuck goroutine).
	created := store.Create(10)
	store.SetRunning(created.ID)

	// Verify the task exists while TTL has not expired.
	if store.Get(created.ID) == nil {
		t.Fatal("expected stuck task to exist before TTL expiry")
	}

	// Wait for the TTL to expire.
	time.Sleep(100 * time.Millisecond)

	// removeExpired should clean up based on CreatedAt even though
	// CompletedAt is nil (task was never completed).
	store.removeExpired()

	if store.Get(created.ID) != nil {
		t.Error("expected stuck (never-completed) task to be cleaned up after TTL based on CreatedAt")
	}
}

func TestTaskStoreTryAcquireRelease(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	// Acquire all available slots.
	for i := 0; i < MaxConcurrentTasks; i++ {
		if !store.TryAcquire() {
			t.Fatalf("expected TryAcquire to succeed on slot %d", i)
		}
	}

	// The next acquire should fail since all slots are taken.
	if store.TryAcquire() {
		t.Error("expected TryAcquire to fail when at capacity")
	}

	// Release one slot and verify we can acquire again.
	store.Release()
	if !store.TryAcquire() {
		t.Error("expected TryAcquire to succeed after Release")
	}

	// Clean up remaining slots.
	for i := 0; i < MaxConcurrentTasks; i++ {
		store.Release()
	}
}

func TestTaskStoreTrackGoroutine(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)

	store.TrackGoroutine()
	done := make(chan struct{})
	go func() {
		defer store.GoroutineDone()
		// Simulate some work.
		time.Sleep(20 * time.Millisecond)
	}()

	go func() {
		store.Stop() // Stop waits for the WaitGroup
		close(done)
	}()

	select {
	case <-done:
		// Stop returned after goroutine finished — success.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return after goroutine finished")
	}
}

// errForTest implements the error interface for test use.
type errForTest string

func (e errForTest) Error() string { return string(e) }
