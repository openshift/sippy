package jobrunscan

import (
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Label defines a label that can be applied to jobs
type Label struct {
	// Immutable identifier used in job_labels table and symptom expressions
	// Must be valid identifier (word chars, not starting with digit)
	// Examples: "InfraFailure", "ClusterDNSFlake", "APIServerTimeout60"
	ID string `gorm:"primaryKey;type:varchar(80)" json:"id"`

	// Human-readable label text (can be changed)
	// Examples: "Infrastructure failure: omit job from CR", "Cluster DNS resolution failure(s)"
	LabelTitle string `gorm:"type:varchar(200);not null;uniqueIndex" json:"label_title"`

	// Markdown explanation of what this label indicates
	Explanation string `gorm:"type:text" json:"explanation"`

	// Where this label should NOT be displayed
	// (As a denylist, displays in a new context without needing updates)
	// Values: "spyglass", "metrics", "jaq choices", etc.
	HideDisplayContexts pq.StringArray `gorm:"type:text[]" json:"hide_display_contexts"`

	// Metadata
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

const (
	MetricsContext  = "metrics"
	SpyglassContext = "spyglass"
	JAQOptsContext  = "jaq-options"
)

func (Label) TableName() string {
	return "job_run_labels"
}
