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

const (
	ModeOpenshift  = "ocp"
	ModeKubernetes = "kube"
	ModeNone       = "none"
)

func NewModeFlags() *ModeFlags {
	return &ModeFlags{
		Mode: ModeOpenshift,
	}
}

func (f *ModeFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.Mode, "mode", f.Mode, "Mode to use: {ocp,kube,none}")
}

func (f *ModeFlags) GetServerMode() sippyserver.Mode {
	if f.Mode == ModeOpenshift {
		return sippyserver.ModeOpenShift
	}

	return sippyserver.ModeKubernetes
}

func (f *ModeFlags) GetVariantManager() testidentification.VariantManager {
	switch f.Mode {
	case ModeOpenshift:
		return testidentification.NewOpenshiftVariantManager()
	case ModeKubernetes:
		return testidentification.NewKubeVariantManager()
	case ModeNone:
		return testidentification.NewEmptyVariantManager()
	default:
		panic("only ocp, kube, or none is allowed")
	}
}

func (f *ModeFlags) GetSyntheticTestManager() synthetictests.SyntheticTestManager {
	if f.Mode == ModeOpenshift {
		return synthetictests.NewOpenshiftSyntheticTestManager()
	}

	return synthetictests.NewEmptySyntheticTestManager()
}
