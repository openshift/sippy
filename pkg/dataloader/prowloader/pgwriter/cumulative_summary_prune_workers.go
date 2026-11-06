package pgwriter

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"cloud.google.com/go/civil"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	// cumulativeSummaryPruneWorkerCount bounds concurrent prune transactions.
	cumulativeSummaryPruneWorkerCount = 10
	// cumulativeSummaryPruneCopyBatchSize bounds queued candidate keys and COPY payload size.
	cumulativeSummaryPruneCopyBatchSize = 1000
)

// cumulativeSummaryPruneCandidate identifies one unchanged key and its assigned batch.
type cumulativeSummaryPruneCandidate struct {
	testID, prowJobID, suiteID int64
	lifecycle                  string
	batchNumber                int64
}

// cumulativeSummaryPruneWorker owns one database session and its temp key table.
type cumulativeSummaryPruneWorker struct {
	conn               *sql.Conn
	table              string
	candidates         chan cumulativeSummaryPruneCandidate
	rowsDeleted        int64
	combinationsPruned int64
	batchesCommitted   int64
}

// pruneUnchangedCumulativeSummariesParallel streams one candidate query into
// worker-local temporary tables, then prunes disjoint batches concurrently.
func pruneUnchangedCumulativeSummariesParallel(ctx context.Context, sqlDB *sql.DB, release string, retentionStart, throughDate civil.Date, batchSize int) (int64, error) {
	workers := make([]cumulativeSummaryPruneWorker, cumulativeSummaryPruneWorkerCount)
	suffix := time.Now().UnixNano()
	defer closeCumulativeSummaryPruneWorkers(workers, release)
	if err := initializeCumulativeSummaryPruneWorkers(ctx, sqlDB, release, suffix, workers); err != nil {
		return 0, err
	}

	collectionStarted := time.Now()
	candidateCount, queryElapsed, err := collectCumulativeSummaryPruneCandidates(ctx, sqlDB, release, retentionStart, throughDate, batchSize, workers)
	if err != nil {
		return 0, err
	}
	indexElapsed, analyzeElapsed, err := prepareCumulativeSummaryPruneTables(ctx, release, candidateCount, workers)
	if err != nil {
		return 0, err
	}

	batchSize64 := int64(batchSize)
	totalBatches := (candidateCount + batchSize64 - 1) / batchSize64
	log.WithFields(log.Fields{
		"release":                      release,
		"combinations_to_prune":        candidateCount,
		"retention_from":               retentionStart,
		"through":                      throughDate,
		"batch_limit":                  batchSize,
		"worker_count":                 cumulativeSummaryPruneWorkerCount,
		"candidate_query_elapsed":      queryElapsed,
		"candidate_index_elapsed":      indexElapsed,
		"candidate_analyze_elapsed":    analyzeElapsed,
		"candidate_collection_elapsed": time.Since(collectionStarted),
	}).Info("counted cumulative summary combinations to prune")
	if candidateCount == 0 {
		log.WithFields(log.Fields{"release": release, "retention_from": retentionStart, "through": throughDate}).Info("no cumulative summary combinations remain to prune")
		return 0, nil
	}

	return pruneCumulativeSummaryBatches(ctx, release, retentionStart, throughDate, batchSize, candidateCount, totalBatches, workers)
}

// initializeCumulativeSummaryPruneWorkers creates each worker session and its temporary candidate table.
func initializeCumulativeSummaryPruneWorkers(ctx context.Context, sqlDB *sql.DB, release string, suffix int64, workers []cumulativeSummaryPruneWorker) error {
	for i := range workers {
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			return fmt.Errorf("getting connection for cumulative summary prune worker %d: %w", i+1, err)
		}
		worker := &workers[i]
		worker.conn = conn
		worker.table = fmt.Sprintf("prune_stale_cumulative_summary_keys_%d_%d", suffix, i)
		// #nosec G202 -- table is an internal identifier derived from a timestamp and worker index.
		if _, err := conn.ExecContext(ctx, `
			CREATE TEMP TABLE `+pgx.Identifier{worker.table}.Sanitize()+` ON COMMIT PRESERVE ROWS AS
			SELECT source.test_id, source.prow_job_id, source.suite_id, source.lifecycle,
				0::bigint AS batch_number
			FROM test_cumulative_summaries AS source
			WHERE false`); err != nil {
			return fmt.Errorf("creating candidate table for release %s worker %d: %w", release, i+1, err)
		}
	}
	return nil
}

// closeCumulativeSummaryPruneWorkers drops temporary tables and closes their dedicated sessions.
func closeCumulativeSummaryPruneWorkers(workers []cumulativeSummaryPruneWorker, release string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := range workers {
		worker := &workers[i]
		if worker.conn == nil {
			continue
		}
		if worker.table != "" {
			if _, err := worker.conn.ExecContext(cleanupCtx, "DROP TABLE IF EXISTS "+pgx.Identifier{worker.table}.Sanitize()); err != nil {
				log.WithError(err).WithFields(log.Fields{"release": release, "worker": i + 1}).Warn("failed to drop temporary cumulative summary key table")
			}
		}
		if err := worker.conn.Close(); err != nil {
			log.WithError(err).WithFields(log.Fields{"release": release, "worker": i + 1}).Warn("failed to close cumulative summary prune worker connection")
		}
	}
}

// collectCumulativeSummaryPruneCandidates streams unchanged keys into worker-local candidate tables.
func collectCumulativeSummaryPruneCandidates(ctx context.Context, sqlDB *sql.DB, release string, retentionStart, throughDate civil.Date, batchSize int, workers []cumulativeSummaryPruneWorker) (int64, time.Duration, error) {
	loadGroup, loadCtx := errgroup.WithContext(ctx)
	for i := range workers {
		workerIndex := i
		worker := &workers[workerIndex]
		worker.candidates = make(chan cumulativeSummaryPruneCandidate, cumulativeSummaryPruneCopyBatchSize)
		loadGroup.Go(func() error {
			keys := make([]cumulativeSummaryPruneCandidate, 0, cumulativeSummaryPruneCopyBatchSize)
			copyKeys := func() error {
				if len(keys) == 0 {
					return nil
				}
				err := worker.conn.Raw(func(driverConn any) error {
					pgxConn, ok := driverConn.(*stdlib.Conn)
					if !ok {
						return fmt.Errorf("unexpected PostgreSQL driver connection type %T", driverConn)
					}
					_, err := pgxConn.Conn().CopyFrom(loadCtx, pgx.Identifier{worker.table},
						[]string{"test_id", "prow_job_id", "suite_id", "lifecycle", "batch_number"},
						pgx.CopyFromSlice(len(keys), func(index int) ([]interface{}, error) {
							key := keys[index]
							return []interface{}{key.testID, key.prowJobID, key.suiteID, key.lifecycle, key.batchNumber}, nil
						}),
					)
					return err
				})
				if err != nil {
					return err
				}
				keys = keys[:0]
				return nil
			}
			for candidate := range worker.candidates {
				keys = append(keys, candidate)
				if len(keys) == cap(keys) {
					if err := copyKeys(); err != nil {
						return fmt.Errorf("copying candidate keys for release %s worker %d: %w", release, workerIndex+1, err)
					}
				}
			}
			if err := copyKeys(); err != nil {
				return fmt.Errorf("copying final candidate keys for release %s worker %d: %w", release, workerIndex+1, err)
			}
			return nil
		})
	}

	queryStarted := time.Now()
	candidateCount, candidateErr := queryAndDispatchCumulativeSummaryPruneCandidates(loadCtx, sqlDB, release, retentionStart, throughDate, int64(batchSize), workers)
	queryElapsed := time.Since(queryStarted)
	closePruneCandidateChannels(workers)
	loadErr := loadGroup.Wait()
	if loadErr != nil {
		return 0, queryElapsed, loadErr
	}
	if candidateErr != nil {
		return 0, queryElapsed, candidateErr
	}
	return candidateCount, queryElapsed, nil
}

// queryAndDispatchCumulativeSummaryPruneCandidates queries unchanged keys and assigns each to a batch.
func queryAndDispatchCumulativeSummaryPruneCandidates(ctx context.Context, sqlDB *sql.DB, release string, retentionStart, throughDate civil.Date, batchSize int64, workers []cumulativeSummaryPruneWorker) (int64, error) {
	candidateConn, err := sqlDB.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("getting candidate query connection for release %s: %w", release, err)
	}
	candidateRows, err := candidateConn.QueryContext(ctx, `
		SELECT source.test_id, source.prow_job_id, source.suite_id, source.lifecycle
		FROM test_cumulative_summaries AS source
		LEFT JOIN test_cumulative_summaries AS baseline
			ON baseline.release = source.release
			AND baseline.date = $1
			AND baseline.test_id = source.test_id
			AND baseline.prow_job_id = source.prow_job_id
			AND baseline.suite_id = source.suite_id
			AND baseline.lifecycle = source.lifecycle
		WHERE source.release = $2
			AND source.date = $3
			AND source.prefix_sum_runs = COALESCE(baseline.prefix_sum_runs, 0)`, retentionStart, release, throughDate)
	if err != nil {
		_ = candidateConn.Close()
		return 0, fmt.Errorf("querying cumulative summary candidates for release %s: %w", release, err)
	}

	var candidateCount int64
	var candidateErr error
	for candidateRows.Next() {
		var candidate cumulativeSummaryPruneCandidate
		if err := candidateRows.Scan(&candidate.testID, &candidate.prowJobID, &candidate.suiteID, &candidate.lifecycle); err != nil {
			candidateErr = fmt.Errorf("scanning cumulative summary candidate for release %s: %w", release, err)
			break
		}
		batch := candidateCount / batchSize
		candidate.batchNumber = batch
		workerIndex := int(batch % int64(len(workers)))
		select {
		case workers[workerIndex].candidates <- candidate:
			candidateCount++
		case <-ctx.Done():
			candidateErr = ctx.Err()
		}
		if candidateErr != nil {
			break
		}
	}
	if rowsErr := candidateRows.Err(); candidateErr == nil && rowsErr != nil {
		candidateErr = fmt.Errorf("reading cumulative summary candidates for release %s: %w", release, rowsErr)
	}
	if closeErr := candidateRows.Close(); candidateErr == nil && closeErr != nil {
		candidateErr = fmt.Errorf("closing cumulative summary candidate query for release %s: %w", release, closeErr)
	}
	if closeErr := candidateConn.Close(); candidateErr == nil && closeErr != nil {
		candidateErr = fmt.Errorf("closing candidate query connection for release %s: %w", release, closeErr)
	}
	return candidateCount, candidateErr
}

// prepareCumulativeSummaryPruneTables indexes and analyzes the candidate tables before deletion.
func prepareCumulativeSummaryPruneTables(ctx context.Context, release string, candidateCount int64, workers []cumulativeSummaryPruneWorker) (time.Duration, time.Duration, error) {
	if candidateCount == 0 {
		return 0, 0, nil
	}
	started := time.Now()
	group, groupCtx := errgroup.WithContext(ctx)
	for i := range workers {
		workerIndex := i
		worker := &workers[workerIndex]
		group.Go(func() error {
			// #nosec G202 -- table is an internally generated identifier and is quoted as a PostgreSQL identifier.
			if _, err := worker.conn.ExecContext(groupCtx, "CREATE INDEX ON "+pgx.Identifier{worker.table}.Sanitize()+" (batch_number, test_id, prow_job_id, suite_id, lifecycle)"); err != nil {
				return fmt.Errorf("indexing candidate table for release %s worker %d: %w", release, workerIndex+1, err)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return 0, 0, err
	}
	indexElapsed := time.Since(started)

	started = time.Now()
	group, groupCtx = errgroup.WithContext(ctx)
	for i := range workers {
		workerIndex := i
		worker := &workers[workerIndex]
		group.Go(func() error {
			// #nosec G202 -- table is an internally generated identifier and is quoted as a PostgreSQL identifier.
			if _, err := worker.conn.ExecContext(groupCtx, "ANALYZE "+pgx.Identifier{worker.table}.Sanitize()); err != nil {
				return fmt.Errorf("analyzing candidate table for release %s worker %d: %w", release, workerIndex+1, err)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return indexElapsed, 0, err
	}
	return indexElapsed, time.Since(started), nil
}

// pruneCumulativeSummaryBatches deletes candidate keys in independently committed worker batches.
func pruneCumulativeSummaryBatches(ctx context.Context, release string, retentionStart, throughDate civil.Date, batchSize int, candidateCount, totalBatches int64, workers []cumulativeSummaryPruneWorker) (int64, error) {
	batchSize64 := int64(batchSize)
	pruneGroup, pruneCtx := errgroup.WithContext(ctx)
	for i := range workers {
		workerIndex := i
		worker := &workers[workerIndex]
		pruneGroup.Go(func() error {
			for batch := int64(workerIndex); batch < totalBatches; batch += int64(len(workers)) {
				if err := pruneCtx.Err(); err != nil {
					return err
				}
				batchNumber := int(batch + 1)
				batchOffset := batch * batchSize64
				planned := batchSize64
				if remaining := candidateCount - batchOffset; remaining < planned {
					planned = remaining
				}
				rowsDeleted, err := pruneCumulativeSummaryBatch(pruneCtx, worker, release, retentionStart, throughDate, batchSize, candidateCount, batch, batchNumber, planned, workerIndex)
				if err != nil {
					return err
				}
				worker.rowsDeleted += rowsDeleted
				worker.combinationsPruned += planned
				worker.batchesCommitted++
				log.WithFields(log.Fields{"release": release, "worker": workerIndex + 1, "batch": batchNumber, "batch_combinations": planned, "batch_rows_deleted": rowsDeleted, "worker_rows_deleted": worker.rowsDeleted}).Info("committed cumulative summary prune batch")
			}
			return nil
		})
	}
	pruneErr := pruneGroup.Wait()
	var totalRowsDeleted, totalPruned, batchesCommitted int64
	for i := range workers {
		totalRowsDeleted += workers[i].rowsDeleted
		totalPruned += workers[i].combinationsPruned
		batchesCommitted += workers[i].batchesCommitted
	}
	if pruneErr != nil {
		return totalRowsDeleted, pruneErr
	}
	log.WithFields(log.Fields{"release": release, "workers": cumulativeSummaryPruneWorkerCount, "batches_committed": batchesCommitted, "combinations_to_prune": candidateCount, "combinations_pruned": totalPruned, "rows_deleted": totalRowsDeleted, "retention_from": retentionStart, "through": throughDate}).Info("completed cumulative summary pruning")
	return totalRowsDeleted, nil
}

// pruneCumulativeSummaryBatch deletes one batch and removes its temporary candidate keys in one transaction.
func pruneCumulativeSummaryBatch(ctx context.Context, worker *cumulativeSummaryPruneWorker, release string, retentionStart, throughDate civil.Date, batchSize int, candidateCount, batch int64, batchNumber int, batchCandidates int64, workerIndex int) (int64, error) {
	log.WithFields(log.Fields{"release": release, "worker": workerIndex + 1, "batch": batchNumber, "batch_limit": batchSize, "combinations_to_prune": candidateCount, "through": throughDate, "retention_from": retentionStart}).Info("processing cumulative summary prune batch")
	tx, err := worker.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning prune transaction for release %s batch %d: %w", release, batchNumber, err)
	}
	rollback := func(message string, cause error) error {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("%s: %w (rollback failed: %v)", message, cause, rollbackErr)
		}
		return fmt.Errorf("%s: %w", message, cause)
	}

	var rowsDeleted int64
	if batchCandidates > 0 {
		started := time.Now()
		// #nosec G202 -- worker.table is internally generated and quoted as a PostgreSQL identifier.
		deleteQuery := `
			DELETE FROM test_cumulative_summaries AS summaries
			USING ` + pgx.Identifier{worker.table}.Sanitize() + ` AS stale_keys
			WHERE summaries.release = $1 AND summaries.date BETWEEN $2 AND $3
				AND summaries.test_id = stale_keys.test_id AND summaries.prow_job_id = stale_keys.prow_job_id
				AND summaries.suite_id = stale_keys.suite_id AND summaries.lifecycle = stale_keys.lifecycle
				AND stale_keys.batch_number = $4`
		deleteResult, err := tx.ExecContext(ctx, deleteQuery, release, retentionStart, throughDate, batch)
		if err != nil {
			return 0, rollback(fmt.Sprintf("deleting cumulative summary rows for release %s batch %d", release, batchNumber), err)
		}
		rowsDeleted, err = deleteResult.RowsAffected()
		if err != nil {
			return 0, rollback(fmt.Sprintf("getting deleted row count for release %s batch %d", release, batchNumber), err)
		}
		log.WithFields(log.Fields{"release": release, "worker": workerIndex + 1, "batch": batchNumber, "batch_combinations": batchCandidates, "retention_from": retentionStart, "through": throughDate, "rows_deleted": rowsDeleted, "elapsed": time.Since(started)}).Info("processed cumulative summary prune batch")
	}
	// #nosec G202 -- worker.table is internally generated and quoted as a PostgreSQL identifier.
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+pgx.Identifier{worker.table}.Sanitize()+" WHERE batch_number = $1", batch); err != nil {
		return 0, rollback(fmt.Sprintf("removing candidates for release %s batch %d", release, batchNumber), err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing cumulative summary prune batch %d for release %s: %w", batchNumber, release, err)
	}
	return rowsDeleted, nil
}

// closePruneCandidateChannels ends candidate loading for every prune worker.
func closePruneCandidateChannels(workers []cumulativeSummaryPruneWorker) {
	for i := range workers {
		if workers[i].candidates != nil {
			close(workers[i].candidates)
		}
	}
}
