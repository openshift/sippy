package sippyserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/openshift/sippy/pkg/api/componentreadiness/utils"
	"github.com/openshift/sippy/pkg/api/jobartifacts"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/openshift/sippy/pkg/util/sets"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	middlewarestd "github.com/slok/go-http-metrics/middleware/std"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/mcp"

	v1 "github.com/openshift/sippy/pkg/apis/config/v1"

	"github.com/andygrunwald/go-jira"

	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/api/jobrunintervals"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
	"github.com/openshift/sippy/pkg/util"
	"github.com/openshift/sippy/pkg/util/param"
)

// Mode defines the server mode of operation, OpenShift or upstream Kubernetes.
type Mode string

const (
	ModeOpenShift  Mode = "openshift"
	ModeKubernetes Mode = "kube"
)

func NewServer(
	mode Mode,
	listenAddr string,
	corsAllowedOrigin string,
	syntheticTestManager synthetictests.SyntheticTestManager,
	variantManager testidentification.VariantManager,
	sippyNG fs.FS,
	static fs.FS,
	dbClient *db.DB,
	gcsClient *storage.Client,
	gcsBucket string,
	bigQueryClient *bigquery.Client,
	pinnedDateTime *time.Time,
	cacheClient cache.Cache,
	crTimeRoundingFactor time.Duration,
	views *apitype.SippyViews,
	config *v1.SippyConfig,
	enableWriteEndpoints bool,
	chatAPIURL string,
	jiraClient *jira.Client,
) *Server {

	server := &Server{
		mode:                 mode,
		listenAddr:           listenAddr,
		corsAllowedOrigin:    corsAllowedOrigin,
		syntheticTestManager: syntheticTestManager,
		variantManager:       variantManager,
		jobartifactsManager:  jobartifacts.NewManager(context.Background()),
		sippyNG:              sippyNG,
		static:               static,
		db:                   dbClient,
		bigQueryClient:       bigQueryClient,
		pinnedDateTime:       pinnedDateTime,
		gcsClient:            gcsClient,
		gcsBucket:            gcsBucket,
		cache:                cacheClient,
		crTimeRoundingFactor: crTimeRoundingFactor,
		views:                views,
		config:               config,
		enableWriteAPIs:      enableWriteEndpoints,
		chatAPIURL:           chatAPIURL,
		jiraClient:           jiraClient,
	}

	if bigQueryClient != nil {
		go componentreadiness.GetComponentTestVariantsFromBigQuery(context.Background(), bigQueryClient)
	}

	return server
}

var matViewRefreshMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "sippy_matview_refresh_millis",
	Help:    "Milliseconds to refresh our postgresql materialized views",
	Buckets: []float64{10, 100, 200, 500, 1000, 5000, 10000, 30000, 60000, 300000},
}, []string{"view"})

var allMatViewsRefreshMetric = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "sippy_all_matviews_refresh_millis",
	Help:    "Milliseconds to refresh our postgresql materialized views",
	Buckets: []float64{5000, 10000, 30000, 60000, 300000, 600000, 1200000, 1800000, 2400000, 3000000, 3600000},
})

type Server struct {
	mode                 Mode
	listenAddr           string
	corsAllowedOrigin    string
	syntheticTestManager synthetictests.SyntheticTestManager
	variantManager       testidentification.VariantManager
	jobartifactsManager  *jobartifacts.Manager
	sippyNG              fs.FS
	static               fs.FS
	httpServer           *http.Server
	db                   *db.DB
	bigQueryClient       *bigquery.Client
	pinnedDateTime       *time.Time
	gcsClient            *storage.Client
	gcsBucket            string
	cache                cache.Cache
	crTimeRoundingFactor time.Duration
	capabilities         []string
	views                *apitype.SippyViews
	config               *v1.SippyConfig
	enableWriteAPIs      bool
	chatAPIURL           string
	jiraClient           *jira.Client
}

func (s *Server) GetReportEnd() time.Time {
	return util.GetReportEnd(s.pinnedDateTime)
}

// refreshMaterializedViews updates the postgresql materialized views backing our reports. It is called by the handler
// for the /refresh API endpoint, which is called by the sidecar script which loads the new data from testgrid into the
// main postgresql tables.
//
// refreshMatviewOnlyIfEmpty is used on startup to indicate that we want to do an initial refresh *only* if
// the views appear to be empty.
func refreshMaterializedViews(dbc *db.DB, refreshMatviewOnlyIfEmpty bool) {
	var promPusher *push.Pusher
	if pushgateway := os.Getenv("SIPPY_PROMETHEUS_PUSHGATEWAY"); pushgateway != "" {
		promPusher = push.New(pushgateway, "sippy-matviews")
		promPusher.Collector(matViewRefreshMetric)
		promPusher.Collector(allMatViewsRefreshMetric)
	}

	log.Info("refreshing materialized views")
	allStart := time.Now()

	if dbc == nil {
		log.Info("skipping materialized view refresh as server has no db connection provided")
		return
	}
	// create a channel for work "tasks"
	ch := make(chan string)

	wg := sync.WaitGroup{}

	// allow concurrent workers for refreshing matviews in parallel
	for t := 0; t < 2; t++ {
		wg.Add(1)
		go refreshMatview(dbc, refreshMatviewOnlyIfEmpty, ch, &wg)
	}

	for _, pmv := range db.PostgresMatViews {
		ch <- pmv.Name
	}

	close(ch)
	wg.Wait()

	allElapsed := time.Since(allStart)
	log.WithField("elapsed", allElapsed).Info("refreshed all materialized views")
	allMatViewsRefreshMetric.Observe(float64(allElapsed.Milliseconds()))

	if promPusher != nil {
		log.Info("pushing metrics to prometheus gateway")
		if err := promPusher.Add(); err != nil {
			log.WithError(err).Error("could not push to prometheus pushgateway")
		} else {
			log.Info("successfully pushed metrics to prometheus gateway")
		}
	}
}

func refreshMatview(dbc *db.DB, refreshMatviewOnlyIfEmpty bool, ch chan string, wg *sync.WaitGroup) {

	for matView := range ch {
		start := time.Now()
		tmpLog := log.WithField("matview", matView)

		// If requested, we only refresh the materialized view if it has no rows
		if refreshMatviewOnlyIfEmpty {
			var count int
			if res := dbc.DB.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", matView)).Scan(&count); res.Error != nil {
				tmpLog.WithError(res.Error).Warn("proceeding with refresh of matview that appears to be empty")
			} else if count > 0 {
				tmpLog.Info("skipping matview refresh as it appears to be populated")
				continue
			}
		}

		// Try to refresh concurrently, if we get an error that likely means the view has never been
		// populated (could be a developer env, or a schema migration on the view), fall back to the normal
		// refresh which locks reads.
		tmpLog.Info("refreshing materialized view")
		if res := dbc.DB.Exec(
			fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", matView)); res.Error != nil {
			tmpLog.WithError(res.Error).Warn("error refreshing materialized view concurrently, falling back to regular refresh")

			if res := dbc.DB.Exec(
				fmt.Sprintf("REFRESH MATERIALIZED VIEW %s", matView)); res.Error != nil {
				tmpLog.WithError(res.Error).Error("error refreshing materialized view")
			} else {
				elapsed := time.Since(start)
				tmpLog.WithField("elapsed", elapsed).Info("refreshed materialized view")
				matViewRefreshMetric.WithLabelValues(matView).Observe(float64(elapsed.Milliseconds()))
			}

		} else {
			elapsed := time.Since(start)
			tmpLog.WithField("elapsed", elapsed).Info("refreshed materialized view concurrently")
			matViewRefreshMetric.WithLabelValues(matView).Observe(float64(elapsed.Milliseconds()))
		}
	}
	wg.Done()
}

func RefreshData(dbc *db.DB, pinnedDateTime *time.Time, refreshMatviewsOnlyIfEmpty bool) {
	log.Infof("Refreshing data")

	refreshMaterializedViews(dbc, refreshMatviewsOnlyIfEmpty)

	log.Infof("Refresh complete")
}

func (s *Server) hasCapabilities(capabilities []string) bool {
	for _, cap := range capabilities {
		found := false
		for _, sCap := range s.capabilities {
			if cap == sCap {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s *Server) determineCapabilities() {
	capabilities := make([]string, 0)
	if s.mode == ModeOpenShift {
		capabilities = append(capabilities, OpenshiftCapability)
	}

	if s.bigQueryClient != nil {
		capabilities = append(capabilities, ComponentReadinessCapability)
	}
	if s.db != nil {
		capabilities = append(capabilities, LocalDBCapability)

		if hasBuildCluster, err := query.HasBuildClusterData(s.db); hasBuildCluster {
			capabilities = append(capabilities, BuildClusterCapability)
		} else if err != nil {
			log.WithError(err).Warningf("could not fetch build cluster data")
		}
	}

	if s.db != nil && s.enableWriteAPIs {
		capabilities = append(capabilities, WriteEndpointsCapability)
	}

	if s.chatAPIURL != "" {
		capabilities = append(capabilities, ChatCapability)
	}

	s.capabilities = capabilities
}

func failureResponse(w http.ResponseWriter, code int, message string) {
	api.RespondWithJSON(code, w, map[string]interface{}{
		"code":    code,
		"message": message,
	})
}

// some standard error types
const APIConfigError = "APIConfigError"
const ParameterMissing = "ParameterMissing"
const ParameterInvalid = "ParameterInvalid"

func typedFailureResponse(w http.ResponseWriter, code int, errorType, errorParam, message string) {
	response := map[string]interface{}{
		"code":      code,
		"errorType": errorType,
		"message":   message,
	}
	if errorParam != "" {
		response["errorParam"] = errorParam
	}
	api.RespondWithJSON(code, w, response)
}

func (s *Server) jsonCapabilitiesReport(w http.ResponseWriter, _ *http.Request) {
	api.RespondWithJSON(http.StatusOK, w, s.capabilities)
}

func (s *Server) jsonAutocompleteFromDB(w http.ResponseWriter, req *http.Request) {
	api.PrintAutocompleteFromDB(w, req, s.db)
}

func (s *Server) jsonReleaseTagsReport(w http.ResponseWriter, req *http.Request) {
	api.PrintReleasesReport(w, req, s.db)
}

func (s *Server) jsonIncidentEvent(w http.ResponseWriter, req *http.Request) {
	start, err := getISO8601Date("start", req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, "couldn't parse start param: "+err.Error())
		return
	}

	end, err := getISO8601Date("end", req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, "couldn't parse end param: "+err.Error())
		return
	}

	results, err := api.GetJIRAIncidentsFromDB(s.db, start, end)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, "couldn't fetch events: "+err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, results)
}

func (s *Server) jsonReleaseTagsEvent(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		filterOpts, err := filter.FilterOptionsFromRequest(req, "release_time", apitype.SortDescending)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't parse filter opts: "+err.Error())
			return
		}

		start, err := getISO8601Date("start", req)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't parse start param: "+err.Error())
			return
		}

		end, err := getISO8601Date("end", req)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't parse end param: "+err.Error())
			return
		}

		results, err := api.GetPayloadEvents(s.db, release, filterOpts, start, end)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't get payload events: "+err.Error())
			return
		}

		api.RespondWithJSON(http.StatusOK, w, results)
	}
}

func (s *Server) jsonReleasePullRequestsReport(w http.ResponseWriter, req *http.Request) {
	api.PrintPullRequestsReport(w, req, s.db)
}

func (s *Server) jsonListPayloadJobRuns(w http.ResponseWriter, req *http.Request) {
	// Release appears optional here, perhaps when listing all job runs for all payloads
	// in the release, but this may not make sense. Likely this API call should be
	// moved away from filters and possible support for multiple payloads at once to
	// URL encoded single payload.
	filterOpts, err := filter.FilterOptionsFromRequest(req, "id", apitype.SortDescending)
	if err != nil {
		log.WithError(err).Error("error")
		failureResponse(w, http.StatusInternalServerError, "Error building job run report: "+err.Error())
		return
	}

	payloadJobRuns, err := api.ListPayloadJobRuns(s.db, filterOpts, param.SafeRead(req, "release"))
	if err != nil {
		log.WithError(err).Error("error listing payload job runs")
		failureResponse(w, http.StatusBadRequest, "error listing payload job runs: "+err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, payloadJobRuns)
}

// TODO: may want to merge with jsonReleaseHealthReport, but this is a fair bit slower, and release health is run
// on startup many times over when we calculate the metrics.
// if we could boil the go logic for building this down into a query, it could become another matview and then
// could be run quickly, assembling into the health api much more easily
func (s *Server) jsonGetPayloadAnalysis(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release == "" {
		return
	}
	stream := s.getParamOrFail(w, req, "stream")
	if stream == "" {
		return
	}
	arch := s.getParamOrFail(w, req, "arch")
	if arch == "" {
		return
	}

	filterOpts, err := filter.FilterOptionsFromRequest(req, "id", apitype.SortDescending)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.WithFields(log.Fields{
		"release": release,
		"stream":  stream,
		"arch":    arch,
	}).Info("analyzing payload stream")

	result, err := api.GetPayloadStreamTestFailures(s.db, release, stream, arch, filterOpts, s.GetReportEnd())
	if err != nil {
		log.WithError(err).Error("error")
		failureResponse(w, http.StatusInternalServerError, "Error analyzing payload: "+err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, result)
}

// jsonGetPayloadTestFailures is an api to fetch information about what tests failed across all jobs in a specific
// payload.
func (s *Server) jsonGetPayloadTestFailures(w http.ResponseWriter, req *http.Request) {
	payload := s.getParamOrFail(w, req, "payload")
	if payload == "" {
		return
	}

	logger := log.WithFields(log.Fields{
		"payload": payload,
	})
	logger.Info("checking for test failures in payload")

	result, err := api.GetPayloadTestFailures(s.db, payload, logger)
	if err != nil {
		log.WithError(err).Error("error")
		failureResponse(w, http.StatusInternalServerError, "Error looking up test failures for payload: "+err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, result)
}

func (s *Server) jsonReleaseHealthReport(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release == "" {
		return
	}

	results, err := api.ReleaseHealthReports(s.db, release, s.GetReportEnd())
	if err != nil {
		log.WithError(err).Error("error generating release health report")
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, results)
}

func (s *Server) jsonPayloadDiff(w http.ResponseWriter, req *http.Request) {
	fromPayload := param.SafeRead(req, "fromPayload")
	toPayload := param.SafeRead(req, "toPayload")
	results, err := api.GetPayloadDiffPullRequests(s.db, fromPayload, toPayload)

	if err != nil {
		log.WithError(err).Error("error generating payload diff")
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, results)
}

func (s *Server) jsonFeatureGates(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		filterOpts, err := filter.FilterOptionsFromRequest(req, "unique_test_count", apitype.SortAscending)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't parse filter opts: "+err.Error())
			return
		}
		gates, err := query.GetFeatureGatesFromDB(s.db.DB, release, filterOpts)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't parse filter opts: "+err.Error())
			return
		}
		api.RespondWithJSON(http.StatusOK, w, gates)
	}
}

func (s *Server) jsonTestAnalysis(w http.ResponseWriter, req *http.Request, dbFN func(*db.DB, *filter.Filter, string, string, time.Time) (map[string][]api.CountByDate, error)) {
	testName := s.getParamOrFail(w, req, "test")
	if testName == "" {
		return
	}
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		filters, err := filter.ExtractFilters(req)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't parse filter opts: "+err.Error())
			return
		}
		results, err := dbFN(s.db, filters, release, testName, s.GetReportEnd())
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.RespondWithJSON(200, w, results)
	}
}

func (s *Server) jsonTestAnalysisByJobFromDB(w http.ResponseWriter, req *http.Request) {
	s.jsonTestAnalysis(w, req, api.GetTestAnalysisByJobFromDB)
}

func (s *Server) jsonTestAnalysisByVariantFromDB(w http.ResponseWriter, req *http.Request) {
	s.jsonTestAnalysis(w, req, api.GetTestAnalysisByVariantFromDB)
}

func (s *Server) jsonTestAnalysisOverallFromDB(w http.ResponseWriter, req *http.Request) {
	s.jsonTestAnalysis(w, req, api.GetTestAnalysisOverallFromDB)
}

func (s *Server) jsonTestBugsFromDB(w http.ResponseWriter, req *http.Request) {
	testName := s.getParamOrFail(w, req, "test")
	if testName == "" {
		return
	}

	bugs, err := query.LoadBugsForTest(s.db, testName, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			api.RespondWithJSON(http.StatusOK, w, []models.Bug{})
			return
		}
		log.WithError(err).Error("error querying test bugs from db")
		failureResponse(w, http.StatusInternalServerError, "error querying test bugs from db")
		return
	}
	api.RespondWithJSON(http.StatusOK, w, bugs)
}

func (s *Server) jsonTestDurationsFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release == "" {
		return
	}

	testName := s.getParamOrFail(w, req, "test")
	if testName == "" {
		return
	}

	filters, err := filter.ExtractFilters(req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, "error processing filter options")
		return
	}

	outputs, err := api.GetTestDurationsFromDB(s.db, release, testName, filters)
	if err != nil {
		log.WithError(err).Error("error querying test outputs from db")
		failureResponse(w, http.StatusInternalServerError, "error querying test outputs from db")
		return
	}
	api.RespondWithJSON(http.StatusOK, w, outputs)
}

func (s *Server) jsonTestOutputsFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release == "" {
		return
	}

	testName := s.getParamOrFail(w, req, "test")
	if testName == "" {
		return
	}

	filters, err := filter.ExtractFilters(req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, "error processing filter options")
		return
	}

	outputs, err := api.GetTestOutputsFromDB(s.db, release, testName, filters, 10)
	if err != nil {
		log.WithError(err).Error("error querying test outputs from db")
		failureResponse(w, http.StatusInternalServerError, "error querying test outputs from db")
		return
	}
	api.RespondWithJSON(http.StatusOK, w, outputs)
}

func (s *Server) jsonComponentTestVariantsFromBigQuery(w http.ResponseWriter, req *http.Request) {
	if s.bigQueryClient == nil {
		failureResponse(w, http.StatusBadRequest, "component report API is only available when google-service-account-credential-file is configured")
		return
	}
	outputs, errs := componentreadiness.GetComponentTestVariantsFromBigQuery(req.Context(), s.bigQueryClient)
	if len(errs) > 0 {
		log.Warningf("%d errors were encountered while querying test variants from big query:", len(errs))
		for _, err := range errs {
			log.Error(err.Error())
		}
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("error querying test variants from big query: %v", errs))
		return
	}
	api.RespondWithJSON(http.StatusOK, w, outputs)
}

func (s *Server) jsonJobVariantsFromBigQuery(w http.ResponseWriter, req *http.Request) {
	if s.bigQueryClient == nil {
		failureResponse(w, http.StatusBadRequest, "job variants API is only available when google-service-account-credential-file is configured")
		return
	}
	outputs, errs := componentreadiness.GetJobVariantsFromBigQuery(req.Context(), s.bigQueryClient)
	if len(errs) > 0 {
		log.Warningf("%d errors were encountered while querying job variants from big query:", len(errs))
		for _, err := range errs {
			log.Error(err.Error())
		}
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("error querying job variants from big query: %v", errs))
		return
	}
	api.RespondWithJSON(http.StatusOK, w, outputs)
}

func (s *Server) jsonComponentReadinessViews(w http.ResponseWriter, req *http.Request) {
	allReleases, err := api.GetReleases(req.Context(), s.bigQueryClient, false)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// deep copy the views and then we'll inject a fixed start/end time using the relative times
	// the view is configured with, so the UI can pre-populate the pickers
	viewsCopy := make([]crview.View, len(s.views.ComponentReadiness))
	copy(viewsCopy, s.views.ComponentReadiness)
	for i := range viewsCopy {
		rro, err := utils.GetViewReleaseOptions(allReleases, "basis", viewsCopy[i].BaseRelease, s.crTimeRoundingFactor)
		if err != nil {
			failureResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		viewsCopy[i].BaseRelease.Start = rro.Start
		viewsCopy[i].BaseRelease.End = rro.End

		rro, err = utils.GetViewReleaseOptions(allReleases, "sample", viewsCopy[i].SampleRelease, s.crTimeRoundingFactor)
		if err != nil {
			failureResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		viewsCopy[i].SampleRelease.Start = rro.Start
		viewsCopy[i].SampleRelease.End = rro.End
	}
	api.RespondWithJSON(http.StatusOK, w, viewsCopy)
}

// getComponentReportFromRequest creates a component report based on the HTTP request parameters
func (s *Server) getComponentReportFromRequest(req *http.Request) (componentreport.ComponentReport, error) {
	if s.bigQueryClient == nil {
		return componentreport.ComponentReport{}, fmt.Errorf("component report API is only available when google-service-account-credential-file is configured")
	}

	allJobVariants, errs := componentreadiness.GetJobVariantsFromBigQuery(req.Context(), s.bigQueryClient)
	if len(errs) > 0 {
		return componentreport.ComponentReport{}, fmt.Errorf("failed to get variants from bigquery")
	}

	allReleases, err := api.GetReleases(req.Context(), s.bigQueryClient, false)
	if err != nil {
		return componentreport.ComponentReport{}, err
	}

	options, warnings, err := utils.ParseComponentReportRequest(s.views.ComponentReadiness, allReleases, req, allJobVariants, s.crTimeRoundingFactor,
		s.config.ComponentReadinessConfig.VariantJunitTableOverrides)
	if err != nil {
		return componentreport.ComponentReport{}, err
	}

	baseURL := api.GetBaseURL(req)

	outputs, errs := componentreadiness.GetComponentReportFromBigQuery(
		req.Context(),
		s.bigQueryClient,
		s.db,
		options,
		s.config.ComponentReadinessConfig.VariantJunitTableOverrides,
		baseURL,
	)
	if len(errs) > 0 {
		return componentreport.ComponentReport{}, fmt.Errorf("error querying component from big query: %v", errs)
	}

	// Add any warnings from parsing to the report
	outputs.Warnings = warnings

	return outputs, nil
}

func (s *Server) jsonComponentReportFromBigQuery(w http.ResponseWriter, req *http.Request) {
	outputs, err := s.getComponentReportFromRequest(req)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, outputs)
}

func (s *Server) jsonComponentReportTestDetailsFromBigQuery(w http.ResponseWriter, req *http.Request) {
	if s.bigQueryClient == nil {
		err := fmt.Errorf("component report API is only available when google-service-account-credential-file is configured")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	allJobVariants, errs := componentreadiness.GetJobVariantsFromBigQuery(req.Context(), s.bigQueryClient)
	if len(errs) > 0 {
		err := fmt.Errorf("failed to get variants from bigquery")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	allReleases, err := api.GetReleases(req.Context(), s.bigQueryClient, false)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	reqOptions, _, err := utils.ParseComponentReportRequest(s.views.ComponentReadiness, allReleases, req, allJobVariants, s.crTimeRoundingFactor,
		s.config.ComponentReadinessConfig.VariantJunitTableOverrides)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	baseURL := api.GetBaseURL(req)
	outputs, errs := componentreadiness.GetTestDetails(req.Context(), s.bigQueryClient, s.db, reqOptions, allReleases, baseURL)
	if len(errs) > 0 {
		log.Warningf("%d errors were encountered while querying component test details from big query:", len(errs))
		for _, err := range errs {
			log.Error(err.Error())
		}
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("error querying component test details from big query: %v", errs))
		return
	}
	api.RespondWithJSON(http.StatusOK, w, outputs)
}

func (s *Server) jsonJobBugsFromDB(w http.ResponseWriter, req *http.Request) {
	release := param.SafeRead(req, "release")

	fil, err := filter.ExtractFilters(req)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "Could not marshal query: "+err.Error())
		return
	}
	jobFilter, _, err := splitJobAndJobRunFilters(fil)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "Could not marshal query: "+err.Error())
		return
	}

	start, boundary, end := getPeriodDates("default", req, s.GetReportEnd())
	limit := getLimitParam(req)
	sortField, sort := getSortParams(req)

	jobIDs, err := query.ListFilteredJobIDs(s.db, release, jobFilter, start, boundary, end, limit, sortField, sort)
	if err != nil {
		log.WithError(err).Error("error querying jobs")
		failureResponse(w, http.StatusInternalServerError, "error querying jobs")
		return
	}

	bugs, err := query.LoadBugsForJobs(s.db, jobIDs, false)
	if err != nil {
		log.WithError(err).Error("error querying job bugs from db")
		failureResponse(w, http.StatusInternalServerError, "error querying job bugs from db")
		return
	}
	api.RespondWithJSON(http.StatusOK, w, bugs)
}

func (s *Server) jsonTestsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		api.PrintTestsJSONFromDB(release, w, req, s.db)
	}
}

func (s *Server) jsonTestsReportFromBigQuery(w http.ResponseWriter, req *http.Request) {
	// Fall back to postgres if dataset is not ci_analysis_us
	if s.bigQueryClient == nil || s.bigQueryClient.Dataset != "ci_analysis_us" {
		s.jsonTestsReportFromDB(w, req)
		return
	}
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		api.PrintTestsJSONFromBigQuery(release, w, req, s.bigQueryClient)
	}
}

func (s *Server) jsonTestDetailsReportFromDB(w http.ResponseWriter, req *http.Request) {
	// Filter to test names containing this query param:
	testSubstring := req.URL.Query()["test"]
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		api.PrintTestsDetailsJSONFromDB(w, release, testSubstring, s.db)
	}
}

func (s *Server) jsonReleasesReportFromDB(w http.ResponseWriter, req *http.Request) {
	forceRefresh := req.URL.Query().Get("forceRefresh") != "" // use to refresh cached releases from BQ
	releases, err := api.GetReleases(req.Context(), s.bigQueryClient, forceRefresh)
	if err != nil {
		log.WithError(err).Error("error querying releases")
		failureResponse(w, http.StatusInternalServerError, "error querying releases")
		return
	}

	// Get last updated time from database if available
	var lastUpdated time.Time
	if s.db != nil {
		type LastUpdatedQuery struct {
			Max time.Time
		}
		var result LastUpdatedQuery
		// Assume our last update is the last time we inserted a prow job run.
		res := s.db.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&result)
		if res.Error != nil {
			log.WithError(res.Error).Error("error querying last updated from db")
			failureResponse(w, http.StatusInternalServerError, "error querying last updated from db")
			return
		}
		lastUpdated = result.Max
	}

	// Build response using shared function
	response := api.BuildReleasesResponse(releases, lastUpdated)
	api.RespondWithJSON(http.StatusOK, w, response)
}

func (s *Server) jsonTestCapabilitiesFromDB(w http.ResponseWriter, req *http.Request) {
	capabilities, err := api.GetTestCapabilitiesFromDB(s.bigQueryClient)
	if err != nil {
		log.WithError(err).Error("error querying test capabilities")
		failureResponse(w, http.StatusInternalServerError, "error querying test capabilities")
		return
	}

	api.RespondWithJSON(http.StatusOK, w, capabilities)
}

func (s *Server) jsonTestLifecyclesFromDB(w http.ResponseWriter, req *http.Request) {
	lifecycles, err := api.GetTestLifecyclesFromDB(s.bigQueryClient)
	if err != nil {
		log.WithError(err).Error("error querying test lifecycles")
		failureResponse(w, http.StatusInternalServerError, "error querying test lifecycles")
		return
	}

	api.RespondWithJSON(http.StatusOK, w, lifecycles)
}

func (s *Server) jsonHealthReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		api.PrintOverallReleaseHealthFromDB(w, s.db, release, s.GetReportEnd())
	}
}

func (s *Server) jsonBuildClusterHealth(w http.ResponseWriter, req *http.Request) {
	start, boundary, end := getPeriodDates("default", req, s.GetReportEnd())

	results, err := api.GetBuildClusterHealthReport(s.db, start, boundary, end)
	if err != nil {
		log.WithError(err).Error("error querying build cluster health from db")
		failureResponse(w, http.StatusInternalServerError, "error querying build cluster health from db: "+err.Error())
		return
	}

	api.RespondWithJSON(200, w, results)
}

func (s *Server) jsonBuildClusterHealthAnalysis(w http.ResponseWriter, req *http.Request) {
	period := getPeriod(req, api.PeriodDay)

	results, err := api.GetBuildClusterHealthAnalysis(s.db, period)
	if err != nil {
		log.WithError(err).Error("error querying build cluster health from db")
		failureResponse(w, http.StatusInternalServerError, "error querying build cluster health from db: "+err.Error())
		return
	}

	api.RespondWithJSON(200, w, results)
}

// getParamOrFail returns the parameter requested; if it's empty, it also issues a failure response as a convenience
// (this does not complete the request; caller still must check for empty string and return up the stack accordingly)
func (s *Server) getParamOrFail(w http.ResponseWriter, req *http.Request, name string) string {
	value := param.SafeRead(req, name)
	if value == "" {
		failureResponse(w, http.StatusBadRequest, fmt.Sprintf("param '%s' is required", name))
	}
	return value
}

func (s *Server) jsonJobsDetailsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	jobName := s.getParamOrFail(w, req, "job")
	if release != "" && jobName != "" {
		err := api.PrintJobDetailsReportFromDB(w, req, s.db, release, jobName, s.GetReportEnd())
		if err != nil {
			log.Errorf("Error from PrintJobDetailsReportFromDB: %v", err)
		}
	}
}

func (s *Server) printReportDate(w http.ResponseWriter, req *http.Request) {
	reportDate := ""
	if s.pinnedDateTime != nil {
		reportDate = s.pinnedDateTime.Format(time.RFC3339)
	}
	api.RespondWithJSON(http.StatusOK, w, map[string]interface{}{"pinnedDateTime": reportDate})
}

func (s *Server) printCanaryReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		api.PrintCanaryTestsFromDB(release, w, s.db)
	}
}

func (s *Server) jsonVariantsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		api.PrintVariantReportFromDB(w, req, s.db, release, s.GetReportEnd())
	}
}

func (s *Server) jsonJobsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		api.PrintJobsReportFromDB(w, req, s.db, release, s.GetReportEnd())
	}
}

func (s *Server) jsonRepositoriesReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		filterOpts, err := filter.FilterOptionsFromRequest(req, "premerge_job_failures", apitype.SortDescending)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't parse filter opts: "+err.Error())
			return
		}

		results, err := api.GetRepositoriesReportFromDB(s.db, release, filterOpts, s.GetReportEnd())
		if err != nil {
			log.WithError(err).Error("error")
			failureResponse(w, http.StatusInternalServerError, "Error fetching repositories: "+err.Error())
			return
		}

		api.RespondWithJSON(http.StatusOK, w, results)
	}
}

func (s *Server) jsonPullRequestsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getParamOrFail(w, req, "release")
	if release != "" {
		filterOpts, err := filter.FilterOptionsFromRequest(req, "merged_at", apitype.SortDescending)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, "couldn't parse filter opts: "+err.Error())
			return
		}

		results, err := api.GetPullRequestsReportFromDB(s.db, release, filterOpts)
		if err != nil {
			log.WithError(err).Error("error")
			failureResponse(w, http.StatusInternalServerError, "Error fetching pull requests: "+err.Error())
			return
		}

		api.RespondWithJSON(http.StatusOK, w, results)
	}
}

func (s *Server) jsonJobRunSummary(w http.ResponseWriter, req *http.Request) {
	jobRunIDStr := s.getParamOrFail(w, req, "prow_job_run_id")
	if jobRunIDStr == "" {
		return
	}

	jobRunID, err := strconv.ParseInt(jobRunIDStr, 10, 64)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "unable to parse prow_job_run_id: "+err.Error())
		return
	}

	summary, err := api.GetJobRunSummary(req.Context(), s.db, s.gcsClient, jobRunID)
	if err != nil {
		api.RespondWithJSON(http.StatusInternalServerError, w, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, summary)
}

// jsonJobRunPayload returns the payload release tag that was used for a given job run.
func (s *Server) jsonJobRunPayload(w http.ResponseWriter, req *http.Request) {
	if s.bigQueryClient == nil {
		failureResponse(w, http.StatusBadRequest, "job run payload API is only available when google-service-account-credential-file is configured")
		return
	}

	jobRunIDStr := s.getParamOrFail(w, req, "prow_job_run_id")
	if jobRunIDStr == "" {
		return
	}

	results, err := api.PayloadForJobRun(req.Context(), s.bigQueryClient, jobRunIDStr)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, results)
}

func (s *Server) jsonJobRunsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := param.SafeRead(req, "release")

	filterOpts, err := filter.FilterOptionsFromRequest(req, "timestamp", "desc")
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "Could not marshal query: "+err.Error())
		return
	}

	pagination, err := getPaginationParams(req)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "Could not parse pagination options: "+err.Error())
		return
	}

	result, err := api.JobsRunsReportFromDB(s.db, filterOpts, release, pagination, s.GetReportEnd())
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, result)
}

// jsonJobRunRiskAnalysis is an API to make a guess at the severity of failures in a prow job run, based on historical
// pass rates for each failed test, on-going incidents, and other factors.
//
// This API can be called in two ways, a GET with a prow_job_run_id query param, or a GET with a
// partial ProwJobRun struct serialized as json in the request body. The ID version will return the
// stored analysis for the job when it was imported into sippy. The other version is a transient
// request to be used when sippy has not yet imported the job, but we wish to analyze the failure risk.
// Soon, we expect the transient version is called from CI to get a risk analysis json result, which will
// be stored in the job run artifacts, then imported with the job run, and will ultimately be the
// data that is returned by the get by ID version.
func (s *Server) jsonJobRunRiskAnalysis(w http.ResponseWriter, req *http.Request) {

	logger := log.WithField("func", "jsonJobRunRiskAnalysis")

	jobRun := &models.ProwJobRun{}

	// API path one where we return a risk analysis for a prow job run ID we already know about:
	jobRunIDStr := req.URL.Query().Get("prow_job_run_id")
	if jobRunIDStr != "" {

		jobRunID, err := strconv.ParseInt(jobRunIDStr, 10, 64)
		if err != nil {
			failureResponse(w, http.StatusBadRequest, "unable to parse prow_job_run_id: "+err.Error())
			return
		}

		logger = logger.WithField("jobRunID", jobRunID)

		// lookup prowjob and run count
		jobRun, err = api.FetchJobRun(s.db, jobRunID, false, nil, logger)

		if err != nil {
			failureResponse(w, http.StatusBadRequest, err.Error())
			return
		}

	} else {
		err := json.NewDecoder(req.Body).Decode(&jobRun)
		if err != nil {
			failureResponse(w, http.StatusBadRequest, fmt.Sprintf("error decoding prow job run json in request body: %s", err))
			return
		}

		// validate the jobRun isn't empty
		// valid case where test artifacts are not available
		// we want to mark this as a high risk
		if isValid, detailReason := isValidProwJobRun(jobRun); !isValid {

			log.Warn("Invalid ProwJob provided for analysis, returning elevated risk")
			result := apitype.ProwJobRunRiskAnalysis{
				OverallRisk: apitype.JobFailureRisk{
					Level:   apitype.FailureRiskLevelMissingData,
					Reasons: []string{fmt.Sprintf("Invalid ProwJob provided for analysis: %s", detailReason)},
				},
			}

			// respond ok since we handle it
			api.RespondWithJSON(http.StatusOK, w, result)
			return
		}

		// We don't expect the caller to fully populate the ProwJob, just its name;
		// override the input by looking up the actual ProwJob so we have access to release and variants.
		job := &models.ProwJob{}
		res := s.db.DB.Where("name = ?", jobRun.ProwJob.Name).First(job)
		if res.Error != nil {
			if errors.Is(res.Error, gorm.ErrRecordNotFound) {
				// sippy does not import all jobs to its prow_jobs table; for context see
				// https://redhat-internal.slack.com/archives/C02K89U2EV8/p1736972504210699
				// for example, PR jobs on GA releases are excluded, and various other jobs
				errMsg := fmt.Sprintf("ProwJob '%s' is not included in imported jobs so risk analysis will not run.", jobRun.ProwJob.Name)
				result := apitype.ProwJobRunRiskAnalysis{
					OverallRisk: apitype.JobFailureRisk{Level: apitype.FailureRiskLevelUnknown, Reasons: []string{errMsg}},
				}
				api.RespondWithJSON(http.StatusOK, w, result)
			} else {
				failureResponse(w, http.StatusBadRequest, fmt.Sprintf("unable to find ProwJob '%s': %v", jobRun.ProwJob.Name, res.Error))
			}
			return
		}
		jobRun.ProwJob = *job

		jobRun.ProwJob.Variants = s.variantManager.IdentifyVariants(jobRun.ProwJob.Name)
		logger = logger.WithField("jobRunID", jobRun.ID)
	}

	logger.Infof("job run = %+v", *jobRun)
	result, err := api.JobRunRiskAnalysis(s.db, s.bigQueryClient, jobRun, logger, false)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, result)
}

// jsonJobRunRiskAnalysis is an API to return the intervals origin builds for interesting things that occurred during
// the test run.
//
// This API is used by the job run intervals chart in the UI.
func (s *Server) jsonJobRunIntervals(w http.ResponseWriter, req *http.Request) {

	logger := log.WithField("func", "jsonJobRunIntervals")

	if s.gcsClient == nil {
		failureResponse(w, http.StatusBadRequest, "server not configured for GCS, unable to use this API")
		return
	}

	jobRunIDStr := s.getParamOrFail(w, req, "prow_job_run_id")
	if jobRunIDStr == "" {
		return
	}

	jobRunID, err := strconv.ParseInt(jobRunIDStr, 10, 64)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "unable to parse prow_job_run_id: "+err.Error())
		return
	}
	logger = logger.WithField("jobRunID", jobRunID)

	jobName := param.SafeRead(req, "job_name")
	repoInfo := param.SafeRead(req, "repo_info")
	pullNumber := param.SafeRead(req, "pull_number")
	intervalFile := param.SafeRead(req, "file")

	// Attempt to calculate a GCS path based on a passed in jobName.
	var gcsPath string
	if len(jobName) > 0 {
		if len(repoInfo) > 0 {
			if repoInfo == "openshift_origin" {
				// GCS bucket path for openshift/origin PRs
				gcsPath = fmt.Sprintf("pr-logs/pull/%s/%s/%s", pullNumber, jobName, jobRunIDStr)
			} else {
				// GCS bucket path for repos other than origin PRs.
				gcsPath = fmt.Sprintf("pr-logs/pull/%s/%s/%s/%s", repoInfo, pullNumber, jobName, jobRunIDStr)
			}
		} else {
			// GCS bucket for periodics
			gcsPath = fmt.Sprintf("logs/%s/%s", jobName, jobRunIDStr)
		}
	} else {
		// JobName was not passed.
		gcsPath = ""
	}
	result, err := jobrunintervals.JobRunIntervals(s.gcsClient, s.db, jobRunID, s.gcsBucket, gcsPath,
		intervalFile, logger.WithField("func", "JobRunIntervals"))
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, result)
}

func isValidProwJobRun(jobRun *models.ProwJobRun) (bool, string) {
	if (jobRun == nil || jobRun == &models.ProwJobRun{} || &jobRun.ProwJob == &models.ProwJob{} || jobRun.ProwJob.Name == "") {

		detailReason := "empty ProwJobRun"

		if (jobRun != nil && jobRun != &models.ProwJobRun{}) {

			// not likely to be empty when we have a non empty ProwJobRun
			detailReason = "empty ProwJob"

			if (&jobRun.ProwJob != &models.ProwJob{}) {
				detailReason = "missing ProwJob Name"
			}
		}

		return false, detailReason
	}

	return true, ""
}

func (s *Server) jsonJobsAnalysisFromDB(w http.ResponseWriter, req *http.Request) {
	release := param.SafeRead(req, "release")

	fil, err := filter.ExtractFilters(req)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "Could not marshal query: "+err.Error())
		return
	}
	jobFilter, jobRunsFilter, err := splitJobAndJobRunFilters(fil)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "Could not marshal query: "+err.Error())
		return
	}

	start, boundary, end := getPeriodDates("default", req, s.GetReportEnd())
	limit := getLimitParam(req)
	sortField, sort := getSortParams(req)
	period := getPeriod(req, api.PeriodDay)

	results, err := api.PrintJobAnalysisJSONFromDB(s.db, release, jobFilter, jobRunsFilter,
		start, boundary, end, limit, sortField, sort, period, s.GetReportEnd())
	if err != nil {
		log.WithError(err).Error("error in PrintJobAnalysisJSONFromDB")
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, results)
}

func (s *Server) jsonGetTriages(w http.ResponseWriter, req *http.Request) {
	triages, err := componentreadiness.ListTriages(s.db, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, triages)
}

// ExpandedTriage allows for additional information to be included in the triage response.
// Currently, this is only the associated ReportTestSummaries which are useful for linking to the test_details report.
type ExpandedTriage struct {
	*models.Triage
	RegressedTests []*componentreport.ReportTestSummary `json:"regressed_tests"`
}

func (s *Server) jsonGetTriageByID(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	idStr := vars["id"]

	var expandRegressions bool
	expand := req.URL.Query().Get("expand")
	if expand == "regressions" {
		expandRegressions = true
	}

	triageID, err := strconv.Atoi(idStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid ID format: "+idStr)
		return
	}

	triage, err := componentreadiness.GetTriage(s.db, triageID, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if triage == nil {
		failureResponse(w, http.StatusNotFound, "triage not found")
		return
	}
	if !expandRegressions {
		api.RespondWithJSON(http.StatusOK, w, triage)
		return
	}

	et := ExpandedTriage{
		Triage: triage,
	}

	associatedViews := sets.NewString()
	for _, regression := range triage.Regressions {
		associatedViews.Insert(regression.View)
	}

	for _, view := range associatedViews.List() {
		// Set the view in the request so that we can obtain the component report to get the regressed test(s) for display
		q := req.URL.Query()
		q.Set("view", view)
		req.URL.RawQuery = q.Encode()
		componentReport, err := s.getComponentReportFromRequest(req)
		if err != nil {
			failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("unable to get component report: %v", err))
			return
		}
		for _, regression := range triage.Regressions {
			regressedTest := componentreadiness.GetMatchingRegressedTestForRegression(regression, componentReport)
			if regressedTest != nil {
				et.RegressedTests = append(et.RegressedTests, regressedTest)
			}
		}
	}

	api.RespondWithJSON(http.StatusOK, w, et)
}

func (s *Server) jsonCreateTriage(w http.ResponseWriter, req *http.Request) {
	user := getUserForRequest(req)
	log.Infof("triage POST made by user: %s", user)
	var triage models.Triage
	if err := json.NewDecoder(req.Body).Decode(&triage); err != nil {
		log.WithError(err).Error("error parsing new triage record")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := context.WithValue(req.Context(), models.CurrentUserKey, user)
	triage, err := componentreadiness.CreateTriage(s.db.DB.WithContext(ctx), s.jiraClient, triage, req)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, triage)
}

func (s *Server) jsonUpdateTriage(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	idStr := vars["id"]
	triageID, err := strconv.Atoi(idStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid ID format: "+idStr)
		return
	}

	user := getUserForRequest(req)
	log.Infof("triage PUT made by user: %s", user)
	var triage models.Triage
	if err := json.NewDecoder(req.Body).Decode(&triage); err != nil {
		log.WithError(err).Error("error parsing new triage record")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if triageID != int(triage.ID) { // nolint:gosec
		failureResponse(w, http.StatusBadRequest, "resource triage ID does not match URL")
		return
	}
	ctx := context.WithValue(req.Context(), models.CurrentUserKey, user)
	triage, err = componentreadiness.UpdateTriage(s.db.DB.WithContext(ctx), s.jiraClient, triage, req)
	if err != nil {
		log.WithError(err).Error("error updating triage")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, triage)
}

func (s *Server) jsonDeleteTriage(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	idStr := vars["id"]
	triageID, err := strconv.Atoi(idStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid ID format: "+idStr)
		return
	}

	user := getUserForRequest(req)
	log.Infof("triage DELETE made by user: %s", user)
	ctx := context.WithValue(req.Context(), models.CurrentUserKey, user)
	if err := componentreadiness.DeleteTriage(s.db.DB.WithContext(ctx), triageID); err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, nil)
}

// jsonTriagePotentialMatchingRegressions returns a json response containing potential matching regressions that to be
// added to the given triage. These are grouped by existing associated regression(s), and given a confidence level
func (s *Server) jsonTriagePotentialMatchingRegressions(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	idStr := vars["id"]

	triageID, err := strconv.Atoi(idStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid ID format: "+idStr)
		return
	}

	triage, err := componentreadiness.GetTriage(s.db, triageID, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if triage == nil {
		failureResponse(w, http.StatusNotFound, "triage not found")
		return
	}
	view := req.URL.Query().Get("view")
	if view == "" {
		failureResponse(w, http.StatusBadRequest, "no view provided")
		return
	}
	// TODO(sgoeddel): I don't think we need the component report anymore, the regressions should contain the test_details link, but do they contain the status?
	componentReport, err := s.getComponentReportFromRequest(req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	allReleases, err := api.GetReleases(req.Context(), s.bigQueryClient, false)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("error getting releases: %v", err))
		return
	}
	regressions, err := componentreadiness.ListRegressions(s.db, view, "", s.views.ComponentReadiness, allReleases, s.crTimeRoundingFactor, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	potentialMatches, err := componentreadiness.GetTriagePotentialMatches(triage, regressions, componentReport, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, potentialMatches)
}

func (s *Server) jsonGetTriageAuditDetails(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	idStr := vars["id"]

	triageID, err := strconv.Atoi(idStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid ID format: "+idStr)
		return
	}

	responseAuditLogs, err := componentreadiness.GetTriageAuditDetails(s.db.DB, triageID, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.RespondWithJSON(http.StatusOK, w, responseAuditLogs)
}

// jsonGetRegressions handles GET requests for listing component readiness regression records.
func (s *Server) jsonGetRegressions(w http.ResponseWriter, req *http.Request) {
	// Get releases for view processing
	allReleases, err := api.GetReleases(req.Context(), s.bigQueryClient, false)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("error getting releases: %v", err))
		return
	}

	// Read query parameters for listing
	view := param.SafeRead(req, "view")
	release := param.SafeRead(req, "release")

	// Error if both view and release are specified
	if view != "" && release != "" {
		failureResponse(w, http.StatusBadRequest, "Cannot specify both 'view' and 'release' parameters. Please use only one.")
		return
	}

	regressions, err := componentreadiness.ListRegressions(s.db, view, release, s.views.ComponentReadiness, allReleases, s.crTimeRoundingFactor, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RespondWithJSON(http.StatusOK, w, regressions)
}

// jsonGetRegressionByID handles GET requests for a specific component readiness regression record by ID.
func (s *Server) jsonGetRegressionByID(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	idStr := vars["id"]

	regressionID, err := strconv.Atoi(idStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid ID format: "+idStr)
		return
	}

	// Get releases for view processing
	allReleases, err := api.GetReleases(req.Context(), s.bigQueryClient, false)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("error getting releases: %v", err))
		return
	}

	regression, err := componentreadiness.GetRegression(s.db, regressionID, s.views.ComponentReadiness, allReleases, s.crTimeRoundingFactor, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, err.Error())
		return
	}
	if regression == nil {
		failureResponse(w, http.StatusNotFound, "regression not found")
		return
	}
	api.RespondWithJSON(http.StatusOK, w, regression)
}

func getUserForRequest(req *http.Request) string {
	user := req.Header.Get("X-Forwarded-User")
	if user == "" && os.Getenv("DEV_MODE") == "1" {
		user = "developer"
	}
	return user
}

// jsonRegressionPotentialMatchingTriages finds the triage entries that currently have regressions that match
// the regression in question. These matches are based on test name and last failure time similarity.
func (s *Server) jsonRegressionPotentialMatchingTriages(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	idStr := vars["id"]

	regressionID, err := strconv.Atoi(idStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "invalid ID format: "+idStr)
		return
	}
	// Get releases for view processing
	allReleases, err := api.GetReleases(req.Context(), s.bigQueryClient, false)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("error getting releases: %v", err))
		return
	}
	regression, err := componentreadiness.GetRegression(s.db, regressionID, s.views.ComponentReadiness, allReleases, s.crTimeRoundingFactor, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("error getting regression: %v", err))
	}
	triages, err := componentreadiness.ListTriages(s.db, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to list triages: %v", err))
		return
	}
	matches, err := componentreadiness.GetRegressionPotentialMatchingTriages(*regression, triages, req)
	if err != nil {
		failureResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to list potential matches: %v", err))
	}
	api.RespondWithJSON(http.StatusOK, w, matches)
}

// FileBugRequest represents the JSON request structure for filing Jira bugs
type FileBugRequest struct {
	Summary         string   `json:"summary"`
	Description     string   `json:"description"`
	AffectsVersions []string `json:"affects_versions"`
	Components      []string `json:"components"`
	ComponentID     string   `json:"component_id"`
	Labels          []string `json:"labels"`
}

// FileBugResponse represents the JSON response structure for filing Jira bugs
type FileBugResponse struct {
	Success bool   `json:"success"`
	DryRun  bool   `json:"dry_run"`
	JiraKey string `json:"jira_key"`
	JiraURL string `json:"jira_url"`
}

// jsonFileJiraBug allows for a Jira "OCPBUGS" card to be created for the given FileBugRequest
// If successful, the response is a jsonified FileBugResponse
func (s *Server) jsonFileJiraBug(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodPost:
		if s.jiraClient == nil {
			log.Warn("jira client not initialized, will not create jira bug, dry run only")
		}

		user := getUserForRequest(req)
		if user == "" {
			failureResponse(w, http.StatusUnauthorized, "User authentication required")
			return
		}
		log.Infof("jira bug creation requested by user: %s", user)

		var bugRequest FileBugRequest
		if err := json.NewDecoder(req.Body).Decode(&bugRequest); err != nil {
			log.WithError(err).Error("error parsing jira bug request")
			failureResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %s", err.Error()))
			return
		}

		var validationErrors []string
		if bugRequest.Summary == "" {
			validationErrors = append(validationErrors, "Summary is required")
		}
		if bugRequest.Description == "" {
			validationErrors = append(validationErrors, "Description is required")
		}
		if len(bugRequest.AffectsVersions) == 0 {
			validationErrors = append(validationErrors, "AffectsVersions is required")
		}
		if bugRequest.ComponentID == "" && len(bugRequest.Components) == 0 {
			validationErrors = append(validationErrors, "At least one Component is required when there is no computed component ID available")
		}

		if len(validationErrors) > 0 {
			failureResponse(w, http.StatusBadRequest, strings.Join(validationErrors, "; "))
			return
		}

		// Due to the way the OCPBUGS project is configured, we cannot set the "Reporter", so we add it to the description for some tracking
		description := fmt.Sprintf("%s\n\nFiled by: [~%s@redhat.com]", bugRequest.Description, user)

		issue := jira.Issue{
			Fields: &jira.IssueFields{
				Description: description,
				Type: jira.IssueType{
					Name: "Bug",
				},
				Project: jira.Project{
					Key: "OCPBUGS",
				},
				Summary: bugRequest.Summary,
			},
		}

		affectsVersions := make([]*jira.AffectsVersion, len(bugRequest.AffectsVersions))
		for i, version := range bugRequest.AffectsVersions {
			affectsVersions[i] = &jira.AffectsVersion{
				Name: version,
			}
		}
		issue.Fields.AffectsVersions = affectsVersions

		components := make([]*jira.Component, 0)
		for _, comp := range bugRequest.Components {
			components = append(components, &jira.Component{Name: comp})
		}
		components = append(components, &jira.Component{ID: bugRequest.ComponentID})
		issue.Fields.Components = components

		if len(bugRequest.Labels) > 0 {
			issue.Fields.Labels = bugRequest.Labels
		}

		var createdIssue *jira.Issue
		dryRun := false
		if s.jiraClient != nil {
			var res *jira.Response
			var err error
			createdIssue, res, err = s.jiraClient.Issue.Create(&issue)
			if err != nil {
				logJiraError(res, err)
				failureResponse(w, http.StatusBadRequest, fmt.Sprintf("couldn't create jira issue: %s", err.Error()))
				return
			}
		} else {
			// No jiraClient, results in dry run for local development and testing
			issue.Key = "OCPBUGS-1234"
			createdIssue = &issue
			dryRun = true
		}

		log.Infof("created jira issue %s for user %s", createdIssue.Key, user)

		jiraURL := fmt.Sprintf("https://issues.redhat.com/browse/%s", createdIssue.Key)
		response := FileBugResponse{
			Success: true,
			DryRun:  dryRun,
			JiraKey: createdIssue.Key,
			JiraURL: jiraURL,
		}
		api.RespondWithJSON(http.StatusOK, w, response)
	case http.MethodOptions:
		// TODO(sgoeddel): should we enable CORS? If so, we will have to do some special logic to allow localhost as well until gorilla is utilized
		// w.Header().Set("Access-Control-Allow-Origin", "https://sippy-auth.dptools.openshift.org")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		api.RespondWithJSON(http.StatusOK, w, nil)
	default:
		failureResponse(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		return
	}
}

func logJiraError(response *jira.Response, err error) {
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		log.WithError(readErr).Errorf("error reading response body. original error is: %v", err)
	} else {
		log.WithError(err).Errorf("error creating or updating jira issue: %q", body)
	}
}

// queryJobArtifacts is an API to query GCS for artifacts from a set of job runs. Parameters:
// - prowJobRuns (required): a comma-separated list of prow job run IDs to query
// - pathGlob (required): a glob pattern to match against the GCS path (ref. https://cloud.google.com/storage/docs/json_api/v1/objects/list#list-object-glob)
// - textContains: a string to search for in the contents of the artifacts, returning the containing line
// - textRegex: a regex string to match in the contents of the artifacts, returning the matching line
// - beforeContext: a positive integer indicating how many lines before the matched line to return
// - afterContext: a positive integer indicating how many lines after the matched line to return
// - maxFileMatches: a positive integer indicating maximum number of matches to return per file
// Returns a JSON jobartifacts.QueryResponse object listing the job runs, the artifacts that matched the glob,
// and (optionally) lines within those artifacts that matched. Errors that occurred are also listed per job run.
// To prevent (accidental) DoS, the number of artifacts and matches returned are limited.
func (s *Server) queryJobArtifacts(w http.ResponseWriter, req *http.Request) {
	if s.gcsClient == nil {
		typedFailureResponse(w, http.StatusServiceUnavailable, APIConfigError, "", "server not configured for GCS, unable to use this API")
		return
	}

	contentMatcher, failed := contentMatcherFromParams(w, req)
	if failed {
		return
	}

	q := &jobartifacts.JobArtifactQuery{
		GcsBucket:      s.gcsClient.Bucket(util.GcsBucketRoot),
		DbClient:       s.db,
		Cache:          s.cache,
		JobRunIDs:      []int64{},
		ContentMatcher: contentMatcher,
	}

	jobRunIDStr := param.SafeRead(req, "prowJobRuns")
	if jobRunIDStr == "" {
		typedFailureResponse(w, http.StatusBadRequest, ParameterMissing, "prowJobRuns",
			"required parameter is missing")
		return
	}
	for _, jobRunIDStr := range strings.Split(jobRunIDStr, ",") {
		id, err := strconv.ParseInt(jobRunIDStr, 10, 64)
		if err != nil {
			typedFailureResponse(w, http.StatusBadRequest, ParameterInvalid, "prowJobRuns",
				fmt.Sprintf("unable to parse prowJobRuns id %q: %s", id, err.Error()))
			return
		}
		q.JobRunIDs = append(q.JobRunIDs, id)
	}

	q.PathGlob = param.SafeRead(req, "pathGlob")
	if q.PathGlob == "" {
		typedFailureResponse(w, http.StatusBadRequest, ParameterMissing, "pathGlob",
			"required parameter is missing")
		return
	}

	// The query looks good to run
	result := s.jobartifactsManager.Query(req.Context(), q)
	// But there's one user input we can't validate without querying: pathGlob. Look for that error and treat it as a bad request.
	if len(result.JobRuns) == 0 && len(result.Errors) > 0 && strings.HasPrefix(result.Errors[0].Error, "googleapi: Error 400: Glob pattern") {
		// the pattern is built differently per run, but a single failure should be representative for all
		typedFailureResponse(w, http.StatusBadRequest, ParameterInvalid, "pathGlob", "invalid pattern according to "+result.Errors[0].Error)
		return
	}
	// The request is good, return as OK; even if we fail at getting results, just note errors
	api.RespondWithJSON(http.StatusOK, w, result)
}

func contentMatcherFromParams(w http.ResponseWriter, req *http.Request) (contentMatcher jobartifacts.ContentMatcher, failed bool) {
	if contains := param.SafeRead(req, "textContains"); contains != "" {
		contextBefore, contextAfter, maxMatches, errs := jobartifacts.ParseLineMatcherParams(req)
		if contentMatcherParamFailure(w, errs) {
			return nil, true
		}
		contentMatcher = jobartifacts.NewStringMatcher(contains, contextBefore, contextAfter, maxMatches)
	} else if regexStr := param.SafeRead(req, "textRegex"); regexStr != "" {
		re, err := regexp.Compile(regexStr)
		if err != nil {
			typedFailureResponse(w, http.StatusBadRequest, ParameterInvalid, "textRegex", fmt.Sprintf("error parsing regex: %q", err))
			return nil, true
		}
		contextBefore, contextAfter, maxMatches, errs := jobartifacts.ParseLineMatcherParams(req)
		if contentMatcherParamFailure(w, errs) {
			return nil, true
		}
		contentMatcher = jobartifacts.NewRegexMatcher(re, contextBefore, contextAfter, maxMatches)
	}

	return // nil matcher means don't bother reading the artifacts, just return metadata without any matching
}

func contentMatcherParamFailure(w http.ResponseWriter, errs map[string]error) (failed bool) {
	if len(errs) == 0 {
		return false
	}
	response := map[string]interface{}{
		"code":      http.StatusBadRequest,
		"errorType": ParameterInvalid,
	}
	for name, err := range errs {
		response[name] = err.Error()  // include all the errors if multiple
		response["errorParam"] = name // but only one will be "the" error
		response["message"] = err.Error()
	}
	api.RespondWithJSON(http.StatusBadRequest, w, response)
	return true
}

func (s *Server) requireCapabilities(capabilities []string, implFn func(w http.ResponseWriter, req *http.Request)) func(http.ResponseWriter, *http.Request) {
	if s.hasCapabilities(capabilities) {
		return implFn
	}

	return func(w http.ResponseWriter, req *http.Request) {
		failureResponse(w, http.StatusNotImplemented, "This Sippy server is not capable of responding to this request.")
	}
}

func (s *Server) Serve() {
	s.determineCapabilities()

	router := mux.NewRouter()
	router.StrictSlash(true)

	// Handle serving React version of frontend with support for browser router, i.e. anything not found
	// goes to index.html
	router.PathPrefix("/sippy-ng/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs := s.sippyNG
		if r.URL.Path != "/sippy-ng/" {
			fullPath := strings.TrimPrefix(r.URL.Path, "/sippy-ng/")
			if _, err := fs.Open(fullPath); err != nil {
				if !os.IsNotExist(err) {
					w.WriteHeader(http.StatusNotFound)
					w.Header().Set("Content-Type", "text/plain")
					if _, err := w.Write([]byte(fmt.Sprintf("404 Not Found: %s", fullPath))); err != nil {
						log.WithError(err).Warningf("could not write response")
					}
					return
				}
				r.URL.Path = "/sippy-ng/"
			}
		}
		http.StripPrefix("/sippy-ng/", http.FileServer(http.FS(fs))).ServeHTTP(w, r)
	})

	router.PathPrefix("/static/").Handler(http.FileServer(http.FS(s.static)))

	// Setup MCP Server
	mcpServer := mcp.NewMCPServer(context.Background(), s.httpServer, s.db, s.bigQueryClient, s.cache)

	type apiEndpoints struct {
		EndpointPath string                                       `json:"path"`
		Description  string                                       `json:"description"`
		Capabilities []string                                     `json:"required_capabilities"`
		CacheTime    time.Duration                                `json:"cache_time"`
		Methods      []string                                     `json:"methods,omitempty"`
		HandlerFunc  func(w http.ResponseWriter, r *http.Request) `json:"-"`
	}

	var endpoints []apiEndpoints
	endpoints = []apiEndpoints{
		{
			EndpointPath: "/mcp/v1/",
			Description:  "Handles MCP Requests",
			Capabilities: []string{},
			HandlerFunc:  http.StripPrefix("/mcp/v1", mcpServer.Handler()).ServeHTTP,
		},
		{
			EndpointPath: "/api",
			Description:  "API docs",
			Methods:      []string{http.MethodGet},
			HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
				var availableEndpoints []apiEndpoints
				for _, ep := range endpoints {
					if s.hasCapabilities(ep.Capabilities) {
						availableEndpoints = append(availableEndpoints, ep)
					}
				}
				api.RespondWithJSON(http.StatusOK, w, availableEndpoints)
			},
		},
		{
			EndpointPath: "/api/job/run/summary",
			Description:  "Returns raw job run summary data including test failures and cluster operators",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonJobRunSummary,
		},
		{
			EndpointPath: "/api/job/run/payload",
			Description:  "Returns the payload a job run was using",
			Capabilities: []string{ComponentReadinessCapability},
			HandlerFunc:  s.jsonJobRunPayload,
			CacheTime:    4 * time.Hour,
		},
		{
			EndpointPath: "/api/autocomplete/{field}",
			Description:  "Autocompletes queries from database",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonAutocompleteFromDB,
		},
		{
			EndpointPath: "/api/jobs",
			Description:  "Returns a list of jobs",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonJobsReportFromDB,
		},
		{
			EndpointPath: "/api/jobs/runs",
			Description:  "Returns a report of job runs",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonJobRunsReportFromDB,
		},
		{
			EndpointPath: "/api/jobs/runs/risk_analysis",
			Description:  "Analyzes risks of job runs",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonJobRunRiskAnalysis,
		},
		{
			EndpointPath: "/api/jobs/runs/intervals",
			Description:  "Reports intervals of job runs",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    4 * time.Hour,
			HandlerFunc:  s.jsonJobRunIntervals,
		},
		{
			EndpointPath: "/api/jobs/analysis",
			Description:  "Analyzes jobs from the database",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonJobsAnalysisFromDB,
		},
		{
			EndpointPath: "/api/jobs/details",
			Description:  "Reports details of jobs",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonJobsDetailsReportFromDB,
		},
		{
			EndpointPath: "/api/jobs/bugs",
			Description:  "Reports bugs related to jobs",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonJobBugsFromDB,
		},
		{
			EndpointPath: "/api/jobs/artifacts",
			Description:  "Queries job artifacts and their contents",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.queryJobArtifacts,
		},
		{
			EndpointPath: "/api/jobs/labels",
			Description:  "List all job run label definitions",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonListLabels,
		},
		{
			EndpointPath: "/api/jobs/labels",
			Description:  "Create a new job run label definition",
			Methods:      []string{http.MethodPost},
			Capabilities: []string{LocalDBCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonCreateLabel,
		},
		{
			EndpointPath: "/api/jobs/labels/{id}",
			Description:  "Get a specific job run label definition",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonGetLabel,
		},
		{
			EndpointPath: "/api/jobs/labels/{id}",
			Description:  "Update a job run label definition",
			Methods:      []string{http.MethodPut},
			Capabilities: []string{LocalDBCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonUpdateLabel,
		},
		{
			EndpointPath: "/api/jobs/labels/{id}",
			Description:  "Delete a job run label definition",
			Methods:      []string{http.MethodDelete},
			Capabilities: []string{LocalDBCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonDeleteLabel,
		},
		{
			EndpointPath: "/api/jobs/symptoms",
			Description:  "List all job run symptom definitions",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonListSymptoms,
		},
		{
			EndpointPath: "/api/jobs/symptoms",
			Description:  "Create a new job run symptom definition",
			Methods:      []string{http.MethodPost},
			Capabilities: []string{LocalDBCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonCreateSymptom,
		},
		{
			EndpointPath: "/api/jobs/symptoms/{id}",
			Description:  "Get a specific job run symptom definition",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonGetSymptom,
		},
		{
			EndpointPath: "/api/jobs/symptoms/{id}",
			Description:  "Update a job run symptom definition",
			Methods:      []string{http.MethodPut},
			Capabilities: []string{LocalDBCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonUpdateSymptom,
		},
		{
			EndpointPath: "/api/jobs/symptoms/{id}",
			Description:  "Delete a job run symptom definition",
			Methods:      []string{http.MethodDelete},
			Capabilities: []string{LocalDBCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonDeleteSymptom,
		},
		{
			EndpointPath: "/api/job_variants",
			Description:  "Reports all job variants defined in BigQuery",
			Capabilities: []string{ComponentReadinessCapability},
			HandlerFunc:  s.jsonJobVariantsFromBigQuery,
		},
		{
			EndpointPath: "/api/pull_requests",
			Description:  "Reports on pull requests",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonPullRequestsReportFromDB,
		},
		{
			EndpointPath: "/api/repositories",
			Description:  "Reports on repositories",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonRepositoriesReportFromDB,
		},
		{
			EndpointPath: "/api/tests",
			Description:  "Reports on tests",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestsReportFromDB,
		},
		{
			EndpointPath: "/api/tests/v2",
			Description:  "Reports on tests",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestsReportFromBigQuery,
		},
		{
			EndpointPath: "/api/tests/details",
			Description:  "Details of tests",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestDetailsReportFromDB,
		},
		{
			EndpointPath: "/api/tests/analysis/overall",
			Description:  "Overall analysis of tests",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestAnalysisOverallFromDB,
		},
		{
			EndpointPath: "/api/tests/analysis/variants",
			Description:  "Analysis of test by variants",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestAnalysisByVariantFromDB,
		},
		{
			EndpointPath: "/api/tests/analysis/jobs",
			Description:  "Analysis of tests by job",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestAnalysisByJobFromDB,
		},
		{
			EndpointPath: "/api/tests/bugs",
			Description:  "Reports bugs in tests",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestBugsFromDB,
		},
		{
			EndpointPath: "/api/tests/outputs",
			Description:  "Outputs of tests",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestOutputsFromDB,
		},
		{
			EndpointPath: "/api/tests/durations",
			Description:  "Durations of tests",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestDurationsFromDB,
		},
		{
			EndpointPath: "/api/tests/capabilities",
			Description:  "Returns list of available test capabilities",
			Capabilities: []string{ComponentReadinessCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestCapabilitiesFromDB,
		},
		{
			EndpointPath: "/api/tests/lifecycles",
			Description:  "Returns list of available test lifecycles",
			Capabilities: []string{ComponentReadinessCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonTestLifecyclesFromDB,
		},
		{
			EndpointPath: "/api/install",
			Description:  "Reports on installations",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonInstallReportFromDB,
		},
		{
			EndpointPath: "/api/upgrade",
			Description:  "Reports on upgrades",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonUpgradeReportFromDB,
		},
		{
			EndpointPath: "/api/releases",
			Description:  "Reports on releases",
			Capabilities: []string{},
			HandlerFunc:  s.jsonReleasesReportFromDB,
		},
		{
			EndpointPath: "/api/health/build_cluster/analysis",
			Description:  "Analyzes build cluster health",
			Capabilities: []string{LocalDBCapability, BuildClusterCapability},
			HandlerFunc:  s.jsonBuildClusterHealthAnalysis,
		},
		{
			EndpointPath: "/api/health/build_cluster",
			Description:  "Reports health of build cluster",
			Capabilities: []string{LocalDBCapability, BuildClusterCapability},
			HandlerFunc:  s.jsonBuildClusterHealth,
		},
		{
			EndpointPath: "/api/health",
			Description:  "Reports general health from DB",
			Capabilities: []string{LocalDBCapability},
			CacheTime:    1 * time.Hour,
			HandlerFunc:  s.jsonHealthReportFromDB,
		},
		{
			EndpointPath: "/api/variants",
			Description:  "Reports on variants",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonVariantsReportFromDB,
		},
		{
			EndpointPath: "/api/canary",
			Description:  "Displays canary report from database",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.printCanaryReportFromDB,
		},
		{
			EndpointPath: "/api/report_date",
			Description:  "Displays report date",
			HandlerFunc:  s.printReportDate,
		},
		{
			EndpointPath: "/api/component_readiness",
			Description:  "Reports component readiness from BigQuery",
			Capabilities: []string{ComponentReadinessCapability},
			HandlerFunc:  s.jsonComponentReportFromBigQuery,
		},
		{
			EndpointPath: "/api/component_readiness/test_details",
			Description:  "Reports test details for component readiness from BigQuery",
			Capabilities: []string{ComponentReadinessCapability},
			HandlerFunc:  s.jsonComponentReportTestDetailsFromBigQuery,
		},
		{
			EndpointPath: "/api/component_readiness/variants",
			Description:  "Reports test variants for component readiness from BigQuery",
			Capabilities: []string{ComponentReadinessCapability},
			HandlerFunc:  s.jsonComponentTestVariantsFromBigQuery,
		},
		{
			EndpointPath: "/api/component_readiness/views",
			Description:  "Lists all predefined server-side views over ComponentReadiness data",
			Capabilities: []string{ComponentReadinessCapability},
			HandlerFunc:  s.jsonComponentReadinessViews,
		},
		{
			EndpointPath: "/api/component_readiness/triages",
			Description:  "List component readiness regression triage records",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability},
			HandlerFunc:  s.jsonGetTriages,
		},
		{
			EndpointPath: "/api/component_readiness/triages",
			Description:  "Create component readiness regression triage record",
			Methods:      []string{http.MethodPost},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonCreateTriage,
		},
		{
			EndpointPath: "/api/component_readiness/triages/{id}",
			Description:  "Get specific component readiness regression triage record",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability},
			HandlerFunc:  s.jsonGetTriageByID,
		},
		{
			EndpointPath: "/api/component_readiness/triages/{id}",
			Description:  "Update component readiness regression triage record",
			Methods:      []string{http.MethodPut},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonUpdateTriage,
		},
		{
			EndpointPath: "/api/component_readiness/triages/{id}",
			Description:  "Delete component readiness regression triage record",
			Methods:      []string{http.MethodDelete},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonDeleteTriage,
		},
		{
			EndpointPath: "/api/component_readiness/triages/{id}/matches",
			Description:  "List potential matching regressions for a given triage.",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability},
			HandlerFunc:  s.jsonTriagePotentialMatchingRegressions,
		},
		{
			EndpointPath: "/api/component_readiness/triages/{id}/audit",
			Description:  "Get audit logs for a given triage.",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability},
			HandlerFunc:  s.jsonGetTriageAuditDetails,
		},
		{
			EndpointPath: "/api/component_readiness/regressions",
			Description:  "List component readiness test regressions. Supports view OR release query parameters (not both).",
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability},
			HandlerFunc:  s.jsonGetRegressions,
		},
		{
			EndpointPath: "/api/component_readiness/regressions/{id}",
			Description:  "Get specific component readiness regression record",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability},
			HandlerFunc:  s.jsonGetRegressionByID,
		},
		{
			EndpointPath: "/api/component_readiness/regressions/{id}/matches",
			Description:  "List potential matching regressions for a given triage.",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{LocalDBCapability, ComponentReadinessCapability},
			HandlerFunc:  s.jsonRegressionPotentialMatchingTriages,
		},
		{
			EndpointPath: "/api/component_readiness/bugs",
			Description:  "Create Jira Bugs from component readiness",
			Capabilities: []string{WriteEndpointsCapability, ComponentReadinessCapability},
			HandlerFunc:  s.jsonFileJiraBug,
		},
		{
			EndpointPath: "/api/capabilities",
			Description:  "Lists available API capabilities",
			Capabilities: []string{},
			HandlerFunc:  s.jsonCapabilitiesReport,
		},
		{
			EndpointPath: "/api/releases/health",
			Description:  "Reports health of releases",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonReleaseHealthReport,
		},
		{
			EndpointPath: "/api/releases/tags/events",
			Description:  "Lists events for release tags",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonReleaseTagsEvent,
		},
		{
			EndpointPath: "/api/releases/tags",
			Description:  "Lists release tags",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonReleaseTagsReport,
		},
		{
			EndpointPath: "/api/releases/pull_requests",
			Description:  "Reports pull requests for releases",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonReleasePullRequestsReport,
		},
		{
			EndpointPath: "/api/releases/job_runs",
			Description:  "Lists job runs for releases",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonListPayloadJobRuns,
		},
		{
			EndpointPath: "/api/incidents",
			Description:  "Reports incident events",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonIncidentEvent,
		},
		{
			EndpointPath: "/api/releases/test_failures",
			Description:  "Analysis of test failures for releases",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonGetPayloadAnalysis,
		},
		{
			EndpointPath: "/api/payloads/test_failures",
			Description:  "Analysis of test failures in payloads",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonGetPayloadTestFailures,
		},
		{
			EndpointPath: "/api/payloads/diff",
			Description:  "Reports pull requests that differ between payloads",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonPayloadDiff,
		},
		{
			EndpointPath: "/api/feature_gates",
			Description:  "Reports feature gates and their test counts for a particular release",
			Capabilities: []string{LocalDBCapability},
			HandlerFunc:  s.jsonFeatureGates,
		},
		{
			EndpointPath: "/api/chat",
			Description:  "HTTP proxy for REST API requests to sippy-chat service",
			Capabilities: []string{ChatCapability},
			HandlerFunc:  s.handleChatProxy,
		},
		{
			EndpointPath: "/api/chat/stream",
			Description:  "Websocket proxy for chat API requests to sippy-chat service (supports HTTP and WebSocket)",
			Capabilities: []string{ChatCapability},
			HandlerFunc:  s.handleChatProxy,
		},
		{
			EndpointPath: "/api/chat/personas",
			Description:  "Proxy for listing personas from sippy-chat service.",
			Capabilities: []string{ChatCapability},
			HandlerFunc:  s.handleChatProxy,
		},
		{
			EndpointPath: "/api/chat/models",
			Description:  "Proxy for listing available models from sippy-chat service.",
			Capabilities: []string{ChatCapability},
			HandlerFunc:  s.handleChatProxy,
		},
		{
			EndpointPath: "/api/chat/prompts",
			Description:  "Proxy for listing available prompt templates from sippy-chat service.",
			Capabilities: []string{ChatCapability},
			HandlerFunc:  s.handleChatProxy,
		},
		{
			EndpointPath: "/api/chat/prompts/render",
			Description:  "Proxy for rendering prompt templates from sippy-chat service.",
			Methods:      []string{http.MethodPost},
			Capabilities: []string{ChatCapability},
			HandlerFunc:  s.handleChatProxy,
		},
		{
			EndpointPath: "/api/chat/ratings",
			Description:  "Create a chat rating record",
			Methods:      []string{http.MethodPost},
			Capabilities: []string{LocalDBCapability, ChatCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonCreateChatRating,
		},
		{
			EndpointPath: "/api/chat/conversations",
			Description:  "Create a new chat conversation",
			Methods:      []string{http.MethodPost},
			Capabilities: []string{ChatCapability, WriteEndpointsCapability},
			HandlerFunc:  s.jsonCreateChatConversation,
		},
		{
			EndpointPath: "/api/chat/conversations/{id}",
			Description:  "Get a specific chat conversation by ID",
			Methods:      []string{http.MethodGet},
			Capabilities: []string{ChatCapability},
			HandlerFunc:  s.jsonGetChatConversation,
		},
	}

	for _, ep := range endpoints {
		fn := ep.HandlerFunc
		if ep.CacheTime > 0 {
			fn = s.cached(ep.CacheTime, fn)
		}
		if len(ep.Capabilities) > 0 {
			fn = s.requireCapabilities(ep.Capabilities, fn)
		}

		// Register endpoint with proper HTTP methods
		route := router.HandleFunc(ep.EndpointPath, fn)
		if len(ep.Methods) > 0 {
			route.Methods(ep.Methods...)
		}
	}

	// Catch-all fallback: serve static files for any unmatched routes, or redirect to sippy-ng
	router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the file from static filesystem (embedded FS keeps directory structure)
		filePath := "static" + r.URL.Path
		if _, err := s.static.Open(filePath); err != nil {
			// File doesn't exist in static, redirect to sippy-ng
			if r.URL.Path == "/" {
				http.Redirect(w, r, "/sippy-ng/", http.StatusMovedPermanently)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		// File exists, rewrite path to include /static prefix and serve
		r.URL.Path = "/static" + r.URL.Path
		http.FileServer(http.FS(s.static)).ServeHTTP(w, r)
	})

	var handler http.Handler = router
	handler = logRequestHandler(handler)

	// Middleware for http metrics
	metricsMiddleware := middleware.New(middleware.Config{
		Recorder: metrics.NewRecorder(metrics.Config{
			DurationBuckets: []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120, 300},
		}),
	})
	handler = middlewarestd.Handler("", metricsMiddleware, handler)
	cors := handlers.CORS(
		handlers.AllowedOrigins([]string{s.corsAllowedOrigin}),
		handlers.AllowedMethods([]string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}),
		handlers.AllowedHeaders([]string{"Content-Type", "X-Forwarded-User", "X-Forwarded-For", "X-Real-IP", "Authorization"}))

	// Store a pointer to the HTTP server for later retrieval.
	s.httpServer = &http.Server{
		Addr:              s.listenAddr,
		Handler:           cors(handler),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Infof("Serving reports on %s ", s.listenAddr)

	if err := s.httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.WithError(err).Error("Server exited")
	}
}

func logRequestHandler(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		log.WithFields(log.Fields{
			"uri":       r.URL.String(),
			"method":    r.Method,
			"elapsed":   time.Since(start),
			"requestor": getRequestorIP(r),
		}).Info("responded to request")
	}
	return http.HandlerFunc(fn)
}

func getRequestorIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For may contain a list of IPs, the first one is the client's IP
		parts := strings.Split(xff, ",")
		clientIP := strings.TrimSpace(parts[0])
		if net.ParseIP(clientIP) != nil {
			return clientIP
		}
	}

	xri := r.Header.Get("X-Real-IP")
	if xri != "" && net.ParseIP(xri) != nil {
		return xri
	}

	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(remoteIP) != nil {
		return remoteIP
	}

	return "unknown"
}

func (s *Server) cached(duration time.Duration, handler func(w http.ResponseWriter, r *http.Request)) func(http.ResponseWriter, *http.Request) {
	if s.cache == nil {
		log.Debugf("no cache configured, making live api call")
		return handler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		content, err := s.cache.Get(context.TODO(), r.RequestURI, duration)
		if err != nil { // cache miss
			log.WithError(err).Debugf("cache miss: could not fetch data from cache for %q", r.RequestURI)
		} else if content != nil && respondFromCache(content, w, r) == nil { // cache hit
			return
		}
		recordResponse(s.cache, duration, w, r, handler)
	}
}

func respondFromCache(content []byte, w http.ResponseWriter, r *http.Request) error {
	apiResponse := cache.APIResponse{}
	if err := json.Unmarshal(content, &apiResponse); err != nil {
		log.WithError(err).Warningf("couldn't unmarshal api response")
		return err
	}
	log.Debugf("cache hit for %q", r.RequestURI)
	for k, v := range apiResponse.Headers {
		w.Header()[k] = v
	}
	w.Header().Set("X-Sippy-Cached", "true")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(apiResponse.Response); err != nil {
		log.WithError(err).Debugf("error writing http response")
		return err
	}

	return nil
}

func recordResponse(c cache.Cache, duration time.Duration, w http.ResponseWriter, r *http.Request, handler func(w http.ResponseWriter, r *http.Request)) {
	apiResponse := cache.APIResponse{}
	recorder := httptest.NewRecorder()
	handler(recorder, r)
	apiResponse.Headers = w.Header()
	for k, v := range recorder.Result().Header {
		w.Header()[k] = v
	}
	w.WriteHeader(recorder.Code)
	content := recorder.Body.Bytes()
	apiResponse.Response = content

	log.Debugf("caching new page: %s for %s\n", r.RequestURI, duration)
	apiResponseBytes, err := json.Marshal(apiResponse)
	if err != nil {
		log.WithError(err).Warningf("couldn't marshal api response")
	}

	if err := c.Set(context.TODO(), r.RequestURI, apiResponseBytes, duration); err != nil {
		log.WithError(err).Warningf("could not cache page")
	}
	if _, err := w.Write(content); err != nil {
		log.WithError(err).Debugf("error writing http response")
	}
}

func (s *Server) GetHTTPServer() *http.Server {
	return s.httpServer
}

// handleChatProxy handles proxying requests to the sippy-chat service
func (s *Server) handleChatProxy(w http.ResponseWriter, r *http.Request) {
	if s.chatAPIURL == "" {
		http.Error(w, "Chat API not configured", http.StatusServiceUnavailable)
		return
	}

	// Create chat proxy if not already created
	chatProxy, err := NewChatProxy(s.chatAPIURL)
	if err != nil {
		log.WithError(err).Error("Failed to create chat proxy")
		http.Error(w, "Failed to initialize chat proxy", http.StatusInternalServerError)
		return
	}

	// Proxy the request
	chatProxy.ServeHTTP(w, r)
}

// jsonCreateChatRating handles POST requests to create a new chat rating record
func (s *Server) jsonCreateChatRating(w http.ResponseWriter, req *http.Request) {
	var rating models.ChatRating
	if err := json.NewDecoder(req.Body).Decode(&rating); err != nil {
		log.WithError(err).Error("error parsing chat rating")
		failureResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// Create the rating in the database
	if err := s.db.DB.Create(&rating).Error; err != nil {
		log.WithError(err).Error("error creating chat rating")
		failureResponse(w, http.StatusInternalServerError, "failed to create rating")
		return
	}

	log.Infof("created chat rating with ID %d, rating: %d", rating.ID, rating.Rating)
	api.RespondWithJSON(http.StatusCreated, w, rating)
}
