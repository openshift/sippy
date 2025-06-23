package middleware

import (
	"context"
	"sync"

	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
)

type List []Middleware

func (l List) Query(ctx context.Context, wg *sync.WaitGroup, allJobVariants crtest.JobVariants, baseStatusCh, sampleStatusCh chan map[string]crtype.TestStatus, errCh chan error) {
	// Invoke the Query phase for each middleware configured:
	for _, mw := range l {
		mw.Query(ctx, wg, allJobVariants, baseStatusCh, sampleStatusCh, errCh)
	}
}

func (l List) QueryTestDetails(ctx context.Context, wg *sync.WaitGroup, errCh chan error, allJobVariants crtest.JobVariants) {
	// Invoke the QueryTestDetails phase for each middleware configured:
	for _, mw := range l {
		mw.QueryTestDetails(ctx, wg, errCh, allJobVariants)
	}
}

func (l List) PreAnalysis(testKey crtest.Identification, testStats *crtype.ReportTestStats) error {
	for _, mw := range l {
		if err := mw.PreAnalysis(testKey, testStats); err != nil {
			return err
		}
	}
	return nil
}

func (l List) PostAnalysis(testKey crtest.Identification, testStats *crtype.ReportTestStats) error {
	for _, mw := range l {
		if err := mw.PostAnalysis(testKey, testStats); err != nil {
			return err
		}
	}
	return nil
}

func (l List) PreTestDetailsAnalysis(testKey crtest.KeyWithVariants, status *crtype.TestJobRunStatuses) error {
	for _, mw := range l {
		if err := mw.PreTestDetailsAnalysis(testKey, status); err != nil {
			return err
		}
	}
	return nil
}
