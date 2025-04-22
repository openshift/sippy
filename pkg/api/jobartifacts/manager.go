package jobartifacts

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openshift/sippy/pkg/util"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

// these limits are rather arbitrary, can be expanded for performance until we run into GCS limitations
const jobRunWorkerPoolSize = 12   // number of concurrent processors of one job run each
const artifactWorkerPoolSize = 12 // number of concurrent processors of one artifact each
const jobRunQueryTimeout = 30 * time.Second
const artifactQueryTimeout = 28 * time.Second // give artifact scans a little time before the job timeout to return (possibly incomplete) results
// these limits are also arbitrary, can be expanded if scenarios arise that justify it.
const maxJobFilesToScan = 12 // limit the number of files inspected under each job
const maxFileMatches = 12    // limit the number of content matches returned for each file

type Manager struct {
	ctxCancelFunc   context.CancelFunc
	jobRunChan      chan jobRunRequest
	jobRunWorkers   *sync.WaitGroup
	artifactChan    chan artifactRequest
	artifactWorkers *sync.WaitGroup
}

// NewManager creates a new Manager with a pool of workers to process job run artifact queries
func NewManager(ctx context.Context) *Manager {
	ctx, cancelFunc := context.WithCancel(ctx)
	manager := &Manager{
		ctxCancelFunc:   cancelFunc,
		jobRunChan:      make(chan jobRunRequest),
		jobRunWorkers:   &sync.WaitGroup{},
		artifactChan:    make(chan artifactRequest),
		artifactWorkers: &sync.WaitGroup{},
	}
	for i := jobRunWorkerPoolSize; i > 0; i-- {
		manager.jobRunWorkers.Add(1)
		go manager.jobRunWorker(ctx)
	}
	for i := artifactWorkerPoolSize; i > 0; i-- {
		manager.artifactWorkers.Add(1)
		go manager.artifactWorker(ctx)
	}
	return manager
}

// Close is the manager's way of closing up show and releasing all its workers
func (m *Manager) Close() {
	log.Info("Shutting down jobartifacts workers")
	m.ctxCancelFunc()
	m.artifactWorkers.Wait()
	m.jobRunWorkers.Wait()
	log.Info("Shutdown complete for jobartifacts workers")
}

// Query executes a top-level job artifacts query with concurrency and timeouts/cancellation
func (m *Manager) Query(ctx context.Context, query *JobArtifactQuery) (result QueryResponse) {
	artifactCtx, cancelArtifact := context.WithTimeout(ctx, artifactQueryTimeout)
	defer cancelArtifact()
	jobCtx, cancelJob := context.WithTimeout(ctx, jobRunQueryTimeout)
	defer cancelJob()

	// set up the request/response workflow
	jobRunResponseChan := make(chan jobRunResponse) // for responses from the workers
	remaining := sets.NewInt64(query.JobRunIDs...)  // keep track of responses still missing
	finished := make(chan bool)                     // indicate we will not receive any more responses

	// start a listener to wait for the responses and collect them; important to prepare
	// the listener before dumping requests to the request channel, since that will block.
	go func() {
		for len(remaining) > 0 { // done if we received a response for every request
			response, expired := receiveFromChannel(jobCtx, jobRunResponseChan)
			if expired != nil {
				break
			}
			remaining.Delete(response.jobRun) // we have seen it, it is no longer remaining
			if response.error != nil {
				result.Errors = append(result.Errors, JobRunError{
					ID:    strconv.FormatInt(response.jobRun, 10),
					Error: response.error.Error(),
				})
			} else {
				result.JobRuns = append(result.JobRuns, response.response)
			}
		}
		finished <- true
	}()

	// send a request per unique job run id, along with our response channel
	for id := range sets.NewInt64(query.JobRunIDs...) {
		request := jobRunRequest{
			jobRun:      id,
			query:       query,
			jobRunsChan: jobRunResponseChan,
			jobCtx:      jobCtx,
			artifactCtx: artifactCtx,
		}
		_ = sendViaChannel(jobCtx, m.jobRunChan, request) // if not sent, will be in remaining
	}
	<-finished // wait for all responses to be processed

	// fill in errors for any that went missing
	for it := range remaining {
		result.Errors = append(result.Errors, JobRunError{
			ID:    strconv.FormatInt(it, 10),
			Error: fmt.Sprintf("request did not complete within %s", jobRunQueryTimeout),
		})
	}

	// sort things for consistency
	slices.SortFunc(result.JobRuns, func(a, b JobRun) int { // sort runs by ID ascending
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortFunc(result.Errors, func(a, b JobRunError) int { // sort errors by ID ascending
		return strings.Compare(a.ID, b.ID)
	})
	return
}

// a jobRunRequest channels a single job run to be processed and returned with matching file paths
type jobRunRequest struct {
	jobRun      int64
	query       *JobArtifactQuery
	jobRunsChan chan jobRunResponse // when processing is done, send response to this channel
	// putting a context in a struct is normally discouraged; but think of this entire struct as just parameters for a function call
	// and then it is clear the usage is in the same spirit of passing a context to a function (just via a channel).
	jobCtx      context.Context // enable cancellation/timeout of query
	artifactCtx context.Context // earlier timeout for artifact queries so they tend to finish before the job query deadline
}

// jobRunReponse channels the response for a single job run that was processed
type jobRunResponse struct {
	jobRun   int64
	response JobRun
	error    error
}

func (m *Manager) jobRunWorker(managerCtx context.Context) {
	defer m.jobRunWorkers.Done()
	logger := log.WithField("func", "Manager.jobRunWorker")
	for {
		request, expired := receiveFromChannel(managerCtx, m.jobRunChan)
		if expired != nil {
			logger.WithError(expired).Debug("Shutting down worker per manager context")
			return
		}

		jobLog := logger.WithField("jobRunId", request.jobRun).WithContext(request.jobCtx)
		jobLog.Debug("Received request from jobRunChan")
		jobRunRes, err := request.query.queryJobArtifacts(request.artifactCtx, request.jobRun, m, jobLog)
		response := jobRunResponse{
			jobRun:   request.jobRun,
			error:    err,
			response: jobRunRes,
		}

		expired = sendViaChannel(request.jobCtx, request.jobRunsChan, response)
		if expired != nil {
			jobLog.WithError(expired).Debug("jobRunResponse cancelled")
		} else {
			jobLog.Debug("Wrote jobRunResponse to jobRunsChan")
		}
	}
}

// QueryJobRunArtifacts scans the content of all matched artifacts for one job, with concurrency and timeouts/cancellation
func (m *Manager) QueryJobRunArtifacts(ctx context.Context, query *JobArtifactQuery, jobRunID int64, paths []string) (artifacts []JobRunArtifact) {
	// set up the request/response workflow
	artifactResponseChan := make(chan artifactResponse) // for responses from the workers
	remaining := sets.NewString(paths...)               // keep track of responses still missing
	finished := make(chan bool)                         // indicate we will not receive any more responses

	// start a listener to wait for the responses and collect them; important to prepare
	// the listener before dumping requests to the request channel, since that will block.
	go func() {
		for len(remaining) > 0 { // done if we received a response for every request
			response, expired := receiveFromChannel(ctx, artifactResponseChan)
			if expired != nil {
				break // timed out or cancelled
			}
			remaining.Delete(response.artifactPath) // we have seen it, so it is no longer remaining
			artifacts = append(artifacts, response.artifact)
		}
		finished <- true
	}()

	// send a request per unique path, along with our response channel
	for path := range sets.NewString(paths...) {
		request := artifactRequest{
			artifactPath:  path,
			query:         query,
			artifactsChan: artifactResponseChan,
			ctx:           ctx,
			jobRunID:      jobRunID,
		}
		_ = sendViaChannel(ctx, m.artifactChan, request) // if not sent, will be in remaining
	}
	<-finished // wait for all responses to be processed

	// fill in errors for any that went missing
	for path := range remaining {
		artifacts = append(artifacts, JobRunArtifact{
			JobRunID:     strconv.FormatInt(jobRunID, 10),
			ArtifactPath: relativeArtifactPath(path, strconv.FormatInt(jobRunID, 10)),
			ArtifactURL:  fmt.Sprintf(artifactURLFmt, util.GcsBucketRoot, path),
			Error:        fmt.Sprintf("request did not complete within %s", artifactQueryTimeout),
		})
	}

	// sort artifacts by path ascending
	slices.SortFunc(artifacts, func(a, b JobRunArtifact) int {
		return strings.Compare(a.ArtifactURL, b.ArtifactURL)
	})
	return
}

// artifactRequest channels a single artifact to be processed and returned with matching contents
type artifactRequest struct {
	artifactPath  string
	query         *JobArtifactQuery
	artifactsChan chan artifactResponse // when processing is done, send response to this channel
	// putting a context in a struct is normally discouraged; but think of this entire struct as just parameters for a function call
	// and then it is clear the usage is in the same spirit of passing a context to a function (just via a channel).
	ctx      context.Context // enable cancellation/timeout of query
	jobRunID int64
}

// artifactReponse channels the response for a single artifact that was processed
type artifactResponse struct {
	artifactPath string
	artifact     JobRunArtifact
}

func (m *Manager) artifactWorker(managerCtx context.Context) {
	defer m.artifactWorkers.Done()
	logger := log.WithField("func", "Manager.artifactWorker")
	for {
		request, expired := receiveFromChannel(managerCtx, m.artifactChan)
		if expired != nil {
			logger.WithError(expired).Debug("Shutting down worker per manager context")
			return
		}

		artLog := logger.WithField("artifactPath", request.artifactPath).WithContext(request.ctx)
		artLog.Debug("Received request from artifactChan")
		response := artifactResponse{
			artifactPath: request.artifactPath,
			artifact:     request.query.getFileContentMatches(request.jobRunID, request.artifactPath),
		}

		expired = sendViaChannel(request.ctx, request.artifactsChan, response)
		if expired != nil {
			artLog.WithError(expired).Debug("artifactResponse cancelled")
		} else {
			artLog.Debug("Wrote artifactResponse to artifactsChan")
		}
	}
}

// sendViaChannel sends a payload to a channel, subject to the given context not expiring
func sendViaChannel[P interface{}, C chan P](ctx context.Context, channel C, payload P) error {
	select {
	case <-ctx.Done(): // expired, don't try to send payload
		return ctx.Err()
	case channel <- payload:
		return nil
	}
}

// receiveFromChannel receives a payload from a channel, subject to the given context not expiring
func receiveFromChannel[P interface{}, C chan P](ctx context.Context, channel C) (payload P, err error) {
	select {
	case <-ctx.Done(): // expired, give up on receiving payload
		err = ctx.Err()
	case payload = <-channel:
	}
	return
}
