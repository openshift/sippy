package flags

import (
	"fmt"
	"os"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crview"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

var (
	defaultCRTimeRoundingFactor = 12 * time.Hour
	maxCRTimeRoundingFactor     = 24 * time.Hour
	defaultCRTimeRoundingOffset = 4 * time.Hour // aligned one hour before EST midnight/noon (04:00/16:00 UTC)
)

// ComponentReadinessFlags holds configuration information for serving ComponentReadiness.
type ComponentReadinessFlags struct {
	ComponentReadinessViewsFile string
	CRTimeRoundingFactor        time.Duration
	CRTimeRoundingOffset        time.Duration
	CORSAllowedOrigin           string
}

func NewComponentReadinessFlags() *ComponentReadinessFlags {
	return &ComponentReadinessFlags{}
}

func (f *ComponentReadinessFlags) BindFlags(fs *pflag.FlagSet) {
	factorUsage := fmt.Sprintf("Set the rounding factor for component readiness release time. The time will be rounded down to the nearest multiple of the factor. Maximum value is %v", maxCRTimeRoundingFactor)
	fs.StringVar(&f.ComponentReadinessViewsFile, "views", "", "Optional yaml file for predefined Component Readiness views")
	fs.DurationVar(&f.CRTimeRoundingFactor, "component-readiness-time-rounding-factor", defaultCRTimeRoundingFactor, factorUsage)
	offsetUsage := "Shift rounding boundaries by this duration. With a 12h factor: 0 aligns to 00:00/12:00 UTC, 4h aligns to 04:00/16:00 UTC, 5h aligns to 05:00/17:00 UTC (midnight/noon EST). The cronjob schedule must match these boundaries."
	fs.DurationVar(&f.CRTimeRoundingOffset, "component-readiness-time-rounding-offset", defaultCRTimeRoundingOffset, offsetUsage)
	fs.StringVar(&f.CORSAllowedOrigin, "cors-allowed-origin", "*", "Optional allowed origin for CORS")
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
		if err := validateSpotCheckSamples(view); err != nil {
			return err
		}
	}

	return nil
}

// validateSpotCheckSamples requires each spot-check sample to select exactly the job tier it is named for.
// The sample name is encoded into synthetic test IDs and stands in for the tier when a drilldown has no view,
// and a sample without a tier would pull ordinary jobs into spot-check results.
func validateSpotCheckSamples(view crview.View) error {
	for name, sample := range view.SpotCheckJobSamples {
		if !crtest.IsSpotCheckTestID(name) {
			return fmt.Errorf("view %s spot-check sample %s must be named with the %q prefix",
				view.Name, name, crtest.SpotCheckTestIDPrefix)
		}
		if tiers := sample.IncludeVariants["JobTier"]; len(tiers) != 1 || tiers[0] != name {
			return fmt.Errorf("view %s spot-check sample %s must select exactly JobTier %s, got %v",
				view.Name, name, name, tiers)
		}
	}
	return nil
}
