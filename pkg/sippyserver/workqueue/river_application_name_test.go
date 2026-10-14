package workqueue

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestConfigurePgxV5ApplicationName(t *testing.T) {
	config, err := pgx.ParseConfig("postgresql://localhost/sippy")
	if err != nil {
		t.Fatalf("parsing test DSN: %v", err)
	}

	configurePgxV5ApplicationName(config, "sippy-daemon/sippy-7d9f")

	if got, want := config.RuntimeParams["application_name"], "sippy-daemon/sippy-7d9f"; got != want {
		t.Fatalf("application_name = %q, want %q", got, want)
	}
}
