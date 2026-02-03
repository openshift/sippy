package bqlabel

import (
	"os"
	"strings"
	"unicode"

	"cloud.google.com/go/bigquery"
	"github.com/sirupsen/logrus"
)

type Context struct {
	OperationalContext
	RequestContext
}

// OperationalContext defines static labels that are known when the runtime begins
type OperationalContext struct {
	App         AppValue // the runtime in use, mostly "sippy"
	Command     string   // subcommand e.g. "load"
	Environment EnvValue // where/how is this running e.g. cli, cloud function, cron
	Host        string   // hostname... often a pod, or a developer's host
	Operator    string   // who is responsible for the execution e.g. developer or service account
}

// RequestContext defines labels that are set dynamically based on request context
type RequestContext struct {
	Query   QueryValue // consistent name for query being run, e.g. "component-readiness-job-variants"
	User    string     // username that made the request if known, otherwise the operator
	URIPath string     // path of the request (for web requests, e.g. "/api/v1/release")
}
type AppValue string
type EnvValue string
type QueryValue string

const (
	KeyApp      = "client-application"
	KeyCmd      = "client-command"
	KeyEnv      = "client-env"
	KeyHost     = "client-host"
	KeyOperator = "client-operator"
	KeyQuery    = "query-details"
	KeyUser     = "request-user"
	KeyURI      = "request-uri"

	// valid values for App

	AppSippy        AppValue = "sippy"
	AppCiDataLoader AppValue = "ci-data-loader"

	// valid values for Environment

	EnvCli           EnvValue = "cli"
	EnvDaemon        EnvValue = "daemon"
	EnvWeb           EnvValue = "sippy-web"
	EnvWebAuth       EnvValue = "sippy-auth"
	EnvWebQE         EnvValue = "sippy-qe"
	EnvCron          EnvValue = "cron"
	EnvCloudFunction EnvValue = "cloud-function"

	// valid values for Query

	CRJobVariants                       QueryValue = "component-readiness-job-variants"
	CRJunitColumnCount                  QueryValue = "component-readiness-junit-column-count"
	CRJunitBase                         QueryValue = "component-readiness-junit-base"
	CRJunitSample                       QueryValue = "component-readiness-junit-sample"
	CRJunitFallback                     QueryValue = "component-readiness-junit-fallback"
	TDJunitBase                         QueryValue = "test-details-junit-base"
	TDJunitSample                       QueryValue = "test-details-junit-sample"
	CRTriagedIssues                     QueryValue = "component-readiness-triaged-issues"
	CRTriagedModifiedTime               QueryValue = "component-readiness-triaged-modified-time"
	CRCurrentRegressions                QueryValue = "component-readiness-current-regressions"
	CRUpdateRegressionClosed            QueryValue = "component-readiness-update-regression-closed"
	DisruptionDelta                     QueryValue = "disruption-delta"
	ReleaseAllReleases                  QueryValue = "release-all-releases"
	BugLoaderJobBugMappings             QueryValue = "bug-loader-job-bug-mappings"
	BugLoaderTestBugMappings            QueryValue = "bug-loader-test-bug-mappings"
	ProwLoaderProwJobs                  QueryValue = "prow-loader-prow-jobs"
	VariantRegistryDeleteJobBatch       QueryValue = "variant-registry-delete-job-batch"
	VariantRegistryDeleteVariant        QueryValue = "variant-registry-delete-variant"
	VariantRegistryUpdateVariant        QueryValue = "variant-registry-update-variant"
	VariantRegistryLoadCurrentVariants  QueryValue = "variant-registry-load-current-variants"
	VariantRegistryLoadExpectedVariants QueryValue = "variant-registry-load-expected-variants"
	TestOutputs                         QueryValue = "test-outputs"
	TestResults                         QueryValue = "test-results"
	TestResultsOverall                  QueryValue = "test-results-overall"
	TestCapabilities                    QueryValue = "test-capabilities"
	TestLifecycles                      QueryValue = "test-lifecycles"
	JobRunPayload                       QueryValue = "job-run-payload"
	PRTestResults                       QueryValue = "pr-test-results"
	CacheLookup                         QueryValue = "cache-lookup"
)

// sanitizeLabelValue sanitizes a label value to meet BigQuery requirements:
// - Lowercase all characters
// - Replace invalid characters with underscores
// - Truncate to 63 characters maximum
// Valid characters are: lowercase letters, digits, underscores, and dashes
func sanitizeLabelValue(value string) string {
	if value == "" {
		return ""
	}

	// Lowercase the entire string
	value = strings.ToLower(value)

	// Replace invalid characters with underscores
	runes := []rune(value)
	for idx, r := range runes {
		if !(unicode.IsLower(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			runes[idx] = '_'
		}
	}

	// Truncate to 63 characters
	if len(runes) > 63 {
		runes = runes[:63]
	}

	return string(runes)
}

func (x Context) ApplyLabels(query *bigquery.Query) {
	labels := map[string]string{
		KeyApp:      string(x.App),
		KeyCmd:      x.Command,
		KeyEnv:      string(x.Environment),
		KeyHost:     x.Host,
		KeyOperator: x.Operator,
		KeyUser:     x.User,
		KeyQuery:    string(x.Query),
	}
	if x.Operator == "" {
		labels[KeyOperator] = os.Getenv("USER")
	}
	if x.User == "" {
		labels[KeyUser] = os.Getenv("USER")
	}
	if x.Host == "" {
		labels[KeyHost] = os.Getenv("HOSTNAME")
	}
	if x.URIPath != "" {
		labels[KeyURI] = x.URIPath
	}

	// Sanitize all label values to meet BigQuery requirements
	sanitizedLabels := make(map[string]string, len(labels))
	for key, value := range labels {
		sanitizedLabels[key] = sanitizeLabelValue(value)
	}

	logrus.Debugf("Sanitized labels for query: %+v", sanitizedLabels)
	query.Labels = sanitizedLabels
}
