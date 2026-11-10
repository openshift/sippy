package main

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

func TestBackfillCommand_ParsesExcludedReleases(t *testing.T) {
	flags := NewBackfillFlags()
	flagSet := pflag.NewFlagSet("backfill", pflag.ContinueOnError)
	flags.BindFlags(flagSet)
	err := flagSet.Parse([]string{"--exclude-release=Presubmits", "--exclude-release=4.21"})
	require.NoError(t, err)
	require.Equal(t, []string{"Presubmits", "4.21"}, flags.ExcludedReleases)
}
