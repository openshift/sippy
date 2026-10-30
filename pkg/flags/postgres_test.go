package flags

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestPostgresFlagsBindFlagsHidesDatabaseDSNFromUsage(t *testing.T) {
	const configuredDSN = "postgresql://sippy:secret@db.example.com/sippy"
	flags := &PostgresFlags{DSN: configuredDSN}
	flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	flags.BindFlags(flagSet)

	if flags.DSN != configuredDSN {
		t.Fatalf("BindFlags changed the configured DSN: got %q", flags.DSN)
	}

	usage := flagSet.FlagUsages()
	if strings.Contains(usage, configuredDSN) {
		t.Fatalf("flag usage exposed the configured DSN: %s", usage)
	}
	if got := flagSet.Lookup("database-dsn").DefValue; got != "<configured>" {
		t.Fatalf("database-dsn displayed default = %q, want %q", got, "<configured>")
	}

	const overrideDSN = "postgresql://other:secret@db.example.com/sippy"
	if err := flagSet.Parse([]string{"--database-dsn=" + overrideDSN}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if flags.DSN != overrideDSN {
		t.Fatalf("parsed database-dsn = %q, want %q", flags.DSN, overrideDSN)
	}
}
