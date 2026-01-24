package jobrunscan

import (
	"time"

	"github.com/lib/pq"
)

// Symptom defines rules for detecting symptoms in job artifacts
type Symptom struct {
	SymptomContent
	ApplicabilityFilters
	Metadata
}

// SymptomContent provides the core definition of a symptom
type SymptomContent struct {
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
}

// ApplicabilityFilters specifies filters for when a symptom is applicable (e.g. only a subset of releases).
type ApplicabilityFilters struct {
	// Time window for applicability (null = no time restriction)
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`

	// If a field is null, all values are considered applicable (no filter).
	FilterReleases        pq.StringArray `gorm:"type:text[]" json:"filter_releases,omitempty"`         // e.g., ["4.17", "4.18"], null = all
	FilterReleaseStatuses pq.StringArray `gorm:"type:text[]" json:"filter_release_statuses,omitempty"` // e.g., ["Development", "Full Support"]
	FilterProducts        pq.StringArray `gorm:"type:text[]" json:"filter_products,omitempty"`         // e.g., ["OCP", "OKD", "HCM"]
}

func (Symptom) TableName() string {
	return "job_run_symptoms"
}

// Matcher type constants
const (
	MatcherTypeString = "string" // Simple substring match
	MatcherTypeRegex  = "regex"  // Regular expression match
	MatcherTypeFile   = "none"   // File exists (no content match)
	MatcherTypeCEL    = "cel"    // Common Expression Language for compound logic
)
