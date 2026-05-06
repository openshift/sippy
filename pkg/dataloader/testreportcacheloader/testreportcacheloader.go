package testreportcacheloader

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/db"
)

type testReportCacheLoader struct {
	dbc         *db.DB
	cacheClient cache.Cache
	releases    []v1.Release
	errs        []error
}

func New(dbc *db.DB, cacheClient cache.Cache, releases []v1.Release) *testReportCacheLoader {
	return &testReportCacheLoader{
		dbc:         dbc,
		cacheClient: cacheClient,
		releases:    releases,
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

	devReleases := l.developmentReleases()
	for _, release := range releases {
		if !devReleases[release] {
			log.Infof("Skipping test report cache for non-development release=%s", release)
			continue
		}
		for _, period := range []string{"default", "twoDay"} {
			for _, collapse := range []bool{true, false} {
				log.Infof("Priming test report cache for release=%s period=%s collapse=%v", release, period, collapse)
				if err := api.PrimeTestResultsCache(ctx, l.dbc, l.cacheClient, release, period, collapse); err != nil {
					log.WithError(err).Errorf("Failed to prime test report cache for release=%s period=%s collapse=%v", release, period, collapse)
					l.errs = append(l.errs, err)
				}
			}
		}
	}
}

// developmentReleases returns a set of release names that are OCP development
// releases (have payloadTags capability and no GA date).
func (l *testReportCacheLoader) developmentReleases() map[string]bool {
	devReleases := make(map[string]bool)
	for _, r := range l.releases {
		if r.Product != "OCP" {
			continue
		}
		if r.GADate != nil {
			continue
		}
		if !r.Capabilities[v1.PayloadTagsCap] {
			continue
		}
		devReleases[r.Release] = true
	}
	return devReleases
}

func (l *testReportCacheLoader) Errors() []error {
	return l.errs
}
