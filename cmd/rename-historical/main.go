package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/klog"
)

// when fixing the URL handling, the historical data files were not renamed.  This binary
// 1. reads the dir content
// 2. parses the names to find the start of the query params
// 3. reorders the query params to match the current expectations
// 4. moves the file
// use `ln -s ../4.4GA/* ../4.5GA/* ../4.6GA/* .` to recreate the links in common

type RenameOptions struct {
	DataDirs []string
	DryRun   bool
}

func main() {
	o := &RenameOptions{}

	klog.InitFlags(nil)
	flag.CommandLine.Set("skip_headers", "true")

	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, arguments []string) {
			if err := o.Run(); err != nil {
				klog.Exitf("error: %v", err)
			}
		},
	}
	flags := cmd.Flags()
	flags.StringSliceVar(&o.DataDirs, "data-dir", o.DataDirs, "Path to testgrid data from local disk")
	flags.BoolVar(&o.DryRun, "dry-run", o.DryRun, "if true, take no action")

	flags.AddGoFlag(flag.CommandLine.Lookup("v"))
	flags.AddGoFlag(flag.CommandLine.Lookup("skip_headers"))

	if err := cmd.Execute(); err != nil {
		klog.Exitf("error: %v", err)
	}
}

func (o *RenameOptions) Run() error {
	for _, dir := range o.DataDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			// new filenames have -grid=old in them, old filenames have &grid=old
			if !strings.Contains(info.Name(), "&grid=old") {
				return nil
			}
			newName := info.Name()
			newName = strings.ReplaceAll(newName, "&grid=old", "")
			newName = strings.ReplaceAll(newName, "&show-stale", "grid=old&show-stale")
			fmt.Printf("renaming %q to %q\n", info.Name(), newName)
			if o.DryRun {
				return nil
			}
			return os.Rename(filepath.Join(dir, info.Name()), filepath.Join(dir, newName))
		})
		if err != nil {
			panic(err)
		}
	}
	return nil
}
