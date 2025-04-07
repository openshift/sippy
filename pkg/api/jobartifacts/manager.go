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
	responseLoop:
		for len(remaining) > 0 { // done if we received a response for every request
			select {
			case <-jobCtx.Done(): // timed out or cancelled
				break responseLoop
			case res := <-jobRunResponseChan:
				remaining.Delete(res.jobRun) // we have seen it, it is no longer remaining
				if res.error != nil {
					result.Errors = append(result.Errors, JobRunError{
						ID:    strconv.FormatInt(res.jobRun, 10),
						Error: res.error.Error(),
					})
				} else {
					result.JobRuns = append(result.JobRuns, res.response)
				}
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
		select {
		case <-jobCtx.Done(): // cancelled, give up on sending
		case m.jobRunChan <- request:
		}
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
		select {
		case <-managerCtx.Done(): // shutting down the workers
			logger.WithError(managerCtx.Err()).Debug("Shutting down per context")
			return
		case req := <-m.jobRunChan:
			jobLog := logger.WithField("jobRunId", req.jobRun).WithContext(req.jobCtx)
			jobLog.Debug("Received request from jobRunChan")
			select {
			case <-req.jobCtx.Done(): // query is starting too late, request already cancelled
				jobLog.WithError(req.jobCtx.Err()).Warn("Aborted request for job run")
			default: // execute the query and respond
				jobRunRes, err := req.query.queryJobArtifacts(req.artifactCtx, req.jobRun, m, jobLog)
				response := jobRunResponse{
					jobRun:   req.jobRun,
					error:    err,
					response: jobRunRes,
				}
				select {
				case <-req.jobCtx.Done(): // cancelled, don't try to respond
					jobLog.WithError(req.jobCtx.Err()).Debug("jobRunResponse cancelled")
				case req.jobRunsChan <- response:
					jobLog.Debug("Wrote jobRunResponse to jobRunsChan")
				}
			}
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
	responseLoop:
		for len(remaining) > 0 { // done if we received a response for every request
			select {
			case <-ctx.Done(): // timed out or cancelled
				break responseLoop
			case res := <-artifactResponseChan:
				remaining.Delete(res.artifactPath) // we have seen it, so it is no longer remaining
				artifacts = append(artifacts, res.artifact)
			}
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
		select {
		case <-ctx.Done(): // cancelled, give up on sending
		case m.artifactChan <- request:
		}
	}
	<-finished // wait for all responses to be processed

	// fill in errors for any that went missing
	for path := range remaining {
		artifacts = append(artifacts, JobRunArtifact{
			JobRunID:    strconv.FormatInt(jobRunID, 10),
			ArtifactURL: fmt.Sprintf(artifactURLFmt, util.GcsBucketRoot, path),
			Error:       fmt.Sprintf("request did not complete within %s", artifactQueryTimeout),
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
		select {
		case <-managerCtx.Done(): // shutting down the workers
			logger.WithError(managerCtx.Err()).Debug("Shutting down per context")
			return
		case req := <-m.artifactChan:
			artLog := logger.WithField("artifactPath", req.artifactPath).WithContext(req.ctx)
			artLog.Debug("Received request from artifactChan")
			select {
			case <-req.ctx.Done(): // query is starting too late, request already cancelled
				artLog.WithError(req.ctx.Err()).Warn("Aborted request for job run artifacts")
			default: // execute the query and respond
				response := artifactResponse{
					artifactPath: req.artifactPath,
					artifact:     req.query.getFileContentMatches(req.jobRunID, req.artifactPath),
				}
				select {
				case <-req.ctx.Done(): // cancelled, don't send response
					artLog.WithError(req.ctx.Err()).Debug("artifactResponse cancelled")
				case req.artifactsChan <- response:
					artLog.Debug("Wrote artifactResponse to artifactsChan")
				}
			}
		}
	}
}
