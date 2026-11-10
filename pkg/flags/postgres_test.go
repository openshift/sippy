package flags

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestPostgresDSNHelpDoesNotExposeDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		dsn  string
	}{
		{name: "environment", dsn: "postgresql://user:dummy-secret@db.example.invalid:5432/prod"},
		{name: "development default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SIPPY_DATABASE_DSN", tc.dsn)
			flags := NewPostgresDatabaseFlags()
			initialDSN := flags.DSN
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.BindFlags(fs)

			if flags.DSN != initialDSN {
				t.Fatalf("binding flags changed runtime DSN")
			}
			if strings.Contains(fs.FlagUsages(), initialDSN) {
				t.Fatal("flag help exposes the runtime DSN")
			}
			if !strings.Contains(fs.FlagUsages(), "example: postgresql://user:password@db.example.invalid:5432/database") {
				t.Fatal("flag help does not show the dummy DSN example")
			}
			if err := fs.Parse([]string{"--database-dsn=postgresql://override@localhost/db"}); err != nil {
				t.Fatal(err)
			}
			if flags.DSN != "postgresql://override@localhost/db" {
				t.Fatalf("flag override was not applied")
			}
		})
	}
}
