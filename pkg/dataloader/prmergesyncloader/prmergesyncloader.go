package prmergesyncloader

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/dataloader/prowloader/github"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

type mergedPR struct {
	org      string
	repo     string
	number   int
	headSHA  string
	mergedAt time.Time
}

type PRMergeSyncLoader struct {
	ctx      context.Context
	dbc      *db.DB
	ghClient *github.Client
	errors   []error
}

func New(ctx context.Context, dbc *db.DB, ghClient *github.Client) *PRMergeSyncLoader {
	return &PRMergeSyncLoader{
		ctx:      ctx,
		dbc:      dbc,
		ghClient: ghClient,
	}
}

func (l *PRMergeSyncLoader) Name() string {
	return "pr-merge-sync"
}

func (l *PRMergeSyncLoader) Errors() []error {
	return l.errors
}

func (l *PRMergeSyncLoader) Load() {
	if l.ghClient == nil {
		log.Info("No GitHub client, skipping PR merge sync")
		return
	}

	if err := l.sync(); err != nil {
		l.errors = append(l.errors, errors.Wrap(err, "error in PR merge sync"))
	}
}

// unmatchedPRs returns a base query scoped to prow_pull_requests rows that
// have no merged_at set and no sibling row with merged_at set. Callers add
// their own Select, Order, Limit, and Scan.
func (l *PRMergeSyncLoader) unmatchedPRs() *gorm.DB {
	return l.dbc.DB.
		Table("prow_pull_requests p").
		Where("p.merged_at IS NULL").
		Where(`NOT EXISTS (
			SELECT 1 FROM prow_pull_requests p2
			WHERE p2.org = p.org AND p2.repo = p.repo AND p2.number = p.number
			  AND p2.merged_at IS NOT NULL
		)`)
}

func (l *PRMergeSyncLoader) sync() error {
	type orgRepo struct {
		Org  string
		Repo string
	}

	var repos []orgRepo
	if res := l.unmatchedPRs().
		Select("DISTINCT p.org, p.repo").
		Scan(&repos); res.Error != nil && !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return errors.Wrap(res.Error, "could not fetch distinct repos from prow_pull_requests")
	}

	log.WithField("repos", len(repos)).Info("pr-merge-sync: repos with unmerged pull requests")

	var merged []mergedPR
	var rateLimitErr error
	for _, r := range repos {
		closedPRs, err := l.ghClient.ListRecentlyClosedPRs(r.Org, r.Repo)

		// Process valid PRs before checking the error; partial pages are common
		// when pagination hits a rate limit or transient API failure.
		for _, pr := range closedPRs {
			if pr.MergedAt == nil || pr.Head == nil || pr.Head.SHA == nil || pr.Number == nil {
				continue
			}
			merged = append(merged, mergedPR{
				org:      r.Org,
				repo:     r.Repo,
				number:   *pr.Number,
				headSHA:  *pr.Head.SHA,
				mergedAt: *pr.MergedAt,
			})
		}

		if err != nil {
			if l.ghClient.IsWithinRateLimitThreshold() {
				rateLimitErr = err
				break
			}
			log.WithError(err).WithField("org", r.Org).WithField("repo", r.Repo).Error("error fetching closed PRs")
		}
	}

	if len(merged) == 0 {
		log.Info("pr-merge-sync: no recently merged PRs found")
		return rateLimitErr
	}

	log.WithField("count", len(merged)).Info("pr-merge-sync: recently merged PRs to process")

	sqlDB, err := l.dbc.DB.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB: %w", err)
	}

	rowsUpdated, rowsDeleted, flushErr := flushMergedPRs(l.ctx, sqlDB, merged)
	if flushErr != nil {
		return flushErr
	}
	log.WithField("rows_updated", rowsUpdated).Info("pr-merge-sync: marked PR rows as merged")
	log.WithField("rows_deleted", rowsDeleted).Info("pr-merge-sync: cleared pending risk analysis comments")

	return rateLimitErr
}

func flushMergedPRs(ctx context.Context, sqlDB *sql.DB, merged []mergedPR) (int64, int64, error) {
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return 0, 0, fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("releasing pgx conn")
		}
	}()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			log.WithError(err).Error("rolling back transaction")
		}
	}()

	cleanup, err := db.CopyToTempTable(ctx, tx, "tmp_merged_prs", merged,
		[]db.TempColumn[mergedPR]{
			{Name: "sha", Type: "text NOT NULL", Value: func(m *mergedPR) any { return m.headSHA }},
			{Name: "org", Type: "text NOT NULL", Value: func(m *mergedPR) any { return m.org }},
			{Name: "repo", Type: "text NOT NULL", Value: func(m *mergedPR) any { return m.repo }},
			{Name: "number", Type: "integer NOT NULL", Value: func(m *mergedPR) any { return m.number }},
			{Name: "merged_at", Type: "timestamptz NOT NULL", Value: func(m *mergedPR) any { return m.mergedAt }},
		},
	)
	if err != nil {
		return 0, 0, fmt.Errorf("creating temp table: %w", err)
	}
	defer cleanup()

	updateTag, err := tx.Exec(ctx, `
		UPDATE prow_pull_requests p
		SET merged_at = tmp.merged_at
		FROM tmp_merged_prs tmp
		WHERE p.sha = tmp.sha
		  AND p.org = tmp.org
		  AND p.repo = tmp.repo
		  AND p.number = tmp.number
		  AND p.merged_at IS NULL
	`)
	if err != nil {
		return 0, 0, fmt.Errorf("updating merged_at: %w", err)
	}

	deleteTag, err := tx.Exec(ctx, `
		DELETE FROM pull_request_comments prc
		WHERE prc.comment_type = $1
		  AND EXISTS (
			SELECT 1 FROM tmp_merged_prs tmp
			WHERE prc.org = tmp.org
			  AND prc.repo = tmp.repo
			  AND prc.pull_number = tmp.number
		  )
	`, models.CommentTypeRiskAnalysis)
	if err != nil {
		return 0, 0, fmt.Errorf("deleting stale comments: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("committing transaction: %w", err)
	}

	return updateTag.RowsAffected(), deleteTag.RowsAffected(), nil
}
