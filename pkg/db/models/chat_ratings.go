package models

import (
	"github.com/google/uuid"
	"github.com/jackc/pgtype"
)

// ChatRating stores user feedback ratings for chat interactions
type ChatRating struct {
	Model

	// Rating is the star rating given by the user (1-5)
	Rating int `json:"rating" gorm:"not null"`

	// ClientID is a unique anonymous identifier
	ClientID uuid.UUID `json:"clientId" gorm:"type:uuid;index"`

	// Metadata contains additional information about the chat session
	// such as message counts, tool calls, LLM thoughts, and interaction size
	Metadata pgtype.JSONB `json:"metadata" gorm:"type:jsonb"`
}
