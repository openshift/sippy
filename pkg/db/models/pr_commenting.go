package models

import (
	"gorm.io/gorm"
	"time"
)

// move to types
type CommentType int8

const (
	CommentTypeRiskAnalysis CommentType = 0
	//	CommentTypePayloadFailure CommentType = 1
)

// need a pr
// key is repo and pr #? (url/link)
// include author (in the key?)
// we have release_pull_requests && prow_pull_requests

// risk analysis
// prow_pull_requests has org,repo, number, author, sha, link, merged_at (timestamp)
// do we want a risk analysis for each sha (if multiple shas between comments then just the last)?
// might not even need this, could just query ProwPullRequest with missing merged_at data?
// still want to know the most recent sha though
// and if the pr has merged
// don't want extra gh lookups so keep this table for now
type GithubPresubmitComment struct {
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	ProwPullNumber   int
	IsCommentPending bool   `gorm:"index"`
	CommentType      int8   `gorm:"primarykey"`
	SHA              string `json:"sha" gorm:"primarykey"`
	Link             string `json:"link,omitempty" gorm:"primarykey"`
}

// Evaluating why sometimes db.Save works for update and when it does we need to use db.Create
// specifying primarykey vs. default in the gorm.Model is part of it...
type GithubPresubmitCommentAlt struct {
	gorm.Model
	ProwPullNumber   int
	IsCommentPending bool
	CommentType      int8
	SHA              string
	Link             string
}

// payload accept / reject
// release_pull_requests has url, pull_request_id

// pr has a list of ProwJob instances

// pr has multiple comments {presubmit & postsubmit} each may occur multiple times
