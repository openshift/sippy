# Scripts

This directory holds operational and developer scripts for Sippy. Most scripts
are self-documenting through `--help` or inline comments. This README documents
the Component Readiness (CR) BigQuery/PostgreSQL parity tooling in detail.

## Component Readiness BQ/PG parity

Component Readiness can be served from two data providers, selected per request
via the `dataSource` query parameter (`bigquery` or `postgres`). These two
scripts measure and diagnose differences between the providers so the PostgreSQL
implementation can reach parity with BigQuery.

The scripts talk to a running Sippy API (staging or prod). They do not require
direct database access, though `cr-parity-check.py` can optionally probe a
Postgres DSN for more precise date resolution.

### `cr-parity-check.py`: measure parity

Runs a distributed sample of CR API configurations against both providers and
reports factual differences without attributing causes:

* grid status-pair mismatches per component/variant cell,
* `regressed_tests` differences per cell (BQ-only, PG-only, and shared tests
  with a different regression severity),
* response times for each provider.

```
# Compare providers on prod for release 5.0, named views only, JSON output
python scripts/cr-parity-check.py \
    --server https://sippy.dptools.openshift.org \
    --releases 5.0 --scenarios views --json > parity.json

# Human-readable tables with per-cell mismatch detail
python scripts/cr-parity-check.py --releases 5.0 --scenarios views --verbose
```

Useful options:

* `--server`: Sippy API base URL (defaults to staging).
* `--releases`: comma-separated releases (e.g. `5.0,4.22`).
* `--scenarios`: `views,cross-variant,filtered,date-range,test-details,all`.
* `--stale-days N`: shift "now" back N days when a database is stale.
* `--database-dsn`: optional Postgres DSN for precise data-availability probing.
* `--json`: emit machine-readable output for `cr-parity-analyze.py`.

Force refresh is on by default and should stay on when measuring parity. Cached
CR results can be up to about 4 hours stale, which produces spurious mismatches;
`forceRefresh=true` makes both providers compute from the same underlying data.
Use `--no-force-refresh` only for quick, non-authoritative spot checks.

### `cr-parity-analyze.py`: diagnose mismatches

Takes the `--json` output from `cr-parity-check.py` and drills into the
`test_details` API for both providers to attribute each mismatch. Grid-level
cells (component x variant) have no `test_id`, so the script resolves them by
drilling the grid for both providers, matching tests across them, and picking a
representative test whose per-test status actually flips (not an arbitrary first
test).

For each cell it reports every contributing factor (a cell often has more than
one):

* `job_scope`: BQ-only jobs not in `config/openshift.yaml` (TRT-2861).
* `ingestion_gap_bq`: job is in config, BQ has runs, PG has none (for example
  the ROSA HCP release-variant assignment, TRT-2912).
* `ingestion_gap_pg`: PG has runs, BQ has none.
* `run_count_divergence`: shared jobs with different run totals, with the net
  BQ-PG direction (the dedup difference tracked in TRT-2804).
* `outcome_divergence`: same total runs, different pass/fail/flake counts.
* `pg_capability_empty`: PG returns no capability/test rows for a component that
  BQ populates (TRT-2911).

```
python scripts/cr-parity-analyze.py parity.json \
    --config config/openshift.yaml --max-mismatches 15 --verbose
```

Useful options:

* `--config`: path to `config/openshift.yaml` (defaults to the repo path).
* `--max-mismatches`: cells analyzed per scenario (default 10).
* `--server`: overrides the server inferred from the input JSON.
* `--json`: emit machine-readable analysis output.

### Typical workflow

```
python scripts/cr-parity-check.py \
    --server https://sippy.dptools.openshift.org \
    --releases 5.0 --scenarios views --json > parity.json

python scripts/cr-parity-analyze.py parity.json \
    --config config/openshift.yaml --verbose
```

The analyzer's grid drill-downs run with force refresh (inherited from the check
URLs), so a full run can take several minutes per scenario.

### Requirements

Both scripts use only the Python standard library, except that
`cr-parity-analyze.py` uses PyYAML to parse `config/openshift.yaml`. If PyYAML
is not installed it falls back to a built-in line parser. Install dependencies
with:

```
pip install -r scripts/requirements.txt
```

### Known parity gaps

Findings from these tools are tracked under epic TRT-2733 (Enable PostgreSQL for
Component Readiness): TRT-2861 (job scope, verified minor on active releases),
TRT-2804 (dedup run-count divergence), TRT-2911 (PG component drill-down returns
no capability/test rows), and TRT-2912 (ROSA HCP release-variant assignment).
