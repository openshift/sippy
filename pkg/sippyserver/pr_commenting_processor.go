package sippyserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"slices"
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
	"github.com/openshift/sippy/pkg/dataloader/prowloader/gcs"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/github/commenter"
	"github.com/openshift/sippy/pkg/util"
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

	checkCommentReadyMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sippy_repo_check_ready_comment",
		Help:    "Tracks the call made to verify a pr is ready for a pr comment",
		Buckets: prometheus.LinearBuckets(0, 500, 10),
	}, []string{"org", "repo"})

	buildPendingWorkMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sippy_repo_build_pending_comment_work",
		Help:    "Tracks the call made to query db and add items to the pending work queue",
		Buckets: prometheus.LinearBuckets(0, 500, 10),
	}, []string{"type"})
)

// NewWorkProcessor creates a standard work processor from parameters.
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
		newTestsWorker:         StandardNewTestsWorker(dbc),
	}
	return wp
}

// WorkProcessor coordinates the initialization, connection, and execution of all the workers that go into generating PR comments
type WorkProcessor struct {
	commentUpdaterRate     time.Duration
	commentAnalysisRate    time.Duration
	commentAnalysisWorkers int
	dbc                    *db.DB
	gcsBucket              *storage.BucketHandle
	ghCommenter            *commenter.GitHubCommenter
	dryRunOnly             bool
	newTestsWorker         *NewTestsWorker
}

// PreparedComment is a comment that is ready to be posted on a github PR
type PreparedComment struct {
	comment     string
	commentType int
	org         string
	repo        string
	number      int
	sha         string
}

type CommentWorker struct {
	commentUpdaterRateLimiter util.RateLimiter
	preparedComments          chan PreparedComment
	ghCommenter               *commenter.GitHubCommenter
	dryRunOnly                bool
}

type AnalysisWorker struct {
	dbc                 *db.DB
	gcsBucket           *storage.BucketHandle
	riskAnalysisLocator *regexp.Regexp
	prCommentProspects  chan models.PullRequestComment
	preparedComments    chan PreparedComment
	newTestsWorker      *NewTestsWorker
}

type RiskAnalysisSummary struct {
	Name             string
	URL              string
	RiskLevel        api.RiskLevel
	OverallReasons   []string
	TestRiskAnalysis []api.TestRiskAnalysis
}

func SortByJobNameRA(ras []RiskAnalysisSummary) {
	slices.SortFunc(ras, func(a, b RiskAnalysisSummary) int {
		return strings.Compare(a.Name, b.Name)
	})
}

func (wp *WorkProcessor) Run(ctx context.Context) {

	// create a channel with a max buffer of 5 for github updates
	// single thread will pull updates from that channel and process them
	// one at a time as the rate limiter allows
	preparedComments := make(chan PreparedComment, 5)
	defer close(preparedComments)

	commentWorker := CommentWorker{
		commentUpdaterRateLimiter: util.NewRateLimiter(wp.commentUpdaterRate),
		preparedComments:          preparedComments,
		ghCommenter:               wp.ghCommenter,

		// want an explicit setting to enable commenting
		// so that we don't make comments running locally
		// or in a test deployment unless configured to do so
		dryRunOnly: wp.dryRunOnly,
	}

	if commentWorker.dryRunOnly {
		log.Warning("Github Comment Worker started in dry run only mode, active commenting is disabled")
	}
	go commentWorker.Run()

	// the work thread below will put items on prospects channel,
	// blocking if the buffer is full.
	// commentAnalysisWorker threads will pull jobs and process them,
	// putting any comments on the preparedComments channel,
	// blocking if the buffer is full
	prospects := make(chan models.PullRequestComment, wp.commentAnalysisWorkers)
	defer close(prospects)

	for i := 0; i < wp.commentAnalysisWorkers; i++ {
		analysisWorker := AnalysisWorker{
			riskAnalysisLocator: gcs.GetDefaultRiskAnalysisSummaryFile(),
			dbc:                 wp.dbc,
			gcsBucket:           wp.gcsBucket,
			prCommentProspects:  prospects,
			preparedComments:    preparedComments,
			newTestsWorker:      wp.newTestsWorker,
		}
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
			if len(prospects) == 0 && len(preparedComments) == 0 {
				err := wp.work(ctx, prospects)

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

func (wp *WorkProcessor) work(ctx context.Context, prospects chan models.PullRequestComment) error {

	start := time.Now()
	defer func() {
		end := time.Now()
		buildPendingWorkMetric.WithLabelValues("work-processor").Observe(float64(end.UnixMilli() - start.UnixMilli()))
	}()

	log.Debug("Checking for work")

	// get a list of commentProspects
	// process each item one at a time while checking for shutdown

	commentProspects, err := wp.fetchPrCommentProspects()

	if err != nil {
		log.WithError(err).Error("Failed to query PR comments")

		// we want to keep our processor loop active so don't pass the error back up
		return nil
	}

	for _, cp := range commentProspects {

		select {

		case <-ctx.Done():

			log.Info("Context is done, stopping processor")
			return errors.New("context closed")

		default:
			log.Debugf("Adding PR comment prospect: %s/%s/%d/%s", cp.Org, cp.Repo, cp.PullNumber, cp.SHA)
			prospects <- cp
			log.Debugf("Finished adding PR comment prospect: %s/%s/%d/%s", cp.Org, cp.Repo, cp.PullNumber, cp.SHA)
		}
	}

	log.Debug("Finished Checking for work")
	return nil
}

func (wp *WorkProcessor) fetchPrCommentProspects() ([]models.PullRequestComment, error) {
	return wp.ghCommenter.QueryForPotentialComments(models.CommentTypeRiskAnalysis)
}

func (cw *CommentWorker) Run() {
	defer cw.commentUpdaterRateLimiter.Close()

	var errCount float64
	for pc := range cw.preparedComments {

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

func (cw *CommentWorker) writeComment(ghCommenter *commenter.GitHubCommenter, preparedComment PreparedComment) error {

	start := time.Now()
	defer func() {
		end := time.Now()
		writeCommentMetric.WithLabelValues(preparedComment.org, preparedComment.repo).Observe(float64(end.UnixMilli() - start.UnixMilli()))
	}()

	// if there is no comment then just delete the record
	if preparedComment.comment == "" {
		return nil
	}

	// could be that the include / exclude lists were updated
	// after the pending record was written
	// double check before we interact with github
	// the record should still get deleted
	if !ghCommenter.IsRepoIncluded(preparedComment.org, preparedComment.repo) {
		return nil
	}

	logger := log.WithField("org", preparedComment.org).
		WithField("repo", preparedComment.repo).
		WithField("number", preparedComment.number)

	// are we still the latest sha?
	prEntry, err := ghCommenter.GetCurrentState(preparedComment.org, preparedComment.repo, preparedComment.number)

	if err != nil {
		logger.WithError(err).Error("Failed to get the current PR state")
		return err
	}

	if prEntry == nil || prEntry.SHA == "" {
		logger.Error("Invalid sha when validating prEntry")
		return nil
	}

	if preparedComment.commentType == int(models.CommentTypeRiskAnalysis) && prEntry.MergedAt != nil {
		logger.Warning("PR has merged, skipping risk analysis comment")
		return nil
	}

	if prEntry.SHA != preparedComment.sha {

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

	// create a constant for the key
	// determine the commentType and build the id off of that and the sha
	// generate the comment

	commentID := ghCommenter.CreateCommentID(models.CommentType(preparedComment.commentType), preparedComment.sha)

	// when running in dryRunOnly mode we do everything up until adding or deleting anything in GitHub
	// this allows for local testing / debugging without actually modifying PRs
	// it is the default setting and needs to be overridden in production / live commenting instances
	if cw.dryRunOnly {
		logger.Infof("Dry run comment for: %s\n%s", commentID, preparedComment.comment)
		return nil
	}

	ghcomment := fmt.Sprintf("<!-- META={\"%s\": \"%s\"} -->\n\n%s", commenter.TrtCommentIDKey, commentID, preparedComment.comment)

	// is there an existing comment of our type that we should remove
	existingCommentID, commentBody, err := ghCommenter.FindExistingCommentID(preparedComment.org, preparedComment.repo, preparedComment.number, commenter.TrtCommentIDKey, commentID)

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
		err = ghCommenter.DeleteComment(preparedComment.org, preparedComment.repo, *existingCommentID)
		// if we had an error then return it, the record will remain, and we will attempt processing again later
		if err != nil {
			return err
		}
	}

	logger.Infof("Adding comment id: %s", commentID)
	return ghCommenter.AddComment(preparedComment.org, preparedComment.repo, preparedComment.number, ghcomment)
}

func (aw *AnalysisWorker) Run() {

	// wait for the next item to be available and process it
	// exit when closed
	for i := range aw.prCommentProspects {

		if i.CommentType == int(models.CommentTypeRiskAnalysis) {
			aw.determinePrComment(i)
		} else {
			log.Warningf("Unsupported comment type: %d for %s/%s/%d/%s", i.CommentType, i.Org, i.Repo, i.PullNumber, i.SHA)
		}

	}
}

// determinePrComment evaluates the potential for a PR comment and produces that comment if appropriate
func (aw *AnalysisWorker) determinePrComment(prCommentProspect models.PullRequestComment) {

	logger := log.WithField("func", "determinePrComment").
		WithField("org", prCommentProspect.Org).
		WithField("repo", prCommentProspect.Repo).
		WithField("pull", prCommentProspect.PullNumber).
		WithField("sha", prCommentProspect.SHA)

	start := time.Now()
	logger.Debug("Processing item")

	// we will likely pull in PRs hours before the jobs finish,
	// so there may be many cycles before a PR is ready for commenting.
	// check to see if all jobs have completed before doing the more intensive query / gcs locate for analysis.
	completedJobs := aw.getPrJobsIfFinished(logger, prCommentProspect.ProwJobRoot)
	if completedJobs == nil {
		logger.Debug("Jobs are still active")

		// record how long it took to decide the jobs are still active
		t := float64(time.Now().UnixMilli() - start.UnixMilli())
		checkCommentReadyMetric.WithLabelValues(prCommentProspect.Org, prCommentProspect.Repo).Observe(t)
		return
	}

	defer func() {
		// record how long it took to determine what the comment should be
		t := float64(time.Now().UnixMilli() - start.UnixMilli())
		buildCommentMetric.WithLabelValues(prCommentProspect.Org, prCommentProspect.Repo).Observe(t)
	}()

	// having determined the PR is ready, scan all the runs for each job so we can find the latest
	for idx, jobInfo := range completedJobs {
		completedJobs[idx].prowJobRuns = aw.buildProwJobRuns(logger, jobInfo.bucketPrefix)
		completedJobs[idx].prShaSum = prCommentProspect.SHA // so we can check whether runs are against the expected PR commit
	}

	riskAnalyses := aw.buildPRJobRiskAnalysis(logger, completedJobs)
	newTestRisks := aw.newTestsWorker.analyzeRisks(logger, completedJobs)
	preparedComment := PreparedComment{
		comment:     buildCommentText(riskAnalyses, newTestRisks, prCommentProspect.SHA),
		sha:         prCommentProspect.SHA,
		org:         prCommentProspect.Org,
		repo:        prCommentProspect.Repo,
		number:      prCommentProspect.PullNumber,
		commentType: prCommentProspect.CommentType,
	}

	// will block if the buffer is full.
	// also if the comment processor sees an empty comment,
	// it will not create a comment but will delete the PR comment record in the db
	log.Debugf("Adding comment to preparedComments: %s/%s/%s", preparedComment.org, preparedComment.repo, preparedComment.sha)
	aw.preparedComments <- preparedComment
	log.Debugf("Comment added to preparedComments: %s/%s/%s", preparedComment.org, preparedComment.repo, preparedComment.sha)
}

func buildCommentText(riskAnalyses []RiskAnalysisSummary, newTestRisks []*JobNewTestRisks, sha string) string {
	sb := &strings.Builder{}
	if len(riskAnalyses) == 0 && len(newTestRisks) == 0 {
		return ""
	}
	if len(riskAnalyses) > 0 {
		buildRiskAnalysisComment(sb, riskAnalyses, sha)
		sb.WriteString("\n\n")
	}
	if len(newTestRisks) > 0 {
		buildNewTestRisksComment(sb, newTestRisks, sha)
	}

	return sb.String()
}

func buildNewTestRisksComment(sb *strings.Builder, jobRisks []*JobNewTestRisks, sha string) {
	notableJobRisks, testSummaries := summarizeNewTestRisks(jobRisks)
	if len(notableJobRisks) > 0 || len(testSummaries) > 0 {
		sb.WriteString("Risk analysis has seen new tests most likely introduced by this PR.\n")
		sb.WriteString("Please ensure that new tests meet [guidelines for naming and stability](https://github.com/openshift-eng/ci-test-mapping/?tab=readme-ov-file#test-sources).\n\n")
	}

	if len(notableJobRisks) > 0 {
		SortByJobNameNT(notableJobRisks)
		sb.WriteString(fmt.Sprintf("New Test Risks for sha: %s\n\n", sha))
		sb.WriteString("| Job Name | New Test Risk |\n|:---|:---|\n")
		for _, jr := range notableJobRisks {
			for _, risk := range sortedTestRisks(jr.NewTestRisks) {
				sb.WriteString(fmt.Sprintf("|%s|**%s** - *%q* **%s**|\n",
					jr.JobName, risk.Level.Name, risk.TestName, risk.Reason))
			}
		}
		sb.WriteString("\n")
	}

	if len(testSummaries) > 0 {
		sb.WriteString(fmt.Sprintf("New tests seen in this PR at sha: %s\n\n", sha))
		for _, test := range testSummaries {
			sb.WriteString(fmt.Sprintf("- *%q* [Total: %d, Pass: %d, Fail: %d, Flake: %d]\n",
				test.TestName, test.Runs, test.Runs-test.Failures, test.Failures, test.Flakes))
		}
		sb.WriteString("\n")
	}
}

func buildRiskAnalysisComment(sb *strings.Builder, riskAnalyses []RiskAnalysisSummary, sha string) {
	SortByJobNameRA(riskAnalyses)
	sb.WriteString(fmt.Sprintf("Job Failure Risk Analysis for sha: %s\n\n", sha))
	sb.WriteString("| Job Name | Failure Risk |\n|:---|:---|\n")

	// don't want the comment to be too large so if we have a high number of jobs to analyze
	// reduce the max tests / reasons we show
	maxSubRows := 3
	if len(riskAnalyses) > 10 {
		maxSubRows = 1
	}

	for idx, analysis := range riskAnalyses {
		if idx > 19 {
			sb.WriteString(fmt.Sprintf("\nShowing %d of %d jobs analysis", idx, len(riskAnalyses)))
			break // top 20 should be more than enough
		}

		tableKey := analysis.Name
		if analysis.URL != "" {
			tableKey = fmt.Sprintf("[%s](%s)", analysis.Name, analysis.URL)
		}

		var riskSb strings.Builder
		riskSb.WriteString(fmt.Sprintf("**%s**", analysis.RiskLevel.Name))

		// if we don't have any TestRiskAnalysis use the OverallReasons
		if len(analysis.TestRiskAnalysis) == 0 {
			for j, r := range analysis.OverallReasons {
				if j > maxSubRows {
					riskSb.WriteString(fmt.Sprintf("<br>Showing %d of %d test risk reasons", j, len(analysis.OverallReasons)))
					break
				}
				riskSb.WriteString(fmt.Sprintf("<br>%s", r))
			}
		} else {

			for i, t := range analysis.TestRiskAnalysis {
				if i > maxSubRows {
					riskSb.WriteString(fmt.Sprintf("<br>---<br>Showing %d of %d test results", i, len(analysis.TestRiskAnalysis)))
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
					// prevent the openshift-ci bot from detecting JIRA references in the link by replacing - with html escaped sequence
					riskSb.WriteString(fmt.Sprintf("<br>[%s](%s)", strings.ReplaceAll(html.EscapeString(b.Summary), "-", "&#45;"), b.URL))
				}
			}
		}
		sb.WriteString(fmt.Sprintf("|%s|%s|\n", tableKey, riskSb.String()))
	}
}

// prJobInfo is an internal record built to represent a job running at least once against a commit on a PR
type prJobInfo struct {
	name          string
	jobID         string          // sippy ID of the job itself
	prShaSum      string          // sha of the PR at the time it was loaded
	bucketPrefix  string          // where the job is found in the GCS bucket
	latestRunID   string          // sippy ID of the latest run
	latestRunPath string          // path to the latest run in the GCS bucket
	prowJobRuns   []*prow.ProwJob // sorted list of ProwJobs (runs for this job)
}

// getPrJobsIfFinished walks the GCS path for this PR to find the most recent run of each PR job;
// returns nil if any have not finished. otherwise, returns a list of jobs found.
// if the map is empty, it indicates that either all tests passed or any analysis for failures was unknown.
func (aw *AnalysisWorker) getPrJobsIfFinished(logger *log.Entry, prRoot string) (jobs []prJobInfo) {
	logger = logger.WithField("prRoot", prRoot).WithField("func", "getPrJobsIfFinished")

	// get the list of objects one level down from our root. the hierarchy looks like pr-logs/pull/org_repo/number/
	it := aw.gcsBucket.Objects(context.Background(), &storage.Query{
		Prefix:    prRoot,
		Delimiter: "/",
	})

	gcsJobRun := gcs.NewGCSJobRun(aw.gcsBucket, "") // dummy instance for looking up bucket content

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			logger.WithError(err).Warningf("gcs bucket iterator failed looking for PR jobs")
			continue
		}
		if attrs == nil || len(attrs.Name) > 0 {
			continue // not a folder, skip
		}

		/* path is e.g. > pr-logs/pull/org_repo/1555/pull-ci-openshift-origin-master-e2e-aws-csi/
		                     .  latest-build.txt
		                     >  1234567890123456789/  (job run ID)
							    .  started.json
							    .  finished.json
							    .  build-log.txt
		*/
		jobPath := strings.Split(attrs.Prefix, "/")
		job := prJobInfo{
			bucketPrefix: attrs.Prefix,
			name:         jobPath[len(jobPath)-2], // record last segment as name; final split is "" so get penultimate
		}

		// we have to get the job run id from latest-build.txt and then check for finished.json in that path
		bytes, err := gcsJobRun.GetContent(context.TODO(), fmt.Sprintf("%s%s", job.bucketPrefix, "latest-build.txt"))
		if err != nil {
			logger.WithError(err).Errorf("Failed to get latest build info for: %s", job.bucketPrefix)
			return nil // latest job result not recorded, so consider testing incomplete for the PR
		}

		latest := string(bytes)
		latestPath := fmt.Sprintf("%s%s/", attrs.Prefix, latest)
		finishedJSON := fmt.Sprintf("%sfinished.json", latestPath)

		// currently we only validate that the file exists, we aren't pulling anything out of it
		if !gcsJobRun.ContentExists(context.TODO(), finishedJSON) {
			return nil // testing not yet complete for the PR
		}

		jobs = append(jobs, job) // officially a finished job if the latest run has finished
	}
	return
}

// buildPRJobRiskAnalysis walks the runs for a PR job to sort out which to analyze;
// if the map is empty, it indicates that either all tests passed or any analysis for failures was unknown.
func (aw *AnalysisWorker) buildPRJobRiskAnalysis(logger *log.Entry, jobs []prJobInfo) []RiskAnalysisSummary {
	logger = logger.WithField("func", "buildPRJobRiskAnalysis")
	riskAnalysisSummaries := make([]RiskAnalysisSummary, 0)
	for _, jobInfo := range jobs {
		// we don't report risk on jobs without 2 or more runs.
		// this is so we can compare failed tests against latest and latest-1,
		// only returning analysis on tests that have failed in both.
		if len(jobInfo.prowJobRuns) < 2 {
			continue
		}

		latest := jobInfo.prowJobRuns[0]
		previous := jobInfo.prowJobRuns[1]

		if latest.Spec.Refs.Pulls[0].SHA != jobInfo.prShaSum {
			logger.Infof(
				"Skipping risk analysis for job %s as latest completed run %s is not against the PR's shasum %s",
				jobInfo.name, latest.Status.BuildID, jobInfo.prShaSum)
			continue
		}

		_, priorRiskAnalysis := aw.getRiskSummary(
			previous.Status.BuildID,
			fmt.Sprintf("%s%s/", jobInfo.bucketPrefix, previous.Status.BuildID),
			nil,
		)

		// if the priorRiskAnalysis is nil then skip since we require consecutive test failures;
		// this can happen if the job hasn't been imported yet and its risk analysis artifact failed to be created in gcs.
		if priorRiskAnalysis == nil {
			logger.Warnf("Failed to determine prior risk analysis for prowjob: %s", previous.Status.BuildID)
			continue
		}

		riskSummary, _ := aw.getRiskSummary(
			latest.Status.BuildID,
			fmt.Sprintf("%s%s/", jobInfo.bucketPrefix, latest.Status.BuildID),
			priorRiskAnalysis,
		)

		// report any risk worth mentioning for this job
		if riskSummary.OverallRisk.Level != api.FailureRiskLevelNone && riskSummary.OverallRisk.Level != api.FailureRiskLevelUnknown {
			riskAnalysisSummaries = append(riskAnalysisSummaries, RiskAnalysisSummary{
				Name:             jobInfo.name,
				URL:              latest.Status.URL,
				RiskLevel:        riskSummary.OverallRisk.Level,
				OverallReasons:   riskSummary.OverallRisk.Reasons,
				TestRiskAnalysis: riskSummary.Tests,
			})
		}
	}

	return riskAnalysisSummaries
}

// buildProwJobRuns Walks the GCS path for this job to find its job runs,
// returning a list of completed runs sorted by decreasing completion time
func (aw *AnalysisWorker) buildProwJobRuns(logger *log.Entry, prJobRoot string) []*prow.ProwJob {
	logger = logger.WithField("func", "buildProwJobRuns").WithField("jobRoot", prJobRoot)
	// get the list of objects one level down from our root
	it := aw.gcsBucket.Objects(context.Background(), &storage.Query{
		Prefix:    prJobRoot,
		Delimiter: "/",
	})

	jobRuns := make([]*prow.ProwJob, 0)
	lookup := gcs.NewGCSJobRun(aw.gcsBucket, "") // dummy instance to look up bucket content

	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break // no more paths to look at
		}
		if len(attrs.Name) > 0 {
			continue // look for empty Name which indicates a folder which should be a job run
		}

		// load the prowjob recorded in this run
		var pj prow.ProwJob
		if bytes, err := lookup.GetContent(context.TODO(), fmt.Sprintf("%s%s", attrs.Prefix, "prowjob.json")); err != nil {
			logger.WithError(err).Errorf("Failed to get prowjob for: %s", attrs.Prefix)
			continue
		} else if err := json.Unmarshal(bytes, &pj); err != nil {
			logger.WithError(err).Errorf("Failed to unmarshall prowjob for: %s", attrs.Prefix)
			continue
		}

		// validate that this job is valid for our purposes. we checked that the job had a complete latest run,
		// but the jobs before that could be in invalid states, and prow could have started a new run in the meantime,
		// so guard against some of these edge cases.
		if pj.Status.CompletionTime == nil {
			logger.Debugf("ignoring prowjob %s with no completion time", pj.Status.BuildID)
			continue
		} else if pj.Status.State != prow.SuccessState && pj.Status.State != prow.FailureState {
			logger.Debugf("ignoring prowjob %s in state %s", pj.Status.BuildID, pj.Status.State)
			continue // filter out jobs that were aborted, failed to launch, just started, etc
		} else if !strings.HasSuffix(attrs.Prefix, pj.Status.BuildID+"/") {
			logger.Warnf("saw prowjob in folder %s with mismatched BuildId %s", attrs.Prefix, pj.Status.BuildID)
			continue // the build id should match the folder name, else WTF?
		}

		jobRuns = append(jobRuns, &pj)
	}

	slices.SortFunc(jobRuns, func(a, b *prow.ProwJob) int {
		return b.Status.CompletionTime.Compare(*a.Status.CompletionTime)
	})
	return jobRuns
}

func (aw *AnalysisWorker) getRiskSummary(jobRunID, jobRunIDPath string, priorRiskAnalysis *api.ProwJobRunRiskAnalysis) (api.RiskSummary, *api.ProwJobRunRiskAnalysis) {
	logger := log.WithField("jobRunID", jobRunID).WithField("func", "getRiskSummary")
	logger.Infof("Summarize risks for job run at %s", jobRunIDPath)

	if jobRunIntID, err := strconv.ParseInt(jobRunID, 10, 64); err != nil {
		log.WithError(err).Errorf("Failed to parse jobRunId id: %s for: %s", jobRunID, jobRunIDPath)
	} else if jobRun, err := jobQueries.FetchJobRun(aw.dbc, jobRunIntID, false, logger); err != nil {
		// RecordNotFound can be expected if the jobRunId job isn't in sippy yet. log any other error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.WithError(err).Errorf("Error fetching job run for: %s", jobRunIDPath)
		}
	} else if ra, err := jobQueries.JobRunRiskAnalysis(aw.dbc, jobRun, logger); err != nil {
		logger.WithError(err).Errorf("Error querying risk analysis for: %s", jobRunIDPath)
	} else {
		// query succeeded so use the riskAnalysis we got
		return buildRiskSummary(&ra, priorRiskAnalysis), &ra
	}

	// in all failure cases fall back to looking directly at gcs content
	return aw.getGCSOverallRiskLevel(jobRunIDPath)
}

func buildRiskSummary(riskAnalysis, priorRiskAnalysis *api.ProwJobRunRiskAnalysis) api.RiskSummary {

	riskSummary := api.RiskSummary{OverallRisk: api.JobFailureRisk{Level: riskAnalysis.OverallRisk.Level, Reasons: riskAnalysis.OverallRisk.Reasons}}

	for _, t := range riskAnalysis.Tests {
		if t.Risk.Level.Level == riskSummary.OverallRisk.Level.Level && !isTestFiltered(t, priorRiskAnalysis) {
			// test failure risk matches the current overall risk level
			// so keep it
			riskSummary.Tests = append(riskSummary.Tests, t)
		}
	}

	if len(riskSummary.Tests) == 0 {
		// if we don't have any tests then only MissingData or IncompleteTests are valid for this scenario
		// otherwise we filtered them all out so set the risk level to none

		// If this is one of the levels that doesn't have tests associated and it matches the prior risk analysis then return the summary
		if riskSummary.OverallRisk.Level == api.FailureRiskLevelIncompleteTests || riskSummary.OverallRisk.Level == api.FailureRiskLevelMissingData {
			if priorRiskAnalysis != nil && riskSummary.OverallRisk.Level == priorRiskAnalysis.OverallRisk.Level {
				return riskSummary
			}
		}

		// otherwise none
		return api.RiskSummary{
			OverallRisk: api.JobFailureRisk{Level: api.FailureRiskLevelNone},
		}
	}

	return riskSummary
}

func isTestFiltered(test api.TestRiskAnalysis, priorRiskAnalysis *api.ProwJobRunRiskAnalysis) bool {
	// TODO: Observe how restrictive this is
	// Many PRs don't appear to have multiple runs
	// Those that do don't have the same failures (because the failures are flakes and not regressions?)
	// When we have searchable results for Mechanical Deads could we check to see
	// if the test is a known regresion and filter it that way?
	if priorRiskAnalysis != nil {
		for _, t := range priorRiskAnalysis.Tests {
			if t.Name == test.Name {
				return false
			}
		}
		return true
	}
	// if we don't have a prior risk analysis nothing is filtered at this level
	// we should get the full results for the first summary
	return false
}

func (aw *AnalysisWorker) getGCSOverallRiskLevel(latestPath string) (api.RiskSummary, *api.ProwJobRunRiskAnalysis) {
	riskAnalysis, err := aw.getJobRunGCSRiskAnalysis(latestPath)
	if err != nil {
		log.WithError(err).Errorf("Error with fallback lookup of gcs RiskAnalysis for: %s", latestPath)
		return api.RiskSummary{
			OverallRisk: api.JobFailureRisk{Level: api.FailureRiskLevelUnknown},
		}, nil
	}

	// it is ok for it to be nil, not everything will have risk analysis
	// in that case we do not include an entry for it
	if riskAnalysis != nil {
		riskSummary := api.RiskSummary{
			OverallRisk: api.JobFailureRisk{Level: riskAnalysis.OverallRisk.Level},
		}

		for _, t := range riskAnalysis.Tests {
			if t.Risk.Level.Level == riskSummary.OverallRisk.Level.Level {
				// test failure risk matches the current overall risk level
				// so keep it
				riskSummary.Tests = append(riskSummary.Tests, t)
			}
		}

		return riskSummary, riskAnalysis

	}

	// default is none if we didn't find one
	return api.RiskSummary{
		OverallRisk: api.JobFailureRisk{Level: api.FailureRiskLevelNone},
	}, nil
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
