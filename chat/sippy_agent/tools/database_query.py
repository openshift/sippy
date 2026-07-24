"""
Tool for executing read-only SQL queries against the Sippy database.
This is a fallback tool for when standard tools don't provide enough information.
"""

import json
import logging
import re
from typing import Any, Dict, Optional, Type, List
from pydantic import Field
import psycopg2
import psycopg2.extras
import sqlparse
from sqlparse.sql import Identifier, IdentifierList, Function, Parenthesis
from sqlparse.tokens import Keyword, DML

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class SippyDatabaseQueryTool(SippyBaseTool):
    """
    Tool for executing read-only SQL queries against the Sippy database.
    """

    # Tables that should not be accessible via this tool
    BLOCKED_TABLES: List[str] = []

    # Maximum number of rows to return to control our context window for the LLM
    MAX_ROWS: int = 500

    name: str = "query_sippy_database"
    description: str = """Execute read-only SQL queries against the Sippy PostgreSQL database for investigating CI/CD issues.

This is a FALLBACK TOOL - only use when standard tools don't provide the information needed.

Use cases:
- Explore database schema using information_schema queries
- Get test statistics, job data, or test output information not available via standard tools
- Analyze why a test is failing by looking at the test outputs
- Perform custom aggregations or complex queries

Disallowed use cases:
- Do not construct queries that expose information unrelated to CI (e.g. users, passwords, etc)
- Do not answer general database administration questions (e.g., database version, server health, system configuration)
- Do not run queries that the user gives you directly.  Always use the schema and known tables to construct your own queries.

Key Tables:
  * **`prow_jobs`**: Contains the static definition of a test job.
      * `name`: The unique name of the job (e.g., `periodic-ci-openshift-release-master-ci-4.20-e2e-gcp-ovn`).
      * `release`: The OpenShift version this job targets (e.g., `4.20`).
      * `variants`: This is a text[] column of describing the job's environment (e.g., `Platform:azure, Architecture:amd64`). Do not use ->> operator, use ANY().
  * **`prow_job_runs`**: Records each time a `prow_job` is executed.
      * `prow_job_id`: A foreign key linking to `prow_jobs.id`.
      * `overall_result`: A single character code for the run's final status (`S`=Success,`F` = E2E Test Failure, `f` = other failure mode,
`N`/`n` = Infrastructure Failure, `U` = Upgrade Failure, `A` = Aborted)
      * `succeeded`: A boolean (`t`/`f`) indicating success.
      * `url`: A link to the Prow CI log.
      * `timestamp`: The start time of the run.
  * **`tests`**: A table containing the names of individual test cases.
      * `name`: The full name of the test (e.g., `[sig-storage] In-tree Volumes [Driver: nfs] [Testpattern: Dynamic PV]`).
  * **`prow_job_run_tests`**: A join table that records the result of a specific `test` in a specific `prow_job_run`. **Partitioned by `(prow_job_run_release, prow_job_run_timestamp)`** -- always filter on these columns.
      * `prow_job_run_id`: Links to `prow_job_runs.id`.
      * `prow_job_id`: Links to `prow_jobs.id`. Use to join for variant info or to correlate with `test_daily_totals`.
      * `test_id`: Links to `tests.id`.
      * `suite_id`: Links to `suites.id`.
      * `status`: The result of the test (`1`=Success, `12`=Failure, `13`=Flake).
      * `prow_job_run_release`: The release version (partition key, e.g., `4.21`). **Required in WHERE clause.**
      * `prow_job_run_timestamp`: The job run timestamp (partition key). **Required in WHERE clause.**
      * `duration`: Test duration in seconds.
  * **`prow_job_run_test_outputs`**: Stores the failure output text for test executions. **Partitioned by `(prow_job_run_test_release, prow_job_run_test_timestamp)`** -- always filter on these columns.
      * `prow_job_run_test_id`: Links to `prow_job_run_tests.id`.
      * `output`: The test failure output text.
      * `prow_job_run_test_release`: The release version (partition key). **Required in WHERE clause.**
      * `prow_job_run_test_timestamp`: The job run timestamp (partition key). **Required in WHERE clause.**
  * **`test_daily_totals`**: Pre-aggregated daily test pass/fail/flake counts. Use this instead of scanning `prow_job_run_tests` with COUNT/GROUP BY. **Partitioned by `(release, date)`**.
      * `test_id`: Links to `tests.id`.
      * `prow_job_id`: Links to `prow_jobs.id`.
      * `suite_id`: Links to `suites.id`.
      * `release`: OpenShift release version (partition key). **Required in WHERE clause.**
      * `date`: The date of the aggregation (partition key). **Required in WHERE clause.**
      * `successes`, `failures`, `flakes`, `runs`: Pre-computed daily counts.
  * **`test_cumulative_summaries`**: Running prefix sums of `test_daily_totals`. For any date range [start, end], compute: `prefix_sum(end) - prefix_sum(start - 1)`. **Partitioned by `(release, date)`**.
      * `date`, `release`, `test_id`, `prow_job_id`, `suite_id`: Same as `test_daily_totals`.
      * `prefix_sum_successes`, `prefix_sum_failures`, `prefix_sum_flakes`, `prefix_sum_runs`: Cumulative totals up to that date.
   * **`suites`**: Defines a collection or group of tests.
      * `name`: The name of the test suite (e.g., openshift-tests).
  * **`release_tags`**: Contains information about specific OpenShift release payloads.
      * `release_tag`: The payload version (e.g., `4.14.0-0.nightly-multi-2023-07-23-183157`).
      * `phase`: The status of the release (`Accepted`, `Rejected`).

There are variants available for job classification, if a user asks you about specific kinds of jobs, you should
look at the list of available variants and filter on the ones most relevant to the question.

CRITICAL: You can get a list of available variants with `SELECT DISTINCT unnest(variants) AS variant FROM prow_jobs;`.  DO NOT
guess at the variants, YOU MUST always use one of the options from the list verbatim when filtering.

If a user asks about single node jobs, use "Topology:single" variant.  If a user asks about GCP jobs, use "Platform:gcp" variants.

**Pre-aggregated Tables and Views (HIGHLY PREFERRED for analysis):**

For performance, **always prefer pre-aggregated summary tables or materialized views** over scanning raw tables. These pre-calculate results.

Use the pg_matviews table to learn about the schema for materialized views.

  * **`test_daily_totals`** (PREFERRED for test statistics): Pre-aggregated daily test pass/fail/flake counts per (test, job, suite, release, date). Use `SUM(successes)`, `SUM(failures)`, `SUM(flakes)`, `SUM(runs)` for aggregation. Always filter on `release` and `date`.
    * **IMPORTANT**: Each test has multiple rows per (prow_job_id, suite_id). When providing an overall overview (e.g., "top failing tests"), aggregate by test name: `GROUP BY t.name` with `SUM(failures)` across all jobs. Only group by `prow_jobs.variants` or `prow_job_id` when the user asks for a per-variant or per-job breakdown.

  * **`test_cumulative_summaries`** (PREFERRED for date-range test statistics): Running prefix sums of `test_daily_totals`. To compute totals for any date range [start, end], query two dates and subtract: `prefix_sum(end) - prefix_sum(start - 1)`. Always filter on `release` and `date`.
    * **IMPORTANT**: Same multi-row structure as `test_daily_totals` — see aggregation guidance above.

  * **`prow_job_runs_report_matview`**: Pre-joined and aggregated data about job runs. Excellent for job pass/fail rates.
    * `release`: OpenShift release version (e.g., `4.19`)
    * `variants`: This is a text[] column of describing the job's environment (e.g., `Platform:azure, Architecture:amd64`). Do not use ->> operator, use ANY().
    * `name`: Full name of the job
    * `job`: Job name (same as name)
    * `overall_result`: A single character code for the run's final status (`S`=Success,`F` = E2E Test Failure, `f` = other failure mode,
    * `url`: URL to the job run logs
    * `succeeded`: Boolean indicating if the job succeeded (t/f)
    * `timestamp`: Bigint representing milliseconds since epoch
    * `prow_id`: Prow job run ID
    * `cluster`: Build cluster name (e.g., `build01`)
    * `flaked_test_names`: Array of test names that flaked
    * `failed_test_names`: Array of test names that failed
    * `pull_request_link`, `pull_request_sha`, `pull_request_org`, `pull_request_repo`, `pull_request_author`: PR information when applicable

### Query Guidelines (MANDATORY)

1.  **Always use `LIMIT`**: The database tool has timeout. Always end your query with `LIMIT 10;` or a similar small number to prevent timeouts.
2.  **Filter by Time**: Whenever possible, use a `WHERE` clause to filter by a time range (e.g., `date >= CURRENT_DATE - 7`).
3.  **Prefer Pre-aggregated Tables**: For test pass/fail/flake rates or counts, use `test_daily_totals` or `test_cumulative_summaries`. For job pass/fail rates, use `prow_job_runs_report_matview`. Only query raw `prow_job_run_tests` when you need individual test execution details (e.g., failure output, specific job run results).
4.  **Read-Only**: You only have `SELECT` permissions. Do not attempt to write data.
5.  **Partition Pruning**: `prow_job_run_tests` and `prow_job_run_test_outputs` are partitioned by release and timestamp. `test_daily_totals` and `test_cumulative_summaries` are partitioned by release and date. **Always** include WHERE filters on the partition key columns (release and timestamp/date) when querying these tables. These partition keys must be **literal values** in the WHERE clause, not subqueries or join conditions, because the query planner needs them at plan time to prune partitions. If partition keys are not known, first query a non-partitioned table (e.g., `prow_job_runs` or `prow_jobs`) to get them, then use those values as literals in a second query against the partitioned table. See examples 3 and 7.

### Example Queries

Here are examples demonstrating how to query this database.

**1. Find the Most Recent Prow Jobs for a Specific Release**

```sql
-- Get the 10 most recently created Prow jobs for release '4.20'
SELECT
  name,
  release,
  variants
FROM
  prow_jobs
WHERE
  release = '4.20'
ORDER BY
  created_at DESC
LIMIT 10;
```

**2. Get the Status of the Last 5 Runs for a Specific Job**

```sql
-- Find the last 5 runs for the 'periodic-ci-openshift-release-master-ci-4.20-e2e-gcp-ovn' job
SELECT
  pj.name,
  pjr.url,
  pjr.succeeded,
  pjr.overall_result,
  pjr.timestamp
FROM
  prow_job_runs pjr
JOIN
  prow_jobs pj ON pjr.prow_job_id = pj.id
WHERE
  pj.name = 'periodic-ci-openshift-release-master-ci-4.20-e2e-gcp-ovn'
ORDER BY
  pjr.timestamp DESC
LIMIT 5;
```

**3. List Failed Test Cases from a Specific Failed Job Run (Two-Step)**

Step 1: Get the run's release and timestamp for partition pruning.
```sql
SELECT prow_job_release, timestamp FROM prow_job_runs WHERE id = 1967736172570480640;
```

Step 2: Use those literal values to query the partitioned table.
```sql
-- For a given Prow job run ID, list all failed tests (status=12)
-- Partition keys must be literals for the planner to prune efficiently
SELECT
  t.name AS test_name,
  pjrt.duration AS test_duration_seconds
FROM
  prow_job_run_tests pjrt
JOIN
  tests t ON pjrt.test_id = t.id
WHERE
  pjrt.prow_job_run_id = 1967736172570480640 -- Prow job run ID
  AND pjrt.prow_job_run_release = '4.20' -- Partition key: from step 1
  AND pjrt.prow_job_run_timestamp = '2026-07-15T10:30:00Z'::timestamptz -- Partition key: from step 1
  AND pjrt.status = 12 -- Status for 'Failure'
LIMIT 20;
```

**4. Find Recent Infrastructure Failures**

```sql
-- Show the 10 most recent jobs that failed due to infrastructure issues ('N') in the last 3 days
SELECT
  pjr.id,
  pj.name,
  pjr.url,
  pjr.timestamp
FROM
  prow_job_runs pjr
JOIN
  prow_jobs pj ON pjr.prow_job_id = pj.id
WHERE
  pjr.overall_result IN ('N', 'n')
  AND pjr.timestamp > NOW() - INTERVAL '3 days'
ORDER BY
  pjr.timestamp DESC
LIMIT 10;
```

**5. Get Job Run Pass Rate by Platform (Variant)**

```sql
-- Calculate job pass rates for release '4.19' grouped by platform over the last 7 days
-- Note: variants is text[], not JSONB. Use unnest() to extract and filter variants.
SELECT
  v.variant AS platform,
  COUNT(*) AS total_runs,
  COUNT(*) FILTER (WHERE m.succeeded = true) AS successful_runs,
  ROUND(100.0 * COUNT(*) FILTER (WHERE m.succeeded = true) / COUNT(*), 2) AS pass_percentage
FROM
  prow_job_runs_report_matview m,
  LATERAL unnest(m.variants) AS v(variant)
WHERE
  m.release = '4.19'
  AND m.timestamp > (EXTRACT(EPOCH FROM NOW() - INTERVAL '7 days') * 1000)::bigint
  AND v.variant LIKE 'Platform:%'
GROUP BY
  v.variant
ORDER BY
  total_runs DESC
LIMIT 10;
```

**6. Identify Top 10 Flakiest Tests in the Last 2 Days**

```sql
-- Find the top 10 tests with the highest flake count in the last 2 days
-- Uses test_daily_totals instead of scanning raw prow_job_run_tests
SELECT
  t.name AS test_name,
  SUM(tdt.flakes) AS flake_count,
  ROUND(100.0 * SUM(tdt.successes) / NULLIF(SUM(tdt.runs), 0), 2) AS pass_percentage
FROM
  test_daily_totals tdt
JOIN
  tests t ON t.id = tdt.test_id
WHERE
  tdt.date BETWEEN CURRENT_DATE - 2 AND CURRENT_DATE - 1
  AND tdt.release = '4.21' -- Partition key: required
GROUP BY
  t.name
ORDER BY
  flake_count DESC
LIMIT 10;
```

**7. Find Failure Output for a Specific Failed Test (Two-Step)**

Step 1: Get the run's release and timestamp (same as example 3, step 1).
```sql
SELECT prow_job_release, timestamp FROM prow_job_runs WHERE id = 1967736172570480640;
```

Step 2: Use those literal values to query both partitioned tables.
```sql
-- Get the failure output for a specific test in a specific job run
-- Both tables are partitioned: partition keys must be literals
SELECT
  pjrt_out.output
FROM
  prow_job_run_test_outputs pjrt_out
JOIN
  prow_job_run_tests pjrt ON pjrt_out.prow_job_run_test_id = pjrt.id
JOIN
  tests t ON pjrt.test_id = t.id
WHERE
  pjrt.prow_job_run_id = 1967736172570480640 -- Prow job run ID
  AND pjrt.prow_job_run_release = '4.20' -- Partition key: from step 1
  AND pjrt.prow_job_run_timestamp = '2026-07-15T10:30:00Z'::timestamptz -- Partition key: from step 1
  AND pjrt_out.prow_job_run_test_release = '4.20' -- Partition key: same release
  AND pjrt_out.prow_job_run_test_timestamp = '2026-07-15T10:30:00Z'::timestamptz -- Partition key: same timestamp
  AND t.name = '[sig-api-machinery] CustomResourceDefinition resources [Privileged:ClusterAdmin] should be able to list CRDs'
LIMIT 10;
```

**8. Show Daily Test Failure Trend for a Specific Job**

```sql
-- Show daily failure counts for a specific test in a specific Prow job over the last 7 days
-- Uses test_daily_totals for pre-aggregated data
SELECT
  tdt.date,
  t.name AS test_name,
  SUM(tdt.failures) AS failures,
  SUM(tdt.runs) AS runs,
  ROUND(100.0 * SUM(tdt.failures) / NULLIF(SUM(tdt.runs), 0), 2) AS failure_rate
FROM
  test_daily_totals tdt
JOIN
  tests t ON t.id = tdt.test_id
WHERE
  tdt.prow_job_id = 6046 -- Example Prow job ID
  AND t.name = 'Job run should complete before timeout'
  AND tdt.date BETWEEN CURRENT_DATE - 7 AND CURRENT_DATE - 1
  AND tdt.release = '4.21' -- Partition key: required
GROUP BY
  tdt.date, t.name
ORDER BY
  tdt.date DESC
LIMIT 10;
```

**9. Get Test Pass Rates Over an Arbitrary Date Range (Prefix-Sum Pattern)**

```sql
-- Compute test pass rates for release '4.21' over the last 7 complete days
-- Uses test_cumulative_summaries with prefix-sum subtraction: range [start, end] = prefix_sum(end) - prefix_sum(start - 1)
-- LEFT JOIN handles tests that started after the range start (no start-of-range row)
SELECT
  t.name AS test_name,
  SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)) AS runs,
  SUM(e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0)) AS successes,
  ROUND(
    100.0 * SUM(e.prefix_sum_successes - COALESCE(s.prefix_sum_successes, 0))
          / NULLIF(SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)), 0),
    2
  ) AS pass_percentage
FROM
  test_cumulative_summaries e
JOIN
  tests t ON t.id = e.test_id
LEFT JOIN
  test_cumulative_summaries s
  ON  s.test_id = e.test_id
  AND s.prow_job_id = e.prow_job_id
  AND s.suite_id = e.suite_id
  AND s.release = '4.21'           -- Partition key: required on both sides
  AND s.date = CURRENT_DATE - 8   -- day before range start (start - 1)
WHERE
  e.date = CURRENT_DATE - 1       -- range end (yesterday)
  AND e.release = '4.21'          -- Partition key: required
GROUP BY
  t.name
HAVING
  SUM(e.prefix_sum_runs - COALESCE(s.prefix_sum_runs, 0)) > 0
ORDER BY
  pass_percentage ASC
LIMIT 10;
```

**Schema Exploration:**
```sql
-- List all tables
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public' ORDER BY table_name;

-- Show columns for a table
SELECT column_name, data_type FROM information_schema.columns
WHERE table_name = 'prow_job_runs' ORDER BY ordinal_position;
```

IMPORTANT: Only read-only queries are allowed.
IMPORTANT: Construct your queries in the most efficient (fastest) way possible.
"""

    # Note: never expose the database DSN as a tool input.
    database_dsn: Optional[str] = Field(default=None, description="PostgreSQL connection string")

    class DatabaseQueryInput(SippyToolInput):
        query: str = Field(description="SQL SELECT query to execute against the Sippy database")

    args_schema: Type[SippyToolInput] = DatabaseQueryInput

    def _is_read_only_query(self, query: str) -> bool:
        """
        Check if a query is read-only using AST parsing.  This is a belt-and-suspenders-and-a-bit-of-paranoia approach
        to ensure that the query is read-only, as we already have the session in read-only mode, and are only giving the
        tool a read-only user.

        Args:
            query: SQL query to check

        Returns:
            True if query is read-only, False otherwise
        """
        try:
            parsed = sqlparse.parse(query)

            if not parsed:
                logger.warning("Failed to parse SQL query")
                return False

            # Check each statement in the query
            for statement in parsed:
                if not self._is_statement_read_only(statement):
                    return False

            return True

        except Exception as e:
            logger.error(f"Error parsing SQL query: {e}")
            # If parsing fails, reject the query for safety
            return False

    def _is_statement_read_only(self, statement) -> bool:
        """
        Check if a parsed SQL statement is read-only.

        Args:
            statement: Parsed SQL statement from sqlparse

        Returns:
            True if statement is read-only, False otherwise
        """
        # Get the statement type
        stmt_type = statement.get_type()

        # Allow only read-only statement types
        allowed_types = ['SELECT', 'WITH', 'SHOW', 'EXPLAIN']
        if stmt_type not in allowed_types:
            logger.warning(f"Disallowed statement type: {stmt_type}")
            return False

        # Recursively check all tokens for dangerous DML operations
        dangerous_dml = {'INSERT', 'UPDATE', 'DELETE', 'DROP', 'CREATE', 'ALTER',
                        'TRUNCATE', 'GRANT', 'REVOKE', 'REPLACE', 'MERGE'}

        for token in statement.flatten():
            if token.ttype is DML:
                if token.value.upper() in dangerous_dml:
                    logger.warning(f"Disallowed DML operation found: {token.value}")
                    return False
            # Also check for DDL keywords
            if token.ttype is Keyword.DDL:
                logger.warning(f"Disallowed DDL operation found: {token.value}")
                return False

        return True

    def _extract_table_names(self, query: str) -> List[str]:
        """
        Extract table names from a SQL query using AST parsing.

        Args:
            query: SQL query to analyze

        Returns:
            List of table names found in the query
        """
        try:
            parsed = sqlparse.parse(query)
            tables = []

            for statement in parsed:
                tables.extend(self._extract_tables_from_tokens(statement.tokens))

            # Remove duplicates and normalize to lowercase
            return list(set(t.lower() for t in tables if t))

        except Exception as e:
            logger.error(f"Error extracting table names from SQL: {e}")
            return []

    def _extract_tables_from_tokens(self, tokens) -> List[str]:
        """
        Recursively extract table names from SQL tokens.

        Args:
            tokens: List of SQL tokens from sqlparse

        Returns:
            List of table names
        """
        tables = []
        from_seen = False
        join_seen = False

        for token in tokens:
            # Skip whitespace and comments
            if token.is_whitespace:
                continue
            if hasattr(token, 'ttype') and token.ttype in sqlparse.tokens.Comment:
                continue

            # Check for FROM or JOIN keywords
            if token.ttype is Keyword and token.value.upper() in ('FROM', 'JOIN', 'INNER JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'FULL JOIN'):
                from_seen = True
                join_seen = True
                continue

            # After FROM/JOIN, extract table names
            if from_seen or join_seen:
                if isinstance(token, Identifier):
                    table_name = self._get_real_table_name(token)
                    if table_name:
                        tables.append(table_name)
                    from_seen = False
                    join_seen = False
                elif isinstance(token, IdentifierList):
                    for identifier in token.get_identifiers():
                        table_name = self._get_real_table_name(identifier)
                        if table_name:
                            tables.append(table_name)
                    from_seen = False
                    join_seen = False
                elif isinstance(token, Function):
                    # Table functions - extract if it's a table source
                    from_seen = False
                    join_seen = False

            # Recursively process subqueries and parenthesized expressions
            if hasattr(token, 'tokens'):
                tables.extend(self._extract_tables_from_tokens(token.tokens))

        return tables

    def _get_real_table_name(self, identifier) -> Optional[str]:
        """
        Extract the actual table name from an identifier.

        Args:
            identifier: SQL identifier from sqlparse

        Returns:
            Table name, or None if not a table
        """
        if isinstance(identifier, Identifier):
            # Get the real name (handles aliases)
            real_name = identifier.get_real_name()
            if real_name:
                # Handle schema.table -> extract table
                if '.' in real_name:
                    return real_name.split('.')[-1]
                return real_name

        # Fallback to string representation
        name = str(identifier).strip()
        if name and not name.upper().startswith('('):
            if '.' in name:
                return name.split('.')[-1]
            return name.split()[0]  # Take first word before any alias

        return None

    def _check_blocked_tables(self, query: str) -> Optional[str]:
        """
        Check if query attempts to access any blocked tables.

        Args:
            query: SQL query to check

        Returns:
            Error message if blocked tables are accessed, None otherwise
        """
        tables = self._extract_table_names(query)

        # Check for PostgreSQL system tables (pg_*), with exceptions for certain allowed tables
        allowed_pg_tables = {'pg_matviews'}
        pg_tables = [t for t in tables if t.startswith('pg_') and t not in allowed_pg_tables]
        if pg_tables:
            return f"Access denied: Query attempts to access PostgreSQL system table(s): {', '.join(pg_tables)}"

        # Check against explicit blocklist
        if self.BLOCKED_TABLES:
            blocked = [t for t in tables if t in [b.lower() for b in self.BLOCKED_TABLES]]
            if blocked:
                return f"Access denied: Query attempts to access blocked table(s): {', '.join(blocked)}"

        return None

    def _run(self, *args, **kwargs: Any) -> str:
        """Execute a read-only SQL query against the Sippy database."""

        input_data = {}
        if args and isinstance(args[0], dict):
            input_data.update(args[0])
        input_data.update(kwargs)

        # Validate input
        try:
            query_input = self.DatabaseQueryInput(**input_data)
        except Exception as e:
            return json.dumps({
                "error": f"Invalid input: {str(e)}"
            }, indent=2)

        # Always use instance DSN for security
        dsn = self.database_dsn

        if not dsn:
            return json.dumps({
                "error": "No database connection configured. Please set SIPPY_READ_ONLY_DATABASE_DSN environment variable.",
                "help": "Set the environment variable to a PostgreSQL connection string like: postgresql://user:pass@host:5432/dbname"
            }, indent=2)

        # Validate query is read-only
        if not self._is_read_only_query(query_input.query):
            return json.dumps({
                "error": "Only SELECT queries are allowed for safety.",
                "help": "This tool only supports read-only operations: SELECT, WITH (for CTEs), EXPLAIN, SHOW"
            }, indent=2)

        # Check for blocked tables
        blocked_error = self._check_blocked_tables(query_input.query)
        if blocked_error:
            return json.dumps({
                "error": blocked_error,
                "help": "This query attempts to access tables that are not permitted."
            }, indent=2)

        # Execute the query
        conn = None
        cursor = None
        try:
            # Connect to the database
            conn = psycopg2.connect(dsn, connect_timeout=10)

            # Set transaction to read-only for extra safety
            conn.set_session(readonly=True, autocommit=True)

            # Set statement timeout to 120 seconds
            cursor = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)
            cursor.execute("SET statement_timeout = '120s'")

            # Execute the user's query
            logger.info(f"Executing database query: {query_input.query[:100]}...")
            cursor.execute(query_input.query)

            # Fetch results with row limit
            rows = cursor.fetchmany(self.MAX_ROWS)

            # Check if there are more rows available
            has_more = cursor.fetchone() is not None

            # Convert to list of dicts for JSON serialization
            results = [dict(row) for row in rows]

            # Return formatted results
            response = {
                "success": True,
                "row_count": len(results),
                "results": results
            }

            # If no results, provide a helpful message
            if len(results) == 0:
                response["message"] = "Query executed successfully but returned no rows."
            elif has_more:
                response["warning"] = f"Results limited to {self.MAX_ROWS} rows. Your query returned more rows than the limit. Consider adding LIMIT or WHERE clauses to narrow your results."
                response["truncated"] = True

            return json.dumps(response, indent=2, default=str)

        except psycopg2.OperationalError as e:
            logger.error(f"Database connection error: {e}")
            return json.dumps({
                "error": "Failed to connect to database",
                "details": str(e),
                "help": "Check that SIPPY_READ_ONLY_DATABASE_DSN is correct and the database is accessible."
            }, indent=2)

        except psycopg2.errors.QueryCanceled as e:
            logger.error(f"Query timeout: {e}")
            return json.dumps({
                "error": "Query execution timeout",
                "details": str(e),
                "help": "Try simplifying your query or adding more specific filters (WHERE clauses, LIMIT, etc.)"
            }, indent=2)

        except psycopg2.Error as e:
            logger.error(f"Database error: {e}")
            return json.dumps({
                "error": "Database query error",
                "details": str(e),
                "help": "Check your SQL syntax and table/column names. Use information_schema to explore the schema."
            }, indent=2)

        except Exception as e:
            logger.error(f"Unexpected error executing database query: {e}")
            return json.dumps({
                "error": "Unexpected error",
                "details": str(e)
            }, indent=2)

        finally:
            # Clean up
            if cursor:
                cursor.close()
            if conn:
                conn.close()
