package verify

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"google.golang.org/api/iterator"
	"k8s.io/apimachinery/pkg/util/sets"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/apis/prow"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/openshift/sippy/pkg/dataloader/prowloader"
	"github.com/openshift/sippy/pkg/releaseoverride"
)

type BQCompletenessVerifier struct {
	PostgreSQL                  ProwJobRunReader
	BigQuery                    BigQueryReader
	BigQueryInitializationError error
	Config                      *v1config.SippyConfig
	SyntheticReleaseOverrides   *releaseoverride.SyntheticReleaseOverrides
}

func (v *BQCompletenessVerifier) Verify(ctx context.Context, scope Scope) Result {
	result := Result{}
	if v.BigQueryInitializationError != nil {
		for _, release := range scope.Releases {
			result.Summaries = append(result.Summaries, operationalSummary(CheckBQCompleteness, release, scope.Date, v.BigQueryInitializationError))
		}
		return result
	}
	if v.BigQuery == nil {
		err := fmt.Errorf("BigQuery client is not initialized")
		for _, release := range scope.Releases {
			result.Summaries = append(result.Summaries, operationalSummary(CheckBQCompleteness, release, scope.Date, err))
		}
		return result
	}

	start := scope.Date.In(time.UTC)
	end := scope.Date.AddDays(1).In(time.UTC)
	jobs, err := v.BigQuery.ProwJobs(ctx, start, end)
	if err != nil {
		for _, release := range scope.Releases {
			result.Summaries = append(result.Summaries, operationalSummary(CheckBQCompleteness, release, scope.Date, err))
		}
		return result
	}

	attributor := prowloader.NewReleaseAttributor(scope.Releases, v.Config, v.SyntheticReleaseOverrides)
	bqIDs := make(map[string]map[BuildID]struct{}, len(scope.Releases))
	malformedSets := make(map[string]sets.Set[string], len(scope.Releases))
	for _, job := range jobs {
		pj := &prow.ProwJob{
			Annotations: job.Annotations,
			Spec:        prow.ProwJobSpec{Job: job.JobName},
		}
		if job.HasRefs {
			pj.Spec.Refs = &prow.Refs{}
		}
		release := attributor.Match(pj)
		if release == "" {
			continue
		}
		value := strings.TrimSpace(job.BuildID)
		id, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			if malformedSets[release] == nil {
				malformedSets[release] = sets.New[string]()
			}
			malformedSets[release].Insert(value)
			continue
		}
		if bqIDs[release] == nil {
			bqIDs[release] = make(map[BuildID]struct{})
		}
		bqIDs[release][BuildID(id)] = struct{}{}
	}
	for _, release := range scope.Releases {
		postgresIDs, err := v.PostgreSQL.ProwJobRunIDs(ctx, release, start, end)
		if err != nil {
			result.Summaries = append(result.Summaries, operationalSummary(CheckBQCompleteness, release, scope.Date, err))
			continue
		}
		malformed := sets.List(malformedSets[release])
		summary, discrepancies := CompareBuildIDs(release, scope.Date, bqIDs[release], postgresIDs, malformed)
		result.Summaries = append(result.Summaries, summary)
		result.Discrepancies = append(result.Discrepancies, discrepancies...)
	}
	return result
}

func (p *PostgreSQL) ProwJobRunIDs(ctx context.Context, release string, start, end time.Time) (map[BuildID]struct{}, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	type row struct {
		ID uint64
	}
	var rows []row
	err := p.dbc.DB.WithContext(ctx).Raw(`
		SELECT id
		FROM prow_job_runs
		WHERE prow_job_release = ? AND timestamp >= ? AND timestamp < ? AND deleted_at IS NULL
		ORDER BY id
	`, release, start, end).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("querying PostgreSQL Prow job runs for release %s: %w", release, err)
	}
	result := make(map[BuildID]struct{}, len(rows))
	for _, row := range rows {
		result[BuildID(row.ID)] = struct{}{}
	}
	return result, nil
}

type BigQuery struct {
	client *bqcachedclient.Client
}

func NewBigQuery(client *bqcachedclient.Client) *BigQuery {
	return &BigQuery{client: client}
}

var bigQueryIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (b *BigQuery) ProwJobs(ctx context.Context, start, end time.Time) ([]BQJob, error) {
	if b == nil || b.client == nil || b.client.BQ == nil {
		return nil, fmt.Errorf("BigQuery client is not initialized")
	}
	project := b.client.BQ.Project()
	dataset := b.client.Dataset
	if !bigQueryIdentifier.MatchString(project) || !bigQueryIdentifier.MatchString(dataset) {
		return nil, fmt.Errorf("invalid BigQuery project or dataset identifier")
	}
	query := b.client.Query(ctx, bqlabel.VerifyProwJobs, fmt.Sprintf(`
		SELECT
			prowjob_job_name,
			IFNULL(CAST(prowjob_build_id AS STRING), '') AS prowjob_build_id,
			prowjob_annotations,
			IFNULL(CAST(pr_number AS STRING), '') AS pr_number
		FROM %s
		WHERE TIMESTAMP(prowjob_start) >= @start
		  AND TIMESTAMP(prowjob_start) < @end
		  AND prowjob_url IS NOT NULL
		  AND prowjob_state NOT IN ('pending', 'triggered')
		ORDER BY TIMESTAMP(prowjob_start), prowjob_build_id
	`, "`"+project+"."+dataset+".jobs`"))
	query.Parameters = []bigquery.QueryParameter{
		{Name: "start", Value: start},
		{Name: "end", Value: end},
	}
	iteratorRows, err := query.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying BigQuery Prow jobs: %w", err)
	}
	type bqRow struct {
		JobName     string   `bigquery:"prowjob_job_name"`
		BuildID     string   `bigquery:"prowjob_build_id"`
		Annotations []string `bigquery:"prowjob_annotations"`
		PRNumber    string   `bigquery:"pr_number"`
	}
	result := make([]BQJob, 0)
	for {
		var row bqRow
		err := iteratorRows.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading BigQuery Prow jobs: %w", err)
		}
		annotations := make(map[string]string, len(row.Annotations))
		for _, annotation := range row.Annotations {
			parts := strings.SplitN(annotation, "=", 2)
			if len(parts) == 2 {
				annotations[parts[0]] = parts[1]
			}
		}
		result = append(result, BQJob{
			BuildID: row.BuildID, JobName: row.JobName, Annotations: annotations, HasRefs: row.PRNumber != "",
		})
	}
	return result, nil
}

func CompareBuildIDs(release string, date civil.Date, bqIDs, postgresIDs map[BuildID]struct{}, malformed []string) (Summary, []Discrepancy) {
	discrepancies := make([]Discrepancy, 0)
	malformedSet := sets.New[string]()
	for _, value := range malformed {
		malformedSet.Insert(strings.TrimSpace(value))
	}
	for _, value := range sets.List(malformedSet) {
		kind := "malformed-build-id"
		detail := "BigQuery build ID is not an unsigned integer"
		if value == "" {
			kind = "missing-build-id"
			detail = "BigQuery build ID is blank"
		}
		discrepancies = append(discrepancies, Discrepancy{
			Check: CheckBQCompleteness, Release: release, Date: date,
			Kind: kind, Key: value, Detail: detail,
		})
	}
	for _, id := range sortedBuildIDs(bqIDs) {
		if _, ok := postgresIDs[id]; !ok {
			discrepancies = append(discrepancies, Discrepancy{
				Check: CheckBQCompleteness, Release: release, Date: date,
				Kind: "missing-in-postgres", Key: fmt.Sprint(uint64(id)), Expected: "present", Actual: "missing",
			})
		}
	}
	for _, id := range sortedBuildIDs(postgresIDs) {
		if _, ok := bqIDs[id]; !ok {
			discrepancies = append(discrepancies, Discrepancy{
				Check: CheckBQCompleteness, Release: release, Date: date,
				Kind: "missing-in-bigquery", Key: fmt.Sprint(uint64(id)), Expected: "present", Actual: "missing",
			})
		}
	}
	return summary(CheckBQCompleteness, release, date, len(bqIDs), len(postgresIDs), len(discrepancies)), discrepancies
}

func sortedBuildIDs(values map[BuildID]struct{}) []BuildID {
	ids := make([]BuildID, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
