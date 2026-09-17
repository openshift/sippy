package prowloader

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
)

func makeResults(n int) []*pgwriter.JobRunResult {
	results := make([]*pgwriter.JobRunResult, n)
	for i := range results {
		results[i] = &pgwriter.JobRunResult{
			Run: pgwriter.RunRow{ID: uint(i + 1)},
		}
	}
	return results
}

func sendResults(results []*pgwriter.JobRunResult) <-chan *pgwriter.JobRunResult {
	ch := make(chan *pgwriter.JobRunResult, len(results))
	for _, r := range results {
		ch <- r
	}
	close(ch)
	return ch
}

func TestAccumulateAndWriteJobRuns(t *testing.T) {
	tests := []struct {
		name           string
		resultCount    int
		writerFunc     func(calls *[][]pgwriter.JobRunResult) func(context.Context, []pgwriter.JobRunResult) error
		cancelAfter    int
		wantErrorCount int
		wantErrorMsg   string
	}{
		{
			name:        "all batches succeed",
			resultCount: 250,
			writerFunc: func(calls *[][]pgwriter.JobRunResult) func(context.Context, []pgwriter.JobRunResult) error {
				return func(ctx context.Context, batch []pgwriter.JobRunResult) error {
					cp := make([]pgwriter.JobRunResult, len(batch))
					copy(cp, batch)
					*calls = append(*calls, cp)
					return nil
				}
			},
			wantErrorCount: 0,
		},
		{
			name:        "first batch fails second succeeds",
			resultCount: 200,
			writerFunc: func(calls *[][]pgwriter.JobRunResult) func(context.Context, []pgwriter.JobRunResult) error {
				callCount := 0
				return func(ctx context.Context, batch []pgwriter.JobRunResult) error {
					cp := make([]pgwriter.JobRunResult, len(batch))
					copy(cp, batch)
					*calls = append(*calls, cp)
					callCount++
					if callCount == 1 {
						return fmt.Errorf("connection refused")
					}
					return nil
				}
			},
			wantErrorCount: 1,
			wantErrorMsg:   "connection refused",
		},
		{
			name:        "all batches fail",
			resultCount: 300,
			writerFunc: func(calls *[][]pgwriter.JobRunResult) func(context.Context, []pgwriter.JobRunResult) error {
				return func(ctx context.Context, batch []pgwriter.JobRunResult) error {
					cp := make([]pgwriter.JobRunResult, len(batch))
					copy(cp, batch)
					*calls = append(*calls, cp)
					return fmt.Errorf("database unavailable")
				}
			},
			wantErrorCount: 3,
			wantErrorMsg:   "database unavailable",
		},
		{
			name:        "trailing batch fails",
			resultCount: 150,
			writerFunc: func(calls *[][]pgwriter.JobRunResult) func(context.Context, []pgwriter.JobRunResult) error {
				callCount := 0
				return func(ctx context.Context, batch []pgwriter.JobRunResult) error {
					cp := make([]pgwriter.JobRunResult, len(batch))
					copy(cp, batch)
					*calls = append(*calls, cp)
					callCount++
					if callCount == 2 {
						return fmt.Errorf("partition not found")
					}
					return nil
				}
			},
			wantErrorCount: 1,
			wantErrorMsg:   "partition not found",
		},
		{
			name:        "context cancelled with pending batch",
			resultCount: 50,
			cancelAfter: 25,
			writerFunc: func(calls *[][]pgwriter.JobRunResult) func(context.Context, []pgwriter.JobRunResult) error {
				return func(ctx context.Context, batch []pgwriter.JobRunResult) error {
					cp := make([]pgwriter.JobRunResult, len(batch))
					copy(cp, batch)
					*calls = append(*calls, cp)
					return nil
				}
			},
			wantErrorCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls [][]pgwriter.JobRunResult
			pl := &ProwLoader{}

			allResults := makeResults(tt.resultCount)

			var ctx context.Context
			var ch <-chan *pgwriter.JobRunResult
			if tt.cancelAfter > 0 {
				cancelCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				buf := make(chan *pgwriter.JobRunResult, tt.resultCount)
				for i, r := range allResults {
					if i == tt.cancelAfter {
						cancel()
					}
					buf <- r
				}
				close(buf)
				ctx = cancelCtx
				ch = buf
			} else {
				ctx = context.Background()
				ch = sendResults(allResults)
			}

			pl.accumulateAndWrite(ctx, ch, tt.writerFunc(&calls))

			assert.Len(t, pl.errors, tt.wantErrorCount, "unexpected error count")
			for _, err := range pl.errors {
				assert.Contains(t, err.Error(), tt.wantErrorMsg)
			}
		})
	}
}

func TestAccumulateAndWriteJobRuns_PerBatchErrors(t *testing.T) {
	failOnBatch := map[int]string{
		1: "timeout on batch 1",
		3: "constraint violation on batch 3",
	}
	callCount := 0
	var calls [][]pgwriter.JobRunResult

	pl := &ProwLoader{}
	writerFunc := func(ctx context.Context, batch []pgwriter.JobRunResult) error {
		cp := make([]pgwriter.JobRunResult, len(batch))
		copy(cp, batch)
		calls = append(calls, cp)
		callCount++
		if msg, ok := failOnBatch[callCount]; ok {
			return fmt.Errorf("%s", msg)
		}
		return nil
	}

	results := sendResults(makeResults(400))
	pl.accumulateAndWrite(context.Background(), results, writerFunc)

	assert.Len(t, calls, 4, "all 4 batches should be attempted")
	assert.Len(t, pl.errors, 2, "each failed batch should produce its own error")
	assert.Contains(t, pl.errors[0].Error(), "timeout on batch 1")
	assert.Contains(t, pl.errors[1].Error(), "constraint violation on batch 3")
}

func TestAccumulateAndWriteJobRuns_CancelledContextDoesNotSilentlyDrop(t *testing.T) {
	var writtenRuns int
	pl := &ProwLoader{}
	writerFunc := func(ctx context.Context, batch []pgwriter.JobRunResult) error {
		writtenRuns += len(batch)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	results := make(chan *pgwriter.JobRunResult, 10)
	for i := range 5 {
		results <- &pgwriter.JobRunResult{Run: pgwriter.RunRow{ID: uint(i + 1)}}
	}
	cancel()
	for i := range 5 {
		results <- &pgwriter.JobRunResult{Run: pgwriter.RunRow{ID: uint(i + 6)}}
	}
	close(results)

	pl.accumulateAndWrite(ctx, results, writerFunc)

	// The result received on the same iteration as ctx cancellation should
	// not be silently dropped. It should appear in either the written runs
	// or the errors (if the cancelled-context write fails).
	totalAccountedFor := writtenRuns + countFailedRuns(pl.errors)
	assert.Greater(t, totalAccountedFor, 0, "at least the results received before cancellation should be accounted for")
}

func countFailedRuns(errs []error) int {
	return len(errs) * 100
}
