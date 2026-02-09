package bigquery

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"github.com/openshift/sippy/pkg/apis/cache"
)

type Client struct {
	BQ                 *bigquery.Client
	operationalContext bqlabel.OperationalContext
	Cache              cache.Cache
	Dataset            string
	ReleasesTable      string
}

func New(ctx context.Context, opCtx bqlabel.OperationalContext, c cache.Cache, credentialFile, project, dataset, releasesTable string) (*Client, error) {
	bqc, err := bigquery.NewClient(ctx, project, option.WithCredentialsFile(credentialFile))
	if err != nil {
		return nil, err
	}
	// Enable Storage API usage for fetching data
	err = bqc.EnableStorageReadClient(context.Background(), option.WithCredentialsFile(credentialFile))
	if err != nil {
		return nil, errors.WithMessage(err, "couldn't enable storage API")
	}

	return &Client{
		BQ:                 bqc,
		Cache:              c,
		operationalContext: opCtx,
		Dataset:            dataset,
		ReleasesTable:      releasesTable,
	}, nil
}

// LoggedRead is a wrapper around the bigquery Read method that logs the query being executed
func LoggedRead(ctx context.Context, q *bigquery.Query) (*bigquery.RowIterator, error) {
	log.Debugf("Querying BQ with Parameters: %v\n%v", q.Parameters, q.QueryConfig.Q)
	return q.Read(ctx)
}

// LogQueryWithParamsReplaced is intended to give developers a query they can copy out of logs and work with directly,
// which has all the parameters replaced. This query is NOT the one we run live, we let bigquery do its param replacement
// itself.
// Without this, logrus logs the query in one line with everything escaped, and parameters have to be manually replaced by the user.
// This will only log if we're logging at Debug level.
func LogQueryWithParamsReplaced(logger log.FieldLogger, query *bigquery.Query) {
	if log.GetLevel() == log.DebugLevel {
		// Attempt to log a usable version of the query with params swapped in.
		strQuery := query.Q
		for _, p := range query.Parameters {
			paramName := "@" + p.Name
			paramValue := p.Value

			switch v := paramValue.(type) {
			case time.Time:
				// Format time.Time to "YYYY-MM-DD HH:MM:SS"
				// Note: BigQuery's DATETIME type does not store timezone info.
				// This format aligns with what BigQuery expects for DATETIME literals.
				// Without it, you'll copy the query and attempt to run it and be told you're not filtering on
				// modified time.
				formattedTime := v.Format("2006-01-02 15:04:05")
				strQuery = strings.ReplaceAll(strQuery, paramName, fmt.Sprintf(`DATETIME("%s")`, formattedTime))
			case []string:
				// Convert a slice of strings to a formatted array string
				quotedArr := make([]string, len(v))
				for i, val := range v {
					quotedArr[i] = fmt.Sprintf("%q", val)
				}
				joinedStrings := strings.Join(quotedArr, ",")
				strArr := fmt.Sprintf("[%s]", joinedStrings)
				strQuery = strings.ReplaceAll(strQuery, paramName, strArr)
			default:
				// Default handling for all other types
				strQuery = strings.ReplaceAll(strQuery, paramName, fmt.Sprintf(`"%v"`, v))

			}
		}
		logger.Debugf("fetching bigquery data with query:")
		fmt.Println(strQuery)
	}
}

type BQReqCtxKey string // type for storing the bqlabel.RequestContext in the context.Context
const RequestContextKey BQReqCtxKey = "bq-request-context"

// Query is a wrapper around the bigquery Query method that adds labels to the query based on the context.
// Outside web handlers, context is often not meaningful, so it can be nil.
// The queryLabel is used to identify the query in the BigQuery metrics.
func (x *Client) Query(ctx context.Context, queryLabel bqlabel.QueryValue, sql string) *bigquery.Query {
	q := x.BQ.Query(sql)
	x.ApplyQueryLabels(ctx, queryLabel, q)
	return q
}

// ApplyQueryLabels adds labels to the query based on the context.
// This can be used directly in cases where the context is not handy where the query is created.
// Outside web handlers, context is often not meaningful, so it can be nil.
// The queryLabel is used to identify the query in the BigQuery metrics.
func (x *Client) ApplyQueryLabels(ctx context.Context, queryLabel bqlabel.QueryValue, q *bigquery.Query) {
	reqCtx := bqlabel.RequestContext{}
	if ctx != nil {
		reqCtx, _ = ctx.Value(RequestContextKey).(bqlabel.RequestContext) // remains empty if not found
	}
	reqCtx.Query = queryLabel
	bqlabel.Context{
		OperationalContext: x.operationalContext,
		RequestContext:     reqCtx,
	}.ApplyLabels(q)
}

// OpCtxForCronEnv provides standard BQ label context for a sippy command intended for use in a cron job.
// When a dev is running it by hand, queries will be labeled as CLI usage with the dev's user.
// But in a cron job with var SIPPY_CRON_ENV set (e.g. to "fetchdata"), it will be labeled for that value.
func OpCtxForCronEnv(ctx context.Context, cmd string) (bqlabel.OperationalContext, context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	opCtx := bqlabel.OperationalContext{
		App:         bqlabel.AppSippy,
		Command:     cmd,
		Environment: bqlabel.EnvCli,
	}
	if cronUser := os.Getenv("SIPPY_CRON_ENV"); cronUser != "" {
		opCtx.Environment = bqlabel.EnvCron
		opCtx.Operator = cronUser // left empty outside prod, defaulting to USER env var
		ctx = context.WithValue(ctx, RequestContextKey, bqlabel.RequestContext{User: cronUser})
	}
	return opCtx, ctx
}
