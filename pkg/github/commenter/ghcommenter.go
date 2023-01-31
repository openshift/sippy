package commenter

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/prowloader/github"
	"github.com/openshift/sippy/pkg/util/sets"
)

type GitHubCommenter struct {
	githubClient *github.Client
	dbc          *db.DB
	includeRepos map[string]sets.String
	excludeRepos map[string]sets.String
}

const TrtCommentIDKey = `trt_comment_id`

func NewGitHubCommenter(githubClient *github.Client, dbc *db.DB, excludedRepos, includedRepos []string) (*GitHubCommenter, error) {
	ghCommenter := &GitHubCommenter{}
	ghCommenter.githubClient = githubClient
	ghCommenter.dbc = dbc

	var err error
	ghCommenter.excludeRepos, err = buildOrgRepos(excludedRepos)
	if err != nil {
		log.WithError(err).Error("Failed GitHub commenter initialization")
		return nil, err
	}

	ghCommenter.includeRepos, err = buildOrgRepos(includedRepos)
	if err != nil {
		log.WithError(err).Error("Failed GitHub commenter initialization")
		return nil, err
	}

	return ghCommenter, nil
}

func buildOrgRepos(in []string) (map[string]sets.String, error) {
	if in == nil || len(in) < 1 {
		return nil, nil
	}

	out := make(map[string]sets.String)

	for _, r := range in {
		ar := strings.Split(r, `/`)
		var org, repo string

		if len(ar) > 2 {
			return nil, fmt.Errorf("invalid OrgRepo setting: %s", r)
		}

		if len(ar) < 2 {
			org = `openshift`
			repo = r
		} else {
			org = ar[0]
			repo = ar[1]
		}

		orgMap := out[org]

		if orgMap == nil {
			orgMap = sets.NewString()
			out[org] = orgMap
		}

		if !out[org].Has(repo) {
			out[org].Insert(repo)
		}
	}

	return out, nil
}

func (ghc *GitHubCommenter) CreateCommentID(commentType models.CommentType, sha string) string {
	if commentType == models.CommentTypeRiskAnalysis {
		return fmt.Sprintf("RISK_ANALYSIS_%s", sha)
	}

	// other types ...

	return fmt.Sprintf("TRT_%s", sha)
}

func (ghc *GitHubCommenter) GetCurrentState(org, repo string, number int) (*github.PREntry, error) {
	return ghc.githubClient.PRFetch(org, repo, number)
}

func (ghc *GitHubCommenter) FindExistingCommentID(org, repo string, number int, commentKey, commentID string) (*int64, error) {
	return ghc.githubClient.FindCommentID(org, repo, number, commentKey, commentID)
}

func (ghc *GitHubCommenter) IsRepoIncluded(org, repo string) bool {
	// remove anything explicitly excluded
	if ghc.excludeRepos != nil {
		if val, ok := ghc.excludeRepos[org]; ok {
			// only return if we are explicitly excluded in this case
			if val.Has(repo) {
				return false
			}
		}
	}

	// remove anything not included
	// if don't we have any includedRepos defined
	// then everything not excluded is included
	if ghc.includeRepos == nil {
		return true
	}

	val, ok := ghc.includeRepos[org]
	if !ok {
		return false
	}

	return val.Has(repo)
}

func (ghc *GitHubCommenter) UpdatePendingCommentRecords(org, repo string, number int, sha string, commentType models.CommentType, mergedAt *time.Time, pjPath string) {
	if !ghc.IsRepoIncluded(org, repo) {
		return
	}

	logger := log.WithField("org", org).
		WithField("repo", repo).
		WithField("number", number).
		WithField("sha", sha)

	// here we should be getting the cached PREntry
	prEntry, err := ghc.githubClient.GetPREntry(org, repo, number)

	if err != nil {
		logger.WithError(err).Error("Error getting the PREntry for updating comment records")
		return
	}

	if prEntry == nil || prEntry.SHA == "" {
		logger.Error("Invalid PREntry SHA")
		return
	}

	if pjPath == "" {
		logger.Error("Missing prow job path")
		return
	}

	prNumber := fmt.Sprintf("%d", number)
	var prRoot string

	position := strings.Index(pjPath, prNumber)
	if position > -1 {
		prRoot = pjPath[:position+len(prNumber)+1]
	}

	if prRoot == "" {
		logger.Error("Missing prow job root")
		return
	}

	// get all existing comment entries for this comment type and org/repo/number
	// if we are merged remove them all, if not then remove any that aren't the current sha
	// if we have a match for the current sha save it so we don't try to create a new one
	foundExistingPRC := false
	pullRequestComments := make([]models.PullRequestComment, 0)
	res := ghc.dbc.DB.Table("pull_request_comments").
		Where("org = ? AND repo = ? AND pull_number = ? AND comment_type = ?", org, repo, number, commentType).
		Scan(&pullRequestComments)

	if res.Error != nil {
		logger.WithError(res.Error).Error("could not query existing pre submit comment records")
	} else {
		for _, cmtupdt := range pullRequestComments {
			// if we already exist but have not been merged then
			// we don't have to create a new record
			if cmtupdt.SHA == prEntry.SHA && mergedAt == nil {
				foundExistingPRC = true
				continue
			}

			// otherwise we either aren't the latest sha or we have merged so clear
			clearRecord := cmtupdt
			ghc.ClearPendingRecord(clearRecord.Org, clearRecord.Repo, clearRecord.PullNumber, clearRecord.SHA, &clearRecord)
		}
	}

	// if we didn't find the record, this is the most recent sha and not merged then record an entry for it
	if !foundExistingPRC && sha == prEntry.SHA && mergedAt == nil {

		var pullRequestComment = &models.PullRequestComment{}
		pullRequestComment.CommentType = int8(commentType)
		pullRequestComment.PullNumber = number
		pullRequestComment.SHA = sha
		pullRequestComment.Org = org
		pullRequestComment.Repo = repo
		pullRequestComment.ProwJobRoot = prRoot

		res = ghc.dbc.DB.Create(&pullRequestComment)

		if res.Error != nil {
			logger.WithError(res.Error).Error("Could not create comment record")
		}
	}
}

func (ghc *GitHubCommenter) ClearPendingRecord(org, repo string, number int, sha string, record *models.PullRequestComment) {
	logger := log.WithField("org", org).
		WithField("repo", repo).
		WithField("number", number).
		WithField("sha", sha)

	logger.Debug("Call to clear pending record")

	var pullRequestComment = &models.PullRequestComment{}
	if record == nil {

		res := ghc.dbc.DB.Where("org = ? AND repo = ? AND pull_number = ? AND sha = ?", org, repo, number, sha).First(&pullRequestComment)

		if res.Error != nil {

			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				logger.WithError(res.Error).Error("Attempted to delete a missing record, likely indicating duplicate processing of the record.")
			} else {
				logger.WithError(res.Error).Error("Failed to clear pending record")
			}
			return
		}
	} else {
		pullRequestComment = record
	}

	res := ghc.dbc.DB.Delete(pullRequestComment)
	if res.Error != nil {
		logger.WithError(res.Error).Error("could not delete existing comment record")
	}
}

func (ghc *GitHubCommenter) QueryPRPendingComments(org, repo string, number int, commentType models.CommentType) ([]models.PullRequestComment, error) {
	pullRequestComments := make([]models.PullRequestComment, 0)

	res := ghc.dbc.DB.Table("pull_request_comments").
		Where("org = ? AND repo = ? AND pull_number = ? AND comment_type = ?", org, repo, number, commentType).
		Order("created_at").
		Scan(&pullRequestComments)

	if res.Error != nil {
		return nil, res.Error
	}

	return pullRequestComments, nil
}

func (ghc *GitHubCommenter) QueryPendingComments(commentType models.CommentType) ([]models.PullRequestComment, error) {
	pullRequestComments := make([]models.PullRequestComment, 0)

	res := ghc.dbc.DB.Table("pull_request_comments").
		Where("comment_type = ?", commentType).
		Order("created_at").
		Scan(&pullRequestComments)

	if res.Error != nil {
		return nil, res.Error
	}

	return pullRequestComments, nil
}

func (ghc *GitHubCommenter) AddComment(org, repo string, number int, comment, commentKey, commentID string) error {
	// could return error or log something but handle silently for now
	// we shouldn't even get called in this case
	if !ghc.IsRepoIncluded(org, repo) {
		return nil
	}

	ghcomment := fmt.Sprintf("<!-- META={\"%s\": \"%s\"} -->\n\n%s", commentKey, commentID, comment)

	return ghc.githubClient.CreatePRComment(org, repo, number, ghcomment)
}

func (ghc *GitHubCommenter) DeleteComment(org, repo string, updateID int64) error {
	// could return error or log something but handle silently for now
	// we shouldn't even get called in this case
	if !ghc.IsRepoIncluded(org, repo) {
		return nil
	}

	return ghc.githubClient.DeletePRComment(org, repo, updateID)
}
