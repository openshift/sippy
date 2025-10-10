package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"gorm.io/gorm"
)

// ChatConversation stores shared chat conversation history
type ChatConversation struct {
	ID        uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	// User who shared/created this conversation
	User string `json:"user" gorm:"not null;index"`

	// ParentID is the UUID of the conversation this was forked from, if any
	ParentID *uuid.UUID `json:"parent_id,omitempty" gorm:"type:uuid;index"`

	// Messages contains the full conversation history in JSONB format
	Messages pgtype.JSONB `json:"messages" gorm:"type:jsonb;not null"`

	// Metadata stores additional information like persona, page context, etc
	Metadata pgtype.JSONB `json:"metadata,omitempty" gorm:"type:jsonb"`

	// Links contains REST links for clients to follow. Most notably "self".
	// These are injected by the API and not stored in the DB.
	Links map[string]string `json:"links,omitempty" gorm:"-"`
}
