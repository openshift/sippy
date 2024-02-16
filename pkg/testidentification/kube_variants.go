package testidentification

import (
	"fmt"
	"regexp"

	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/util/sets"
)

var (
	// variant regexes
	kubeConformanceRegex = regexp.MustCompile(`[Cc]onformance-`)
	kubeKindRegex        = regexp.MustCompile(`kind-`)
	kubeKubeadmRegex     = regexp.MustCompile(`kubeadm-`)
	kubeSerialRegex      = regexp.MustCompile(`-serial`)
	kubeWindowsRegex     = regexp.MustCompile(`windows-`)
	kubeUpgradeRegex     = regexp.MustCompile(`upgrade-`)
	kubeE2eRegex         = regexp.MustCompile(`-parallel`)

	allKubeVariants = sets.NewString(
		"conformance",
		"kind",
		"kubeadm",
		"serial",
		"windows",
		"upgrade",
		"e2e",
	)

	// these jobs don't have clear patterns, but I think they're running similar tests
	kubeParallelE2eJobs = sets.NewString(
		"gce-master-scale-correctness",
		"kubeadm-kinder-latest",
		"kubeadm-kinder-latest-on-1-19",
		"kubeadm-kinder-upgrade-1-19-latest",
		"gce-ubuntu-master-default",

		"gce-cos-master-default",
		"gce-ubuntu-master-containerd",
	)
)

type kubeVariants struct{}

func NewKubeVariantManager() VariantManager {
	return kubeVariants{}
}

func (kubeVariants) AllVariants() sets.String {
	return allKubeVariants
}

func (kubeVariants) AllPlatforms() sets.String {
	return sets.String{}
}

func (v kubeVariants) IdentifyVariants(jobName, release string, jobVariants models.ClusterData) []string {
	variants := []string{}

	defer func() {
		for _, variant := range variants {
			if !allKubeVariants.Has(variant) {
				panic(fmt.Sprintf("coding error: missing variant: %q", variant))
			}
		}
	}()

	if kubeConformanceRegex.MatchString(jobName) {
		variants = append(variants, "conformance")
	}
	if kubeKindRegex.MatchString(jobName) {
		variants = append(variants, "kind")
	}
	if kubeKubeadmRegex.MatchString(jobName) {
		variants = append(variants, "kubeadm")
	}
	if kubeSerialRegex.MatchString(jobName) {
		variants = append(variants, "serial")
	}
	if kubeWindowsRegex.MatchString(jobName) {
		variants = append(variants, "windows")
	}
	if kubeUpgradeRegex.MatchString(jobName) {
		variants = append(variants, "upgrade")
	}
	if kubeUpgradeRegex.MatchString(jobName) {
		variants = append(variants, "upgrade")
	}

	if kubeE2eRegex.MatchString(jobName) || kubeParallelE2eJobs.Has(jobName) {
		variants = append(variants, "e2e")
	}

	return variants
}

func (kubeVariants) IsJobNeverStable(jobName string) bool {
	return false
}
