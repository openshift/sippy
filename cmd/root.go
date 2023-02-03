package cmd

import (
	"io/fs"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sippy",
	Short: "CIPI (Continuous Integration Private Investigator) aka Sippy",
	Long: `Sippy reports on job and test statistics, sliced by various filters
including name, suite, or NURP+ variants (network, upgrade, release,
platform, etc).`,
}

var frontendFS, staticFS fs.FS

func Execute(frontend, staticAssets fs.FS) {
	frontendFS = frontend
	staticFS = staticAssets

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {

}
