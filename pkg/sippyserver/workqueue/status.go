package workqueue

// ItemStateCounts holds aggregated River job state counts for a batch's items.
type ItemStateCounts struct {
	Total     int
	Pending   int
	Running   int
	Completed int
	Failed    int
}

// OverallStatus derives the batch status from item state counts.
// When all items have reached a terminal state (completed + failed >= total),
// returns BatchStatusComplete unless every item failed, in which case it
// returns BatchStatusFailed.
func OverallStatus(counts ItemStateCounts) BatchStatus {
	if counts.Total == 0 {
		return BatchStatusComplete
	}
	terminal := counts.Completed + counts.Failed
	if terminal >= counts.Total {
		if counts.Completed == 0 {
			return BatchStatusFailed
		}
		return BatchStatusComplete
	}
	if counts.Running > 0 {
		return BatchStatusRunning
	}
	return BatchStatusPending
}
