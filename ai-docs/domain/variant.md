# Domain Concept: Variant

**Type**: CI Job Dimension  
**Data Source**: Extracted from job names  
**Primary API**: `/api/variants`

## Purpose

Variants are dimensions that characterize how a job is configured. Sippy uses the NURP+ model to slice test results by variants.

## NURP+ Model

**NURP** = **N**etwork, **U**pgrade, **R**elease, **P**latform  
**Plus (+)**: Architecture, Installer, Topology, FeatureSet, etc.

| Variant | Examples | Description |
|---------|----------|-------------|
| **Network** | `sdn`, `ovn` | Network plugin |
| **Upgrade** | `upgrade`, `micro` | Upgrade type |
| **Release** | `4.15`, `4.16` | OpenShift version |
| **Platform** | `aws`, `gcp`, `azure`, `metal` | Cloud provider |
| **Architecture** | `amd64`, `arm64`, `s390x` | CPU architecture |
| **Installer** | `ipi`, `upi` | Installation method |
| **Topology** | `ha`, `single-node` | Cluster topology |
| **FeatureSet** | `techpreview`, `default` | Feature gates |

## Variant Extraction

Sippy parses job names using regex patterns:

```go
// pkg/variantregistry/registry.go
type VariantRegistry struct {
    Variants []VariantDefinition
}

type VariantDefinition struct {
    Name    string // "Platform"
    Pattern *regexp.Regexp // Regex to extract from job name
    Values  []string // Valid values
}
```

**Example job name**: `periodic-ci-openshift-release-master-nightly-4.16-e2e-aws-ovn-upgrade`

**Extracted variants**:
- Release: `4.16`
- Platform: `aws`
- Network: `ovn`
- Upgrade: `upgrade`

## Variant Registry

**Location**: `pkg/variantregistry/`

**Configuration**: Variant definitions live in code (not configuration files)

**Adding new variant**:
1. Define pattern in `pkg/variantregistry/`
2. Update API to expose variant
3. Update frontend to filter by variant

## API Usage

**Get jobs by variant**: `/api/jobs?release=4.16&platform=aws&network=ovn`

**Component readiness by variant**: `/api/componentreadiness?release=4.16&variant=network:ovn`

## Database Schema

Variants are stored as JSONB in PostgreSQL:

```sql
CREATE TABLE prow_job_run_tests (
    ...
    variants JSONB, -- {"network": "ovn", "platform": "aws"}
    ...
);
```

**Index**: `CREATE INDEX idx_variants ON prow_job_run_tests USING gin(variants);`

## Common Variant Combinations

| Combination | Purpose |
|-------------|---------|
| `4.16 + aws + ovn` | Standard AWS OVN testing |
| `4.16 + metal + sdn + upgrade` | Bare metal upgrade testing |
| `4.16 + gcp + ovn + single-node` | Single-node GCP testing |

## Related Concepts

- [Job](job.md) - Jobs are characterized by variants
- [Release](release.md) - Release is a primary variant
- [Component Readiness](component-readiness.md) - Statistics sliced by variant

## References

- Variant registry: `pkg/variantregistry/`
- API filtering: `pkg/api/filter.go`
- Frontend variant selector: `sippy-ng/src/components/VariantSelector.js`
