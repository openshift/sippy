package sippyserver

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/filter"
	"github.com/openshift/sippy/pkg/sippyserver/metrics"
	"github.com/openshift/sippy/pkg/synthetictests"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	workloadmetricsv1 "github.com/openshift/sippy/pkg/apis/workloadmetrics/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/query"
	"github.com/openshift/sippy/pkg/perfscaleanalysis"
	"github.com/openshift/sippy/pkg/testidentification"
)

// Mode defines the server mode of operation, OpenShift or upstream Kubernetes.
type Mode string

const (
	ModeOpenShift  Mode = "openshift"
	ModeKubernetes Mode = "kube"
)

func NewServer(
	mode Mode,
	testGridLoadingConfig TestGridLoadingConfig,
	rawJobResultsAnalysisOptions RawJobResultsAnalysisConfig,
	displayDataOptions DisplayDataConfig,
	dashboardCoordinates []TestGridDashboardCoordinates,
	listenAddr string,
	syntheticTestManager synthetictests.SyntheticTestManager,
	variantManager testidentification.VariantManager,
	bugCache buganalysis.BugCache,
	sippyNG fs.FS,
	static fs.FS,
	dbClient *db.DB,
) *Server {

	server := &Server{
		mode:                 mode,
		listenAddr:           listenAddr,
		dashboardCoordinates: dashboardCoordinates,

		syntheticTestManager: syntheticTestManager,
		variantManager:       variantManager,
		bugCache:             bugCache,
		testReportGeneratorConfig: TestReportGeneratorConfig{
			TestGridLoadingConfig:       testGridLoadingConfig,
			RawJobResultsAnalysisConfig: rawJobResultsAnalysisOptions,
			DisplayDataConfig:           displayDataOptions,
		},
		sippyNG: sippyNG,
		static:  static,
		db:      dbClient,
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
	dashboardCoordinates []TestGridDashboardCoordinates

	syntheticTestManager       synthetictests.SyntheticTestManager
	variantManager             testidentification.VariantManager
	bugCache                   buganalysis.BugCache
	testReportGeneratorConfig  TestReportGeneratorConfig
	perfscaleMetricsJobReports []workloadmetricsv1.WorkloadMetricsRow
	sippyNG                    fs.FS
	static                     fs.FS
	httpServer                 *http.Server
	db                         *db.DB
}

type TestGridDashboardCoordinates struct {
	// this is how we index and display.  it gets wired to ?release for now
	ReportName string
	// this is generic and is required
	TestGridDashboardNames []string
	// this is openshift specific, used for BZ lookup and not required
	BugzillaRelease string
}

func (s *Server) refresh(w http.ResponseWriter, req *http.Request) {
	s.RefreshData(false)

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) refreshMetrics() {
	err := metrics.RefreshMetricsDB(s.db)
	if err != nil {
		log.WithError(err).Error("error refreshing metrics")
	}
}

// refreshMaterializedViews updates the postgresql materialized views backing our reports. It is called by the handler
// for the /refresh API endpoint, which is called by the sidecar script which loads the new data from testgrid into the
// main postgresql tables.
//
// refreshMatviewOnlyIfEmpty is used on startup to indicate that we want to do an initial refresh *only* if
// the views appear to be empty.
func (s *Server) refreshMaterializedViews(refreshMatviewOnlyIfEmpty bool) {
	log.Info("refreshing materialized views")
	allStart := time.Now()

	if s.db == nil {
		log.Info("skipping materialized view refresh as server has no db connection provided")
		return
	}
	// create a channel for work "tasks"
	ch := make(chan string)

	wg := sync.WaitGroup{}

	// allow concurrent workers for refreshing matviews in parallel
	for t := 0; t < 3; t++ {
		wg.Add(1)
		go refreshMatview(s.db, refreshMatviewOnlyIfEmpty, ch, &wg)
	}

	for _, pmv := range db.PostgresMatViews {
		ch <- pmv.Name
	}

	close(ch)
	wg.Wait()

	allElapsed := time.Since(allStart)
	log.WithField("elapsed", allElapsed).Info("refreshed all materialized views")
	allMatViewsRefreshMetric.Observe(float64(allElapsed.Milliseconds()))
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

func (s *Server) RefreshData(refreshMatviewsOnlyIfEmpty bool) {
	log.Infof("Refreshing data")

	s.refreshMaterializedViews(refreshMatviewsOnlyIfEmpty)

	// TODO: skip if not enabled or data does not exist.
	// Load the scale job reports from disk:
	scaleJobsFilePath := filepath.Join(s.testReportGeneratorConfig.TestGridLoadingConfig.LocalData,
		perfscaleanalysis.ScaleJobsSubDir, perfscaleanalysis.ScaleJobsFilename)
	if _, err := os.Stat(scaleJobsFilePath); err == nil {
		log.Debugf("loading scale job data from: %s", scaleJobsFilePath)
		jsonFile, err := os.Open(scaleJobsFilePath)
		if err != nil {
			log.Errorf("error opening %s: %v", scaleJobsFilePath, err)
		}
		defer jsonFile.Close()
		scaleJobsBytes, err := ioutil.ReadAll(jsonFile)
		if err != nil {
			log.Errorf("error reading %s: %v", scaleJobsFilePath, err)
		}
		err = json.Unmarshal(scaleJobsBytes, &s.perfscaleMetricsJobReports)
		if err != nil {
			log.Errorf("error parsing json from %s: %v", scaleJobsFilePath, err)
		}
	}

	s.refreshMetrics()

	log.Infof("Refresh complete")
}

func (s *Server) reportNameToDashboardCoordinates(reportName string) (TestGridDashboardCoordinates, bool) {
	for _, dashboard := range s.dashboardCoordinates {
		if dashboard.ReportName == reportName {
			return dashboard, true
		}
	}
	return TestGridDashboardCoordinates{}, false
}

func (s *Server) reportNames() []string {
	ret := []string{}
	for _, dashboard := range s.dashboardCoordinates {
		ret = append(ret, dashboard.ReportName)
	}
	return ret
}

func (s *Server) jsonCapabilitiesReport(w http.ResponseWriter, _ *http.Request) {
	capabilities := make([]string, 0)
	if s.mode == ModeOpenShift {
		capabilities = append(capabilities, "openshift_releases")
	}

	if hasBuildCluster, err := query.HasBuildClusterData(s.db); hasBuildCluster {
		capabilities = append(capabilities, "build_clusters")
	} else if err != nil {
		log.WithError(err).Warningf("could not fetch build cluster data")
	}

	api.RespondWithJSON(http.StatusOK, w, capabilities)
}

func (s *Server) jsonAutocompleteFromDB(w http.ResponseWriter, req *http.Request) {
	api.PrintAutocompleteFromDB(w, req, s.db)
}

func (s *Server) jsonReleaseTagsReport(w http.ResponseWriter, req *http.Request) {
	api.PrintReleasesReport(w, req, s.db)
}

func (s *Server) jsonReleaseTagsEvent(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		filterOpts, err := filter.FilterOptionsFromRequest(req, "release_time", apitype.SortDescending)
		if err != nil {
			api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError,
				"message": "couldn't parse filter opts " + err.Error()})
			return
		}

		start, err := getISO8601Date("start", req)
		if err != nil {
			api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError,
				"message": "couldn't parse start param" + err.Error()})
			return
		}

		end, err := getISO8601Date("end", req)
		if err != nil {
			api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError,
				"message": "couldn't parse start param" + err.Error()})
			return
		}

		results, err := api.GetPayloadEvents(s.db, release, filterOpts, start, end)
		if err != nil {
			api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError,
				"message": "couldn't parse start param" + err.Error()})
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
	release := req.URL.Query().Get("release")
	filterOpts, err := filter.FilterOptionsFromRequest(req, "id", apitype.SortDescending)
	if err != nil {
		log.WithError(err).Error("error")
		api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError,
			"message": "Error building job run report:" + err.Error()})
		return
	}

	payloadJobRuns, err := api.ListPayloadJobRuns(s.db, filterOpts, release)
	if err != nil {
		log.WithError(err).Error("error listing payload job runs")
		api.RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": err.Error(),
		})
	}
	api.RespondWithJSON(http.StatusOK, w, payloadJobRuns)
}

// TODO: may want to merge with jsonReleaseHealthReport, but this is a fair bit slower, and release health is run
// on startup many times over when we calculate the metrics.
// if we could boil the go logic for building this down into a query, it could become another matview and then
// could be run quickly, assembling into the health api much more easily
func (s *Server) jsonGetPayloadAnalysis(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	if release == "" {
		api.RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Errorf(`"release" is required`),
		})
		return
	}
	stream := req.URL.Query().Get("stream")
	if release == "" {
		api.RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Errorf(`"stream" is required`),
		})
		return
	}
	arch := req.URL.Query().Get("arch")
	if release == "" {
		api.RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Errorf(`"arch" is required`),
		})
		return
	}

	filterOpts, err := filter.FilterOptionsFromRequest(req, "id", apitype.SortDescending)
	if err != nil {
		api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError, "message": err.Error()})
		return
	}

	log.WithFields(log.Fields{
		"release": release,
		"stream":  stream,
		"arch":    arch,
	}).Info("analyzing payload stream")

	result, err := api.GetPayloadStreamTestFailures(s.db, release, stream, arch, filterOpts)
	if err != nil {
		log.WithError(err).Error("error")
		api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{"code": http.StatusInternalServerError,
			"message": "Error analyzing payload: " + err.Error()})
		return
	}

	api.RespondWithJSON(http.StatusOK, w, result)
}

func (s *Server) jsonReleaseHealthReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	if release == "" {
		api.RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": fmt.Errorf(`"release" is required`),
		})
		return
	}

	results, err := api.ReleaseHealthReports(s.db, release)
	if err != nil {
		log.WithError(err).Error("error generating release health report")
		api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": err.Error(),
		})
		return
	}

	api.RespondWithJSON(http.StatusOK, w, results)
}

func (s *Server) jsonTestAnalysisReportFromDB(w http.ResponseWriter, req *http.Request) {
	testName := req.URL.Query().Get("test")
	if testName == "" {
		api.RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    http.StatusBadRequest,
			"message": "'test' is required.",
		})
		return
	}
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		err := api.PrintTestAnalysisJSONFromDB(s.db, w, req, release, testName)
		if err != nil {
			log.Errorf("error querying test analysis from db: %v", err)
		}
	}
}

func (s *Server) jsonTestsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintTestsJSONFromDB(release, w, req, s.db)
	}
}

func (s *Server) jsonTestDetailsReportFromDB(w http.ResponseWriter, req *http.Request) {
	// Filter to test names containing this query param:
	testSubstring := req.URL.Query()["test"]
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintTestsDetailsJSONFromDB(w, release, testSubstring, s.db)
	}
}

func (s *Server) jsonReleasesReportFromDB(w http.ResponseWriter, _ *http.Request) {
	type jsonResponse struct {
		Releases    []string  `json:"releases"`
		LastUpdated time.Time `json:"last_updated"`
	}

	response := jsonResponse{}
	releases, err := query.ReleasesFromDB(s.db)
	if err != nil {
		log.WithError(err).Error("error querying releases from db")
		api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": "error querying releases from db",
		})
		return
	}

	for _, release := range releases {
		response.Releases = append(response.Releases, release.Release)
	}

	type LastUpdated struct {
		Max time.Time
	}
	var lastUpdated LastUpdated
	// Assume our last update is the last time we inserted a prow job run.
	res := s.db.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&lastUpdated)
	if res.Error != nil {
		log.WithError(res.Error).Error("error querying last updated from db")
		api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": "error querying last updated from db",
		})
		return
	}

	response.LastUpdated = lastUpdated.Max
	api.RespondWithJSON(http.StatusOK, w, response)
}

func (s *Server) jsonHealthReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintOverallReleaseHealthFromDB(w, s.db, release)
	}
}

func (s *Server) jsonBuildClusterHealth(w http.ResponseWriter, req *http.Request) {
	start, boundary, end := getPeriodDates("default", req)

	results, err := api.GetBuildClusterHealthReport(s.db, start, boundary, end)
	if err != nil {
		log.WithError(err).Error("error querying build cluster health from db")
		api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": "error querying build cluster health from db " + err.Error(),
		})
		return
	}

	api.RespondWithJSON(200, w, results)
}

func (s *Server) jsonBuildClusterHealthAnalysis(w http.ResponseWriter, req *http.Request) {
	period := req.URL.Query().Get("period")
	if period == "" {
		period = api.PeriodDay
	}

	results, err := api.GetBuildClusterHealthAnalysis(s.db, period)
	if err != nil {
		log.WithError(err).Error("error querying build cluster health from db")
		api.RespondWithJSON(http.StatusInternalServerError, w, map[string]interface{}{
			"code":    http.StatusInternalServerError,
			"message": "error querying build cluster health from db " + err.Error(),
		})
		return
	}

	api.RespondWithJSON(200, w, results)
}

func (s *Server) getRelease(req *http.Request) string {
	return req.URL.Query().Get("release")
}

func (s *Server) getReleaseOrFail(w http.ResponseWriter, req *http.Request) string {
	release := req.URL.Query().Get("release")

	if release == "" {
		api.RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    "400",
			"message": "release is required",
		})
		return release
	}

	return release
}

func (s *Server) jsonJobsDetailsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	jobName := req.URL.Query().Get("job")
	if release != "" && jobName != "" {
		err := api.PrintJobDetailsReportFromDB(w, req, s.db, release, jobName)
		if err != nil {
			log.Errorf("Error from PrintJobDetailsReportFromDB: %v", err)
		}
	}
}

func (s *Server) printCanaryReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintCanaryTestsFromDB(release, w, s.db)
	}
}

func (s *Server) jsonVariantsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintVariantReportFromDB(w, req, s.db, release)
	}
}

func (s *Server) jsonJobsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintJobsReportFromDB(w, req, s.db, release)
	}
}

func (s *Server) jsonJobRunsReportFromDB(w http.ResponseWriter, req *http.Request) {
	api.PrintJobsRunsReportFromDB(w, req, s.db)
}

func (s *Server) jsonJobsAnalysisFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getRelease(req)
	api.PrintJobAnalysisJSONFromDB(w, req, s.db, release)
}

func (s *Server) jsonPerfScaleMetricsReport(w http.ResponseWriter, req *http.Request) {
	reports := s.perfscaleMetricsJobReports

	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintPerfscaleWorkloadMetricsReport(w, req, release, reports)
	}
}

func (s *Server) Serve() {
	// Use private ServeMux to prevent tests from stomping on http.DefaultServeMux
	serveMux := http.NewServeMux()

	// Handle serving React version of frontend with support for browser router, i.e. anything not found
	// goes to index.html
	serveMux.HandleFunc("/sippy-ng/", func(w http.ResponseWriter, r *http.Request) {
		fs := s.sippyNG
		if r.URL.Path != "/sippy-ng/" {
			fullPath := strings.TrimPrefix(r.URL.Path, "/sippy-ng/")
			if _, err := fs.Open(fullPath); err != nil {
				if !os.IsNotExist(err) {
					panic(err)
				}
				r.URL.Path = "/sippy-ng/"
			}
		}
		http.StripPrefix("/sippy-ng/", http.FileServer(http.FS(fs))).ServeHTTP(w, r)
	})

	serveMux.Handle("/static/", http.FileServer(http.FS(s.static)))

	// Re-direct "/" to sippy-ng
	serveMux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		http.Redirect(w, req, "/sippy-ng/", 301)
	})

	serveMux.HandleFunc("/api/autocomplete/", s.jsonAutocompleteFromDB)
	serveMux.HandleFunc("/api/jobs", s.jsonJobsReportFromDB)
	serveMux.HandleFunc("/api/jobs/runs", s.jsonJobRunsReportFromDB)
	serveMux.HandleFunc("/api/jobs/analysis", s.jsonJobsAnalysisFromDB)
	serveMux.HandleFunc("/api/jobs/details", s.jsonJobsDetailsReportFromDB)
	serveMux.HandleFunc("/api/tests", s.jsonTestsReportFromDB)
	serveMux.HandleFunc("/api/tests/details", s.jsonTestDetailsReportFromDB)
	serveMux.HandleFunc("/api/tests/analysis", s.jsonTestAnalysisReportFromDB)
	serveMux.HandleFunc("/api/install", s.jsonInstallReportFromDB)
	serveMux.HandleFunc("/api/upgrade", s.jsonUpgradeReportFromDB)
	serveMux.HandleFunc("/api/releases", s.jsonReleasesReportFromDB)
	serveMux.HandleFunc("/api/health/build_cluster/analysis", s.jsonBuildClusterHealthAnalysis)
	serveMux.HandleFunc("/api/health/build_cluster", s.jsonBuildClusterHealth)
	serveMux.HandleFunc("/api/health", s.jsonHealthReportFromDB)
	serveMux.HandleFunc("/api/variants", s.jsonVariantsReportFromDB)
	serveMux.HandleFunc("/api/canary", s.printCanaryReportFromDB)

	serveMux.HandleFunc("/refresh", s.refresh)
	serveMux.HandleFunc("/api/perfscalemetrics", s.jsonPerfScaleMetricsReport)
	serveMux.HandleFunc("/api/capabilities", s.jsonCapabilitiesReport)
	if s.db != nil {
		serveMux.HandleFunc("/api/releases/health", s.jsonReleaseHealthReport)
		serveMux.HandleFunc("/api/releases/tags/events", s.jsonReleaseTagsEvent)
		serveMux.HandleFunc("/api/releases/tags", s.jsonReleaseTagsReport)
		serveMux.HandleFunc("/api/releases/pull_requests", s.jsonReleasePullRequestsReport)
		serveMux.HandleFunc("/api/releases/job_runs", s.jsonListPayloadJobRuns)

		serveMux.HandleFunc("/api/releases/test_failures",
			s.jsonGetPayloadAnalysis)
	}

	var handler http.Handler = serveMux
	// wrap mux with our logger. this will
	handler = logRequestHandler(handler)
	// ... potentially add more middleware handlers

	// Store a pointer to the HTTP server for later retrieval.
	s.httpServer = &http.Server{
		Addr:    s.listenAddr,
		Handler: handler,
	}

	log.Infof("Serving reports on %s ", s.listenAddr)

	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.WithError(err).Error("Server exited")
	}
}

func logRequestHandler(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		log.WithFields(log.Fields{
			"uri":     r.URL.String(),
			"method":  r.Method,
			"elapsed": time.Since(start),
		}).Info("responded to request")
	}
	return http.HandlerFunc(fn)
}

func (s *Server) GetHTTPServer() *http.Server {
	return s.httpServer
}
