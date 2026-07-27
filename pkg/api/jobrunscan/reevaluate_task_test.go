package jobrunscan

import (
	"sync"
	"testing"
	"time"
)

func TestTaskStoreCreateAndGet(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	task := store.Create(10)
	if task.ID == "" {
		t.Fatal("expected non-empty task ID")
	}
	if task.Status != ReEvalTaskPending {
		t.Errorf("expected status %q, got %q", ReEvalTaskPending, task.Status)
	}
	if task.Total != 10 {
		t.Errorf("expected total 10, got %d", task.Total)
	}
	if task.Processed != 0 {
		t.Errorf("expected processed 0, got %d", task.Processed)
	}
	if len(task.Results) != 0 {
		t.Errorf("expected empty results, got %d", len(task.Results))
	}
	if task.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
	if task.CompletedAt != nil {
		t.Error("expected nil CompletedAt")
	}

	// Get returns a copy
	got := store.Get(task.ID)
	if got == nil {
		t.Fatal("expected to find task")
	}
	if got.ID != task.ID {
		t.Errorf("expected ID %q, got %q", task.ID, got.ID)
	}
	if got.Status != ReEvalTaskPending {
		t.Errorf("expected status %q, got %q", ReEvalTaskPending, got.Status)
	}

	// Get unknown ID returns nil
	if store.Get("nonexistent-id") != nil {
		t.Error("expected nil for unknown task ID")
	}
}

func TestTaskStoreGetReturnsCopy(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	task := store.Create(5)
	got := store.Get(task.ID)

	// Mutating the returned copy should not affect the store
	got.Status = ReEvalTaskCompleted
	got.Processed = 99

	original := store.Get(task.ID)
	if original.Status != ReEvalTaskPending {
		t.Errorf("store mutation leaked: expected status %q, got %q", ReEvalTaskPending, original.Status)
	}
	if original.Processed != 0 {
		t.Errorf("store mutation leaked: expected processed 0, got %d", original.Processed)
	}
}

func TestTaskStoreProgressTracking(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	task := store.Create(3)

	store.SetRunning(task.ID)
	got := store.Get(task.ID)
	if got.Status != ReEvalTaskRunning {
		t.Errorf("expected status %q, got %q", ReEvalTaskRunning, got.Status)
	}

	// Append results one at a time
	store.AppendResult(task.ID, ReEvaluationResult{
		ProwJobBuildID: "111",
		Status:         ReEvalSuccess,
	})
	got = store.Get(task.ID)
	if got.Processed != 1 {
		t.Errorf("expected processed 1, got %d", got.Processed)
	}
	if len(got.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(got.Results))
	}

	store.AppendResult(task.ID, ReEvaluationResult{
		ProwJobBuildID: "222",
		Status:         ReEvalMissingError,
		Error:          "not found",
	})
	store.AppendResult(task.ID, ReEvaluationResult{
		ProwJobBuildID: "333",
		Status:         ReEvalSuccess,
	})

	got = store.Get(task.ID)
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
	store.Complete(task.ID, nil)
	got = store.Get(task.ID)
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

	task := store.Create(5)
	store.SetRunning(task.ID)

	store.Complete(task.ID, errForTest("symptom load failed"))
	got := store.Get(task.ID)
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

	task := store.Create(100)
	store.SetRunning(task.ID)

	var wg sync.WaitGroup

	// Concurrently append results from multiple goroutines
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.AppendResult(task.ID, ReEvaluationResult{
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
			got := store.Get(task.ID)
			if got == nil {
				t.Error("expected to find task during concurrent read")
			}
		}()
	}

	wg.Wait()

	got := store.Get(task.ID)
	if got.Processed != 100 {
		t.Errorf("expected processed 100 after concurrent writes, got %d", got.Processed)
	}
}

func TestTaskStoreCleanupExpired(t *testing.T) {
	// Use a very short TTL for testing
	store := NewTaskStore(50 * time.Millisecond)
	defer store.Stop()

	task1 := store.Create(1)
	task2 := store.Create(1)

	// Complete task1 but leave task2 pending
	store.Complete(task1.ID, nil)

	// Wait for the TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Manually trigger cleanup (don't wait for the 5-minute ticker)
	store.removeExpired()

	// task1 should be cleaned up (completed + expired)
	if store.Get(task1.ID) != nil {
		t.Error("expected completed+expired task to be cleaned up")
	}

	// task2 should still exist (not completed)
	if store.Get(task2.ID) == nil {
		t.Error("expected pending task to survive cleanup")
	}
}

func TestTaskStoreCleanupKeepsRecent(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	task := store.Create(1)
	store.Complete(task.ID, nil)

	// Cleanup should not remove recently completed tasks
	store.removeExpired()

	if store.Get(task.ID) == nil {
		t.Error("expected recently completed task to survive cleanup")
	}
}

func TestTaskStoreMultipleTasks(t *testing.T) {
	store := NewTaskStore(1 * time.Hour)
	defer store.Stop()

	task1 := store.Create(5)
	task2 := store.Create(10)

	if task1.ID == task2.ID {
		t.Error("expected unique task IDs")
	}

	store.SetRunning(task1.ID)
	store.AppendResult(task1.ID, ReEvaluationResult{
		ProwJobBuildID: "111",
		Status:         ReEvalSuccess,
	})

	// task2 should be unaffected
	got2 := store.Get(task2.ID)
	if got2.Status != ReEvalTaskPending {
		t.Errorf("expected task2 status %q, got %q", ReEvalTaskPending, got2.Status)
	}
	if got2.Processed != 0 {
		t.Errorf("expected task2 processed 0, got %d", got2.Processed)
	}

	got1 := store.Get(task1.ID)
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

// errForTest implements the error interface for test use.
type errForTest string

func (e errForTest) Error() string { return string(e) }
