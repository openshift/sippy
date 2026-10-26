package symptomre

import (
	"testing"

	"github.com/openshift/sippy/pkg/sippyserver/workqueue"
)

func TestClassifyItemStates(t *testing.T) {
	tests := []struct {
		name  string
		items []ItemStatus
		want  workqueue.ItemStateCounts
	}{
		{
			name:  "empty list",
			items: nil,
			want:  workqueue.ItemStateCounts{},
		},
		{
			name: "all completed",
			items: []ItemStatus{
				{ItemKey: "a", State: ItemStateCompleted},
				{ItemKey: "b", State: ItemStateCompleted},
			},
			want: workqueue.ItemStateCounts{Total: 2, Completed: 2},
		},
		{
			name: "all failed variants",
			items: []ItemStatus{
				{ItemKey: "a", State: ItemStateDiscarded},
				{ItemKey: "b", State: ItemStateCancelled},
				{ItemKey: "c", State: ItemStateOrphaned},
			},
			want: workqueue.ItemStateCounts{Total: 3, Failed: 3},
		},
		{
			name: "all running",
			items: []ItemStatus{
				{ItemKey: "a", State: ItemStateRunning},
			},
			want: workqueue.ItemStateCounts{Total: 1, Running: 1},
		},
		{
			name: "pending variants",
			items: []ItemStatus{
				{ItemKey: "a", State: ItemStateNotEnqueued},
				{ItemKey: "b", State: ItemStateAvailable},
				{ItemKey: "c", State: ItemStateScheduled},
				{ItemKey: "d", State: ItemStateRetryable},
				{ItemKey: "e", State: ItemStatePending},
			},
			want: workqueue.ItemStateCounts{Total: 5, Pending: 5},
		},
		{
			name: "mixed states",
			items: []ItemStatus{
				{ItemKey: "a", State: ItemStateCompleted},
				{ItemKey: "b", State: ItemStateDiscarded},
				{ItemKey: "c", State: ItemStateRunning},
				{ItemKey: "d", State: ItemStateNotEnqueued},
				{ItemKey: "e", State: ItemStateCompleted},
				{ItemKey: "f", State: ItemStateOrphaned},
			},
			want: workqueue.ItemStateCounts{Total: 6, Completed: 2, Failed: 2, Running: 1, Pending: 1},
		},
		{
			name: "unknown state falls into pending",
			items: []ItemStatus{
				{ItemKey: "a", State: "some_future_river_state"},
			},
			want: workqueue.ItemStateCounts{Total: 1, Pending: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyItemStates(tt.items)
			if got != tt.want {
				t.Errorf("classifyItemStates() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
