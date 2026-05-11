package main

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift/sippy/pkg/db"
	sippymigrate "github.com/openshift/sippy/pkg/db/migrate"
	"github.com/openshift/sippy/pkg/flags"
)

func init() {
	f := flags.NewPostgresDatabaseFlags()

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrates or initializes the PostgreSQL database to the latest schema.",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbc, err := db.New(f.DSN, gormlogger.LogLevel(f.LogLevel))
			if err != nil {
				return errors.WithMessage(err, "could not connect to db")
			}

			t := f.GetPinnedTime()
			if err := dbc.UpdateSchema(t); err != nil {
				return errors.WithMessage(err, "could not migrate db")
			}

			return nil
		},
	}

	f.BindFlags(cmd.Flags())

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the current migration version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbc, err := db.New(f.DSN, gormlogger.LogLevel(f.LogLevel))
			if err != nil {
				return errors.WithMessage(err, "could not connect to db")
			}

			version, dirty, err := sippymigrate.CurrentVersion(dbc.DB)
			if err != nil {
				return errors.WithMessage(err, "could not get migration version")
			}

			fmt.Printf("version: %d, dirty: %v\n", version, dirty)
			return nil
		},
	}
	f.BindFlags(versionCmd.Flags())

	forceCmd := &cobra.Command{
		Use:   "force VERSION",
		Short: "Force the migration version without running migrations. Use to recover from a dirty state.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := strconv.Atoi(args[0])
			if err != nil {
				return errors.WithMessage(err, "invalid version number")
			}

			dbc, err := db.New(f.DSN, gormlogger.LogLevel(f.LogLevel))
			if err != nil {
				return errors.WithMessage(err, "could not connect to db")
			}

			if err := sippymigrate.ForceVersion(dbc.DB, version); err != nil {
				return errors.WithMessage(err, "could not force migration version")
			}

			fmt.Printf("forced migration version to %d\n", version)
			return nil
		},
	}
	f.BindFlags(forceCmd.Flags())

	cmd.AddCommand(versionCmd, forceCmd)
	rootCmd.AddCommand(cmd)
}
