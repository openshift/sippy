package jobrunscan

import (
	"time"

	"gorm.io/gorm"
)

type Metadata struct {
	// User tracking
	CreatedBy string `gorm:"type:varchar(255)" json:"created_by"`
	UpdatedBy string `gorm:"type:varchar(255)" json:"updated_by"`

	// Record keeping (and gorm metadata)
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Links contains REST links for clients to follow for this specific type.
	// These are injected by the API and not stored in the DB.
	Links map[string]string `json:"links" gorm:"-"`
}
