package main

import (
	"fmt"
	"os"

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
		Run: func(cmd *cobra.Command, args []string) {
			dbc, err := db.New(f.DSN, gormlogger.LogLevel(f.LogLevel))
			if err != nil {
				fmt.Printf("could not connect to db: %+v", err)
				os.Exit(1)
			}

			t := f.GetPinnedTime()
			if err := dbc.UpdateSchema(t); err != nil {
				fmt.Printf("could not migrate db: %+v", err)
				os.Exit(1)
			}
		},
	}

	f.BindFlags(cmd.Flags())

	rootCmd.AddCommand(cmd)
}
