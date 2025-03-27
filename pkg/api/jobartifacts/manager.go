package jobartifacts

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

const jobRunWorkerPoolSize = 12 // number of concurrent processors of one job run each
const jobRunQueryTimeout = 30 * time.Second

type Manager struct {
	ctxCancelFunc context.CancelFunc
	jobRunChan    chan jobRunRequest
	jobRunWorkers *sync.WaitGroup
}

func NewManager(ctx context.Context) Manager {
	ctx, cancelFunc := context.WithCancel(ctx)
	manager := Manager{
		jobRunChan:    make(chan jobRunRequest),
		jobRunWorkers: &sync.WaitGroup{},
		ctxCancelFunc: cancelFunc,
	}
	for i := jobRunWorkerPoolSize; i > 0; i-- {
		manager.jobRunWorkers.Add(1)
		go manager.startJobRunWorker(ctx)
	}
	return manager
}

func (m *Manager) Done() {
	log.Info("Shutting down jobartifacts workers")
	m.ctxCancelFunc()
	m.jobRunWorkers.Wait()
	log.Info("Shutdown complete for jobartifacts workers")
}

func (m *Manager) Query(ctx context.Context, query *JobArtifactQuery) (result QueryResponse) {
	ctx, cancelTimeout := context.WithTimeout(ctx, jobRunQueryTimeout)
	defer cancelTimeout()
	jobRunResponseChan := make(chan jobRunResponse)
	for _, id := range query.JobRunIDs {
		m.jobRunChan <- jobRunRequest{
			jobRun:          id,
			query:           query,
			jobRunPathsChan: jobRunResponseChan,
			ctx:             ctx,
		}
	}

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
	for it := range remaining {
		result.Errors = append(result.Errors, JobRunError{
			ID:    strconv.FormatInt(it, 10),
			Error: fmt.Sprintf("request did not complete within %s", jobRunQueryTimeout),
		})
	}
	slices.SortFunc(result.JobRuns, func(a, b JobRun) int { // sort runs by ID ascending
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortFunc(result.Errors, func(a, b JobRunError) int { // sort errors by ID ascending
		return strings.Compare(a.ID, b.ID)
	})
	return
}

// jobRunReponse channels the response for a single job run that was processed
type jobRunResponse struct {
	jobRun   int64
	response JobRun
	error    error
}

// a jobRunRequest represents a single job run to be processed and returned with matching file paths
type jobRunRequest struct {
	jobRun          int64
	query           *JobArtifactQuery
	jobRunPathsChan chan jobRunResponse // when processing is done, send response to this channel
	ctx             context.Context     // enable cancellation/timeout of query
}

func (m *Manager) startJobRunWorker(ctx context.Context) {
	defer m.jobRunWorkers.Done()
	logger := log.WithField("func", "Manager.startJobRunWorker")
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
				req.jobRunPathsChan <- jobRunResponse{
					jobRun: req.jobRun,
					error:  fmt.Errorf("job run request was not serviced soon enough: %v", req.ctx.Err()),
				}
			default: // execute the query and respond
				jobRunRes, err := req.query.queryJobArtifacts(req.jobRun, jobLog)
				req.jobRunPathsChan <- jobRunResponse{
					jobRun:   req.jobRun,
					error:    err,
					response: jobRunRes,
				}
			}
		}
	}
}
