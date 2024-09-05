package flags

import (
	"fmt"
	"os"
	"time"

	"github.com/openshift/sippy/pkg/apis/api"
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

func (f *ComponentReadinessFlags) ParseViewsFile() *api.SippyViews {
	vf := &api.SippyViews{}
	if f.ComponentReadinessViewsFile != "" {
		yamlFile, err := os.ReadFile(f.ComponentReadinessViewsFile)
		if err != nil {
			log.WithError(err).Fatalf("unable to read component readiness views from %s", f.ComponentReadinessViewsFile)
		}
		err = yaml.Unmarshal(yamlFile, vf)
		if err != nil {
			log.WithError(err).Fatalf("unable to parse component readiness views from %s", f.ComponentReadinessViewsFile)
		}

		err = f.validateViews(vf)
		if err != nil {
			log.WithError(err).Fatal("invalid view definition found")
		}

		log.Infof("parsed views: %+v", vf)
	}
	return vf
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
