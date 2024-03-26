package api

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
)

func GetDisruptionVsPrevGAReportFromBigQuery(client *bqcachedclient.Client) (apitype.DisruptionReport, []error) {
	generator := disruptionReportGenerator{
		client:   client.BQ,
		ViewName: "BackendDisruptionPercentilesDeltaCurrentVsPrevGA",
	}

	return getDataFromCacheOrGenerate[apitype.DisruptionReport](client.Cache, cache.RequestOptions{}, generator, generator.GenerateReport, apitype.DisruptionReport{})
}

func GetDisruptionVsTwoWeeksAgoReportFromBigQuery(client *bqcachedclient.Client) (apitype.DisruptionReport, []error) {
	generator := disruptionReportGenerator{
		client:   client.BQ,
		ViewName: "BackendDisruptionPercentilesDeltaCurrentVs14DaysAgo",
	}

	return getDataFromCacheOrGenerate[apitype.DisruptionReport](client.Cache, cache.RequestOptions{}, generator, generator.GenerateReport, apitype.DisruptionReport{})
}

type disruptionReportGenerator struct {
	client   *bigquery.Client
	ViewName string
}

func (c *disruptionReportGenerator) GenerateReport() (apitype.DisruptionReport, []error) {
	before := time.Now()
	disruptionReport, err := c.getDisruptionDeltasFromBigQuery()
	if err != nil {
		return apitype.DisruptionReport{}, []error{err}
	}
	log.Infof("Disruption report fetched from bigquery in %s with %d rows", time.Since(before), len(disruptionReport.Rows))

	return disruptionReport, nil
}

func (c *disruptionReportGenerator) getDisruptionDeltasFromBigQuery() (apitype.DisruptionReport, error) {
	// We'll publish a metric for whatever is in the views, which need to be updated for each GA release:
	queryString := fmt.Sprintf(`
						SELECT *
						FROM openshift-ci-data-analysis.ci_data.%s
						WHERE LookbackDays = 3`, c.ViewName)

	query := c.client.Query(queryString)
	it, err := query.Read(context.TODO())
	if err != nil {
		log.WithError(err).Error("error querying disruption data from bigquery")
		return apitype.DisruptionReport{}, err
	}

	// Using a set since sometimes bigquery has multiple copies of the same prow job
	rows := []apitype.DisruptionReportRow{}
	for {
		r := apitype.DisruptionReportRow{}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.WithError(err).Error("error parsing disruption report row from bigquery")
			return apitype.DisruptionReport{}, err
		}
		rows = append(rows, r)
	}
	return apitype.DisruptionReport{
		Rows: rows,
	}, nil
}
