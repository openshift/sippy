# ADR-0003: Variant Extraction from Job Names

**Status**: Accepted  
**Date**: 2021-08-20  
**Component**: Sippy

## Context

Sippy needs to slice CI results by variants (platform, network, upgrade type, etc.) to identify variant-specific issues. Prow job names encode variant information, but there's no standardized structure.

**Example job name**: `periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn-upgrade`

**Requirements**:
- Extract variants: platform (aws), network (ovn), upgrade (upgrade), release (4.16)
- Support new variants without schema changes
- Handle variant combinations (e.g., aws + ovn + upgrade)
- Performant variant filtering in queries

**Scope**: This ADR is component-specific. For general data modeling patterns, see [Tier 1 ADRs](https://github.com/openshift/enhancements/tree/master/ai-docs/decisions).

## Decision

Extract variants from job names using **regex patterns** defined in a variant registry, store as JSONB in PostgreSQL.

**Implementation**:
```go
// pkg/variantregistry/registry.go
type VariantDefinition struct {
    Name    string         // "Platform"
    Pattern *regexp.Regexp // e.g., `(aws|gcp|azure|metal)`
    Values  []string       // Valid values
}
```

**Storage**:
```sql
CREATE TABLE prow_job_run_tests (
    ...
    variants JSONB, -- {"network": "ovn", "platform": "aws"}
    ...
);
CREATE INDEX idx_variants ON prow_job_run_tests USING gin(variants);
```

## Rationale

**Regex extraction advantages**:
- Flexible (adapt to job naming changes)
- Centralized (variant definitions in one place)
- Easy to add new variants (just add pattern)

**JSONB storage advantages**:
- Schemaless (don't need columns for each variant)
- GIN index for fast filtering
- Handles variant combinations naturally
- Easy to query: `WHERE variants @> '{"platform": "aws"}'`

**Variant registry rationale**:
- Single source of truth for variant definitions
- Code-based (version controlled, testable)
- Type-safe (Go structs)

## Consequences

### Positive
- Easy to add new variants (update registry, no schema migration)
- Fast filtering with GIN index
- Natural handling of variant combinations
- Future-proof (schemaless storage)

### Negative
- Regex fragility (job name changes can break extraction)
- No enforcement of job naming conventions
- JSONB queries more complex than column queries
- Need to validate regex patterns carefully

### Neutral
- Variant definitions live in code (not configuration)
- Need variant validation tests
- GIN index maintenance overhead

## Alternatives Considered

### Alternative 1: Separate Columns per Variant
**Description**: `platform TEXT, network TEXT, upgrade TEXT, ...`  
**Rejected because**:
- Schema change for every new variant
- Sparse columns (many NULLs)
- Harder to handle variant combinations
- Limited to pre-defined variants

### Alternative 2: Normalized Variant Tables
**Description**: `job_variants(job_id, variant_type, variant_value)`  
**Rejected because**:
- More complex queries (joins for filtering)
- Slower performance (join overhead)
- More storage overhead (multiple rows per job)

### Alternative 3: Parse from Prow Metadata
**Description**: Extract variants from Prow job config  
**Rejected because**:
- Not all variants in config (some in job name only)
- Need access to Prow config repo
- Config changes over time (historical data issues)

### Alternative 4: Manual Variant Tagging
**Description**: Manually tag jobs with variants  
**Rejected because**:
- Not scalable (thousands of jobs)
- Prone to errors
- Lag in tagging new jobs

## References

- Variant registry implementation: `pkg/variantregistry/`
- API filtering: `pkg/api/filter.go`
- Frontend variant selector: `sippy-ng/src/components/VariantSelector.js`
- Job naming conventions: https://github.com/openshift/release (unofficial)
