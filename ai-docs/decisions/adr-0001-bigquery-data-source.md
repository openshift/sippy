# ADR-0001: BigQuery as Primary Data Source

**Status**: Accepted  
**Date**: 2021-05-15  
**Component**: Sippy

## Context

Sippy needs to analyze OpenShift CI job results from Prow. Job results are stored in multiple formats and locations (TestGrid, GCS, BigQuery).

**Requirements**:
- Query millions of test results across thousands of jobs
- Historical analysis (weeks to months of data)
- Complex filtering (by job, test, variant, time range)
- Performance for dashboard queries

**Scope**: This ADR is component-specific. For general data architecture patterns, see [Tier 1 ADRs](https://github.com/openshift/enhancements/tree/master/ai-docs/decisions).

## Decision

Use Google BigQuery as the primary data source for Prow CI results, with PostgreSQL as secondary storage for aggregated data and caching.

**Architecture**:
1. Prow uploads results to BigQuery (prow.jobs table)
2. Sippy dataloader queries BigQuery periodically
3. Results stored in PostgreSQL for fast dashboard queries
4. Redis cache for expensive aggregations

## Rationale

**BigQuery advantages**:
- Already populated by Prow (no new data pipeline)
- Excellent performance for large dataset queries
- SQL interface (familiar to developers)
- Handles schema evolution (new columns added over time)

**PostgreSQL rationale**:
- Faster for dashboard queries (indexed, materialized views)
- Component readiness views (pre-aggregated statistics)
- Offline operation (not dependent on BigQuery availability)

## Consequences

### Positive
- Leverage existing Prow data infrastructure
- Fast queries for large historical datasets
- No need to build custom data ingestion pipeline
- SQL-based analysis (accessible to non-developers)

### Negative
- BigQuery costs (queries are not free)
- Data freshness lag (dataloader runs periodically, not real-time)
- Dependency on Google Cloud (vendor lock-in)
- Dual storage complexity (BigQuery + PostgreSQL + Redis)

### Neutral
- Need dataloader process to sync BigQuery → PostgreSQL
- Need cache invalidation strategy

## Alternatives Considered

### Alternative 1: TestGrid as Data Source
**Description**: Parse TestGrid HTML pages  
**Rejected because**:
- No structured API (HTML scraping fragile)
- Limited historical data access
- Poor performance for complex queries

### Alternative 2: Direct GCS Access
**Description**: Parse junit XML files directly from GCS  
**Rejected because**:
- Massive number of files (millions)
- No indexing (slow queries)
- Need custom parser for junit XML
- Already done by Prow → BigQuery pipeline

### Alternative 3: PostgreSQL Only (no BigQuery)
**Description**: Build custom ingestion from Prow  
**Rejected because**:
- Duplicate effort (Prow already sends to BigQuery)
- Need to handle schema evolution ourselves
- More infrastructure to maintain

## References

- BigQuery schema: `prow.jobs` table documentation
- Data loader implementation: `pkg/dataloader/`
- Prow BigQuery documentation: https://github.com/kubernetes/test-infra/tree/master/prow/bigquery
