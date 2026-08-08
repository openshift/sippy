package util

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/openshift/sippy/pkg/db"

	gormlogger "gorm.io/gorm/logger"
)

func init() {
	configurePodmanIfNeeded()
}

// configurePodmanIfNeeded detects Podman and sets the environment
// variables that testcontainers-go needs to use it.
func configurePodmanIfNeeded() {
	if os.Getenv("DOCKER_HOST") != "" {
		return
	}

	podmanPath, err := exec.LookPath("podman")
	if err != nil || podmanPath == "" {
		return
	}

	socketPath := podmanSocketPath()
	if socketPath == "" {
		return
	}

	if !strings.HasPrefix(socketPath, "unix://") {
		socketPath = "unix://" + socketPath
	}
	os.Setenv("DOCKER_HOST", socketPath)
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
}

// podmanSocketPath returns the host-side Podman socket path.
// It tries "podman machine inspect" first (macOS/Windows where Podman
// runs in a VM), then falls back to "podman info" (Linux where Podman
// runs natively).
func podmanSocketPath() string {
	out, err := exec.Command("podman", "machine", "inspect", "--format", "{{.ConnectionInfo.PodmanSocket.Path}}").Output()
	if err == nil {
		if path := strings.TrimSpace(string(out)); path != "" {
			return path
		}
	}

	out, err = exec.Command("podman", "info", "--format", "{{.Host.RemoteSocket.Path}}").Output()
	if err == nil {
		if path := strings.TrimSpace(string(out)); path != "" {
			return path
		}
	}

	return ""
}

const (
	postgresImage    = "docker.io/library/postgres:16"
	postgresUser     = "test"
	postgresPassword = "test"
	postgresDB       = "sippy_integration"
	templateDB       = "template_integration"
)

// PostgresContainer holds a running testcontainers Postgres instance and
// provides helpers to create per-test database clones.
type PostgresContainer struct {
	container testcontainers.Container
	baseDSN   string
}

// StartPostgresContainer launches a Postgres container, creates a template
// database with the integration schema applied, and returns a handle for
// creating per-test clones. Call cleanup() (or use the returned container's
// Terminate) when done.
func StartPostgresContainer(ctx context.Context) (*PostgresContainer, error) {
	pgContainer, err := tcpostgres.Run(ctx,
		postgresImage,
		tcpostgres.WithDatabase(postgresDB),
		tcpostgres.WithUsername(postgresUser),
		tcpostgres.WithPassword(postgresPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("starting postgres container: %w", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		if termErr := pgContainer.Terminate(ctx); termErr != nil {
			err = fmt.Errorf("%w (also failed to terminate container: %v)", err, termErr)
		}
		return nil, fmt.Errorf("getting connection string: %w", err)
	}

	if err := createTemplateDB(connStr); err != nil {
		if termErr := pgContainer.Terminate(ctx); termErr != nil {
			err = fmt.Errorf("%w (also failed to terminate container: %v)", err, termErr)
		}
		return nil, fmt.Errorf("creating template database: %w", err)
	}

	return &PostgresContainer{
		container: pgContainer,
		baseDSN:   connStr,
	}, nil
}

// ConnectToExternalPostgres connects to an existing Postgres instance and
// creates a template database with the integration schema applied. Use this
// in CI environments where a Postgres service is provided externally and
// testcontainers cannot be used (no container runtime available).
func ConnectToExternalPostgres(ctx context.Context, dsn string) (*PostgresContainer, error) {
	if err := createTemplateDB(dsn); err != nil {
		return nil, fmt.Errorf("creating template database: %w", err)
	}
	return &PostgresContainer{
		baseDSN: dsn,
	}, nil
}

// Terminate stops and removes the container. When using an external
// Postgres (container is nil), this is a no-op.
func (pc *PostgresContainer) Terminate(ctx context.Context) error {
	if pc.container != nil {
		return pc.container.Terminate(ctx)
	}
	return nil
}

// createTemplateDB creates a fresh database, applies the integration schema,
// and marks it as a template for fast cloning.
func createTemplateDB(baseDSN string) error {
	adminDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		return fmt.Errorf("connecting to admin db: %w", err)
	}
	defer adminDB.Close()

	// Unmark as template first (required before dropping a template database),
	// then drop if it exists. Ignore errors here because the database may not
	// exist yet on a fresh Postgres instance.
	_, _ = adminDB.Exec(fmt.Sprintf("ALTER DATABASE %s IS_TEMPLATE = false", templateDB))
	if _, err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", templateDB)); err != nil {
		return fmt.Errorf("dropping old template: %w", err)
	}
	if _, err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", templateDB)); err != nil {
		return fmt.Errorf("creating template db: %w", err)
	}

	templateDSN := replaceDBName(baseDSN, templateDB)
	dbc, err := db.New(templateDSN, gormlogger.Silent)
	if err != nil {
		return fmt.Errorf("connecting to template db: %w", err)
	}
	sqlDB, err := dbc.DB.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB from template: %w", err)
	}
	if err := SetupIntegrationSchema(dbc); err != nil {
		sqlDB.Close()
		return fmt.Errorf("setting up schema: %w", err)
	}

	sqlDB.Close()

	if _, err := adminDB.Exec(fmt.Sprintf("ALTER DATABASE %s IS_TEMPLATE = true", templateDB)); err != nil {
		return fmt.Errorf("marking db as template: %w", err)
	}

	return nil
}

// NewTestDB creates a fresh database cloned from the template and returns
// a *db.DB connected to it. The database is dropped when the test finishes.
func NewTestDB(t *testing.T, pc *PostgresContainer) *db.DB {
	t.Helper()

	dbName := fmt.Sprintf("test_%s_%d", sanitize(t.Name()), time.Now().UnixNano())

	adminDB, err := sql.Open("pgx", pc.baseDSN)
	if err != nil {
		t.Fatalf("connecting to admin db: %v", err)
	}

	if _, err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", dbName, templateDB)); err != nil {
		adminDB.Close()
		t.Fatalf("cloning template db: %v", err)
	}
	adminDB.Close()

	testDSN := replaceDBName(pc.baseDSN, dbName)
	dbc, err := db.New(testDSN, gormlogger.Warn)
	if err != nil {
		dropTestDB(t, pc.baseDSN, dbName)
		t.Fatalf("connecting to test db: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, dbErr := dbc.DB.DB()
		if dbErr == nil {
			sqlDB.Close()
		}
		dropTestDB(t, pc.baseDSN, dbName)
	})

	return dbc
}

func dropTestDB(t *testing.T, dsn, dbName string) {
	t.Helper()
	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Logf("warning: failed to connect to admin db to drop %s: %v", dbName, err)
		return
	}
	defer adminDB.Close()
	if _, err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)); err != nil {
		t.Logf("warning: failed to drop test database %s: %v", dbName, err)
	}
}

func replaceDBName(dsn, newDB string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	u.Path = "/" + newDB
	return u.String()
}

// sanitize replaces characters that are not valid in Postgres identifiers.
func sanitize(name string) string {
	result := make([]byte, 0, len(name))
	for i := range len(name) {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // lowercase
		} else {
			result = append(result, '_')
		}
	}
	if len(result) > 38 {
		result = result[:38]
	}
	return string(result)
}
