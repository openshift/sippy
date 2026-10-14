package db

import (
	"testing"

	"github.com/jackc/pgx/v4"
)

func TestDefaultApplicationNameUsesHostname(t *testing.T) {
	t.Setenv("SIPPY_PG_APPLICATION_NAME", "")

	got, err := DefaultApplicationName()
	if err != nil {
		t.Fatalf("DefaultApplicationName() returned an unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("DefaultApplicationName() returned an empty name")
	}

	if got, want := defaultPostgresApplicationName("sippy-api-7d9f"), "sippy/sippy-api-7d9f"; got != want {
		t.Fatalf("defaultPostgresApplicationName = %q, want %q", got, want)
	}
}

func TestDefaultApplicationNameTruncatesLongNames(t *testing.T) {
	name := defaultPostgresApplicationName("sippy-api-012345678901234567890123456789012345678901234567890123456789")
	if len(name) != 63 {
		t.Fatalf("application name length = %d, want 63", len(name))
	}
}

func TestDefaultApplicationNameRejectsLongExplicitName(t *testing.T) {
	t.Setenv("SIPPY_PG_APPLICATION_NAME", "sippy/012345678901234567890123456789012345678901234567890123456789")

	if _, err := DefaultApplicationName(); err == nil {
		t.Fatal("DefaultApplicationName() returned no error for an oversized explicit name")
	}
}

func TestConfigurePgxApplicationName(t *testing.T) {
	config, err := pgx.ParseConfig("postgresql://localhost/sippy")
	if err != nil {
		t.Fatalf("parsing test DSN: %v", err)
	}

	configurePgxApplicationName(config, "sippy-api/sippy-7d9f")

	if got, want := config.RuntimeParams["application_name"], "sippy-api/sippy-7d9f"; got != want {
		t.Fatalf("application_name = %q, want %q", got, want)
	}
}
