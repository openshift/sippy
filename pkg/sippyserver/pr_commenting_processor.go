package sippyserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"gorm.io/gorm"

	jobQueries "github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/prow"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/github/commenter"
	"github.com/openshift/sippy/pkg/prowloader/gcs"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/sets"
)

var (
	writeCommentMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "sippy_repo_add_comment",
		Help: "Tracks the call made to add a pr comment",
	}, []string{"org", "repo"})

	writeCommentErrorMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_repo_add_comment_errors",
		Help: "Tracks the number of errors we receive when trying to add a a pr comment",
	}, []string{"org", "repo"})

	buildCommentMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sippy_repo_build_ra_comment",
		Help:    "Tracks the call made to build a risk analysis pr comment",
		Buckets: prometheus.LinearBuckets(0, 5000, 10),
	}, []string{"org", "repo"})

	checkCommentReady = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sippy_repo_check_ready_comment",
		Help:    "Tracks the call made to verify a pr is ready for a pr comment",
		Buckets: prometheus.LinearBuckets(0, 500, 10),
	}, []string{"org", "repo"})

	buildPendingWork = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sippy_repo_build_pending_comment_work",
		Help:    "Tracks the call made to query db and add items to the pending work queue",
		Buckets: prometheus.LinearBuckets(0, 500, 10),
	}, []string{"type"})
)

type RiskAnalysisEntry struct {
	Key   string
	Value RiskAnalysisSummary
}

// dbc: our database
// gcsBucket: handle to our root gcs bucket
// commentAnalysisWorkers: the number of threads active to process pending comment jobs
// commentAnalysisRate: the minimun duration between querying the db for pending jobs
// commentUpdaterRate: the minimum duration between adding a comment before we begin work on adding the next
// ghCommenter: the commenting implmentation
// dryRunOnly: default is true to prevent unintended commenting when running locally or in a test deployment
func NewWorkProcessor(dbc *db.DB, gcsBucket *storage.BucketHandle, commentAnalysisWorkers int, commentAnalysisRate, commentUpdaterRate time.Duration, ghCommenter *commenter.GitHubCommenter, dryRunOnly bool) *WorkProcessor {
	wp := &WorkProcessor{dbc: dbc, gcsBucket: gcsBucket, ghCommenter: ghCommenter,
		commentAnalysisRate:    commentAnalysisRate,
		commentUpdaterRate:     commentUpdaterRate,
		commentAnalysisWorkers: commentAnalysisWorkers,
		dryRunOnly:             dryRunOnly,
	}
	return wp
}

type WorkProcessor struct {
	commentUpdaterRate     time.Duration
	commentAnalysisRate    time.Duration
	commentAnalysisWorkers int
	dbc                    *db.DB
	gcsBucket              *storage.BucketHandle
	ghCommenter            *commenter.GitHubCommenter
	dryRunOnly             bool
}

type PendingComment struct {
	comment     string
	sha         string
	org         string
	repo        string
	number      int
	commentType int8
}

type CommentWorker struct {
	commentUpdaterRateLimiter util.RateLimiter
	pendingComments           chan PendingComment
	ghCommenter               *commenter.GitHubCommenter
	restrictedLogins          sets.String
	dryRunOnly                bool
}

type AnalysisWorker struct {
	dbc                 *db.DB
	gcsBucket           *storage.BucketHandle
	riskAnalysisLocator *regexp.Regexp
	pendingAnalysis     chan models.PullRequestComment
	pendingComments     chan PendingComment
}

type RiskAnalysisSummary struct {
	Name             string
	URL              string
	RiskLevel        api.RiskLevel
	TestRiskAnalysis []api.ProwJobRunTestRiskAnalysis
}

type RiskAnalysisEntryList []RiskAnalysisEntry

func (r RiskAnalysisEntryList) Len() int      { return len(r) }
func (r RiskAnalysisEntryList) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r RiskAnalysisEntryList) Less(i, j int) bool {
	if r[i].Value.RiskLevel.Level == r[j].Value.RiskLevel.Level {
		return r[i].Value.Name > r[j].Value.Name
	}
	return r[i].Value.RiskLevel.Level > r[j].Value.RiskLevel.Level
}

func buildRestrictedUsers(in []string) sets.String {
	if in == nil || len(in) < 1 {
		return nil
	}

	set := sets.NewString()

	for _, l := range in {
		set.Insert(l)
	}

	return set
}

func (wp *WorkProcessor) Run(ctx context.Context) {

	// create a channel with a max buffer of 5 for github updates
	// single thread will pull updates from that channel and process them
	// one at a time as the rate limiter allows
	pendingComments := make(chan PendingComment, 5)
	defer close(pendingComments)

	commentWorker := CommentWorker{
		commentUpdaterRateLimiter: util.NewRateLimiter(wp.commentUpdaterRate),
		pendingComments:           pendingComments,
		ghCommenter:               wp.ghCommenter,

		// want an explicit setting to enable commenting
		// so that we don't make comments running locally
		// or in a test deployment unless configured to do so
		dryRunOnly: wp.dryRunOnly,

		// internal testing, will go away
		// don't want long term support for per login access
		// but do want to restrict the initial rollout
		restrictedLogins: buildRestrictedUsers([]string{"deads2k", "neisw", "dgoodwin", "stbenjam", "xueqzhan", "deepsm007", "DennisPeriquet"}),
	}

	if commentWorker.dryRunOnly {
		log.Warning("Github Comment Worker started in dry run only mode, active commenting is disabled")
	}
	go commentWorker.Run()

	// the work thread below will put items on pendingWork
	// blocking if the buffer is full
	// commentAnalysisWorker threads will pull jobs and process them
	// putting any comments on the pendingComments channel
	// blocking if the buffer is full
	pendingWork := make(chan models.PullRequestComment, wp.commentAnalysisWorkers)
	defer close(pendingWork)

	for i := 0; i < wp.commentAnalysisWorkers; i++ {
		analysisWorker := AnalysisWorker{riskAnalysisLocator: gcs.GetDefaultRiskAnalysisSummaryFile(), dbc: wp.dbc, gcsBucket: wp.gcsBucket, pendingAnalysis: pendingWork, pendingComments: pendingComments}
		go analysisWorker.Run()
	}

	// check context to verify we are still active
	// use ticker to check next batch of jobs
	ticker := time.NewTicker(wp.commentAnalysisRate)
	defer ticker.Stop()

	running := true
	for running {

		select {
		case <-ctx.Done():
			running = false

		case <-ticker.C:

			// if we haven't finished processing the prior iteration
			// before we begin our next we may get duplicate records
			// we handle that but if we are backed up then wait

			// if we have pending items still then skip the next work cycle
			if len(pendingWork) == 0 && len(pendingComments) == 0 {
				err := wp.work(ctx, pendingWork)

				// we only expect an error when we are in a terminal state like context is done
				if err != nil {
					running = false
				}
			} else {
				log.Info("Work still pending, skipping WorkProcessor work cycle")
			}

		}
	}

	log.Info("No longer active, shutting down")

}

func (wp *WorkProcessor) work(ctx context.Context, pendingWork chan models.PullRequestComment) error {

	start := time.Now()
	defer func() {
		end := time.Now()
		buildPendingWork.WithLabelValues("work-processor").Observe(float64(end.UnixMilli() - start.UnixMilli()))
	}()

	log.Debug("Checking for work")

	// get a list of items
	// process each item one at a time while checking for shutdown

	items, err := wp.fetchItems()

	if err != nil {
		log.WithError(err).Error("Failed to query pending comments")

		// we want to keep our processor loop active so don't pass the error back up
		return nil
	}

	for _, i := range items {

		select {

		case <-ctx.Done():

			log.Info("Context is done, stopping processor")
			return errors.New("context closed")

		default:
			log.Debugf("Adding item to pending work: %s/%s/%d/%s", i.Org, i.Repo, i.PullNumber, i.SHA)
			pendingWork <- i
			log.Debugf("Item added to pending work: %s/%s/%d/%s", i.Org, i.Repo, i.PullNumber, i.SHA)
		}
	}

	log.Debug("Finished Checking for work")
	return nil
}

func (wp *WorkProcessor) fetchItems() ([]models.PullRequestComment, error) {
	return wp.ghCommenter.QueryPendingComments(models.CommentTypeRiskAnalysis)
}

func (cw *CommentWorker) Run() {
	defer cw.commentUpdaterRateLimiter.Close()

	var errCount float64
	for pc := range cw.pendingComments {

		cw.commentUpdaterRateLimiter.Tick()

		commentReady, err := cw.ghCommenter.ValidateAndUpdatePendingRecordComment(pc.org, pc.repo, pc.number, pc.sha, models.CommentType(pc.commentType))

		// if we had an error here this is different from errors with GitHub
		// log them but don't include in the rate limiter
		if err != nil {
			log.WithError(err).Errorf("Error validating pending record %s/%s/%d - %s", pc.org, pc.repo, pc.number, pc.sha)
			continue
		}
		if !commentReady {
			log.Infof("Skipping pending record %s/%s/%d - %s", pc.org, pc.repo, pc.number, pc.sha)
			continue
		}

		err = cw.writeComment(cw.ghCommenter, pc)

		if err == nil {
			// if we had an error writing the comment then keep the record
			// we will attempt to process the record again and overwrite any previous comment for the same sha
			// otherwise, clear the record
			cw.ghCommenter.ClearPendingRecord(pc.org, pc.repo, pc.number, pc.sha, models.CommentType(pc.commentType), nil)
			if errCount > 0 {
				errCount--
				writeCommentErrorMetric.WithLabelValues(pc.org, pc.repo).Set(errCount)
			}
		} else {
			log.WithError(err).Errorf("Error processing record %s/%s/%d - %s", pc.org, pc.repo, pc.number, pc.sha)
			errCount++
			writeCommentErrorMetric.WithLabelValues(pc.org, pc.repo).Set(errCount)
			err = cw.ghCommenter.UpdatePendingRecordErrorCount(pc.org, pc.repo, pc.number, pc.sha, models.CommentType(pc.commentType))
			if err != nil {
				log.WithError(err).Errorf("Error updating error count for record %s/%s/%d - %s", pc.org, pc.repo, pc.number, pc.sha)
			}
		}

		// any error from ghCommenter impacts our backoff
		// if no errors then we reduce any current backoff
		cw.commentUpdaterRateLimiter.UpdateRate(err != nil)

		log.Debug("Pending comment processed")
	}
}

// internal testing, will go away
// don't want long term support for per login access
// but do want to restrict the initial rollout
func (cw *CommentWorker) IsRestrictedLogin(login *string) bool {
	if cw.restrictedLogins != nil {
		if login != nil {
			return !cw.restrictedLogins.Has(*login)
		}
		// if we have restricted logins defined and this login is missing or not in the map then they are restricted
		return true
	}

	return false
}

func (cw *CommentWorker) writeComment(ghCommenter *commenter.GitHubCommenter, pendingComment PendingComment) error {

	start := time.Now()
	defer func() {
		end := time.Now()
		writeCommentMetric.WithLabelValues(pendingComment.org, pendingComment.repo).Observe(float64(end.UnixMilli() - start.UnixMilli()))
	}()

	// if there is no comment then just delete the record
	if pendingComment.comment == "" {
		return nil
	}

	// could be that the include / exclude lists were updated
	// after the pending record was written
	// double check before we interact with github
	// the record should still get deleted
	if !ghCommenter.IsRepoIncluded(pendingComment.org, pendingComment.repo) {
		return nil
	}

	logger := log.WithField("org", pendingComment.org).
		WithField("repo", pendingComment.repo).
		WithField("number", pendingComment.number)

	// are we still the latest sha?
	prEntry, err := ghCommenter.GetCurrentState(pendingComment.org, pendingComment.repo, pendingComment.number)

	if err != nil {
		logger.WithError(err).Error("Failed to get the current PR state")
		return err
	}

	if prEntry == nil || prEntry.SHA == "" {
		logger.Error("Invalid sha when validating prEntry")
		return nil
	}

	if pendingComment.commentType == int8(models.CommentTypeRiskAnalysis) && prEntry.MergedAt != nil {
		logger.Warning("PR has merged, skipping risk analysis comment")
		return nil
	}

	if prEntry.SHA != pendingComment.sha {

		// we leave comments for previous shas in place
		// but, we don't want to add a comment for an older sha
		// we should have a new record with the current sha
		// and will analyze latest against that
		// we do want to delete our pending comment record though
		// which should happen if we return nil
		return nil
	}

	if prEntry.State == nil {
		logger.Error("Invalid state for PR")
		return nil
	}

	if !strings.EqualFold(*prEntry.State, "open") {
		logger.Warningf("Skipping commenting for PR state: %s", *prEntry.State)
		return nil
	}

	// we allow all other processing up to this point
	// but for now we only comment on specific logins
	// this goes away completely when we have validated
	// things are working as expected after roll out
	if cw.IsRestrictedLogin(prEntry.Login) {
		return nil
	}

	// create a constant for the key
	// determine the commentType and build the id off of that and the sha
	// generate the comment

	commentID := ghCommenter.CreateCommentID(models.CommentType(pendingComment.commentType), pendingComment.sha)

	// when running in dryRunOnly mode we do everything up until adding or deleting anything in GitHub
	// this allows for local testing / debugging without actually modifying PRs
	// it is the default setting and needs to be overridden in production / live commenting instances
	if cw.dryRunOnly {
		logger.Infof("Dry run comment for: %s\n%s", commentID, pendingComment.comment)
		return nil
	}

	ghcomment := fmt.Sprintf("<!-- META={\"%s\": \"%s\"} -->\n\n%s", commenter.TrtCommentIDKey, commentID, pendingComment.comment)

	// is there an existing comment of our type that we should remove
	existingCommentID, commentBody, err := ghCommenter.FindExistingCommentID(pendingComment.org, pendingComment.repo, pendingComment.number, commenter.TrtCommentIDKey, commentID)

	// for now, we return any errors when interacting with gitHub so that we backoff our processing rate
	// to do, select which ones indicate a need to backoff
	if err != nil {
		return err
	}

	if existingCommentID != nil {
		// compare the current body against the pending body
		// if they are the same then don't comment again
		if commentBody != nil && strings.TrimSpace(*commentBody) == strings.TrimSpace(ghcomment) {
			logger.Infof("Existing comment matches pending comment for id: %s", commentID)
			return nil
		}
		// we delete the existing comment and add a new one so the comment will be at the end of the comment list
		err = ghCommenter.DeleteComment(pendingComment.org, pendingComment.repo, *existingCommentID)
		// if we had an error then return it, the record will remain, and we will attempt processing again later
		if err != nil {
			return err
		}
	}

	logger.Infof("Adding comment id: %s", commentID)
	return ghCommenter.AddComment(pendingComment.org, pendingComment.repo, pendingComment.number, ghcomment)
}

func (aw *AnalysisWorker) Run() {

	// wait for the next item to be available and process it
	// exit when closed
	for i := range aw.pendingAnalysis {

		if i.CommentType == int8(models.CommentTypeRiskAnalysis) {
			aw.processRiskAnalysisComment(i)
		} else {
			log.Warningf("Unsupported comment type: %d for %s/%s/%d/%s", i.CommentType, i.Org, i.Repo, i.PullNumber, i.SHA)
		}

	}
}

func (aw *AnalysisWorker) processRiskAnalysisComment(prPendingComment models.PullRequestComment) {

	logger := log.WithField("org", prPendingComment.Org).
		WithField("repo", prPendingComment.Repo).
		WithField("Number", prPendingComment.PullNumber).
		WithField("sha", prPendingComment.SHA)

	start := time.Now()
	logger.Debug("Processing item")

	if prPendingComment.CommentType != int8(models.CommentTypeRiskAnalysis) {
		logger.Warningf("Unsupported comment type: %d", prPendingComment.CommentType)
		return
	}

	// we will likely pull in prs hours before the jobs finish
	// meaning there will be many checks before it is ready for commenting
	// skip the db / gcs lookup of analysis on the first pass and just check to see
	// if all jobs have completed to save the more intensive query / gcs locate for analysis
	allCompleted, _ := aw.buildPRJobRiskAnalysis(prPendingComment.ProwJobRoot, true)
	if !allCompleted {
		logger.Debug("Jobs are still active")

		t := float64(time.Now().UnixMilli() - start.UnixMilli())
		checkCommentReady.WithLabelValues(prPendingComment.Org, prPendingComment.Repo).Observe(t)
		return
	}

	defer func() {
		t := float64(time.Now().UnixMilli() - start.UnixMilli())
		buildCommentMetric.WithLabelValues(prPendingComment.Org, prPendingComment.Repo).Observe(t)
	}()

	// do the full pass if we got the all clear, we will still validate all are completed again
	allCompleted, analysis := aw.buildPRJobRiskAnalysis(prPendingComment.ProwJobRoot, false)
	if !allCompleted {
		logger.Debug("Jobs are still active")
		return
	}

	if analysis == nil {
		logger.Errorf("Invalid Risk Analysis result")
		return
	}

	// when the comment processor sees an empty comment it will
	// not create a comment but will delete the comment record
	comment := ""
	if len(analysis) > 0 {

		sortedAnalysis := make(RiskAnalysisEntryList, 0)
		for k, v := range analysis {
			sortedAnalysis = append(sortedAnalysis, RiskAnalysisEntry{k, v})
		}
		sort.Sort(sortedAnalysis)

		comment = buildComment(sortedAnalysis, prPendingComment.SHA)
	}

	pendingComment := PendingComment{
		comment:     comment,
		sha:         prPendingComment.SHA,
		org:         prPendingComment.Org,
		repo:        prPendingComment.Repo,
		number:      prPendingComment.PullNumber,
		commentType: prPendingComment.CommentType,
	}

	// will block if the buffer is full
	log.Debugf("Adding comment to pendingComments: %s/%s/%s", pendingComment.org, pendingComment.repo, pendingComment.sha)
	aw.pendingComments <- pendingComment
	log.Debugf("Comment added to pendingComments: %s/%s/%s", pendingComment.org, pendingComment.repo, pendingComment.sha)
}

func buildComment(sortedAnalysis RiskAnalysisEntryList, sha string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Job Failure Risk Analysis for sha: %s\n\n| Job Name | Failure Risk |\n|:---|:---|\n", sha))

	// don't want the comment to be too large so if we have a high number of jobs to analyze
	// reduce the max tests / reasons we show
	maxSubRows := 3
	if len(sortedAnalysis) > 10 {
		maxSubRows = 1
	}

	for a, value := range sortedAnalysis {
		tableKey := value.Key

		// top 20 should be more than enough
		if a > 19 {
			sb.WriteString(fmt.Sprintf("\nShowing %d of %d jobs analysis", a, len(sortedAnalysis)))
			break
		}

		if value.Value.URL != "" {
			tableKey = fmt.Sprintf("[%s](%s)", value.Key, value.Value.URL)
		}

		var riskSb strings.Builder
		riskSb.WriteString(fmt.Sprintf("**%s**", value.Value.RiskLevel.Name))
		for i, t := range value.Value.TestRiskAnalysis {
			if i > maxSubRows {
				riskSb.WriteString(fmt.Sprintf("<br>---<br>Showing %d of %d test results", i, len(value.Value.TestRiskAnalysis)))
				break
			}
			if i > 0 {
				riskSb.WriteString("<br>---")
			}
			riskSb.WriteString(fmt.Sprintf("<br>*%s*", t.Name))
			for j, r := range t.Risk.Reasons {
				if j > maxSubRows {
					riskSb.WriteString(fmt.Sprintf("<br>Showing %d of %d test risk reasons", j, len(t.Risk.Reasons)))
					break
				}
				riskSb.WriteString(fmt.Sprintf("<br>%s", r))
			}

			// Do we have open bugs?  Stack them vertically to preserve real estate
			for k, b := range t.OpenBugs {

				// Currently we don't limit the number of open bugs we show
				if k == 0 {
					if len(t.Risk.Reasons) > 0 {
						riskSb.WriteString("<br><br>")
					}

					riskSb.WriteString("Open Bugs")
				}
				riskSb.WriteString(fmt.Sprintf("<br>[%s](%s)", b.Summary, b.URL))
			}
		}

		sb.WriteString(fmt.Sprintf("|%s|%s|\n", tableKey, riskSb.String()))

	}

	return sb.String()
}

// Walks the GCS path for this job to find the most recent job runs, if they have not finished then returns false
// otherwise returns a map of the test name and the overall RiskAnalysis for that test
// if the map is empty it indicates either all jobs passed or any analysis for failures was unknown
func (aw *AnalysisWorker) buildPRJobRiskAnalysis(prRoot string, dryrun bool) (bool, map[string]RiskAnalysisSummary) {

	// get the list of objects one level down from our root
	it := aw.gcsBucket.Objects(context.Background(), &storage.Query{
		Prefix:    prRoot,
		Delimiter: "/",
	})

	analysisByJobs := make(map[string]RiskAnalysisSummary)
	jobRun := gcs.NewGCSJobRun(aw.gcsBucket, "")

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}

		// want empty Name indicating a folder
		if len(attrs.Name) > 0 {
			continue
		}

		// jobName
		// pr-logs/pull/org_repo/1555/pull-ci-openshift-origin-master-e2e-aws-csi/
		jobPath := strings.Split(attrs.Prefix, "/")

		// last string is "" so -2
		jobName := jobPath[len(jobPath)-2]

		// we have to get the data from latest-build.txt and then check for finished.json in that path
		bytes, err := jobRun.GetContent(context.TODO(), fmt.Sprintf("%s%s", attrs.Prefix, "latest-build.txt"))
		if err != nil {
			log.WithError(err).Errorf("Failed to get latest build info for: %s", attrs.Prefix)
			return false, nil
		}

		latest := string(bytes)
		latestPath := fmt.Sprintf("%s%s/", attrs.Prefix, latest)
		finishedJSON := fmt.Sprintf("%sfinished.json", latestPath)

		var riskSummary api.RiskSummary

		// currently we only validate that the file exists, we aren't pulling anything out of it
		if !jobRun.ContentExists(context.TODO(), finishedJSON) {
			return false, nil
		}

		if dryrun {
			// we only want to evaluate if we are finished or not
			continue
		}

		prowLink := ""
		prowJobJSON := fmt.Sprintf("%sprowjob.json", latestPath)
		bytes, err = jobRun.GetContent(context.TODO(), prowJobJSON)
		if err != nil {
			log.WithError(err).Errorf("Failed to get prow job for: %s", latestPath)

			// we don't have to return, but we can't include a prow link
		} else {
			pj := prow.ProwJob{}
			if bytes != nil {

				err := json.Unmarshal(bytes, &pj)
				if err != nil {
					log.WithError(err).Errorf("Failed to unmarshal prow job for: %s", latestPath)
				} else {
					prowLink = pj.Status.URL
				}

			}
		}

		jobRunID, err := strconv.ParseInt(latest, 10, 64)

		if err != nil {
			log.WithError(err).Errorf("Failed to parse latest id: %s for: %s", latest, latestPath)

			// skip the db lookup and go right to gcs
			riskSummary = aw.getGCSOverallRiskLevel(latestPath)
		} else {

			// lookup prowjob and run count
			logger := log.WithField("jobRunID", jobRunID)
			jobRun, jobRunTestCount, err := jobQueries.FetchJobRun(aw.dbc, jobRunID, logger)

			if err != nil {

				// RecordNotFound can be expected if the latest job isn't in sippy yet
				// so don't log an error
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					logger.WithError(err).Errorf("Error fetching job run for: %s", latestPath)
				}

				// in all failure cases fall back to gcs
				riskSummary = aw.getGCSOverallRiskLevel(latestPath)

			} else {
				riskAnalysis, err := jobQueries.JobRunRiskAnalysis(aw.dbc, jobRun, jobRunTestCount, logger.WithField("func", "JobRunRiskAnalysis"))

				if err != nil {
					logger.WithError(err).Errorf("Error querying risk analysis for: %s", latestPath)
					riskSummary = aw.getGCSOverallRiskLevel(latestPath)
				} else {
					riskSummary = api.RiskSummary{OverallRisk: api.FailureRisk{Level: riskAnalysis.OverallRisk.Level}}

					for _, t := range riskAnalysis.Tests {
						if t.Risk.Level.Level == riskSummary.OverallRisk.Level.Level {
							// test failure risk matches the current overall risk level
							// so keep it
							riskSummary.Tests = append(riskSummary.Tests, t)
						}
					}
				}
			}
		}

		// don't include none or unknown in our report
		if riskSummary.OverallRisk.Level != api.FailureRiskLevelNone && riskSummary.OverallRisk.Level != api.FailureRiskLevelUnknown {
			riskAnalysisSummary := RiskAnalysisSummary{Name: jobName, URL: prowLink, RiskLevel: riskSummary.OverallRisk.Level, TestRiskAnalysis: riskSummary.Tests}
			analysisByJobs[jobName] = riskAnalysisSummary
		}
	}

	// if we get here it means all the latest jobRuns have finished
	return true, analysisByJobs
}

func (aw *AnalysisWorker) getGCSOverallRiskLevel(latestPath string) api.RiskSummary {
	riskAnalysis, err := aw.getJobRunGCSRiskAnalysis(latestPath)
	if err != nil {
		log.WithError(err).Errorf("Error with fallback lookup of gcs RiskAnalysis for: %s", latestPath)
		return api.RiskSummary{
			OverallRisk: api.FailureRisk{Level: api.FailureRiskLevelUnknown},
		}
	}

	// it is ok for it to be nil, not everything will have risk analysis
	// in that case we do not include an entry for it
	if riskAnalysis != nil {
		riskSummary := api.RiskSummary{
			OverallRisk: api.FailureRisk{Level: riskAnalysis.OverallRisk.Level},
		}

		for _, t := range riskAnalysis.Tests {
			if t.Risk.Level.Level == riskSummary.OverallRisk.Level.Level {
				// test failure risk matches the current overall risk level
				// so keep it
				riskSummary.Tests = append(riskSummary.Tests, t)
			}
		}

		return riskSummary

	}

	// default is none if we didn't find one
	return api.RiskSummary{
		OverallRisk: api.FailureRisk{Level: api.FailureRiskLevelNone},
	}
}

func (aw *AnalysisWorker) getJobRunGCSRiskAnalysis(jobPath string) (*api.ProwJobRunRiskAnalysis, error) {
	// create a new gcs job for each entry
	// try to locate the risk analysis file
	// if we can't find it then it is unknown
	jobRun := gcs.NewGCSJobRun(aw.gcsBucket, "")
	rawData := jobRun.FindFirstFile(jobPath, aw.riskAnalysisLocator)

	ra := api.ProwJobRunRiskAnalysis{}
	if rawData != nil {
		err := json.Unmarshal(rawData, &ra)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s risk analysis: %w", jobPath, err)
		}
		return &ra, nil
	}

	return nil, nil
}
