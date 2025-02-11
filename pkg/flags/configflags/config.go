package configflags

import (
	"os"

	"github.com/pkg/errors"
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

func (f *ConfigFlags) GetConfig() (*v1.SippyConfig, error) {
	var sippyConfig v1.SippyConfig

	if f.Path == "" {
		sippyConfig.Prow = v1.ProwConfig{
			URL: "https://prow.ci.openshift.org/prowjobs.js",
		}
	} else {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, errors.WithMessage(err, "could not load config")
		}
		if err := yaml.Unmarshal(data, &sippyConfig); err != nil {
			return nil, errors.WithMessage(err, "couldn't unmarshal config")
		}
	}

	return &sippyConfig, nil
}
