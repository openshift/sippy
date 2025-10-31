# Job Symptoms Data Structures Design

Design document for TRT-2371: Job symptoms and observations feature

## Overview

This document defines the data structures needed to implement job symptom detection and labeling. The design uses a boolean expression tree pattern (JSON AST) for compound symptom logic, avoiding the need to implement a custom expression language while remaining database-friendly and easy to work with in both Go and Python.

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
    ID string `gorm:"primaryKey;type:varchar(100)" json:"id"`

    // Human-readable summary (can be changed)
    Summary string `gorm:"type:varchar(500);not null;uniqueIndex" json:"summary"`

    // Detection rule (stored as JSON)
    // See SymptomMatcher for structure
    Rule datatypes.JSON `gorm:"type:jsonb;not null" json:"rule"`

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
```

### 3. SymptomMatcher - Matching Rule Structure

This is not a database model but the JSON structure stored in `SymptomDefinition.Rule`.

```go
// SymptomMatcher represents the matching logic for a symptom
// This is serialized to JSON and stored in SymptomDefinition.Rule
type SymptomMatcher struct {
    // Type of matcher
    Type MatcherType `json:"type"`

    // For simple matchers (type: "file", "regex", "substring", "exact")
    FilePattern string `json:"file_pattern,omitempty"` // Glob pattern, e.g., "**/*build-log.txt"
    MatchString string `json:"match_string,omitempty"` // What to match in the file

    // For compound matchers (type: "and", "or", "not")
    Children []SymptomMatcher `json:"children,omitempty"`

    // For reference matcher (type: "symptom")
    SymptomID string `json:"symptom_id,omitempty"` // Reference another symptom's result
}

type MatcherType string

const (
    MatcherTypeSubstring MatcherType = "substring" // Simple substring match
    MatcherTypeRegex     MatcherType = "regex"     // Regular expression match
    MatcherTypeExact     MatcherType = "exact"     // Exact string match
    MatcherTypeFile      MatcherType = "file"      // File exists (no content match)
    MatcherTypeAnd       MatcherType = "and"       // Logical AND
    MatcherTypeOr        MatcherType = "or"        // Logical OR
    MatcherTypeNot       MatcherType = "not"       // Logical NOT
    MatcherTypeSymptom   MatcherType = "symptom"   // Reference another symptom
)
```

## Example Symptom Definitions

### Simple Symptom: File Content Match

```json
{
  "id": "ClusterDNSFlake",
  "summary": "Cluster DNS resolution failures",
  "rule": {
    "type": "regex",
    "file_pattern": "**/e2e-timelines/**/*.json",
    "match_string": "dial tcp.*i/o timeout"
  },
  "label_ids": ["ClusterDNSFlake"],
  "releases": ["4.17", "4.18"],
  "valid_from": "2025-01-01T00:00:00Z",
  "valid_until": null
}
```

### Compound Symptom: AND/OR/NOT Logic

Find infrastructure failures (install timeout AND NOT user error):

```json
{
  "id": "InfraFailureInstallTimeout",
  "summary": "Infrastructure install timeout (not user error)",
  "rule": {
    "type": "and",
    "children": [
      {
        "type": "substring",
        "file_pattern": "**/build-log.txt",
        "match_string": "timeout waiting for cluster install"
      },
      {
        "type": "not",
        "children": [
          {
            "type": "substring",
            "file_pattern": "**/build-log.txt",
            "match_string": "insufficient quota"
          }
        ]
      }
    ]
  },
  "label_ids": ["InfraFailure"],
  "releases": null,
  "valid_from": null,
  "valid_until": null
}
```

### Compound Symptom: Reference Other Symptoms

```json
{
  "id": "CriticalInfraIssue",
  "summary": "Critical infrastructure issue (network or storage)",
  "rule": {
    "type": "or",
    "children": [
      {
        "type": "symptom",
        "symptom_id": "ClusterDNSFlake"
      },
      {
        "type": "symptom",
        "symptom_id": "StorageProvisionFailure"
      }
    ]
  },
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

### Matcher Evaluation

The matcher evaluation should be recursive:

```go
func (m *SymptomMatcher) Evaluate(ctx context.Context, artifacts JobArtifacts) (bool, error) {
    switch m.Type {
    case MatcherTypeSubstring:
        return m.evaluateSubstring(ctx, artifacts)
    case MatcherTypeRegex:
        return m.evaluateRegex(ctx, artifacts)
    case MatcherTypeAnd:
        return m.evaluateAnd(ctx, artifacts)
    case MatcherTypeOr:
        return m.evaluateOr(ctx, artifacts)
    case MatcherTypeNot:
        return m.evaluateNot(ctx, artifacts)
    case MatcherTypeSymptom:
        return m.evaluateSymptomReference(ctx, artifacts)
    default:
        return false, fmt.Errorf("unknown matcher type: %s", m.Type)
    }
}
```

### Python Compatibility

Python code can easily work with these structures:

```python
import json

# Load symptom definition
symptom_def = json.loads(symptom_json)

def evaluate_matcher(matcher, artifacts):
    match_type = matcher['type']

    if match_type == 'substring':
        return evaluate_substring(matcher, artifacts)
    elif match_type == 'and':
        return all(evaluate_matcher(child, artifacts) for child in matcher['children'])
    elif match_type == 'or':
        return any(evaluate_matcher(child, artifacts) for child in matcher['children'])
    elif match_type == 'not':
        return not evaluate_matcher(matcher['children'][0], artifacts)
    # ... etc
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
  "id": "E2EDNS_Timeout_2025_Q4",
  "summary": "E2E DNS timeout failures",
  "rule": {
    "type": "regex",
    "file_pattern": "**/e2e-*.log",
    "match_string": "context deadline exceeded.*dns"
  },
  "label_ids": ["TestInfraIssue"],
  "releases": ["4.18"]
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
CREATE INDEX idx_symptom_definitions_releases ON symptom_definitions USING GIN (releases);
CREATE INDEX idx_symptom_definitions_label_ids ON symptom_definitions USING GIN (label_ids);
CREATE INDEX idx_symptom_definitions_valid_from ON symptom_definitions(valid_from);
CREATE INDEX idx_symptom_definitions_valid_until ON symptom_definitions(valid_until);
CREATE INDEX idx_symptom_definitions_product ON symptom_definitions(product);
CREATE INDEX idx_symptom_definitions_release_status ON symptom_definitions(release_status);
```
