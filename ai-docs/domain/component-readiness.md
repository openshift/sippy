# Domain Concept: Component Readiness

**Type**: Statistical Analysis  
**Data Source**: Aggregated test results  
**Primary API**: `/api/componentreadiness`

## Purpose

Component Readiness provides statistical assessment of OpenShift component health based on CI test results. Used by release managers to make go/no-go decisions.

## Key Metrics

| Metric | Description | Formula |
|--------|-------------|---------|
| **Pass Rate** | Percentage of tests passing | `passed / (passed + failed)` |
| **Regression** | Significant drop in pass rate | `current - baseline < -threshold` |
| **Risk Level** | Overall component health | `low`, `medium`, `high`, `extreme` |
| **Test Coverage** | Number of tests for component | Count of tests |

## Component Classification

Components are identified by:
1. **Test name patterns** (e.g., `[sig-network]` → Networking component)
2. **Jira components** (mapped from bug reports)
3. **Manual configuration** (component mappings)

**Mapping**: `pkg/componentreadiness/component_mapping.go`

## Regression Detection

**Baseline**: Historical pass rate (e.g., last 7 days)  
**Current**: Recent pass rate (e.g., last 24 hours)  
**Threshold**: Configurable per release (default: 5%)

```go
if currentPassRate < (baselinePassRate - threshold) {
    flagAsRegression()
}
```

## Risk Assessment

| Risk Level | Pass Rate | Action |
|------------|-----------|--------|
| **Low** | > 95% | Ship |
| **Medium** | 90-95% | Monitor |
| **High** | 80-90% | Investigate |
| **Extreme** | < 80% | Block release |

## Database Views

Component readiness uses PostgreSQL views for efficient querying:

**Example view**: `component_readiness_4_16`

```sql
CREATE VIEW component_readiness_4_16 AS
SELECT
    component,
    COUNT(*) FILTER (WHERE status='Passed') AS passed,
    COUNT(*) FILTER (WHERE status='Failed') AS failed,
    COUNT(*) FILTER (WHERE status='Passed')::float / NULLIF(COUNT(*), 0) AS pass_rate
FROM tests
WHERE release='4.16'
GROUP BY component;
```

**View generation**: See `/sippy-generate-release-views` skill

## API Usage

**Get component readiness**: `/api/componentreadiness?release=4.16`

**Filter by variant**: `/api/componentreadiness?release=4.16&variant=network:ovn`

**Component details**: `/api/componentreadiness/Networking?release=4.16`

## Frontend Display

**Location**: `sippy-ng/src/component_readiness/`

**Visualization**:
- Table view: Components with pass rates
- Trend charts: Historical pass rate
- Regression alerts: Highlighted in red

## Related Concepts

- [Test](test.md) - Individual test results aggregated into component readiness
- [Release](release.md) - Component readiness tracked per release
- [Variant](variant.md) - Component readiness sliced by variant

## References

- API implementation: `pkg/componentreadiness/`
- Database views: `pkg/db/componentreadiness/views.go`
- Frontend: `sippy-ng/src/component_readiness/`
- View generation skill: `/sippy-generate-release-views`
