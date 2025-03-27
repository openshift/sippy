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

const jobRunWorkerPoolSize = 12 // number of concurrent processors of one job run each
const jobRunQueryTimeout = 30 * time.Second
const artifactWorkerPoolSize = 12 // number of concurrent processors of one artifact each

type Manager struct {
	ctxCancelFunc   context.CancelFunc
	jobRunChan      chan jobRunRequest
	jobRunWorkers   *sync.WaitGroup
	artifactChan    chan artifactRequest
	artifactWorkers *sync.WaitGroup
}

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

func (m *Manager) Close() {
	log.Info("Shutting down jobartifacts workers")
	m.ctxCancelFunc()
	m.jobRunWorkers.Wait()
	m.artifactWorkers.Wait()
	log.Info("Shutdown complete for jobartifacts workers")
}

func (m *Manager) Query(ctx context.Context, query *JobArtifactQuery) (result QueryResponse) {
	ctx, cancelTimeout := context.WithTimeout(ctx, jobRunQueryTimeout)
	defer cancelTimeout()

	// send all the requests with a return channel
	jobRunResponseChan := make(chan jobRunResponse)
	for _, id := range query.JobRunIDs {
		m.jobRunChan <- jobRunRequest{
			jobRun:      id,
			query:       query,
			jobRunsChan: jobRunResponseChan,
			ctx:         ctx,
		}
	}

	// wait for the responses
	remaining := sets.NewInt64(query.JobRunIDs...)
responseLoop:
	for {
		select {
		case <-ctx.Done(): // timed out
			break responseLoop
		case res := <-jobRunResponseChan:
			remaining.Delete(res.jobRun) // we have seen it, no longer remaining
			if res.error != nil {
				result.Errors = append(result.Errors, JobRunError{
					ID:    strconv.FormatInt(res.jobRun, 10),
					Error: res.error.Error(),
				})
			} else {
				result.JobRuns = append(result.JobRuns, res.response)
			}
			if len(remaining) == 0 {
				break responseLoop // we have received a response for every request
			}
		}
	}

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
	ctx         context.Context     // enable cancellation/timeout of query
}

// jobRunReponse channels the response for a single job run that was processed
type jobRunResponse struct {
	jobRun   int64
	response JobRun
	error    error
}

func (m *Manager) jobRunWorker(ctx context.Context) {
	defer m.jobRunWorkers.Done()
	logger := log.WithField("func", "Manager.jobRunWorker")
	for {
		select {
		case <-ctx.Done(): // shutting down the workers
			logger.Debugf("Shutting down per context: %v", ctx.Err())
			return
		case req := <-m.jobRunChan:
			jobLog := logger.WithField("jobRunId", req.jobRun).WithContext(req.ctx)
			jobLog.Debug("Received from jobRunChan")
			select {
			case <-req.ctx.Done(): // query is starting too late, request already cancelled
				jobLog.WithError(ctx.Err()).Warn("Context aborted request for job run artifacts - getting backlogged?")
				req.jobRunsChan <- jobRunResponse{
					jobRun: req.jobRun,
					error:  fmt.Errorf("job run request was not serviced soon enough: %v", req.ctx.Err()),
				}
			default: // execute the query and respond
				jobRunRes, err := req.query.queryJobArtifacts(req.ctx, req.jobRun, m, jobLog)
				req.jobRunsChan <- jobRunResponse{
					jobRun:   req.jobRun,
					error:    err,
					response: jobRunRes,
				}
				jobLog.Debug("Wrote jobRunResponse")
			}
		}
	}
}

func (m *Manager) QueryJobRunArtifacts(ctx context.Context, query *JobArtifactQuery, jobRunID int64, paths []string) (artifacts []JobRunArtifact) {
	// send all the requests with a return channel
	artifactResponseChan := make(chan artifactResponse)
	for _, path := range paths {
		m.artifactChan <- artifactRequest{
			artifactPath:  path,
			query:         query,
			artifactsChan: artifactResponseChan,
			ctx:           ctx,
			jobRunID:      jobRunID,
		}
	}

	// wait for the responses
	remaining := sets.NewString(paths...)
responseLoop:
	for {
		select {
		case <-ctx.Done(): // timed out
			break responseLoop
		case res := <-artifactResponseChan:
			remaining.Delete(res.artifactPath) // we have seen it, no longer remaining
			artifacts = append(artifacts, res.artifact)
			if len(remaining) == 0 {
				break responseLoop // we have received a response for every request
			}
		}
	}

	// fill in errors for any that went missing
	for path := range remaining {
		artifacts = append(artifacts, JobRunArtifact{
			JobRunID:    strconv.FormatInt(jobRunID, 10),
			ArtifactURL: fmt.Sprintf(artifactUrlFmt, util.GcsBucketRoot, path),
			Error:       fmt.Sprintf("request did not complete within %s", jobRunQueryTimeout),
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
	ctx           context.Context       // enable cancellation/timeout of query
	jobRunID      int64
}

// artifactReponse channels the response for a single artifact that was processed
type artifactResponse struct {
	artifactPath string
	artifact     JobRunArtifact
}

func (m *Manager) artifactWorker(ctx context.Context) {
	defer m.artifactWorkers.Done()
	logger := log.WithField("func", "Manager.artifactWorker")
	for {
		select {
		case <-ctx.Done(): // shutting down the workers
			logger.Debugf("Shutting down per context: %v", ctx.Err())
			return
		case req := <-m.artifactChan:
			artLog := logger.WithField("artifactPath", req.artifactPath).WithContext(req.ctx)
			artLog.Debug("Received from artifactChan")
			select {
			case <-req.ctx.Done(): // query is starting too late, request already cancelled
				artLog.WithError(ctx.Err()).Warn("Context aborted request for job run artifacts - getting backlogged?")
				req.artifactsChan <- artifactResponse{
					artifactPath: req.artifactPath,
					artifact: JobRunArtifact{
						JobRunID:    strconv.FormatInt(req.jobRunID, 10),
						ArtifactURL: fmt.Sprintf(artifactUrlFmt, util.GcsBucketRoot, req.artifactPath),
						Error:       fmt.Sprintf("artifact request was not serviced soon enough: %v", req.ctx.Err()),
					},
				}
			default: // execute the query and respond
				req.artifactsChan <- artifactResponse{
					artifactPath: req.artifactPath,
					artifact:     req.query.getFileContentMatches(req.jobRunID, req.artifactPath),
				}
				artLog.Debug("Wrote artifactResponse")
			}
		}
	}
}
