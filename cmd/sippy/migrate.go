package main

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift/sippy/pkg/db"
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

	rootCmd.AddCommand(cmd)
}
