# ADR-0002: PostgreSQL Views for Component Readiness

**Status**: Accepted  
**Date**: 2022-03-10  
**Component**: Sippy

## Context

Component Readiness queries are expensive: aggregate millions of test results, group by component, calculate pass rates, detect regressions. Running these calculations on-demand is too slow for dashboard responsiveness.

**Requirements**:
- Component readiness queries must be < 500ms (dashboard UX)
- Support filtering by release, variant, time range
- Update daily (not real-time)
- Support historical comparison (current vs baseline)

**Scope**: This ADR is component-specific. For general database patterns, see [Tier 1 ADRs](https://github.com/openshift/enhancements/tree/master/ai-docs/decisions).

## Decision

Use PostgreSQL **materialized views** for component readiness, refreshed daily by the dataloader.

**Implementation**:
- One view per release: `component_readiness_4_16`, `component_readiness_4_17`
- Views pre-aggregate: component, pass_rate, test_count, regression_status
- Indexed by component, variant for fast filtering
- Refreshed after dataloader runs (REFRESH MATERIALIZED VIEW)

## Rationale

**Materialized views advantages**:
- Pre-computed aggregations (fast queries)
- Standard PostgreSQL feature (no custom caching logic)
- Easy to add/remove releases (create/drop view)
- SQL-based (can inspect with standard tools)

**Per-release views rationale**:
- Releases have different component mappings
- Easier to manage lifecycle (drop old releases)
- Simpler queries (no release filtering in WHERE clause)

## Consequences

### Positive
- Dashboard queries < 100ms (vs 10+ seconds raw)
- Reliable performance (pre-computed, indexed)
- Simple query logic in API handlers
- Easy to debug (standard SQL views)

### Negative
- Data freshness lag (views refreshed daily, not real-time)
- Disk usage (materialized views consume storage)
- View management overhead (create views for new releases)
- Schema changes require view recreation

### Neutral
- Need view generation skill (`/sippy-generate-release-views`)
- Need view refresh after dataloader runs
- Need view cleanup for old releases

## Alternatives Considered

### Alternative 1: On-Demand Calculation
**Description**: Calculate component readiness on every API call  
**Rejected because**:
- Too slow (10+ second queries unacceptable for dashboard)
- High database load (expensive aggregations on every request)
- Cache invalidation complexity

### Alternative 2: Redis Cache Only
**Description**: Cache aggregated results in Redis  
**Rejected because**:
- Cache warm-up complexity
- Cache invalidation logic needed
- Lost on Redis restart (need persistent storage anyway)
- Harder to debug than SQL views

### Alternative 3: Separate Component Readiness Table
**Description**: Dedicated table with daily updates  
**Rejected because**:
- More complex data loading logic
- Need custom aggregation code (vs SQL views)
- Harder to keep in sync with test results

### Alternative 4: Regular Views (not materialized)
**Description**: Use standard PostgreSQL views  
**Rejected because**:
- Still slow (re-compute on every query)
- No benefit over raw queries

## References

- View generation skill: `/sippy-generate-release-views`
- View update skill: `/sippy-update-ga-release-views`
- View implementation: `pkg/db/componentreadiness/views.go`
- API usage: `pkg/api/componentreadiness.go`
