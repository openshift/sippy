package regressionloader

import (
	crtype "github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
	log "github.com/sirupsen/logrus"
)

// RegressionLoader checks the component readiness report for each view with regression tracking enabled,
// and updates a bigquery table citing how long the regression has been open for.
type RegressionLoader struct {
	errors    []error
	bqClient  *bqclient.Client
	cacheOpts cache.RequestOptions
	views     []crtype.View
	releases  []v1.Release
}

func New(bqClient *bqclient.Client, cacheOpts cache.RequestOptions, views []crtype.View, releases []v1.Release) *RegressionLoader {
	return &RegressionLoader{
		bqClient:  bqClient,
		cacheOpts: cacheOpts,
		views:     views,
		releases:  releases,
	}
}

func (jl *RegressionLoader) Name() string {
	return "regression"
}

func (jl *RegressionLoader) Load() {
	for _, view := range jl.views {
		if view.Metrics.Enabled || view.RegressionTracking.Enabled {
			// dummy prowURL and gcsBucket, these are only used for test details report, no need
			// to complicate our options here.
			err := metrics.UpdateComponentReadinessTrackingForView(jl.bqClient,
				"", "", jl.cacheOpts, view, jl.releases,
				true, false)
			log.WithError(err).Error("error")
			if err != nil {
				jl.errors = append(jl.errors, err)
				log.WithError(err).WithField("view", view.Name).Error("error refreshing regressions for view")
			}
		}
	}

}

func (jl *RegressionLoader) Errors() []error {
	return jl.errors
}
