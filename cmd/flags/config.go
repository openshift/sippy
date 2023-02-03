package flags

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	v1 "github.com/openshift/sippy/pkg/apis/config/v1"
)

// ConfigFlags holds configuration information for Sippy such as the location
// of its configuration file.
type ConfigFlags struct {
	Path string
}

func NewConfigFlags() *ConfigFlags {
	return &ConfigFlags{}
}

func (f *ConfigFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.Path,
		"config",
		f.Path,
		"Configuration file for Sippy, required if using Prow-based Sippy")
}

func (f *ConfigFlags) LoadConfig() *v1.SippyConfig {
	var sippyConfig v1.SippyConfig

	if f.Path == "" {
		sippyConfig.Prow = v1.ProwConfig{
			URL: "https://prow.ci.openshift.org/prowjobs.js",
		}
	} else {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			log.WithError(err).Fatalf("could not load config")
		}
		if err := yaml.Unmarshal(data, &sippyConfig); err != nil {
			log.WithError(err).Fatalf("could not unmarshal config")
		}
	}

	return &sippyConfig
}
