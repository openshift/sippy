package testidentification

import (
	"context"
	_ "embed"
	"fmt"
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
	"Owner",
	"Topology",
	"FeatureSet",
	"Upgrade",
	"SecurityMode",
	"Installer",
	"JobTier",
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

	// Ensure filtered by important variants; including them all
	// significantly increases cardinality and slows matview refreshes
	// to a crawl.
	return filterVariants(allVariants, importantVariants)
}

func (*openshiftVariants) IsJobNeverStable(jobName string) bool {
	for _, ns := range openshiftJobsNeverStable {
		if ns == jobName {
			return true
		}
	}

	return false
}

// filterVariants only includes the important variants, returns them sorted
// according to the original array's order
func filterVariants(arr, prefixes []string) []string {
	var result []string
	prefixMap := make(map[string]string)

	// Create a map of prefix to full string
	for _, item := range arr {
		for _, prefix := range prefixes {
			if strings.HasPrefix(item, prefix+":") {
				prefixMap[prefix] = item
				break
			}
		}
	}

	// Add items to result in the order of prefixes
	for _, prefix := range prefixes {
		if val, exists := prefixMap[prefix]; exists {
			result = append(result, val)
		}
	}

	return result
}
