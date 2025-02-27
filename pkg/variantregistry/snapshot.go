package variantregistry

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
)

// JobVariants is a map of jobs to variant key/value pairs
type JobVariants map[string]map[string]string

type VariantSnapshot struct {
	config *v1.SippyConfig
	log    logrus.FieldLogger
}

func NewVariantSnapshot(config *v1.SippyConfig, log logrus.FieldLogger) *VariantSnapshot {
	return &VariantSnapshot{
		config: config,
		log:    log,
	}
}

func (s *VariantSnapshot) Identify() JobVariants {
	newVariants := map[string]map[string]string{}
	variantSyncer := OCPVariantLoader{config: s.config}
	for _, releaseCfg := range s.config.Releases {
		for job := range releaseCfg.Jobs {
			if isIgnoredJob(job) {
				continue
			}

			newVariants[job] = variantSyncer.IdentifyVariants(s.log, job)
		}
	}

	return newVariants
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
