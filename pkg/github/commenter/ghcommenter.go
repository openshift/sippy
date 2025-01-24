package commenter

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/github"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
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

func (ghc *GitHubCommenter) FindExistingCommentID(org, repo string, number int, commentKey, commentID string) (*int64, *string, error) {
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
	// if we don't have any includedRepos defined
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

func (ghc *GitHubCommenter) UpdatePendingCommentRecords(org, repo string, prNumber int, sha string, commentType models.CommentType, mergedAt *time.Time, pjPath string) {
	if !ghc.IsRepoIncluded(org, repo) {
		return
	}

	logger := log.WithField("org", org).
		WithField("repo", repo).
		WithField("number", prNumber).
		WithField("sha", sha)

	// here we should be getting the cached PREntry
	prEntry, err := ghc.githubClient.GetPREntry(org, repo, prNumber)

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

	// find the storage path prefix up through the PR number
	prNumberPath := fmt.Sprintf("/%d/", prNumber)
	position := strings.Index(pjPath, prNumberPath)
	if position < 0 {
		logger.Errorf("Prow job path %s does not contain PR number %s", pjPath, prNumberPath)
		return
	}
	prRoot := pjPath[:position] + prNumberPath

	// get any pending DB comment entries for this comment type and org/repo/number.
	// if PR has merged, remove them all; if not then remove any that aren't the current sha.
	// if an entry exists for the current sha, keep it rather than create a new one.
	foundExistingPRC := false
	pullRequestComments := make([]models.PullRequestComment, 0)
	res := ghc.dbc.DB.Table("pull_request_comments").
		Where("org = ? AND repo = ? AND pull_number = ? AND comment_type = ?", org, repo, prNumber, commentType).
		Scan(&pullRequestComments)

	if res.Error != nil {
		logger.WithError(res.Error).Error("could not query existing pre submit comment records")
	} else {
		for _, prc := range pullRequestComments {
			// if entry already exists for an unmerged PR just leave it for processing
			if prc.SHA == prEntry.SHA && mergedAt == nil {
				foundExistingPRC = true
				continue
			}

			// otherwise entry is either not for the latest sha or PR has merged, so clear it out
			prcCopy := prc
			ghc.ClearPendingRecord(prc.Org, prc.Repo, prc.PullNumber, prc.SHA, commentType, &prcCopy)
		}
	}

	// if we didn't find an entry, this is the most recent sha, and PR is not merged, then record an entry for it
	if !foundExistingPRC && sha == prEntry.SHA && mergedAt == nil {
		res = ghc.dbc.DB.Create(&models.PullRequestComment{
			CommentType: int(commentType),
			PullNumber:  prNumber,
			SHA:         sha,
			Org:         org,
			Repo:        repo,
			ProwJobRoot: prRoot,
		})

		if res.Error != nil {
			logger.WithError(res.Error).Error("Could not create comment record")
		}
	}
}

// check for this record in our table
// check to see what the last time we attempted to write a comment was
// if within threshold then skip
// if not then update the last attempt time
// return true if the record is valid for processing, false otherwise
func (ghc *GitHubCommenter) ValidateAndUpdatePendingRecordComment(org, repo string, number int, sha string, commentType models.CommentType) (bool, error) {

	pullRequestComment, err := ghc.getPendingRecord(org, repo, number, sha, commentType)

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}

		return false, err
	}

	// default is wait 10 minutes but then increase the wait by the number of errors we have encountered for this record
	waitMillis := int64(600000 * (pullRequestComment.FailedCommentAttempts + 1))

	if (time.Now().UnixMilli() - pullRequestComment.LastCommentAttempt.UnixMilli()) < waitMillis {
		return false, nil
	}

	res := ghc.dbc.DB.Model(&models.PullRequestComment{}).Where("org = ? AND repo = ? AND pull_number = ? AND sha = ? AND comment_type = ?  ", pullRequestComment.Org, pullRequestComment.Repo, pullRequestComment.PullNumber, pullRequestComment.SHA, pullRequestComment.CommentType).Update("last_comment_attempt", time.Now())
	if res.Error != nil {
		return false, res.Error
	}

	return true, nil
}

func (ghc *GitHubCommenter) UpdatePendingRecordErrorCount(org, repo string, number int, sha string, commentType models.CommentType) error {
	pullRequestComment, err := ghc.getPendingRecord(org, repo, number, sha, commentType)

	if err != nil {
		return err
	}

	// this would be the tenth failure so just delete it
	if pullRequestComment.FailedCommentAttempts > 8 {
		res := ghc.dbc.DB.Delete(pullRequestComment)
		if res.Error != nil {
			return res.Error
		}
		log.WithField("org", org).
			WithField("repo", repo).
			WithField("number", number).
			WithField("sha", sha).Warn("Exceeded failed attempts, deleted comment record.")
		return nil
	}

	res := ghc.dbc.DB.Model(&models.PullRequestComment{}).Where("org = ? AND repo = ? AND pull_number = ? AND sha = ? AND comment_type = ?  ", pullRequestComment.Org, pullRequestComment.Repo, pullRequestComment.PullNumber, pullRequestComment.SHA, pullRequestComment.CommentType).Update("failed_comment_attempts", pullRequestComment.FailedCommentAttempts+1)
	if res.Error != nil {
		return res.Error
	}
	return nil
}

func (ghc *GitHubCommenter) ClearPendingRecord(org, repo string, number int, sha string, commentType models.CommentType, record *models.PullRequestComment) {
	logger := log.WithField("org", org).
		WithField("repo", repo).
		WithField("number", number).
		WithField("sha", sha)

	logger.Debug("Call to clear pending record")

	var pullRequestComment *models.PullRequestComment
	var err error
	if record == nil {

		pullRequestComment, err = ghc.getPendingRecord(org, repo, number, sha, commentType)

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				logger.WithError(err).Error("Attempted to delete a missing record, likely indicating duplicate processing of the record.")
			} else {
				logger.WithError(err).Error("Failed to clear pending record")
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

func (ghc *GitHubCommenter) getPendingRecord(org, repo string, number int, sha string, commentType models.CommentType) (*models.PullRequestComment, error) {
	var pullRequestComment = &models.PullRequestComment{}
	res := ghc.dbc.DB.Where("org = ? AND repo = ? AND pull_number = ? AND sha = ? AND comment_type = ?", org, repo, number, sha, commentType).First(&pullRequestComment)

	if res.Error != nil {
		return nil, res.Error
	}
	return pullRequestComment, nil
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

func (ghc *GitHubCommenter) AddComment(org, repo string, number int, comment string) error {
	// could return error or log something but handle silently for now
	// we shouldn't even get called in this case
	if !ghc.IsRepoIncluded(org, repo) {
		return nil
	}

	return ghc.githubClient.CreatePRComment(org, repo, number, comment)
}

func (ghc *GitHubCommenter) DeleteComment(org, repo string, updateID int64) error {
	// could return error or log something but handle silently for now
	// we shouldn't even get called in this case
	if !ghc.IsRepoIncluded(org, repo) {
		return nil
	}

	return ghc.githubClient.DeletePRComment(org, repo, updateID)
}
