package models

import (
	"time"
)

type CommentType int

const (
	CommentTypeRiskAnalysis CommentType = 0
)

// PullRequestComment tracks the risk analysis comment lifecycle for a single PR
type PullRequestComment struct {
	CreatedAt             time.Time
	UpdatedAt             time.Time
	PullNumber            int       `json:"pullNumber" gorm:"primaryKey"`
	CommentType           int       `json:"commentType" gorm:"primaryKey"`
	SHA                   string    `json:"sha" gorm:"primaryKey"`
	Org                   string    `json:"org" gorm:"primaryKey"`
	Repo                  string    `json:"repo" gorm:"primaryKey"`
	ProwJobRoot           string    `json:"prowJobRoot"`
	LastCommentAttempt    time.Time `json:"lastCommentAttempt"`
	FailedCommentAttempts int       `json:"failedCommentAttempts"`
}
