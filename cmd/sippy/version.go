package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/openshift/sippy/pkg/version"
)

// NewVersionCommand prints out sippy version information, in a similar style to other kube tools,
// e.g. https://github.com/openshift/kubernetes/blob/6892b57d65d25fb0588693bce3d338d8b8b1d2b4/cmd/kubeadm/app/cmd/version.go#L67
func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Report version information for sippy",
		// Set persistent pre run to empty so we don't double print version info
		PersistentPreRun: NoPrintVersion,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := version.Get()
			const flag = "output"
			of, err := cmd.Flags().GetString(flag)
			if err != nil {
				return errors.Wrapf(err, "error accessing flag %s for command %s", flag, cmd.Name())
			}
			switch of {
			case "":
				fmt.Fprintf(os.Stdout, "sippy built from %s\n", v.GitCommit)
			case "short":
				fmt.Fprintf(os.Stdout, "%s\n", v.GitCommit)
			case "yaml":
				y, err := yaml.Marshal(&v)
				if err != nil {
					return err
				}
				fmt.Fprintln(os.Stdout, string(y))
			case "json":
				y, err := json.MarshalIndent(&v, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(os.Stdout, string(y))
			default:
				return errors.Errorf("invalid output format: %s", of)
			}

			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "Output format; available options are 'yaml', 'json' and 'short'")
	return cmd
}

// PrintVersion is used as a PersistentPreRun function to ensure we always print the version.
var PrintVersion = func(cmd *cobra.Command, args []string) {
	fmt.Fprintf(os.Stdout, "sippy built from %s\n", version.Get().GitCommit)
}

// NoPrintVersion is used as an empty PersistentPreRun function so we don't print version info
// for some commands.
var NoPrintVersion = func(cmd *cobra.Command, args []string) {
}
