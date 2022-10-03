package perfscaleanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	es7 "github.com/elastic/go-elasticsearch/v7"
	workloadmetricsv1 "github.com/openshift/sippy/pkg/apis/workloadmetrics/v1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const (
	esURL       = "http://perf-results-elastic.apps.keith-cluster.perfscale.devcluster.openshift.com"
	esIndex     = "ci-reports"
	esQuerySize = 4000

	ScaleJobsSubDir   = "perfscale-metrics"
	ScaleJobsFilename = "perfscale-metrics.json"
)

func DownloadPerfScaleData(storagePath string) error {
	log.Debugf("Downloading perfscale data from %s index %s", esURL, esIndex)
	cfg := es7.Config{
		Addresses: []string{esURL},
	}
	es, err := es7.NewClient(cfg)
	if err != nil {
		log.Debugf("Error creating ElasticSearch client: %v\n", err)
		return err
	}

	// Calculate dates for our buckets:
	now := time.Now()
	// Hardcoded for now:
	currPeriod := 7 * 24 * time.Hour  // last week
	prevPeriod := 30 * 24 * time.Hour // last month
	currStart := now.Add(-currPeriod)
	prevStart := now.Add(-currPeriod).Add(-prevPeriod)
	log.Debugf("Current period start: %s", currStart.Format(time.RFC3339))
	log.Debugf("Previous period start: %s", prevStart.Format(time.RFC3339))

	q := fmt.Sprintf(`
{
  "query": {
    "bool": {
      "filter": [
        {
          "range": {
            "metadata.start_date": {
              "gte": "%s"
            }
          }
        }
      ]
    }
  },
  "size": %d,
  "sort": [
    {
      "timestamp": "desc"
    }
  ]
}
`, prevStart.Format(time.RFC3339), esQuerySize)
	// TODO: paginate all results
	log.Debugf("Elasticsearch query: %s", q)

	var b bytes.Buffer
	b.WriteString(q)
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(esIndex),
		es.Search.WithBody(&b),
		es.Search.WithTrackTotalHits(true),
		es.Search.WithPretty(),
	)
	defer res.Body.Close()
	if err != nil {
		log.Errorf("Error getting response: %s", err)
		return err
	}

	bb, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return errors.Wrap(err, "error reading elasticsearch response")
	}
	j := string(bb)

	totalHits := gjson.Get(j, "hits.total.value").Int()
	log.Debugf(
		"\n[%s] %d hits; took: %dms\n\n",
		res.Status(),
		totalHits,
		gjson.Get(j, "took").Int(),
	)

	// We are not presently preapred for pagination. At time of writing we expect roughly 600 results, and we're requesting 2000 at a time.
	// This will start erroring once if we surpass those limits and we'll need to bump our request, or paginate.
	if totalHits > esQuerySize {
		return fmt.Errorf("%d results returned which is greater than our query size of %d, code needs to be updated", totalHits, esQuerySize)
	}

	aggregatedResults, err := processPerfscaleJobRuns(now, currStart, prevStart, j)
	if err != nil {
		return err
	}

	postProcessPerfscaleWorkloadMetricsRows(aggregatedResults)

	file, err := json.MarshalIndent(aggregatedResults, "", "    ")
	if err != nil {
		return errors.Wrap(err, "error marshalling scale jobs")
	}
	err = ioutil.WriteFile(filepath.Join(storagePath, ScaleJobsFilename), file, 0600)
	if err != nil {
		return errors.Wrap(err, "error writing scalejobs.json")
	}
	log.Debug("finished downloading scale job data")

	return nil
}

func processPerfscaleJobRuns(now, currStart, prevStart time.Time, j string) ([]*workloadmetricsv1.WorkloadMetricsRow, error) {
	aggregatedResults := []*workloadmetricsv1.WorkloadMetricsRow{}

	results := gjson.Get(j, "hits.hits").Array()
	for _, hit := range results {

		hitMap := hit.Get("_source").Map()
		hitMetadata := hitMap["metadata"].Map()

		log.Debugf("Processing result: %s %s %s %s", hitMetadata["uuid"].String(), hitMetadata["release_stream"].String(), hitMetadata["platform"].String(), hitMetadata["network_type"].String())

		// We track the count of machines as some metrics are aggregated across all instances of pods, which
		// run as DaemonSets as nodes. This makes the node count an important metric when examining CPU/Memory use.
		cpCount := hitMetadata["master_count"].Int()
		iCount := hitMetadata["infra_count"].Int()
		wCount := hitMetadata["worker_count"].Int()

		for workload, workloadCPU := range hitMap["metrics"].Map()["podCPU"].Map() {
			workloadMetrics, ok := hitMap["metrics"].Map()["podMemory"].Map()[workload]
			if !ok {
				// Can the inverse happen as well?
				log.Debugf("Warning: workload %s has podCPU metrics but no podMemory in result %s", workload, hitMetadata["uuid"])
			}

			var sj *workloadmetricsv1.WorkloadMetricsRow
			for _, psja := range aggregatedResults {
				if psja.Workload == workload &&
					psja.UpstreamJob == hitMetadata["upstream_job"].String() &&
					psja.Platform == hitMap["openshift_platform"].String() &&
					psja.Release == hitMetadata["release_stream"].String() &&
					psja.NetworkType == hitMap["openshift_network_type"].String() &&
					psja.ControlPlaneCount == cpCount &&
					psja.InfraCount == iCount &&
					psja.WorkerCount == wCount {

					sj = psja
					break
				}
			}
			if sj == nil {
				sj = &workloadmetricsv1.WorkloadMetricsRow{
					Workload:          workload,
					UpstreamJob:       hitMetadata["upstream_job"].String(),
					Platform:          hitMap["openshift_platform"].String(),
					Release:           hitMetadata["release_stream"].String(),
					NetworkType:       hitMap["openshift_network_type"].String(),
					ControlPlaneCount: cpCount,
					InfraCount:        iCount,
					WorkerCount:       wCount,
					CurrentStart:      currStart,
					CurrentEnd:        now,
					PreviousStart:     prevStart,
					PreviousEnd:       currStart,
				}
				aggregatedResults = append(aggregatedResults, sj)
			}

			err := sj.ProcessResult(hitMap, workloadCPU.Map(), workloadMetrics.Map())
			if err != nil {
				return aggregatedResults, err
			}
		}
	}

	return aggregatedResults, nil
}

func postProcessPerfscaleWorkloadMetricsRows(aggregatedResults []*workloadmetricsv1.WorkloadMetricsRow) []*workloadmetricsv1.WorkloadMetricsRow {
	for i := range aggregatedResults {

		sj := aggregatedResults[i]

		// Our JS DataGrid needs a unique ID for each row, we don't have one, so
		// we'll make one up arbitrarily.
		sj.ID = i + 1

		// Sanitize data:
		switch sj.NetworkType {
		case "OpenShiftSDN":
			sj.NetworkType = "SDN"
		case "OVNKubernetes":
			sj.NetworkType = "OVN"
		}

		// Convert release streams such as 4.10.0-0.nightly to just 4.10
		tkns := strings.Split(sj.Release, ".")
		release := fmt.Sprintf("%s.%s", tkns[0], tkns[1])
		sj.Release = release

		// Calculate averages:
		if sj.CurrentPassCount > 0 {
			sj.CurrentPassAvgDur = sj.CurrentPassTotalDur / sj.CurrentPassCount
		} else {
			sj.CurrentPassAvgDur = 0
		}
		if sj.PreviousPassCount > 0 {
			sj.PreviousPassAvgDur = sj.PreviousPassTotalDur / sj.PreviousPassCount
		} else {
			sj.PreviousPassAvgDur = 0
		}

		if sj.CurrentTotalCPUCount > 0 {
			sj.CurrentAvgCPU = sj.CurrentAvgTotalCPU / float64(sj.CurrentTotalCPUCount)
			sj.CurrentMaxCPU = sj.CurrentMaxTotalCPU / float64(sj.CurrentTotalCPUCount)
		}
		if sj.CurrentTotalMemCount > 0 {
			sj.CurrentAvgMemBytes = sj.CurrentAvgTotalMemBytes / float64(sj.CurrentTotalMemCount)
			sj.CurrentMaxMemBytes = sj.CurrentMaxTotalMemBytes / float64(sj.CurrentTotalMemCount)
		}

		if sj.PreviousTotalCPUCount > 0 {
			sj.PreviousAvgCPU = sj.PreviousAvgTotalCPU / float64(sj.PreviousTotalCPUCount)
			sj.PreviousMaxCPU = sj.PreviousMaxTotalCPU / float64(sj.PreviousTotalCPUCount)
		}
		if sj.PreviousTotalMemCount > 0 {
			sj.PreviousAvgMemBytes = sj.PreviousAvgTotalMemBytes / float64(sj.PreviousTotalMemCount)
			sj.PreviousMaxMemBytes = sj.PreviousMaxTotalMemBytes / float64(sj.PreviousTotalMemCount)
		}
	}
	return aggregatedResults
}
