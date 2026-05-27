package variantregistry

import (
	"fmt"

	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/releaseoverride"
)

// BuildSyntheticReleaseJobOverrides builds overrides for all jobs and regexp
// patterns listed in synthetic releases. These overrides give synthetic releases
// priority over name-based version matching when determining which release a job
// belongs to.
func BuildSyntheticReleaseJobOverrides(releases map[string]v1.ReleaseConfig) (*releaseoverride.SyntheticReleaseOverrides, error) {
	overrides := releaseoverride.New()
	for releaseName, releaseCfg := range releases {
		if !releaseCfg.Synthetic {
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
