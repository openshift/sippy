package variantregistry

import (
	"fmt"

	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/openshift/sippy/pkg/releaseoverride"
	"k8s.io/apimachinery/pkg/util/sets"
)

// BuildSyntheticReleaseJobOverrides builds overrides for all jobs and regexp
// patterns listed in synthetic releases. These overrides give synthetic releases
// priority over name-based version matching when determining which release a job
// belongs to.
//
// releaseConfigs identifies which releases are synthetic (from BigQuery),
// while the job-to-release mappings come from the YAML config.
func BuildSyntheticReleaseJobOverrides(releases map[string]v1.ReleaseConfig, releaseConfigs []sippyv1.Release) (*releaseoverride.SyntheticReleaseOverrides, error) {
	syntheticSet := syntheticReleaseNames(releaseConfigs)
	overrides := releaseoverride.New()
	for releaseName, releaseCfg := range releases {
		if !syntheticSet.Has(releaseName) {
			continue
		}
		for jobName, enabled := range releaseCfg.Jobs {
			if !enabled {
				continue
			}
			if err := overrides.AddExact(jobName, releaseName); err != nil {
				return nil, err
			}
		}
		for _, expr := range releaseCfg.Regexp {
			if err := overrides.AddRegexp(expr, releaseName); err != nil {
				return nil, fmt.Errorf(
					"invalid regexp %q in synthetic release %q: %w",
					expr, releaseName, err,
				)
			}
		}
	}
	return overrides, nil
}

func syntheticReleaseNames(releaseConfigs []sippyv1.Release) sets.Set[string] {
	names := sets.New[string]()
	for _, rc := range releaseConfigs {
		if rc.Synthetic {
			names.Insert(rc.Release)
		}
	}
	return names
}
