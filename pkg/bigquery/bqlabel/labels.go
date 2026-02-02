package bqlabel

import (
	"os"

	"cloud.google.com/go/bigquery"
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
)

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
	query.Labels = labels
}
