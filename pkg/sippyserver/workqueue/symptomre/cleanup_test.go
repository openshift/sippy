package symptomre

import (
	"testing"
	"time"
)

func TestRetentionConstants(t *testing.T) {
	t.Run("completed batch retention is 7 days", func(t *testing.T) {
		expected := 7 * 24 * time.Hour
		if CompletedBatchRetention != expected {
			t.Errorf("CompletedBatchRetention = %v, want %v", CompletedBatchRetention, expected)
		}
	})

	t.Run("stale batch timeout is 24 hours", func(t *testing.T) {
		expected := 24 * time.Hour
		if StaleBatchTimeout != expected {
			t.Errorf("StaleBatchTimeout = %v, want %v", StaleBatchTimeout, expected)
		}
	})

	t.Run("stale timeout is shorter than completed retention", func(t *testing.T) {
		if StaleBatchTimeout >= CompletedBatchRetention {
			t.Errorf("StaleBatchTimeout (%v) should be less than CompletedBatchRetention (%v)",
				StaleBatchTimeout, CompletedBatchRetention)
		}
	})
}

func TestNewBatchCleanupProcess(t *testing.T) {
	// NewBatchCleanupProcess requires a *gorm.DB, but we can verify the
	// struct is created with default retention values by passing nil.
	// The process won't be started, so a nil DB is safe here.
	p := NewBatchCleanupProcess(nil)
	if p == nil {
		t.Fatal("NewBatchCleanupProcess returned nil")
	}
	if p.completedRetention != CompletedBatchRetention {
		t.Errorf("completedRetention = %v, want %v", p.completedRetention, CompletedBatchRetention)
	}
	if p.staleTimeout != StaleBatchTimeout {
		t.Errorf("staleTimeout = %v, want %v", p.staleTimeout, StaleBatchTimeout)
	}
}

// NOTE: Testing deleteCompletedBatches and deleteStaleBatches requires a real
// PostgreSQL database because the project conventions prohibit mocking storage
// clients. These methods execute GORM queries that cannot be meaningfully tested
// without a database connection. Integration tests using testcontainers-go
// (see the "make integration" target) are the appropriate place for that coverage.
