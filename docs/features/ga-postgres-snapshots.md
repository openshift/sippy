# PostgreSQL GA test snapshots

Component Readiness stores GA test counts in `prow_ga_raw_test_data` so reports can use them after raw Prow partitions expire. The GA loader reads PostgreSQL Prow runs and test rows. BigQuery remains an upstream Prow ingestion source, but is not queried to build a GA snapshot.

For each GA release, the loader records the 1-, 30-, and 90-day windows by test, suite, and job. Each window is `[GA date - window days, GA date + 1 day)` in UTC. Status 1 is a pass, status 13 is a flake, and other statuses are failures. Deleted rows and runs labeled `InfraFailure` are excluded. Run release attribution comes from the historical run, even if the job is later assigned to another release. Variant and ownership grouping uses current metadata at query time. A snapshot contains counts, not individual run IDs or URLs. When a snapshot row exists for the release and requested GA window, base test details use snapshot counts and include retained raw runs, including runs for jobs or test keys absent from the snapshot. Those raw-only entries have zero snapshot counts. These run lists can be incomplete or differ from the snapshot counts. Before that window has a snapshot, reports use ordinary prefix-sum counts and details use the ordinary raw-run path. Individual raw-run displays count a flake as both a success and a flake; summary `Stats` use the snapshot's exclusive pass, failure, and flake counters. The regression cache loader can skip GA base run reads when it is not caching test details, while retaining sample runs for regression tracking. Cached test details include available base and sample runs.

The loader waits for two complete UTC calendar days after the GA date before building or forcibly refreshing a snapshot. For example, a November 19 GA date becomes eligible on November 22 UTC. The loader atomically replaces a release's snapshot and records `ga_data_loaded_date`. Changes to raw Prow data do not invalidate an existing GA snapshot or interrupt report reads. Rebuild a snapshot explicitly after a historical raw-data correction. Until that rebuild, GA aggregate reports can differ from raw run details. Preexisting aggregates built from BigQuery remain readable when their PostgreSQL raw windows are incomplete. The schema does not track snapshot provenance, so a matching loaded date alone does not establish PostgreSQL parity.

Ordinary loads skip any snapshot whose loaded GA date still matches the release GA date. Use `--force-ga-refresh` after correcting raw Prow history, including corrections to attached partitions, or to replace a preexisting aggregate after deployment. A raw-data change does not automatically trigger a snapshot rebuild.

Partition cleanup runs before the GA snapshot loader: initialization cleanup finishes before loaders start, and the Prow loader performs its cleanup before returning. Loaders run sequentially, with Prow before GA. No separate process runs partition cleanup concurrently with the loader. The GA loader checks that both raw tables have attached partitions covering the full 90-day window and that the window contains eligible test rows, then replaces the snapshot atomically. Missing parents and pending concurrent detaches are treated as incomplete coverage. An incomplete raw window leaves the snapshot unchanged. For windows within retention, the loader logs an incomplete-window skip without an error when `ga_data_loaded_date` matches `ga_date`, including during a forced refresh. Without a matching loaded date, it reports an error. Ordinary loads skip windows that start on or before the 365-day retention cutoff. Forced loads attempt those windows and skip them without an error if partitions are incomplete, regardless of the loaded date. Existing snapshot rows remain unchanged in either case; a forced load can rebuild after all partitions are restored. Neither the GA loader nor raw detail queries take partition DDL locks; raw detail queries may overlap cleanup. Partitions alone do not prove that ingestion is complete: before conversion or a historical rebuild, verify the raw import against the authoritative source, including the earliest days. The previously investigated 4.22 sample had missing early Prow runs in PostgreSQL.

## Controlled rebuild

Use this procedure only after PostgreSQL has the raw Prow history that the rebuilt snapshots should represent. It rebuilds from the existing PostgreSQL raw rows. Do not restore partitions or backfill Prow as part of this procedure.

1. Suspend the `fetchdata` CronJob and wait for its active Jobs to finish, so no Prow writes overlap the rebuild:

   ```sh
   oc -n sippy patch cronjob/fetchdata --type=merge -p '{"spec":{"suspend":true}}'
   oc -n sippy get jobs -l batch.kubernetes.io/cronjob-name=fetchdata --watch
   ```

   Continue only after the fetchdata Jobs have completed and no active fetchdata Job remains.

2. Force-refresh every eligible GA release from PostgreSQL. Do not pass `--release`, because that would limit the rebuild:

   ```sh
   sippy load --loader=ga-test-status \
     --force-ga-refresh --skip-matview-refresh --database-dsn="$SIPPY_DATABASE_DSN"
   ```

3. Compare each rebuilt snapshot with its raw aggregate by window, test, suite, and job. The following read-only query returns mismatched keys and duplicate snapshot keys. Set `ga_release` in `psql` first and run it while source writes remain quiesced.

   ```sql
   WITH target AS (
       SELECT release, ga_date FROM release_definitions WHERE release = :'ga_release'
   ), windows(window_days) AS (VALUES (1), (30), (90)),
   raw AS (
       SELECT target.release, windows.window_days, t.test_id, r.prow_job_id,
              COALESCE(t.suite_id, 0) AS suite_id, COUNT(*) AS runs,
              COUNT(*) FILTER (WHERE t.status = 1) AS passes,
              COUNT(*) FILTER (WHERE t.status IS DISTINCT FROM 1 AND t.status IS DISTINCT FROM 13) AS failures,
              COUNT(*) FILTER (WHERE t.status = 13) AS flakes
       FROM target CROSS JOIN windows
       JOIN prow_job_run_tests t ON t.prow_job_run_release = target.release
         AND t.prow_job_run_timestamp >= ((target.ga_date - windows.window_days)::timestamp AT TIME ZONE 'UTC')
         AND t.prow_job_run_timestamp < ((target.ga_date + 1)::timestamp AT TIME ZONE 'UTC')
       JOIN prow_job_runs r ON r.id = t.prow_job_run_id
         AND r.prow_job_release = t.prow_job_run_release AND r.timestamp = t.prow_job_run_timestamp
       WHERE t.deleted_at IS NULL AND r.deleted_at IS NULL
         AND (r.labels IS NULL OR NOT r.labels @> ARRAY['InfraFailure'])
       GROUP BY target.release, windows.window_days, t.test_id, r.prow_job_id, COALESCE(t.suite_id, 0)
   ), snapshot AS (
       SELECT release, window_days, test_id, prow_job_id, suite_id,
              COUNT(*) AS snapshot_rows,
              SUM(runs) AS runs, SUM(passes) AS passes,
              SUM(failures) AS failures, SUM(flakes) AS flakes
       FROM prow_ga_raw_test_data
       WHERE release = :'ga_release'
       GROUP BY release, window_days, test_id, prow_job_id, suite_id
   )
   SELECT COALESCE(raw.release, snapshot.release) AS release,
          COALESCE(raw.window_days, snapshot.window_days) AS window_days,
          COALESCE(raw.test_id, snapshot.test_id) AS test_id,
          COALESCE(raw.prow_job_id, snapshot.prow_job_id) AS prow_job_id,
          COALESCE(raw.suite_id, snapshot.suite_id) AS suite_id,
          raw.runs AS raw_runs, snapshot.snapshot_rows, snapshot.runs AS snapshot_runs,
          raw.passes AS raw_passes, snapshot.passes AS snapshot_passes,
          raw.failures AS raw_failures, snapshot.failures AS snapshot_failures,
          raw.flakes AS raw_flakes, snapshot.flakes AS snapshot_flakes
   FROM raw FULL JOIN snapshot USING (release, window_days, test_id, prow_job_id, suite_id)
   WHERE ROW(raw.runs, raw.passes, raw.failures, raw.flakes)
       IS DISTINCT FROM ROW(snapshot.runs, snapshot.passes, snapshot.failures, snapshot.flakes)
       OR snapshot.snapshot_rows > 1;
   ```

   Also confirm `ga_data_loaded_date = ga_date`. A returned `snapshot_rows` greater than one is a duplicate full key even when its summed counters match raw data. Record releases that the loader could not rebuild because their raw windows are incomplete; their existing snapshot rows are preserved and do not establish parity. Recheck the row counts after the comparison if source writes resume. Equal counts imply equal derived pass rates for the same filters and flake policy.

4. Resume `fetchdata` after recording the comparison and any preserved, unbuildable snapshots:

   ```sh
   oc -n sippy patch cronjob/fetchdata --type=merge -p '{"spec":{"suspend":false}}'
   ```

The GA loader runs only when `ga-test-status` is selected, including the default loader list. When selected together with `regression-cache`, it runs first, and a failed GA refresh stops that cache loader before it publishes. PostgreSQL report and details reads continue to use the existing snapshot after raw Prow changes. Cached results may remain until their normal expiry or an explicit refresh. Other loaders continue and the overall load reports any GA loader errors.

GA window matching uses UTC calendar dates. Snapshot counts and GA raw details cover complete UTC days for those dates, including when the request supplies non-midnight times or timezone offsets.
