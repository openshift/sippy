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
