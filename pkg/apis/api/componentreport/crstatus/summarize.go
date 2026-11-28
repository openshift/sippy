package crstatus

import "math/big"

// PromoteLifecycle returns the lifecycle that should win when merging
// incoming into current. "informing" always takes priority; otherwise
// the first non-empty value is kept.
func PromoteLifecycle(current, incoming string) string {
	if current == "informing" {
		return current
	}
	if incoming == "informing" {
		return incoming
	}
	if current == "" && incoming != "" {
		return incoming
	}
	return current
}

// SummarizeTestJobRuns groups per-run TestJobRunRows by (job, test key) and
// produces pre-computed TestDetailsSummary entries. The result map is keyed
// by normalized prow job name, matching the input map's keys.
func SummarizeTestJobRuns(rows map[string][]TestJobRunRows) map[string][]TestDetailsSummary {
	result := map[string][]TestDetailsSummary{}

	for jobName, jobRows := range rows {
		byJobAndTestKey := map[string]*TestDetailsSummary{}
		var orderedKeys []string

		for _, row := range jobRows {
			identity := row.TestKeyStr + "\x00" + row.ProwJob
			summary, ok := byJobAndTestKey[identity]
			if !ok {
				summary = &TestDetailsSummary{
					TestKey:    row.TestKey,
					TestKeyStr: row.TestKeyStr,
					ProwJob:    row.ProwJob,
				}
				byJobAndTestKey[identity] = summary
				orderedKeys = append(orderedKeys, identity)
			}

			// flakeAsFailure=false: counts are policy-independent; callers recompute SuccessRate
			// with the request's FlakeAsFailure setting via Stats.Add(). PostgreSQL raw
			// runs retain a flake-as-success projection for their display Count, but the
			// summary must use mutually exclusive result counts.
			statsCount := row.Count
			if row.StatsCount != nil {
				statsCount = *row.StatsCount
			}
			summary.Stats = summary.Stats.AddTestCount(statsCount, false)

			if row.ProwJobRunID != "" {
				summary.JobRuns = append(summary.JobRuns, JobRunDetail{
					ProwJobRunID: row.ProwJobRunID,
					ProwJobURL:   row.ProwJobURL,
					StartTime:    row.StartTime,
					Count:        row.Count,
					JobLabels:    row.JobLabels,
					TestFailures: row.TestFailures,
				})
			}

			if summary.JiraComponent == "" && row.JiraComponent != "" {
				summary.JiraComponent = row.JiraComponent
			}
			if summary.JiraComponentID == nil && row.JiraComponentID != nil {
				summary.JiraComponentID = new(big.Rat).Set(row.JiraComponentID)
			}
			if summary.TestName == "" && row.TestName != "" {
				summary.TestName = row.TestName
			}
			summary.Lifecycle = PromoteLifecycle(summary.Lifecycle, row.Lifecycle)
		}

		for _, key := range orderedKeys {
			result[jobName] = append(result[jobName], *byJobAndTestKey[key])
		}
	}

	return result
}
