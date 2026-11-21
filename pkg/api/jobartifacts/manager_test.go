package jobartifacts

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewJobRunError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantTimedOut bool
	}{
		{
			name:         "ordinary error is final",
			err:          errors.New("DB entry for job run 42 has no URL"),
			wantTimedOut: false,
		},
		{
			name:         "wrapped deadline from the partition key lookup can be retried",
			err:          fmt.Errorf("looking up partition keys for job run 42: %w", context.DeadlineExceeded),
			wantTimedOut: true,
		},
		{
			name:         "caller cancellation is not a timeout",
			err:          fmt.Errorf("looking up partition keys for job run 42: %w", context.Canceled),
			wantTimedOut: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newJobRunError(42, tt.err)
			assert.Equal(t, JobRunError{ID: "42", Error: tt.err.Error(), TimedOut: tt.wantTimedOut}, got)
		})
	}
}
