# Job Symptoms Data Structures Design

Design document for TRT-2371: Job symptoms and observations feature

## Overview

This document defines the data structures needed to implement job symptom
detection and labeling. Simple symptoms use straightforward pattern matching
against job artifacts. The design leaves room for implementing compound symptom
logic (probably as a future expansion) using Common Expression Language (CEL)
for logic combining labels with boolean operators.

## BigQuery Schema Changes (`job_labels` table)

Expand the existing `job_labels` table with these additional fields:

```sql
-- New fields to add to job_labels table
created_at TIMESTAMP        -- When this label was applied for this job
updated_at TIMESTAMP        -- Last update timestamp
source_tool STRING          -- Tool that created it (e.g., 'annotate-job-runs', 'cloud-function', 'jaq-ui')
symptom_id STRING           -- Immutable ID of the symptom that matched (e.g., 'InfraFailure', 'ClusterDNSFlake')
display_contexts ARRAY<STRING>  -- Where to display (e.g., ['spyglass', 'metrics', 'test-details'])

-- Existing field:
-- label STRING remains for backward compatibility, will contain ID from jobrunscan.Label
```

### Example `job_labels` row

```json
{
  "job_name": "periodic-ci-openshift-release-master-ci-4.18-upgrade-from-stable-4.17-e2e-gcp-upgrade",
  "job_run_name": "1234567890",
  "label": "ClusterDNSFlake",
  "created_at": "2025-10-30T12:34:56Z",
  "updated_at": "2025-10-30T12:34:56Z",
  "source_tool": "symptom-detector",
  "symptom_id": "ClusterDNSFlake",
  "display_contexts": ["spyglass", "test-details"],
  "comment": {
    "matched_files": ["build-log.txt"],
    "match_count": 3,
    "custom_data": {}
  }
}
```

## PostgreSQL Schema (Sippy Database)

### 1. Label - Label Definition

Defines what a label means and how it should be displayed.

```go
// In pkg/db/models/jobrunscan/
package jobrunscan

import (
    "time"
    "github.com/lib/pq"
    "gorm.io/gorm"
)

// Label defines a label that can be applied to jobs
type Label struct {
    gorm.Model

    // Immutable identifier used in job_labels table and symptom expressions
    // Must be valid identifier (word chars, not starting with digit)
    // Examples: "InfraFailure", "ClusterDNSFlake", "APIServerTimeout60"
    ID string `gorm:"primaryKey;type:varchar(80)" json:"id"`

    // Human-readable label text (can be changed)
    // Examples: "Infrastructure failure: omit job from CR", "Cluster DNS resolution failure(s)"
    LabelTitle string `gorm:"type:varchar(200);not null;uniqueIndex" json:"label_title"`

    // Markdown explanation of what this label indicates
    Explanation string `gorm:"type:text" json:"explanation"`

    // Where this label should be displayed
    // Values: "spyglass", "metrics", "component-readiness", "jaq", etc.
    DisplayContexts pq.StringArray `gorm:"type:text[]" json:"display_contexts"`

    // Metadata
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Label) TableName() string {
    return "job_run_labels"
}
```

### 2. Symptom - Symptom Definition

Defines how to detect symptoms in job artifacts.

```go
// Symptom defines rules for detecting symptoms in job artifacts
type Symptom struct {
    gorm.Model

    // Immutable identifier for this symptom
    // Must be valid identifier (word chars, not starting with digit)
    ID string `gorm:"primaryKey;type:varchar(100)" json:"id"`

    // Human-readable summary (can be changed)
    Summary string `gorm:"type:varchar(200);not null;uniqueIndex" json:"summary"`

    // Type of matcher
    // Simple types: "string", "regex", "jq", "xpath", "none"
    // Compound type: "cel" (Common Expression Language against label names)
    MatcherType string `gorm:"type:varchar(50);not null" json:"matcher_type"`

    // File pattern for simple matchers (glob pattern)
    // Examples: "**/build-log.txt", "**/e2e-timelines/**/*.json"
    // Null for CEL matcher type
    FilePattern string `gorm:"type:varchar(500)" json:"file_pattern,omitempty"`

    // Match string - interpretation depends on MatcherType:
    // - "string": substring to find in file
    // - "regex": regular expression pattern
    // - "none": ignored (just checks file existence)
    // - "cel": CEL expression referencing applied labels (e.g. "DNSTimeout && !OperatorError")
    MatchString string `gorm:"type:text" json:"match_string,omitempty"`

    // Labels to apply when this symptom matches (typically none or one, but can be multiple)
    LabelIDs pq.StringArray `gorm:"type:text[]" json:"label_ids"`

    // Applicability filters
    FilterReleases pq.StringArray `gorm:"type:text[]" json:"filter_releases,omitempty"` // e.g., ["4.17", "4.18"], null = all
    FilterReleaseStatuses pq.StringArray  `gorm:"type:text[]" json:"filter_release_statuses,omitempty"` // e.g., ["Development", "Full Support"]
    FilterProducts pq.StringArray `gorm:"type:text[]" json:"filter_products,omitempty"` // e.g., ["OCP", "OKD", "HCM"]

    // Time window for applicability (null = no time restriction)
    ValidFrom *time.Time `json:"valid_from,omitempty"`
    ValidUntil *time.Time `json:"valid_until,omitempty"`

    // Metadata
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Symptom) TableName() string {
    return "job_run_symptoms"
}

// Matcher type constants
const (
    MatcherTypeString    = "string"    // Simple substring match
    MatcherTypeRegex     = "regex"     // Regular expression match
    MatcherTypeFile      = "none"      // File exists (no content match)
    MatcherTypeCEL       = "cel"       // Common Expression Language for compound logic
)
```

## Example Symptom Definitions

### Simple Symptom: Substring Match

```json
{
  "id": "DNS_Timeout",
  "summary": "Cluster DNS resolution failures",
  "matcher_type": "string",
  "file_pattern": "**/e2e-timelines/**/*.json",
  "match_string": "dial tcp",
  "label_ids": ["ClusterDNSFlake"],
  "filter_releases": ["4.17", "4.18"],
  "valid_from": "2025-01-01T00:00:00Z",
  "valid_until": null
}
```

### Simple Symptom: Regex Match

```json
{
  "id": "Install_Timeout",
  "summary": "Cluster install timeout",
  "matcher_type": "regex",
  "file_pattern": "**/build-log.txt",
  "match_string": "timeout waiting for.*install",
  "label_ids": ["ClusterInstallTimeout"],
  "filter_releases": null,
  "valid_from": null,
  "valid_until": null
}
```

### Simple Symptom: File Existence

```json
{
  "id": "HasIntervals",
  "summary": "Has interval file(s)",
  "matcher_type": "none",
  "file_pattern": "**/intervals*.json",
  "match_string": "",
  "label_ids": ["IntervalFile"],
  "filter_releases": null
}
```

### Compound Symptom: CEL Expression

Find infrastructure failures (install timeout AND NOT user error):

```json
{
  "id": "InfraFailure_Install",
  "summary": "Infrastructure install timeout (did not get to intervals)",
  "matcher_type": "cel",
  "file_pattern": null,
  "match_string": "ClusterInstallTimeout && !IntervalFile",
  "label_ids": ["InfraFailure"],
  "filter_releases": null,
  "valid_from": null,
  "valid_until": null
}
```

### Compound Symptom: Complex CEL Logic

Critical infrastructure issue (network or storage):

```json
{
  "id": "Critical_Infra",
  "summary": "Critical infrastructure issue (network or storage)",
  "matcher_type": "cel",
  "match_string": "DNS_Timeout || Storage_Provision_Failure || (API_Timeout && Etcd_Unavailable)",
  "label_ids": ["InfraFailure", "RequiresInvestigation"],
  "filter_releases": ["4.17", "4.18"]
}
```

## BigQuery jobRunAnnotation Struct Changes

The existing `jobRunAnnotation` struct in `pkg/componentreadiness/jobrunannotator/jobrunannotator.go` should be updated to include the new fields:

```go
// In pkg/componentreadiness/jobrunannotator/jobrunannotator.go

type jobRunAnnotation struct {
    ID        string              `bigquery:"prowjob_build_id"`
    StartTime civil.DateTime      `bigquery:"prowjob_start"`
    Label     string              `bigquery:"label"`
    Comment   string              `bigquery:"comment"` // Existing field - keep as string (JSON serialized)
    User      bigquery.NullString `bigquery:"user"`

    // New fields to add
    AddedAt         time.Time `bigquery:"added_at"`
    UpdatedAt       time.Time `bigquery:"updated_at"`
    SourceTool      string    `bigquery:"source_tool"`
    SymptomID       string    `bigquery:"symptom_id"`
    DisplayContexts []string  `bigquery:"display_contexts"`

    url       string // internal field (not persisted to BQ)
}
```

Note: The `Comment` field should remain a string but will contain JSON-serialized data. The existing code already uses `json.MarshalIndent` to serialize data into this field (see `generateComment()` method).

## Migration Considerations

1. **BigQuery Schema Evolution**: Add new columns to existing job_labels table (backward compatible)
2. **PostgreSQL Migrations**:
   - Create `job_run_labels` table
   - Create `job_run_symptoms` table
   - Add indexes on frequently queried fields (releases, valid_from, valid_until)
3. **Backward Compatibility**:
   - Existing job_labels entries will have null values for new fields
   - Tools should handle missing symptom_id gracefully
   - The `label` field remains for display purposes

## Implementation Notes

### Relationship to Existing Code

This design leverages existing infrastructure in the codebase:

1. **Matcher Interface**: The `ContentMatcher` interface in `pkg/api/jobartifacts/query.go` already
   exists and is used for matching content in job artifacts. The symptom detection system will build
   on this pattern.

2. **Existing Matchers**: The codebase already has `stringLineMatcher` and `regexLineMatcher`
   implementations in `pkg/api/jobartifacts/content_line_matcher.go`. These can be reused or adapted
   for symptom matching.

3. **Job Artifact Query System**: The `JobArtifactQuery` struct and `QueryJobRunArtifacts` method
   provide the foundation for scanning job artifacts from GCS buckets.

4. **Job Run Annotator**: The existing `JobRunAnnotator` in
   `pkg/componentreadiness/jobrunannotator/jobrunannotator.go` already writes to the BigQuery
   `job_labels` table. The symptom detection system will extend this.

### New Matcher Types Needed

While string and regex matchers exist, we'll need to add:

1. **File existence matcher** (`MatcherTypeFile`) - checks if a file matching the pattern exists
2. **CEL matcher** (`MatcherTypeCEL`) - evaluates boolean expressions against applied labels (future)
3. **JQ matcher** - for JSON content matching (future)
4. **XPath matcher** - for XML content matching (future)

### CEL Integration for Compound Symptoms

The `cel` matcher type will be evaluated AFTER all simple symptoms have been processed and labels have been applied. Here's how it works:

```go
import (
    "github.com/google/cel-go/cel"
)

// evaluateCEL evaluates a CEL expression using labels that have been applied to the job run
// This is called AFTER simple symptom matchers have been evaluated and labels applied
func (s *Symptom) evaluateCEL(ctx context.Context, appliedLabels map[string]bool) (bool, error) {
    // Create CEL environment with applied labels as boolean variables
    // The labels are referenced directly by their ID in the expression
    // Example: "ClusterDNSFlake && !UserError"
    env, err := cel.NewEnv(
        cel.Variable("ClusterDNSFlake", cel.BoolType),
        cel.Variable("UserError", cel.BoolType),
        // ... all label IDs would be declared as variables
    )
    if err != nil {
        return false, fmt.Errorf("failed to create CEL environment: %w", err)
    }

    // Parse the CEL expression from MatchString
    ast, issues := env.Compile(s.MatchString)
    if issues != nil && issues.Err() != nil {
        return false, fmt.Errorf("failed to compile CEL expression: %w", issues.Err())
    }

    // Create program
    prg, err := env.Program(ast)
    if err != nil {
        return false, fmt.Errorf("failed to create CEL program: %w", err)
    }

    // Evaluate with applied labels as variables
    // Labels not present are treated as false
    out, _, err := prg.Eval(appliedLabels)
    if err != nil {
        return false, fmt.Errorf("failed to evaluate CEL expression: %w", err)
    }

    result, ok := out.Value().(bool)
    if !ok {
        return false, fmt.Errorf("CEL expression did not return boolean")
    }

    return result, nil
}
```

### Symptom Evaluation Workflow

Automated application of labels will occur as the job runs and after it finishes:

1. In our cloud function that runs as job files are written to the bucket:
   1. **Load simple symptom definitions** from PostgreSQL (filtered to the extent possible)
   2. **Evaluate simple symptoms** against files they match
   3. **Apply labels** from matching symptoms if not already applied
2. In sippy fetchdata cronjob:
   1. **Load CEL symptom definitions** from PostgreSQL (filtered to the extent possible)
   2. For each completed job, **Load existing labels** from BQ and **Evaluate CEL symptoms** 
   3. **Apply additional labels** from matching CEL symptoms

Symptoms can also be applied retroactively by annotate-job-runs when they are created or changed.

1. **Load symptom definitions** from PostgreSQL (filtered by release, time window, etc.)
2. **Separate simple and CEL symptoms** into two groups
3. **Evaluate simple symptoms** first (string, regex, file existence matchers) against job artifacts
4. **Apply labels** from matching simple symptoms to job run
5. **Evaluate CEL symptoms** using the set of applied labels
6. **Apply additional labels** from matching CEL symptoms
7. **Write all labels** to BigQuery `job_labels` table

### Python Compatibility

Python code can use the `celpy` library for CEL evaluation:

```python
import celpy
import re

def evaluate_symptom(symptom_def, artifacts, applied_labels):
    """Evaluate a symptom definition against job artifacts.

    Args:
        symptom_def: Symptom definition dict
        artifacts: Dict mapping file paths to content
        applied_labels: Dict mapping label IDs to bool (already applied labels)

    Returns:
        bool: Whether the symptom matched
    """
    matcher_type = symptom_def['matcher_type']

    if matcher_type == 'string':
        return evaluate_substring(symptom_def, artifacts)
    elif matcher_type == 'regex':
        return evaluate_regex(symptom_def, artifacts)
    elif matcher_type == 'none':
        return evaluate_file_exists(symptom_def, artifacts)
    elif matcher_type == 'cel':
        return evaluate_cel(symptom_def, applied_labels)
    else:
        raise ValueError(f"Unknown matcher type: {matcher_type}")

def evaluate_cel(symptom_def, applied_labels):
    """Evaluate CEL expression against applied labels.

    The CEL expression in match_string references label IDs directly.
    Example: "ClusterDNSFlake && !UserError"
    """
    # Create CEL environment with label IDs as boolean variables
    env = celpy.Environment()

    # Compile the expression
    ast = env.compile(symptom_def['match_string'])

    # Create program
    program = env.program(ast)

    # Evaluate with applied labels (labels not present default to false)
    result = program.evaluate(applied_labels)

    return bool(result)
```

### AI-Friendly Format

The JSON format is ideal for AI-generated symptom definitions. A chat interface could:

1. Analyze test failure patterns
2. Generate a symptom definition JSON
3. Allow user to review and adjust
4. Save to database via API

Example prompt: "Create a symptom for DNS timeout failures in e2e tests"

AI Response:
```json
{
  "id": "E2E_DNS_Timeout_2025_Q4",
  "summary": "E2E DNS timeout failures",
  "matcher_type": "regex",
  "file_pattern": "**/e2e-*.log",
  "match_string": "context deadline exceeded.*dns",
  "label_ids": ["TestInfraIssue"],
  "releases": ["4.18"]
}
```

Example prompt: "Create a symptom for install failures that are not quota errors"

AI Response:
```json
{
  "id": "Install_Failure_Not_Quota",
  "summary": "Install failure (excluding quota errors)",
  "matcher_type": "cel",
  "match_string": "symptoms.Install_Timeout && !symptoms.User_Quota_Error",
  "label_ids": ["InfraFailure"],
  "releases": ["4.17", "4.18"]
}
```

## API Endpoints (Future)

```
POST   /api/v1/symptoms                    # Create symptom definition
GET    /api/v1/symptoms                    # List all symptoms
GET    /api/v1/symptoms/:id                # Get symptom details
PUT    /api/v1/symptoms/:id                # Update symptom
DELETE /api/v1/symptoms/:id                # Delete symptom

POST   /api/v1/labels                      # Create label prototype
GET    /api/v1/labels                      # List all labels
GET    /api/v1/labels/:id                  # Get label details
PUT    /api/v1/labels/:id                  # Update label
```

## Database Indexes

```sql
-- job_run_labels
CREATE INDEX idx_label_prototypes_display_contexts ON job_run_labels USING GIN (display_contexts);
CREATE INDEX idx_label_prototypes_severity ON job_run_labels(severity);

-- job_run_symptoms
CREATE INDEX idx_symptom_definitions_matcher_type ON job_run_symptoms(matcher_type);
CREATE INDEX idx_symptom_definitions_releases ON job_run_symptoms USING GIN (releases);
CREATE INDEX idx_symptom_definitions_label_ids ON job_run_symptoms USING GIN (label_ids);
CREATE INDEX idx_symptom_definitions_valid_from ON job_run_symptoms(valid_from);
CREATE INDEX idx_symptom_definitions_valid_until ON job_run_symptoms(valid_until);
CREATE INDEX idx_symptom_definitions_product ON job_run_symptoms(product);
CREATE INDEX idx_symptom_definitions_release_status ON job_run_symptoms(release_status);
```

## Dependencies

### Go
- `github.com/google/cel-go` - CEL expression evaluation (already vendored in codebase)
- `gorm.io/gorm` - ORM (already in use)
- `github.com/lib/pq` - PostgreSQL array support (already in use)
- `cloud.google.com/go/bigquery` - BigQuery client (already in use)
- `cloud.google.com/go/storage` - GCS client for artifact access (already in use)

### Python
- `celpy` - CEL expression evaluation for Python (to be added)
- Standard library for simple matchers (regex, string operations)

## Summary of Changes to Existing Code

### Files to Modify

1. **`pkg/componentreadiness/jobrunannotator/jobrunannotator.go`**
   - Add new fields to `jobRunAnnotation` struct
   - Update `bulkInsertJobRunAnnotations` to handle new fields

2. **BigQuery Schema**
   - Add columns to `job_labels` table: `added_at`, `updated_at`, `source_tool`, `symptom_id`, `display_contexts`

### New Files to Create

1. **`pkg/db/models/jobrunscan/label.go`**
   - Define `Label` struct for label prototypes

2. **`pkg/db/models/jobrunscan/symptom.go`**
   - Define `Symptom` struct for symptom definitions
   - Define matcher type constants

3. **Database migrations**
   - Create `prow_job_run_labels` table
   - Create `prow_job_run_symptoms` table
   - Add indexes

### Integration Points

The symptom detection system will:
- Use the existing `JobArtifactQuery` and `ContentMatcher` infrastructure from `pkg/api/jobartifacts/`
- Extend the existing `JobRunAnnotator` to support symptom-based labeling
- Write results to the same BigQuery `job_labels` table that's already in use
- Store symptom and label definitions in new PostgreSQL tables
