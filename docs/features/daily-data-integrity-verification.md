# Daily data integrity verification

The `sippy verify` command performs read-only checks of one UTC calendar day in
the Prow data pipeline. It reports discrepancies but never repairs data.

## Usage

```console
sippy verify [--date YYYY-MM-DD] [--check CHECK]... [--release RELEASE]
```

`--date` defaults to the UTC calendar day before yesterday. `--check` may be
repeated and accepts `bq-completeness`, `daily-totals`, and
`cumulative-summaries`; omitting it runs all three. `--release` limits every
selected check to one release. Without it, the command checks every release
definition and every non-empty, non-deleted historical release discovered in
`prow_jobs`. There is intentionally no active-release filter, so this can
include pseudo-releases with no data on the selected day.

## Checks

- `bq-completeness` compares deduplicated numeric Prow build IDs attributed to
  each release in BigQuery with `prow_job_runs`. Both sources use the Prow
  start-time half-open interval `[date 00:00:00Z, next date 00:00:00Z)`.
  BigQuery retains the loader's terminal-state and non-null URL filters.
  Malformed BigQuery build IDs are failures.
- `daily-totals` recomputes counts from `prow_job_run_tests` and compares them
  in both directions with `test_daily_totals`. It uses the production composite
  run join, normalizes a null suite to ID 0, separates lifecycle values, and
  excludes runs labeled `InfraFailure`. Only successes, failures, flakes, and
  runs are compared; timestamps are not compared.
- `cumulative-summaries` checks that each target-day cumulative row equals the
  previous day's prefix counters plus the target day's daily counters. Keys
  without daily data must carry forward, and first-day keys equal their daily
  counters. The four prefix counters checked are successes, failures, flakes,
  and runs.

## Credentials and output

PostgreSQL uses the standard Sippy database flags. BigQuery and Google
credential flags are only used when `bq-completeness` is selected. Selecting
that check without usable service-account credentials is a failed check; it is
not silently skipped. PostgreSQL-only selections require no Google
credentials.

The command emits one bounded structured summary record for every applicable
`(check, release, date)` and separate deterministically ordered discrepancy
records. It runs all selected checks before returning final status.

Exit status is 0 only when every selected check passes. Mismatches, malformed
IDs, missing selected BigQuery credentials, and operational errors return exit
status 1. The command has no `--fix` mode and performs no writes, migrations,
remediation, alerting, or `prow_job_run_test_outputs` verification.
