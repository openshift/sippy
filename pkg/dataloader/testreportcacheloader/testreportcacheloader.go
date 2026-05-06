package testreportcacheloader

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/db"
)

type testReportCacheLoader struct {
	dbc         *db.DB
	cacheClient cache.Cache
	errs        []error
}

func New(dbc *db.DB, cacheClient cache.Cache) *testReportCacheLoader {
	return &testReportCacheLoader{
		dbc:         dbc,
		cacheClient: cacheClient,
	}
}

func (l *testReportCacheLoader) Name() string {
	return "test-report-cache"
}

func (l *testReportCacheLoader) Load() {
	ctx := context.Background()

	var releases []string
	if err := l.dbc.DB.Raw("SELECT DISTINCT release FROM prow_test_report_7d_matview").Scan(&releases).Error; err != nil {
		l.errs = append(l.errs, err)
		return
	}

	for _, release := range releases {
		for _, period := range []string{"default", "twoDay"} {
			log.Infof("Priming test report cache for release=%s period=%s collapse=true", release, period)
			if err := api.PrimeTestResultsCache(ctx, l.dbc, l.cacheClient, release, period, true); err != nil {
				log.WithError(err).Errorf("Failed to prime test report cache for release=%s period=%s collapse=true", release, period)
				l.errs = append(l.errs, err)
			}
		}
	}
}

func (l *testReportCacheLoader) Errors() []error {
	return l.errs
}
