package flags

import (
	"fmt"
	"os"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
}

func NewComponentReadinessFlags() *ComponentReadinessFlags {
	return &ComponentReadinessFlags{}
}

func (f *ComponentReadinessFlags) BindFlags(fs *pflag.FlagSet) {
	factorUsage := fmt.Sprintf("Set the rounding factor for component readiness release time. The time will be rounded down to the nearest multiple of the factor. Maximum value is %v", maxCRTimeRoundingFactor)
	fs.StringVar(&f.ComponentReadinessViewsFile, "views", "", "Optional yaml file for predefined Component Readiness views")
	fs.DurationVar(&f.CRTimeRoundingFactor, "component-readiness-time-rounding-factor", defaultCRTimeRoundingFactor, factorUsage)
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
			log.WithError(err).Fatal("invalid view definition found")
		}

		log.Infof("parsed views: %+v", vf)
	}
	return vf, nil
}

func (f *ComponentReadinessFlags) validateViews(views *api.SippyViews) error {

	// Maps release (4.18) to views in that release with regression tracking on. Length of the slice should not be > 1.
	viewsWithRegressionTracking := map[string][]string{}

	for _, view := range views.ComponentReadiness {
		// If using variant cross compare, those variants must not appear in the dbGroupBy:
		if len(view.VariantOptions.VariantCrossCompare) > 0 {
			for _, vcc := range view.VariantOptions.VariantCrossCompare {
				if view.VariantOptions.DBGroupBy.Has(vcc) {
					return fmt.Errorf("view %s db_group_by cannot contain variant being cross-compared: %s", view.Name, vcc)
				}
			}
		}

		if view.RegressionTracking.Enabled {

			if _, ok := viewsWithRegressionTracking[view.SampleRelease.Release]; !ok {
				viewsWithRegressionTracking[view.SampleRelease.Release] = []string{}
			}
			viewsWithRegressionTracking[view.SampleRelease.Release] = append(viewsWithRegressionTracking[view.SampleRelease.Release], view.Name)
		}

	}

	for release, viewsWithRegTracking := range viewsWithRegressionTracking {
		if len(viewsWithRegTracking) > 1 {
			return fmt.Errorf("only one view in release %s can have regression tracking enabled: %v", release, viewsWithRegTracking)
		}
	}

	return nil
}
