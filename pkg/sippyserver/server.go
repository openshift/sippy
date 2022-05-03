package sippyserver

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	workloadmetricsv1 "github.com/openshift/sippy/pkg/apis/workloadmetrics/v1"
	"github.com/openshift/sippy/pkg/buganalysis"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/html/generichtml"
	"github.com/openshift/sippy/pkg/html/releasehtml"
	"github.com/openshift/sippy/pkg/perfscaleanalysis"
	"github.com/openshift/sippy/pkg/testgridanalysis/testgridconversion"
	"github.com/openshift/sippy/pkg/testgridanalysis/testidentification"
)

func NewServer(
	testGridLoadingConfig TestGridLoadingConfig,
	rawJobResultsAnalysisOptions RawJobResultsAnalysisConfig,
	displayDataOptions DisplayDataConfig,
	dashboardCoordinates []TestGridDashboardCoordinates,
	listenAddr string,
	syntheticTestManager testgridconversion.SyntheticTestManager,
	variantManager testidentification.VariantManager,
	bugCache buganalysis.BugCache,
	sippyNG fs.FS,
	static fs.FS,
	dbClient *db.DB,
	dbOnlyMOde bool,
) *Server {

	server := &Server{
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
		currTestReports: map[string]StandardReport{},
		sippyNG:         sippyNG,
		static:          static,
		db:              dbClient,
		dbOnlyMode:      dbOnlyMOde,
	}

	return server
}

type Server struct {
	listenAddr           string
	dashboardCoordinates []TestGridDashboardCoordinates

	syntheticTestManager       testgridconversion.SyntheticTestManager
	variantManager             testidentification.VariantManager
	bugCache                   buganalysis.BugCache
	testReportGeneratorConfig  TestReportGeneratorConfig
	currTestReports            map[string]StandardReport
	perfscaleMetricsJobReports []workloadmetricsv1.WorkloadMetricsRow
	sippyNG                    fs.FS
	static                     fs.FS
	httpServer                 *http.Server
	db                         *db.DB
	// dbOnlyMode disabled all use of the in-memory analysis from testgrid files on disk, instead relying on
	// the postgresql database. Swaps each API endpoint for an equivalent.
	// This flag is temporary and will eventually become the default.
	dbOnlyMode bool
}

type TestGridDashboardCoordinates struct {
	// this is how we index and display.  it gets wired to ?release for now
	ReportName string
	// this is generic and is required
	TestGridDashboardNames []string
	// this is openshift specific, used for BZ lookup and not required
	BugzillaRelease string
}

type StandardReport struct {
	CurrentPeriodReport sippyprocessingv1.TestReport
	CurrentTwoDayReport sippyprocessingv1.TestReport
	PreviousWeekReport  sippyprocessingv1.TestReport
}

func (s *Server) refresh(w http.ResponseWriter, req *http.Request) {
	s.RefreshData()

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
}

var (
	jobPassRatioMetric = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sippy_job_pass_ratio",
		Help: "Ratio of passed job runs for the given job in a period (2 day, 7 day, etc)",
	}, []string{"release", "period", "name"})

	matViewRefreshMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sippy_matview_refresh_millis",
		Help:    "Milliseconds to refresh our postgresql materialized views",
		Buckets: []float64{10, 100, 200, 500, 1000, 5000, 10000, 30000, 60000, 300000},
	}, []string{"view"})
)

func (s *Server) refreshMetrics() {

	// Report metrics for all jobs:
	for r, stdReport := range s.currTestReports {
		for _, testReport := range []sippyprocessingv1.TestReport{stdReport.CurrentTwoDayReport, stdReport.CurrentPeriodReport, stdReport.PreviousWeekReport} {
			for _, jobResult := range testReport.ByJob {
				jobPassRatioMetric.WithLabelValues(r, string(testReport.ReportType), jobResult.Name).Set(jobResult.PassPercentage / 100)
			}
		}
	}
}

// refreshMaterializedViews updates the postgresql materialized views backing our reports. It is called by the handler
// for the /refresh API endpoint, which is called by the sidecar script which loads the new data from testgrid into the
// main postgresql tables.
func (s *Server) refreshMaterializedViews() {
	log.Info("refreshing materialized views")

	if s.db == nil {
		log.Info("skipping materialized view refresh as server has no db connection provided")
		return
	}

	for _, pmv := range db.PostgresMatViews {
		start := time.Now()
		if res := s.db.DB.Exec(
			fmt.Sprintf("REFRESH MATERIALIZED VIEW %s", pmv.Name)); res.Error != nil {
			log.Errorf("error refreshing materialized view %s: %v", pmv.Name, res.Error)
		} else {
			elapsed := time.Since(start)
			log.Infof("Refreshed materialized view %s in %s", pmv.Name, elapsed)
			matViewRefreshMetric.WithLabelValues(pmv.Name).Observe(float64(elapsed.Milliseconds()))
		}
	}
}

func (s *Server) RefreshData() {
	log.Infof("Refreshing data")

	if !s.dbOnlyMode {
		s.bugCache.Clear()

		for _, dashboard := range s.dashboardCoordinates {
			s.currTestReports[dashboard.ReportName] = s.testReportGeneratorConfig.PrepareStandardTestReports(
				dashboard, s.syntheticTestManager, s.variantManager, s.bugCache)
		}
	} else {
		s.refreshMaterializedViews()
	}

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

func (s *Server) printHTMLReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	dashboard, found := s.reportNameToDashboardCoordinates(reportName)
	if !found {
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}
	if _, hasReport := s.currTestReports[dashboard.ReportName]; !hasReport {
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}

	releasehtml.PrintHTMLReport(w, req,
		s.currTestReports[dashboard.ReportName].CurrentPeriodReport,
		s.currTestReports[dashboard.ReportName].CurrentTwoDayReport,
		s.currTestReports[dashboard.ReportName].PreviousWeekReport,
		s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays,
		15,
		s.reportNames())
}

func (s *Server) printCanaryReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")
	dashboard, found := s.reportNameToDashboardCoordinates(reportName)
	if !found {
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}
	if _, hasReport := s.currTestReports[dashboard.ReportName]; !hasReport {
		releasehtml.WriteLandingPage(w, s.reportNames())
		return
	}

	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	testReport := s.currTestReports[dashboard.ReportName].CurrentPeriodReport
	for i := len(testReport.ByTest) - 1; i >= 0; i-- {
		t := testReport.ByTest[i]
		if t.TestResultAcrossAllJobs.PassPercentage > 99 {
			fmt.Fprintf(w, "%q:struct{}{},\n", t.TestName)
		} else {
			break
		}
	}
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

func (s *Server) printJSONReport(w http.ResponseWriter, req *http.Request) {
	reportName := req.URL.Query().Get("release")

	releaseReports := make(map[string][]sippyprocessingv1.TestReport)
	if reportName == "all" {
		// return all available json reports
		// store [currentReport, prevReport] in a slice
		for _, reportName := range s.reportNames() {
			if _, ok := s.currTestReports[reportName]; ok {
				releaseReports[reportName] = []sippyprocessingv1.TestReport{s.currTestReports[reportName].CurrentPeriodReport, s.currTestReports[reportName].PreviousWeekReport}
			} else {
				log.Errorf("unable to load test report for reportName version %s", reportName)
				continue
			}
		}
		api.PrintJSONReport(w, req, releaseReports, s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays, 15)
		return
	} else if _, ok := s.currTestReports[reportName]; !ok {
		api.RespondWithJSON(404, w, map[string]interface{}{
			"code":   "404",
			"detail": fmt.Sprintf("No valid reportName specified, valid reportNames are: %v", s.reportNames()),
		})

		return
	}
	releaseReports[reportName] = []sippyprocessingv1.TestReport{s.currTestReports[reportName].CurrentPeriodReport, s.currTestReports[reportName].PreviousWeekReport}
	api.PrintJSONReport(w, req, releaseReports, s.testReportGeneratorConfig.RawJobResultsAnalysisConfig.NumDays, 15)
}

func (s *Server) detailed(w http.ResponseWriter, req *http.Request) {
	reportName := "4.8"

	// Default to the first release given on the command-line
	reportNames := s.reportNames()
	if len(reportNames) > 0 {
		reportName = reportNames[0]
	}

	t := req.URL.Query().Get("release")
	if t != "" {
		reportName = t
	}

	startDay := 0
	t = req.URL.Query().Get("startDay")
	if t != "" {
		startDay, _ = strconv.Atoi(t)
	}

	numDays := 7
	t = req.URL.Query().Get("endDay")
	if t != "" {
		endDay, _ := strconv.Atoi(t)
		numDays = endDay - startDay
	}

	testSuccessThreshold := 98.0
	t = req.URL.Query().Get("testSuccessThreshold")
	if t != "" {
		testSuccessThreshold, _ = strconv.ParseFloat(t, 64)
	}

	jobFilterString := ""
	t = req.URL.Query().Get("jobFilter")
	if t != "" {
		jobFilterString = t
	}

	minTestRuns := 10
	t = req.URL.Query().Get("minTestRuns")
	if t != "" {
		minTestRuns, _ = strconv.Atoi(t)
	}

	failureClusterThreshold := 10
	t = req.URL.Query().Get("failureClusterThreshold")
	if t != "" {
		failureClusterThreshold, _ = strconv.Atoi(t)
	}

	jobTestCount := 10
	t = req.URL.Query().Get("jobTestCount")
	if t != "" {
		jobTestCount, _ = strconv.Atoi(t)
	}

	var jobFilter *regexp.Regexp
	if len(jobFilterString) > 0 {
		var err error
		jobFilter, err = regexp.Compile(jobFilterString)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid jobFilter: %s", err), http.StatusBadRequest)
			return
		}
	}

	testReportConfig := TestReportGeneratorConfig{
		TestGridLoadingConfig: TestGridLoadingConfig{
			LocalData: s.testReportGeneratorConfig.TestGridLoadingConfig.LocalData,
			Loader:    s.testReportGeneratorConfig.TestGridLoadingConfig.Loader,
			JobFilter: jobFilter,
		},
		RawJobResultsAnalysisConfig: RawJobResultsAnalysisConfig{
			StartDay: startDay,
			NumDays:  numDays,
		},
		DisplayDataConfig: DisplayDataConfig{
			MinTestRuns:             minTestRuns,
			TestSuccessThreshold:    testSuccessThreshold,
			FailureClusterThreshold: failureClusterThreshold,
		},
	}

	dashboardCoordinates, found := s.reportNameToDashboardCoordinates(reportName)
	if !found {
		releasehtml.WriteLandingPage(w, reportNames)
		return
	}
	// TODO: db connection handed off as nil here
	testReports := testReportConfig.PrepareStandardTestReports(dashboardCoordinates, s.syntheticTestManager, s.variantManager, s.bugCache)

	releasehtml.PrintHTMLReport(w, req,
		testReports.CurrentPeriodReport,
		testReports.CurrentTwoDayReport,
		testReports.PreviousWeekReport,
		numDays, jobTestCount, reportNames)

}

func (s *Server) jsonCapabilitiesReport(w http.ResponseWriter, _ *http.Request) {
	capabilities := make([]string, 0)
	if s.db != nil {
		capabilities = append(capabilities, "openshift_releases")
	}
	api.RespondWithJSON(http.StatusOK, w, capabilities)
}

func (s *Server) jsonAutocompleteFromDB(w http.ResponseWriter, req *http.Request) {
	api.PrintAutocompleteFromDB(w, req, s.db)
}

func (s *Server) jsonReleaseTagsReport(w http.ResponseWriter, req *http.Request) {
	api.PrintReleasesReport(w, req, s.db)
}
func (s *Server) jsonReleasePullRequestsReport(w http.ResponseWriter, req *http.Request) {
	api.PrintPullRequestsReport(w, req, s.db)
}

func (s *Server) jsonReleaseJobRunsReport(w http.ResponseWriter, req *http.Request) {
	api.PrintReleaseJobRunsReport(w, req, s.db)
}

func (s *Server) jsonReleaseHealthReport(w http.ResponseWriter, req *http.Request) {
	api.PrintReleaseHealthReport(w, req, s.db)
}

func (s *Server) jsonJobAnalysisReport(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		curr := s.currTestReports[release].CurrentPeriodReport
		prev := s.currTestReports[release].PreviousWeekReport

		api.PrintJobAnalysisJSON(w, req, curr, prev)
	}
}

func (s *Server) jsonTestAnalysisReport(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		curr := s.currTestReports[release].CurrentPeriodReport
		prev := s.currTestReports[release].PreviousWeekReport

		api.PrintTestAnalysisJSON(w, req, curr, prev)
	}
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
		err := api.PrintTestAnalysisJSONFromDB(s.db, w, release, testName)
		if err != nil {
			log.Errorf("error querying test analysis from db: %v", err)
		}
	}
}

func (s *Server) jsonTestsReport(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		currTests := s.currTestReports[release].CurrentPeriodReport.ByTest
		twoDay := s.currTestReports[release].CurrentTwoDayReport.ByTest
		prevTests := s.currTestReports[release].PreviousWeekReport.ByTest

		api.PrintTestsJSON(release, w, req, currTests, twoDay, prevTests)
	}
}

func (s *Server) jsonTestsReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintTestsJSONFromDB(release, w, req, s.db)
	}
}

func (s *Server) jsonTestDetailsReport(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		currTests := s.currTestReports[release].CurrentPeriodReport
		prevTests := s.currTestReports[release].PreviousWeekReport
		api.PrintTestsDetailsJSON(w, req, currTests, prevTests)
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

func (s *Server) jsonReleasesReport(w http.ResponseWriter, req *http.Request) {
	type jsonResponse struct {
		Releases    []string  `json:"releases"`
		LastUpdated time.Time `json:"last_updated"`
	}

	response := jsonResponse{}
	if len(s.dashboardCoordinates) > 0 {
		firstReport := s.dashboardCoordinates[0].ReportName
		if report, ok := s.currTestReports[firstReport]; ok {
			response.LastUpdated = report.CurrentPeriodReport.Timestamp
		}
	}

	for _, release := range s.dashboardCoordinates {
		response.Releases = append(response.Releases, release.ReportName)
	}

	api.RespondWithJSON(http.StatusOK, w, response)
}

func (s *Server) jsonReleasesReportFromDB(w http.ResponseWriter, _ *http.Request) {
	type jsonResponse struct {
		Releases    []string  `json:"releases"`
		LastUpdated time.Time `json:"last_updated"`
	}
	type Release struct {
		Release string
	}

	response := jsonResponse{}

	var releases []Release
	// The string_to_array trick ensures releases are sorted in version order, descending
	res := s.db.DB.Raw(`
		SELECT DISTINCT(release), case when position('.' in release) != 0 then string_to_array(release, '.')::int[] end as sortable_release
                FROM prow_jobs
                ORDER BY sortable_release desc`).Scan(&releases)
	if res.Error != nil {
		log.Errorf("error querying releases from db: %v", res.Error)
	}
	for _, release := range releases {
		response.Releases = append(response.Releases, release.Release)
	}

	type LastUpdated struct {
		Max time.Time
	}
	var lastUpdated LastUpdated
	// Assume our last update is the last time we inserted a prow job run.
	res = s.db.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&lastUpdated)
	if res.Error != nil {
		log.Errorf("error querying last updated from db: %v", res.Error)
	}
	response.LastUpdated = lastUpdated.Max

	api.RespondWithJSON(http.StatusOK, w, response)
}

func (s *Server) jsonHealthReport(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		curr := s.currTestReports[release].CurrentPeriodReport
		twoDay := s.currTestReports[release].CurrentTwoDayReport
		prev := s.currTestReports[release].PreviousWeekReport
		api.PrintOverallReleaseHealth(w, curr, twoDay, prev)
	}
}

func (s *Server) jsonHealthReportFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintOverallReleaseHealthFromDB(w, s.db, release)
	}
}

func (s *Server) variantsReport(w http.ResponseWriter, req *http.Request) (*sippyprocessingv1.VariantResults, *sippyprocessingv1.VariantResults) {
	release := req.URL.Query().Get("release")
	variant := req.URL.Query().Get("variant")
	reports := s.currTestReports

	if variant == "" || release == "" {
		generichtml.PrintStatusMessage(w, http.StatusBadRequest, "Please specify a variant and release.")
		return nil, nil
	}

	if _, ok := reports[release]; !ok {
		generichtml.PrintStatusMessage(w, http.StatusNotFound, fmt.Sprintf("Release %q not found.", release))
		return nil, nil
	}

	var currentWeek *sippyprocessingv1.VariantResults
	for i, report := range reports[release].CurrentPeriodReport.ByVariant {
		if report.VariantName == variant {
			currentWeek = &reports[release].CurrentPeriodReport.ByVariant[i]
			break
		}
	}

	var previousWeek *sippyprocessingv1.VariantResults
	for i, report := range reports[release].PreviousWeekReport.ByVariant {
		if report.VariantName == variant {
			previousWeek = &reports[release].PreviousWeekReport.ByVariant[i]
			break
		}
	}

	if currentWeek == nil {
		generichtml.PrintStatusMessage(w, http.StatusNotFound, fmt.Sprintf("Variant %q not found.", variant))
		return nil, nil
	}

	return currentWeek, previousWeek
}

func (s *Server) htmlVariantsReport(w http.ResponseWriter, req *http.Request) {
	release := req.URL.Query().Get("release")
	variant := req.URL.Query().Get("variant")

	current, previous := s.variantsReport(w, req)
	if current == nil {
		return
	}
	timestamp := s.currTestReports[release].CurrentPeriodReport.Timestamp
	releasehtml.PrintVariantsReport(w, release, variant, current, previous, timestamp)
}

func (s *Server) getReleaseOrFail(w http.ResponseWriter, req *http.Request) string {
	release := req.URL.Query().Get("release")
	reports := s.currTestReports

	if release == "" {
		api.RespondWithJSON(http.StatusBadRequest, w, map[string]interface{}{
			"code":    "400",
			"message": "release is required",
		})
		return release
	}

	if !s.dbOnlyMode {
		if _, ok := reports[release]; !ok {
			api.RespondWithJSON(http.StatusNotFound, w, map[string]interface{}{
				"code":    "404",
				"message": fmt.Sprintf("release %q not found", release),
			})
			return ""
		}
	}

	return release
}

func (s *Server) jsonJobsDetailsReport(w http.ResponseWriter, req *http.Request) {
	reports := s.currTestReports

	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintJobDetailsReport(w, req, reports[release].CurrentPeriodReport.ByJob, reports[release].PreviousWeekReport.ByJob)
	}
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

func (s *Server) jsonJobRunsReport(w http.ResponseWriter, req *http.Request) {
	reports := s.currTestReports
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintJobRunsReport(w, req, reports[release].CurrentPeriodReport, reports[release].PreviousWeekReport)
	}
}

func (s *Server) jsonJobsReport(w http.ResponseWriter, req *http.Request) {
	reports := s.currTestReports

	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintJobsReport(w, req, reports[release].CurrentPeriodReport, reports[release].CurrentTwoDayReport, reports[release].PreviousWeekReport)
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
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintJobsRunsReportFromDB(w, req, s.db, release)
	}
}

func (s *Server) jsonJobsAnalysisFromDB(w http.ResponseWriter, req *http.Request) {
	release := s.getReleaseOrFail(w, req)
	if release != "" {
		api.PrintJobAnalysisJSONFromDB(w, req, s.db, release)
	}
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

	// Fork the endpoints if we're using postgresql db only mode vs the old in-memory.
	// Temporary until we drop the old legacy mode.
	if s.dbOnlyMode {
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
		serveMux.HandleFunc("/api/health", s.jsonHealthReportFromDB)
		serveMux.HandleFunc("/api/variants", s.jsonVariantsReportFromDB)
	} else {
		// Preserve old sippy at /legacy for now
		serveMux.HandleFunc("/legacy", s.printHTMLReport)
		serveMux.HandleFunc("/install", s.printInstallHTMLReport)
		serveMux.HandleFunc("/upgrade", s.printUpgradeHTMLReport)
		serveMux.HandleFunc("/operator-health", s.printOperatorHealthHTMLReport)
		serveMux.HandleFunc("/testdetails", s.printTestDetailHTMLReport)
		serveMux.HandleFunc("/detailed", s.detailed)
		serveMux.HandleFunc("/canary", s.printCanaryReport)
		serveMux.HandleFunc("/variants", s.htmlVariantsReport)
		// Old API
		serveMux.HandleFunc("/json", s.printJSONReport)

		// New API's
		serveMux.HandleFunc("/api/jobs", s.jsonJobsReport)
		serveMux.HandleFunc("/api/jobs/details", s.jsonJobsDetailsReport)
		serveMux.HandleFunc("/api/jobs/analysis", s.jsonJobAnalysisReport)
		serveMux.HandleFunc("/api/jobs/runs", s.jsonJobRunsReport)

		serveMux.HandleFunc("/api/tests", s.jsonTestsReport)
		serveMux.HandleFunc("/api/tests/details", s.jsonTestDetailsReport)
		serveMux.HandleFunc("/api/tests/analysis", s.jsonTestAnalysisReport)

		serveMux.HandleFunc("/api/releases", s.jsonReleasesReport)

		serveMux.HandleFunc("/api/health", s.jsonHealthReport)
		serveMux.HandleFunc("/api/install", s.jsonInstallReport)
		serveMux.HandleFunc("/api/upgrade", s.jsonUpgradeReport)
	}

	serveMux.HandleFunc("/refresh", s.refresh)
	serveMux.HandleFunc("/api/perfscalemetrics", s.jsonPerfScaleMetricsReport)
	serveMux.HandleFunc("/api/capabilities", s.jsonCapabilitiesReport)
	if s.db != nil {
		serveMux.HandleFunc("/api/releases/health", s.jsonReleaseHealthReport)
		serveMux.HandleFunc("/api/releases/tags", s.jsonReleaseTagsReport)
		serveMux.HandleFunc("/api/releases/pull_requests", s.jsonReleasePullRequestsReport)
		serveMux.HandleFunc("/api/releases/job_runs", s.jsonReleaseJobRunsReport)
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
