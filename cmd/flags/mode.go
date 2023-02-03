package flags

import (
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
)

type ModeFlags struct {
	Mode string
}

func NewModeFlags() *ModeFlags {
	return &ModeFlags{
		Mode: "ocp",
	}
}

func (f *ModeFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.Mode, "mode", f.Mode, "Mode to use: {ocp,kube,none}")
}

func (f *ModeFlags) GetServerMode() sippyserver.Mode {
	if f.Mode == "ocp" {
		return sippyserver.ModeOpenShift
	}

	return sippyserver.ModeKubernetes
}

func (f *ModeFlags) GetVariantManager() testidentification.VariantManager {
	switch f.Mode {
	case "ocp":
		return testidentification.NewOpenshiftVariantManager()
	case "kube":
		return testidentification.NewKubeVariantManager()
	case "none":
		return testidentification.NewEmptyVariantManager()
	default:
		panic("only ocp, kube, or none is allowed")
	}
}

func (f *ModeFlags) GetSyntheticTestManager() synthetictests.SyntheticTestManager {
	if f.Mode == "ocp" {
		return synthetictests.NewOpenshiftSyntheticTestManager()
	}

	return synthetictests.NewEmptySyntheticTestManager()
}
