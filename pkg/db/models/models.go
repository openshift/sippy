package models

import (
	"time"

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
