# Cumulative summary retention

The `sippy prune-cumulative-summaries` command removes cumulative summary rows
for key combinations whose `prefix_sum_runs` did not change during the
configured retention window. Presubmits use 90 days. Other releases use the
default 365-day partition retention period.

## Usage

```console
sippy prune-cumulative-summaries [--release RELEASE] [--batch-size N] [--database-dsn DSN]
```

Passing `--release` limits the command to that release. When omitted, it
processes each non-deleted release definition sequentially. For each release,
the command uses up to 10 workers, each on its own database connection and
temporary candidate table. One candidate query streams keys to those workers.
Each worker prunes up to `--batch-size` unchanged key combinations per batch. The
default batch size is 100; larger positive values can be supplied to reduce the
number of transactions. Before the batch loop, it counts and logs the total
candidate combinations. In one transaction per batch, it issues one delete
query covering the full retention date range.
It logs each batch's candidate and deleted row counts, then reports committed
totals. It repeats until no candidate combinations remain. If a batch fails,
that batch rolls back while previously committed batches remain pruned. The
temporary table is scoped to the connection and dropped when pruning finishes.
Pruning assumes no concurrent writer is updating cumulative summaries for the
same release while the command runs.

Daily carry-forward treats pruning as best effort. If pruning fails after the
new snapshot has been carried forward, it logs a warning and does not fail the
rest of the load. A later carry-forward retries pruning; already committed
prune batches remain applied.

Backfills for either summary table can be limited to one release with
`sippy backfill --release`:

```console
sippy backfill --table=cumulative-summaries --release=Presubmits --start-date=YYYY-MM-DD --end-date=YYYY-MM-DD --database-dsn DSN
```

The command prepares partitions and backfills only that release. When
`--release` is omitted, it processes every release definition.

When `--end-date` is omitted, daily-total backfills end on the current UTC date
and cumulative-summary backfills end tomorrow UTC for all selected releases.
This matches the cumulative carry-forward target. An explicit `--end-date`
overrides the default. For a cumulative backfill, stopping before a later
existing snapshot leaves that snapshot and subsequent snapshots with their old
prefix sums; include the latest existing snapshot date when it is later than
tomorrow.

The retention baseline is calculated from the current UTC date. Presubmits uses
the date 90 days earlier. Other releases use the oldest date partition kept by
the default 365-day cleanup, which is 364 days earlier because cleanup compares
partition dates at midnight with a timestamp cutoff. The command finds the
latest cumulative summary snapshot on or before tomorrow for each release,
searching backward for up to 30 days, and compares its run sums with the
baseline. For unchanged keys, it deletes rows from the baseline through that
latest snapshot. Keys whose run sum changed are retained. The command does not
carry snapshots forward or require a partition for tomorrow to exist.

This command modifies `test_cumulative_summaries`. If processing a release
fails, earlier releases may already have been pruned because each release is
committed separately. Releases without a snapshot in the 30-day search window
are logged and skipped so processing can continue with later releases.
