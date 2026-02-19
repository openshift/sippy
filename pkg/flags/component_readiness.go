package flags

import (
	"fmt"
	"os"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

var (
	defaultCRTimeRoundingFactor = 4 * time.Hour
	maxCRTimeRoundingFactor     = 12 * time.Hour
)

// ComponentReadinessFlags holds configuration information for serving ComponentReadiness.
type ComponentReadinessFlags struct {
	ComponentReadinessViewsFile string
	CRTimeRoundingFactor        time.Duration
	CORSAllowedOrigin           string
	ExcludeMassFailures         bool
}

func NewComponentReadinessFlags() *ComponentReadinessFlags {
	return &ComponentReadinessFlags{}
}

func (f *ComponentReadinessFlags) BindFlags(fs *pflag.FlagSet) {
	factorUsage := fmt.Sprintf("Set the rounding factor for component readiness release time. The time will be rounded down to the nearest multiple of the factor. Maximum value is %v", maxCRTimeRoundingFactor)
	fs.StringVar(&f.ComponentReadinessViewsFile, "views", "", "Optional yaml file for predefined Component Readiness views")
	fs.DurationVar(&f.CRTimeRoundingFactor, "component-readiness-time-rounding-factor", defaultCRTimeRoundingFactor, factorUsage)
	fs.StringVar(&f.CORSAllowedOrigin, "cors-allowed-origin", "*", "Optional allowed origin for CORS")
	fs.BoolVar(&f.ExcludeMassFailures, "exclude-mass-failures", false, "Exclude other tests from jobs containing tests known to cause mass failures")
}

func (f *ComponentReadinessFlags) ParseViewsFile() (*api.SippyViews, error) {
	vf := &api.SippyViews{}
	if f.ComponentReadinessViewsFile != "" {
		yamlFile, err := os.ReadFile(f.ComponentReadinessViewsFile)
		if err != nil {
			err = errors.Wrapf(err, "unable to read component readiness views from %s", f.ComponentReadinessViewsFile)
			return vf, err
		}
		err = yaml.Unmarshal(yamlFile, vf)
		if err != nil {
			err = errors.Wrapf(err, "unable to parse component readiness views from %s", f.ComponentReadinessViewsFile)
			return vf, err
		}

		err = f.validateViews(vf)
		if err != nil {
			return vf, errors.Wrap(err, "invalid view definition found")
		}
	}
	return vf, nil
}

// GetMassFailureTestNames returns the hard-coded list of test names that are known to cause mass failures.
// When these tests appear in a job, all other tests in that job should be excluded from analysis.
func (f *ComponentReadinessFlags) GetMassFailureTestNames() []string {
	if !f.ExcludeMassFailures {
		return nil
	}
	return []string{
		"install should succeed: overall",
		"[sig-cluster-lifecycle] Cluster completes upgrade",
		"[Jira:\"Test Framework\"] there should not be mass test failures",
	}
}

func (f *ComponentReadinessFlags) validateViews(views *api.SippyViews) error {
	for _, view := range views.ComponentReadiness {
		// If using variant cross compare, those variants must not appear in the dbGroupBy:
		if len(view.VariantOptions.VariantCrossCompare) > 0 {
			for _, vcc := range view.VariantOptions.VariantCrossCompare {
				if view.VariantOptions.DBGroupBy.Has(vcc) {
					return fmt.Errorf("view %s db_group_by cannot contain variant being cross-compared: %s", view.Name, vcc)
				}
			}
		}
	}

	return nil
}
