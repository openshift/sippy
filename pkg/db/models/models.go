package models

import (
	"time"

	"github.com/jackc/pgtype"
	"gorm.io/gorm"
)

// Model is similar to gorm.Model, but sends lower snake case JSON,
// which is what the UI expects.
type Model struct {
	ID        uint           `json:"id" gorm:"primaryKey,column:id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// SchemaHash stores a hash of schema we apply programatically in sippy on startup. This is used to manage
// materialized views, their indicies, and functions that are not a good fit for schema management with goose.
type SchemaHash struct {
	gorm.Model

	// Type is "matview", "index", "function".
	Type string `json:"type"`
	// Name of the resource
	Name string `json:"name"`
	// Hash is the SHA256 hash of the string we generate programatically to see if anything
	// has changed since last applied.
	Hash string `json:"hash"`
}

// Migration stores a list of previously completed DB migrations, so they are only run once.
type Migration struct {
	gorm.Model

	// Duration is how long it took the migration to apply
	Duration time.Duration

	// Name is the name of the migration
	Name string `json:"name"`
}

// APISnapshot is a minimal implementation of historical data tracking. On GA or other dates of interest, we use the snapshot CLI command
// to query some of the main API endpoints, and store the resulting json with an type (indicating the API) into our database.
type APISnapshot struct {
	gorm.Model
	// Name is a user friendly name for this snapshot, i.e. "4.12 GA"
	Name    string `json:"name" gorm:"unique"`
	Release string `json:"release"`

	// OverallHealth is json from the /api/health?release=X API.
	OverallHealth pgtype.JSONB `json:"overall_health" gorm:"type:jsonb"`

	// PayloadHealth is json from the /api/releases/health?release=4.12 API and contains stats on payload health for
	// each stream in the release.
	PayloadHealth pgtype.JSONB `json:"payload_health" gorm:"type:jsonb"`

	// VariantHealth is json from the /api/variants?release=4.12 API and contains stats on job pass rates
	// for each variant.
	VariantHealth pgtype.JSONB `json:"variant_health" gorm:"type:jsonb"`

	// InstallHealth is json from the /api/install?release=4.12 API and contains stats on install success rates
	// by variant.
	InstallHealth pgtype.JSONB `json:"install_health" gorm:"type:jsonb"`

	// UpgradeHealth is json from the /api/upgrade?release=4.12 API and contains stats on upgrade success rates
	// by variant.
	UpgradeHealth pgtype.JSONB `json:"upgrade_health" gorm:"type:jsonb"`
}

// JiraIncident is an implementation of incident tracking.
type JiraIncident struct {
	Model

	// Key is the jira key, i.e. TRT-162
	Key string `json:"key" gorm:"index"`

	// Summary contains a description of the incident
	Summary string `json:"summary"`

	// StartTime is the issue was opened
	StartTime *time.Time `json:"start_time" gorm:"index"`

	// ResolutionTime is the time the issue was resolved
	ResolutionTime *time.Time `json:"resolution_time" gorm:"index"`
}
