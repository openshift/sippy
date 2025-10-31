# Job Symptoms Data Structures Design

Design document for TRT-2371: Job symptoms and observations feature

## Overview

This document defines the data structures needed to implement job symptom detection and labeling. The design uses Common Expression Language (CEL) for compound symptom logic, providing a standard expression language for combining simple symptoms with boolean operators. Simple symptoms use straightforward pattern matching against job artifacts.

## BigQuery Schema Changes (job_labels table)

Expand the existing `job_labels` table with these additional fields:

```sql
-- New fields to add to job_labels table
added_at TIMESTAMP         -- When this label was added/updated
updated_at TIMESTAMP        -- Last update timestamp
source_tool STRING          -- Tool that created it (e.g., 'annotate-job-runs', 'cloud-function', 'jaq-ui', 'symptom-detector')
symptom_id STRING           -- Immutable ID of the symptom that matched (e.g., 'InfraFailure', 'ClusterDNSFlake')
display_contexts ARRAY<STRING>  -- Where to display (e.g., ['spyglass', 'metrics', 'component-readiness'])
comment JSON                -- Free-form JSON for symptom-specific data (e.g., disruption metrics, timing data)

-- Existing field (repurposed):
-- label STRING remains for backward compatibility, will contain label_text from LabelPrototype
```

### Example job_labels row

```json
{
  "job_name": "periodic-ci-openshift-release-master-ci-4.18-upgrade-from-stable-4.17-e2e-gcp-upgrade",
  "job_run_name": "1234567890",
  "label": "Infrastructure failure: omit job from CR",
  "added_at": "2025-10-30T12:34:56Z",
  "updated_at": "2025-10-30T12:34:56Z",
  "source_tool": "symptom-detector",
  "symptom_id": "InfraFailure",
  "display_contexts": ["spyglass", "component-readiness"],
  "comment": {
    "matched_files": ["build-log.txt"],
    "match_count": 3,
    "custom_data": {}
  }
}
```

## PostgreSQL Schema (Sippy Database)

### 1. LabelPrototype - Label Definition

Defines what a label means and how it should be displayed.

```go
package models

import (
    "time"
    "github.com/lib/pq"
    "gorm.io/gorm"
)

// LabelPrototype defines a label that can be applied to jobs
type LabelPrototype struct {
    gorm.Model

    // Immutable identifier used in job_labels table and symptom expressions
    // Must be valid identifier (word chars, not starting with digit)
    // Examples: "InfraFailure", "ClusterDNSFlake", "APIServerTimeout"
    ID string `gorm:"primaryKey;type:varchar(100)" json:"id"`

    // Human-readable label text (can be changed)
    // Examples: "Infrastructure failure: omit job from CR", "Cluster DNS resolution failure(s)"
    LabelText string `gorm:"type:varchar(500);not null;uniqueIndex" json:"label_text"`

    // Markdown explanation of what this label indicates
    Description string `gorm:"type:text" json:"description"`

    // Where this label should be displayed
    // Values: "spyglass", "metrics", "component-readiness", "jaq", etc.
    DisplayContexts pq.StringArray `gorm:"type:text[]" json:"display_contexts"`

    // Optional: Severity or priority
    Severity string `gorm:"type:varchar(50)" json:"severity,omitempty"` // e.g., "critical", "warning", "info"

    // Metadata
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (LabelPrototype) TableName() string {
    return "label_prototypes"
}
```

### 2. SymptomDefinition - Symptom Detection Rules

Defines how to detect symptoms in job artifacts.

```go
// SymptomDefinition defines rules for detecting symptoms in job artifacts
type SymptomDefinition struct {
    gorm.Model

    // Immutable identifier for this symptom
    // Must be valid identifier (word chars, not starting with digit)
    ID string `gorm:"primaryKey;type:varchar(100)" json:"id"`

    // Human-readable summary (can be changed)
    Summary string `gorm:"type:varchar(500);not null;uniqueIndex" json:"summary"`

    // Type of matcher
    // Simple types: "substring", "regex", "exact", "file"
    // Compound type: "cel" (Common Expression Language)
    MatcherType string `gorm:"type:varchar(50);not null" json:"matcher_type"`

    // File pattern for simple matchers (glob pattern)
    // Examples: "**/build-log.txt", "**/e2e-timelines/**/*.json"
    // Null for CEL matcher type
    FilePattern string `gorm:"type:varchar(500)" json:"file_pattern,omitempty"`

    // Match string - interpretation depends on MatcherType:
    // - "substring": substring to find in file
    // - "regex": regular expression pattern
    // - "exact": exact string match
    // - "file": ignored (just checks file existence)
    // - "cel": CEL expression referencing other symptom IDs
    //   Example: "symptoms.DNS_Timeout && !symptoms.User_Error"
    MatchString string `gorm:"type:text" json:"match_string,omitempty"`

    // Labels to apply when this symptom matches (typically one, but can be multiple)
    LabelIDs pq.StringArray `gorm:"type:text[];not null" json:"label_ids"`

    // Applicability filters
    Releases pq.StringArray `gorm:"type:text[]" json:"releases,omitempty"` // e.g., ["4.17", "4.18"], null = all
    ReleaseStatus string `gorm:"type:varchar(50)" json:"release_status,omitempty"` // e.g., "accepted", "rejected", "ga"
    Product string `gorm:"type:varchar(50)" json:"product,omitempty"` // e.g., "ocp", "okd"

    // Time window for applicability (null = no time restriction)
    ValidFrom *time.Time `json:"valid_from,omitempty"`
    ValidUntil *time.Time `json:"valid_until,omitempty"`

    // Metadata
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (SymptomDefinition) TableName() string {
    return "symptom_definitions"
}

// Matcher type constants
const (
    MatcherTypeSubstring = "substring" // Simple substring match
    MatcherTypeRegex     = "regex"     // Regular expression match
    MatcherTypeExact     = "exact"     // Exact string match
    MatcherTypeFile      = "file"      // File exists (no content match)
    MatcherTypeCEL       = "cel"       // Common Expression Language for compound logic
)
```

## Example Symptom Definitions

### Simple Symptom: Substring Match

```json
{
  "id": "DNS_Timeout",
  "summary": "Cluster DNS resolution failures",
  "matcher_type": "substring",
  "file_pattern": "**/e2e-timelines/**/*.json",
  "match_string": "dial tcp",
  "label_ids": ["ClusterDNSFlake"],
  "releases": ["4.17", "4.18"],
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
  "label_ids": ["InfraFailure"],
  "releases": null,
  "valid_from": null,
  "valid_until": null
}
```

### Simple Symptom: File Existence

```json
{
  "id": "User_Quota_Error",
  "summary": "Insufficient quota error",
  "matcher_type": "substring",
  "file_pattern": "**/build-log.txt",
  "match_string": "insufficient quota",
  "label_ids": ["UserError"],
  "releases": null
}
```

### Compound Symptom: CEL Expression

Find infrastructure failures (install timeout AND NOT user error):

```json
{
  "id": "InfraFailure_Install",
  "summary": "Infrastructure install timeout (not user error)",
  "matcher_type": "cel",
  "file_pattern": null,
  "match_string": "symptoms.Install_Timeout && !symptoms.User_Quota_Error",
  "label_ids": ["InfraFailure"],
  "releases": null,
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
  "match_string": "symptoms.DNS_Timeout || symptoms.Storage_Provision_Failure || (symptoms.API_Timeout && symptoms.Etcd_Unavailable)",
  "label_ids": ["InfraFailure", "RequiresInvestigation"],
  "releases": ["4.17", "4.18"]
}
```

## BigQuery JobRunAnnotation Struct Changes

Update the Go struct that maps to BigQuery's job_labels table:

```go
// In pkg/bigquery/types.go or similar

type JobRunAnnotation struct {
    JobName     string    `bigquery:"job_name"`
    JobRunName  string    `bigquery:"job_run_name"`
    Label       string    `bigquery:"label"`

    // New fields
    AddedAt     time.Time `bigquery:"added_at"`
    UpdatedAt   time.Time `bigquery:"updated_at"`
    SourceTool  string    `bigquery:"source_tool"`
    SymptomID   string    `bigquery:"symptom_id"`
    DisplayContexts []string `bigquery:"display_contexts"`
    Comment     map[string]interface{} `bigquery:"comment"` // Free-form JSON
}
```

## Migration Considerations

1. **BigQuery Schema Evolution**: Add new columns to existing job_labels table (backward compatible)
2. **PostgreSQL Migrations**:
   - Create `label_prototypes` table
   - Create `symptom_definitions` table
   - Add indexes on frequently queried fields (releases, valid_from, valid_until)
3. **Backward Compatibility**:
   - Existing job_labels entries will have null values for new fields
   - Tools should handle missing symptom_id gracefully
   - The `label` field remains for display purposes

## Implementation Notes

### CEL Integration

Go implementation using github.com/google/cel-go:

```go
import (
    "github.com/google/cel-go/cel"
    "github.com/google/cel-go/checker/decls"
)

// EvaluateSymptom evaluates a single symptom definition against job artifacts
func (s *SymptomDefinition) Evaluate(ctx context.Context, artifacts JobArtifacts, symptomResults map[string]bool) (bool, error) {
    switch s.MatcherType {
    case MatcherTypeSubstring:
        return s.evaluateSubstring(ctx, artifacts)
    case MatcherTypeRegex:
        return s.evaluateRegex(ctx, artifacts)
    case MatcherTypeExact:
        return s.evaluateExact(ctx, artifacts)
    case MatcherTypeFile:
        return s.evaluateFileExists(ctx, artifacts)
    case MatcherTypeCEL:
        return s.evaluateCEL(ctx, symptomResults)
    default:
        return false, fmt.Errorf("unknown matcher type: %s", s.MatcherType)
    }
}

// evaluateCEL evaluates a CEL expression using results from other symptoms
func (s *SymptomDefinition) evaluateCEL(ctx context.Context, symptomResults map[string]bool) (bool, error) {
    // Create CEL environment with symptom results as variables
    env, err := cel.NewEnv(
        cel.Declarations(
            decls.NewVar("symptoms", decls.NewMapType(decls.String, decls.Bool)),
        ),
    )
    if err != nil {
        return false, fmt.Errorf("failed to create CEL environment: %w", err)
    }

    // Parse the CEL expression
    ast, issues := env.Compile(s.MatchString)
    if issues != nil && issues.Err() != nil {
        return false, fmt.Errorf("failed to compile CEL expression: %w", issues.Err())
    }

    // Create program
    prg, err := env.Program(ast)
    if err != nil {
        return false, fmt.Errorf("failed to create CEL program: %w", err)
    }

    // Evaluate with symptom results
    out, _, err := prg.Eval(map[string]interface{}{
        "symptoms": symptomResults,
    })
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

### Python Compatibility

Python code can use the `celpy` library:

```python
import celpy
import json

def evaluate_symptom(symptom_def, artifacts, symptom_results):
    matcher_type = symptom_def['matcher_type']

    if matcher_type == 'substring':
        return evaluate_substring(symptom_def, artifacts)
    elif matcher_type == 'regex':
        return evaluate_regex(symptom_def, artifacts)
    elif matcher_type == 'exact':
        return evaluate_exact(symptom_def, artifacts)
    elif matcher_type == 'file':
        return evaluate_file_exists(symptom_def, artifacts)
    elif matcher_type == 'cel':
        return evaluate_cel(symptom_def, symptom_results)
    else:
        raise ValueError(f"Unknown matcher type: {matcher_type}")

def evaluate_cel(symptom_def, symptom_results):
    # Create CEL environment
    env = celpy.Environment()

    # Compile the expression
    ast = env.compile(symptom_def['match_string'])

    # Create program
    program = env.program(ast)

    # Evaluate with symptom results
    activation = {'symptoms': symptom_results}
    result = program.evaluate(activation)

    return bool(result)
```

### Evaluation Order

For symptoms with CEL expressions that reference other symptoms, evaluation must be done in dependency order:

1. Build dependency graph from CEL expressions
2. Perform topological sort
3. Evaluate symptoms in order (simple matchers first, then CEL expressions)
4. Detect circular dependencies and fail gracefully

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

POST   /api/v1/jobs/:name/:run/evaluate    # Evaluate symptoms for a job run
GET    /api/v1/jobs/:name/:run/symptoms    # Get detected symptoms for a job run
```

## Database Indexes

```sql
-- label_prototypes
CREATE INDEX idx_label_prototypes_display_contexts ON label_prototypes USING GIN (display_contexts);
CREATE INDEX idx_label_prototypes_severity ON label_prototypes(severity);

-- symptom_definitions
CREATE INDEX idx_symptom_definitions_matcher_type ON symptom_definitions(matcher_type);
CREATE INDEX idx_symptom_definitions_releases ON symptom_definitions USING GIN (releases);
CREATE INDEX idx_symptom_definitions_label_ids ON symptom_definitions USING GIN (label_ids);
CREATE INDEX idx_symptom_definitions_valid_from ON symptom_definitions(valid_from);
CREATE INDEX idx_symptom_definitions_valid_until ON symptom_definitions(valid_until);
CREATE INDEX idx_symptom_definitions_product ON symptom_definitions(product);
CREATE INDEX idx_symptom_definitions_release_status ON symptom_definitions(release_status);
```

## Dependencies

### Go
- `github.com/google/cel-go` - CEL expression evaluation
- `gorm.io/gorm` - ORM (already in use)
- `github.com/lib/pq` - PostgreSQL array support (already in use)

### Python
- `celpy` - CEL expression evaluation for Python
- Standard library for simple matchers (regex, string operations)
