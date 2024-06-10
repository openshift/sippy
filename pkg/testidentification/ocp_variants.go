package testidentification

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/util/sets"
)

// openshiftJobsNeverStable is a list of jobs that have permafailed
// (0%) for at least two weeks. They are excluded from "normal" variants. The list
// is generated programatically via scripts/update-neverstable.sh
//
//go:embed ocp_never_stable.txt
var openshiftJobsNeverStableRaw string
var openshiftJobsNeverStable = strings.Split(openshiftJobsNeverStableRaw, "\n")

var importantVariants = []string{
	"Platform",
	"Architecture",
	"Network",
	"NetworkStack",
	"Topology",
	"FeatureSet",
	"Upgrade",
	"SecurityMode",
	"Installer",
}

const (
	NeverStable = "never-stable"

	jobVariantsQuery = `SELECT
  job_name,
  ARRAY_AGG(STRUCT(variant_name, variant_value)) AS variants
FROM 
  $$DATASET$$.job_variants
WHERE variant_value IS NOT NULL AND variant_value != ""
GROUP BY 
  job_name`
)

type openshiftVariants struct {
	jobVariants   map[string][]string
	variantValues map[string]sets.String
}

type variant struct {
	VariantName  string `json:"variant_name" bigquery:"variant_name"`
	VariantValue string `json:"variant_value" bigquery:"variant_value"`
}

type jobVariant struct {
	JobName  string    `json:"job_name" bigquery:"job_name"`
	Variants []variant `json:"variants" bigquery:"variants"`
}

func NewOpenshiftVariantManager(ctx context.Context, bqc *bqcachedclient.Client) (VariantManager, error) {
	if bqc == nil {
		return nil, fmt.Errorf("openshift variant manager requires bigquery")
	}

	mgr := openshiftVariants{
		variantValues: make(map[string]sets.String),
		jobVariants:   make(map[string][]string),
	}

	start := time.Now()
	log.Infof("loading variants from bigquery...")
	// Read variants mapping from bigquery
	variantsQuery := strings.ReplaceAll(jobVariantsQuery, "$$DATASET$$", bqc.Dataset)
	log.Debugf("variant query is %+v", variantsQuery)
	it, err := bqc.BQ.Query(variantsQuery).Read(ctx)
	if err != nil {
		return nil, err
	}

	jobVariants := make(map[string][]string)
	variantKeyValues := make(map[string]sets.String)
	for {
		var row jobVariant
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.WithMessage(err, "failed to read row")
		}
		for _, v := range row.Variants {
			jobVariants[row.JobName] = append(jobVariants[row.JobName], fmt.Sprintf("%s:%s", v.VariantName, v.VariantValue))
			if _, ok := variantKeyValues[v.VariantValue]; ok {
				variantKeyValues[v.VariantName].Insert(v.VariantValue)
			} else {
				variantKeyValues[v.VariantName] = sets.NewString(v.VariantValue)
			}
		}
	}
	mgr.jobVariants = jobVariants
	mgr.variantValues = variantKeyValues

	log.WithFields(log.Fields{
		"jobs": len(jobVariants),
	}).Infof("variants loaded from bigquery in %+v", time.Since(start))
	return &mgr, nil
}

func (v *openshiftVariants) AllPlatforms() sets.String {
	return v.variantValues["Platform"]
}

func (v *openshiftVariants) IdentifyVariants(jobName string) []string {
	allVariants := v.jobVariants[jobName]
	if v.IsJobNeverStable(jobName) {
		allVariants = append(allVariants, NeverStable)
	}

	// Ensure sorted by PANTFUSI (or whatever ordering we determine is useful)
	return sortByPrefixes(allVariants, importantVariants)
}

func (*openshiftVariants) IsJobNeverStable(jobName string) bool {
	for _, ns := range openshiftJobsNeverStable {
		if ns == jobName {
			return true
		}
	}

	return false
}

// sortByPrefixes sorts a slice by prefix order, with alphabetical fallback
func sortByPrefixes(arr, prefixes []string) []string {
	// Create a map to store the order of prefixes
	prefixOrder := make(map[string]int)
	for i, prefix := range prefixes {
		prefixOrder[prefix] = i
	}

	// Custom sort function
	sort.SliceStable(arr, func(i, j int) bool {
		prefixI, foundI := getPrefix(arr[i], prefixOrder)
		prefixJ, foundJ := getPrefix(arr[j], prefixOrder)

		if foundI && foundJ {
			// Both items have prefixes, sort by their order
			return prefixOrder[prefixI] < prefixOrder[prefixJ]
		} else if foundI {
			// Only the first item has a prefix, it goes first
			return true
		} else if foundJ {
			// Only the second item has a prefix, it goes first
			return false
		}
		// Neither item has a prefix, sort alphabetically
		return arr[i] < arr[j]
	})

	return arr
}

func getPrefix(item string, prefixOrder map[string]int) (string, bool) {
	for prefix := range prefixOrder {
		if strings.HasPrefix(item, fmt.Sprintf("%s:", prefix)) {
			return prefix, true
		}
	}
	return "", false
}
