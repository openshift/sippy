package variantregistry

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
)

// JobVariants is a map of jobs to variant key/value pairs
type JobVariants map[string]map[string]string

type VariantSnapshot struct {
	config *v1.SippyConfig
	views  []crview.View
	log    logrus.FieldLogger
}

func NewVariantSnapshot(config *v1.SippyConfig, views []crview.View, log logrus.FieldLogger) *VariantSnapshot {
	return &VariantSnapshot{
		config: config,
		views:  views,
		log:    log,
	}
}

func (s *VariantSnapshot) Identify() JobVariants {
	newVariants := map[string]map[string]string{}
	variantSyncer := OCPVariantLoader{config: s.config, views: s.views}
	for _, releaseCfg := range s.config.Releases {
		for job := range releaseCfg.Jobs {
			if isIgnoredJob(job) {
				continue
			}

			newVariants[job] = variantSyncer.calculateVariantsForJob(s.log, job, nil, syntheticClusterDataOS(job))
		}
	}

	return newVariants
}

func syntheticClusterDataOS(jobName string) clusterDataOS {
	lower := strings.ToLower(jobName)
	if strings.Contains(lower, "rhcos9-10") {
		return clusterDataOS{ControlPlane: "rhel-9", Workers: "rhel-10"}
	}
	if strings.Contains(lower, "rhcos10") {
		return clusterDataOS{Default: "rhel-10"}
	}
	return clusterDataOS{}
}

func (s *VariantSnapshot) Load(path string) (JobVariants, error) {
	oldVariants := map[string]map[string]string{}
	oldVariantsYAML, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(oldVariantsYAML, &oldVariants); err != nil {
		return nil, err
	}

	return oldVariants, nil
}

func (s *VariantSnapshot) Save(path string) error {
	newVariants := s.Identify()
	y, err := yaml.Marshal(newVariants)
	if err != nil {
		return err
	}
	return os.WriteFile(path, y, 0o600)
}

func isIgnoredJob(jobName string) bool {
	// Some CI jobs don't have stable variants because they move between OCP release versions, or other
	// reasons
	ignoredJobSubstrings := []string{
		"periodic-ci-openshift-hive-master-",
	}
	for _, entry := range ignoredJobSubstrings {
		if strings.Contains(jobName, entry) {
			return true
		}
	}

	return false
}
