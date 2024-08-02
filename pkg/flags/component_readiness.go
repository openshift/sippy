package flags

import (
	"fmt"
	"os"
	"time"

	apitype "github.com/openshift/sippy/pkg/apis/api"
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
	fs.StringVar(&f.ComponentReadinessViewsFile, "component-readiness-views", "", "Optional yaml file for server side predefined Component Readiness views")
	fs.DurationVar(&f.CRTimeRoundingFactor, "component-readiness-time-rounding-factor", defaultCRTimeRoundingFactor, factorUsage)
}

func (f *ComponentReadinessFlags) ParseViewsFile() []apitype.ComponentReportView {
	crViews := []apitype.ComponentReportView{}
	if f.ComponentReadinessViewsFile != "" {
		yamlFile, err := os.ReadFile(f.ComponentReadinessViewsFile)
		if err != nil {
			log.WithError(err).Fatalf("unable to read component readiness views from %s", f.ComponentReadinessViewsFile)
		}
		err = yaml.Unmarshal(yamlFile, &crViews)
		if err != nil {
			log.WithError(err).Fatalf("unable to parse component readiness views from %s", f.ComponentReadinessViewsFile)
		}
		log.Infof("parsed views: %+v", crViews)
	}
	return crViews
}
