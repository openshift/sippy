package flags

import (
	"context"

	"github.com/spf13/pflag"

	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/sippyserver"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
)

type ModeFlags struct {
	Mode string
}

const (
	ModeOpenshift = "ocp"
	ModeNone      = "none"
)

func NewModeFlags() *ModeFlags {
	return &ModeFlags{
		Mode: ModeOpenshift,
	}
}

func (f *ModeFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.Mode, "mode", f.Mode, "Mode to use: {ocp,none}")
}

func (f *ModeFlags) GetServerMode() sippyserver.Mode {
	if f.Mode == ModeOpenshift {
		return sippyserver.ModeOpenShift
	}

	return sippyserver.ModeKubernetes
}

func (f *ModeFlags) GetVariantManager(ctx context.Context, bqc *bqcachedclient.Client) testidentification.VariantManager {
	switch f.Mode {
	case ModeOpenshift:
		mgr, err := testidentification.NewOpenshiftVariantManager(ctx, bqc)
		if err != nil {
			panic(err)
		}
		return mgr
	case ModeNone:
		return testidentification.NewEmptyVariantManager()
	default:
		panic("only ocp or none is allowed")
	}
}

func (f *ModeFlags) GetSyntheticTestManager() synthetictests.SyntheticTestManager {
	if f.Mode == ModeOpenshift {
		return synthetictests.NewOpenshiftSyntheticTestManager()
	}

	return synthetictests.NewEmptySyntheticTestManager()
}
