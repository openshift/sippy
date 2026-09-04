package prowloader

import (
	"regexp"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/apis/prow"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/releaseoverride"
)

// ReleaseAttributor applies the same release matching rules to Prow jobs no
// matter whether they are being loaded or independently verified.
type ReleaseAttributor struct {
	releases                     []string
	releaseSet                   sets.Set[string]
	releaseRegexps               map[string][]*regexp.Regexp
	config                       *v1config.SippyConfig
	syntheticReleaseJobOverrides *releaseoverride.SyntheticReleaseOverrides
}

func NewReleaseAttributor(releases []string, config *v1config.SippyConfig, syntheticReleaseJobOverrides *releaseoverride.SyntheticReleaseOverrides) *ReleaseAttributor {
	if config == nil {
		config = &v1config.SippyConfig{}
	}
	compiledRegexps := make(map[string][]*regexp.Regexp, len(releases))
	for _, release := range releases {
		if cfg, ok := config.Releases[release]; ok {
			for _, expr := range cfg.Regexp {
				re, err := regexp.Compile(expr)
				if err != nil {
					log.WithError(err).WithFields(log.Fields{"release": release, "regex": expr}).Error("invalid regex in configuration")
					continue
				}
				compiledRegexps[release] = append(compiledRegexps[release], re)
			}
		}
	}

	return &ReleaseAttributor{
		releases:                     releases,
		releaseSet:                   sets.New[string](releases...),
		releaseRegexps:               compiledRegexps,
		config:                       config,
		syntheticReleaseJobOverrides: syntheticReleaseJobOverrides,
	}
}

// Match returns the release a Prow job belongs to, or an empty string when it
// does not match one of the releases supplied to the attributor.
func (a *ReleaseAttributor) Match(pj *prow.ProwJob) string {
	if a == nil || pj == nil {
		return ""
	}
	if _, ok := pj.Annotations["releaseJobName"]; ok && pj.Spec.Refs != nil {
		if a.releaseSet.Has(models.ReleasePresubmits) {
			return models.ReleasePresubmits
		}
		return ""
	}

	jobName := pj.Spec.Job
	if release, ok := a.syntheticReleaseJobOverrides.Lookup(jobName); ok {
		if a.releaseSet.Has(release) {
			return release
		}
		return ""
	}

	for _, release := range a.releases {
		cfg, ok := a.config.Releases[release]
		if !ok {
			continue
		}
		if enabled, ok := cfg.Jobs[jobName]; ok && enabled {
			return release
		}
		for _, re := range a.releaseRegexps[release] {
			if re.MatchString(jobName) {
				return release
			}
		}
	}
	return ""
}
