package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/cmd/flags"
	"github.com/openshift/sippy/pkg/releasesync"
)

type LoadReleasesFlags struct {
	DBFlags       *flags.PostgresDatabaseFlags
	Releases      []string
	Architectures []string
}

func NewLoadReleasesFlags() *LoadReleasesFlags {
	return &LoadReleasesFlags{
		DBFlags: flags.NewPostgresDatabaseFlags(),
	}
}

func (f *LoadReleasesFlags) BindFlags(fs *pflag.FlagSet) {
	f.DBFlags.BindFlags(fs)
	fs.StringArrayVar(&f.Releases, "releases", f.Releases, "list of openshift releases")
	fs.StringArrayVar(&f.Architectures, "architectures", f.Releases, "list of architectures")
}

func init() {
	f := NewLoadReleasesFlags()

	cmd := &cobra.Command{
		Use:   "releases",
		Short: "Load releases from the OpenShift release controllers",
		Run: func(cmd *cobra.Command, args []string) {
			dbc := f.DBFlags.GetDBClient()
			releaseStreams := make([]string, 0)
			for _, release := range f.Releases {
				for _, stream := range []string{"nightly", "ci"} {
					releaseStreams = append(releaseStreams, fmt.Sprintf("%s.0-0.%s", release, stream))
				}
			}

			if errs := releasesync.Import(dbc, releaseStreams, f.Architectures); len(errs) > 0 {
				fmt.Printf("could not load releases:\n")
				for _, err := range errs {
					fmt.Printf("%+v\n", err.Error())
				}
				os.Exit(1)
			}
		},
	}

	f.BindFlags(cmd.Flags())
	cmd.MarkFlagRequired("releases")      //nolint:errcheck
	cmd.MarkFlagRequired("architectures") //nolint:errcheck
	loadCmd.AddCommand(cmd)
}
