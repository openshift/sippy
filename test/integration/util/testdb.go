package util

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
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
)

func randomSuffix() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random suffix: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// PostgresContainer holds a Postgres instance (either a testcontainers
// container or an external server) and provides helpers to create per-test
// database clones.
type PostgresContainer struct {
	container  testcontainers.Container
	baseDSN    string
	templateDB string
}

// StartPostgresContainer returns a PostgresContainer backed by either an
// external Postgres server (when INTEGRATION_DATABASE_DSN is set) or a
// testcontainers-managed container. The external DSN path allows CI
// environments to supply a sidecar Postgres without needing a container
// runtime.
func StartPostgresContainer(ctx context.Context) (*PostgresContainer, error) {
	if dsn := os.Getenv("INTEGRATION_DATABASE_DSN"); dsn != "" {
		log.Printf("Using external Postgres from INTEGRATION_DATABASE_DSN")
		return startExternalPostgres(dsn)
	}
	log.Printf("Starting Postgres via testcontainers (set INTEGRATION_DATABASE_DSN to use an external instance)")
	return startTestcontainersPostgres(ctx)
}

func startExternalPostgres(dsn string) (*PostgresContainer, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return nil, fmt.Errorf("INTEGRATION_DATABASE_DSN must be a URL (e.g. postgresql://host/db)")
	}

	templateDB, err := createTemplateDB(dsn)
	if err != nil {
		return nil, fmt.Errorf("creating template database on external postgres: %w", err)
	}
	return &PostgresContainer{baseDSN: dsn, templateDB: templateDB}, nil
}

func startTestcontainersPostgres(ctx context.Context) (*PostgresContainer, error) {
	configurePodmanIfNeeded()

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

	templateDB, err := createTemplateDB(connStr)
	if err != nil {
		if termErr := pgContainer.Terminate(ctx); termErr != nil {
			err = fmt.Errorf("%w (also failed to terminate container: %v)", err, termErr)
		}
		return nil, fmt.Errorf("creating template database: %w", err)
	}

	return &PostgresContainer{
		container:  pgContainer,
		baseDSN:    connStr,
		templateDB: templateDB,
	}, nil
}

// Terminate stops and removes the container, or cleans up the template
// database for external Postgres instances.
func (pc *PostgresContainer) Terminate(ctx context.Context) error {
	if pc.container != nil {
		return pc.container.Terminate(ctx)
	}
	return dropTemplateDB(ctx, pc.baseDSN, pc.templateDB)
}

func dropTemplateDB(ctx context.Context, baseDSN, templateDB string) error {
	adminDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		return fmt.Errorf("connecting to admin db: %w", err)
	}
	defer adminDB.Close()
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("ALTER DATABASE %s IS_TEMPLATE = false", templateDB)); err != nil {
		return fmt.Errorf("unmarking template db: %w", err)
	}
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", templateDB)); err != nil {
		return fmt.Errorf("dropping template db: %w", err)
	}
	return nil
}

// createTemplateDB creates a fresh database with a random name, applies the
// integration schema, and marks it as a template for fast cloning.
func createTemplateDB(baseDSN string) (string, error) {
	templateDB := "tmpl_" + randomSuffix()

	adminDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		return "", fmt.Errorf("connecting to admin db: %w", err)
	}
	defer adminDB.Close()

	if _, err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", templateDB)); err != nil {
		return "", fmt.Errorf("creating template db: %w", err)
	}
	cleanupOnErr := func(err error) (string, error) {
		_, _ = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", templateDB))
		return "", err
	}

	templateDSN := replaceDBName(baseDSN, templateDB)
	dbc, err := db.New(templateDSN, gormlogger.Silent)
	if err != nil {
		return cleanupOnErr(fmt.Errorf("connecting to template db: %w", err))
	}
	sqlDB, err := dbc.DB.DB()
	if err != nil {
		return cleanupOnErr(fmt.Errorf("getting sql.DB from template: %w", err))
	}
	if err := SetupIntegrationSchema(dbc); err != nil {
		sqlDB.Close()
		return cleanupOnErr(fmt.Errorf("setting up schema: %w", err))
	}

	sqlDB.Close()

	if _, err := adminDB.Exec(fmt.Sprintf("ALTER DATABASE %s IS_TEMPLATE = true", templateDB)); err != nil {
		return "", fmt.Errorf("marking db as template: %w", err)
	}

	return templateDB, nil
}

// NewTestDB creates a fresh database cloned from the template and returns
// a *db.DB connected to it. The database is dropped when the test finishes.
func NewTestDB(t *testing.T, pc *PostgresContainer) *db.DB {
	t.Helper()

	dbName := "test_" + randomSuffix()

	adminDB, err := sql.Open("pgx", pc.baseDSN)
	if err != nil {
		t.Fatalf("connecting to admin db: %v", err)
	}

	if _, err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", dbName, pc.templateDB)); err != nil {
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
