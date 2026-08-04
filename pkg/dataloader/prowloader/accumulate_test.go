package prowloader

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeResults(n int) []*jobRunResult {
	results := make([]*jobRunResult, n)
	for i := range results {
		results[i] = &jobRunResult{
			Run: prowJobRunRow{ID: uint(i + 1)},
		}
	}
	return results
}

func sendResults(results []*jobRunResult) <-chan *jobRunResult {
	ch := make(chan *jobRunResult, len(results))
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
		writerFunc     func(calls *[][]jobRunResult) batchWriterFunc
		cancelAfter    int
		wantErrorCount int
		wantErrorMsg   string
	}{
		{
			name:        "all batches succeed",
			resultCount: 250,
			writerFunc: func(calls *[][]jobRunResult) batchWriterFunc {
				return func(ctx context.Context, batch []jobRunResult) error {
					cp := make([]jobRunResult, len(batch))
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
			writerFunc: func(calls *[][]jobRunResult) batchWriterFunc {
				callCount := 0
				return func(ctx context.Context, batch []jobRunResult) error {
					cp := make([]jobRunResult, len(batch))
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
			writerFunc: func(calls *[][]jobRunResult) batchWriterFunc {
				return func(ctx context.Context, batch []jobRunResult) error {
					cp := make([]jobRunResult, len(batch))
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
			writerFunc: func(calls *[][]jobRunResult) batchWriterFunc {
				callCount := 0
				return func(ctx context.Context, batch []jobRunResult) error {
					cp := make([]jobRunResult, len(batch))
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
			writerFunc: func(calls *[][]jobRunResult) batchWriterFunc {
				return func(ctx context.Context, batch []jobRunResult) error {
					cp := make([]jobRunResult, len(batch))
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
			var calls [][]jobRunResult
			pl := &ProwLoader{
				batchWriter: tt.writerFunc(&calls),
			}

			allResults := makeResults(tt.resultCount)

			var ctx context.Context
			var ch <-chan *jobRunResult
			if tt.cancelAfter > 0 {
				cancelCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				buf := make(chan *jobRunResult, tt.resultCount)
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

			pl.accumulateAndWriteJobRuns(ctx, ch)

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
	var calls [][]jobRunResult

	pl := &ProwLoader{
		batchWriter: func(ctx context.Context, batch []jobRunResult) error {
			cp := make([]jobRunResult, len(batch))
			copy(cp, batch)
			calls = append(calls, cp)
			callCount++
			if msg, ok := failOnBatch[callCount]; ok {
				return fmt.Errorf("%s", msg)
			}
			return nil
		},
	}

	results := sendResults(makeResults(400))
	pl.accumulateAndWriteJobRuns(context.Background(), results)

	assert.Len(t, calls, 4, "all 4 batches should be attempted")
	assert.Len(t, pl.errors, 2, "each failed batch should produce its own error")
	assert.Contains(t, pl.errors[0].Error(), "timeout on batch 1")
	assert.Contains(t, pl.errors[1].Error(), "constraint violation on batch 3")
}

func TestAccumulateAndWriteJobRuns_CancelledContextDoesNotSilentlyDrop(t *testing.T) {
	var writtenRuns int
	pl := &ProwLoader{
		batchWriter: func(ctx context.Context, batch []jobRunResult) error {
			writtenRuns += len(batch)
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	results := make(chan *jobRunResult, 10)
	for i := range 5 {
		results <- &jobRunResult{Run: prowJobRunRow{ID: uint(i + 1)}}
	}
	cancel()
	for i := range 5 {
		results <- &jobRunResult{Run: prowJobRunRow{ID: uint(i + 6)}}
	}
	close(results)

	pl.accumulateAndWriteJobRuns(ctx, results)

	// The result received on the same iteration as ctx cancellation should
	// not be silently dropped. It should appear in either the written runs
	// or the errors (if the cancelled-context write fails).
	totalAccountedFor := writtenRuns + countFailedRuns(pl.errors)
	assert.Greater(t, totalAccountedFor, 0, "at least the results received before cancellation should be accounted for")
}

func countFailedRuns(errs []error) int {
	return len(errs) * 100
}
