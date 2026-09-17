package testownershiploader

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4/stdlib"
	v1 "github.com/openshift-eng/ci-test-mapping/pkg/api/types/v1"
	"github.com/openshift-eng/ci-test-mapping/pkg/bigquery"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
)

// TestOwnershipLoader loads test ownership information from BigQuery. This data is generated and
// pushed to BigQuery from https://github.com/openshift-eng/ci-test-mapping
type TestOwnershipLoader struct {
	ctx             context.Context
	dbc             *db.DB
	mappingTableMgr *bigquery.MappingTableManager
	errors          []error
}

func New(ctx context.Context, dbc *db.DB, googleServiceAccountCredentialFile, googleOAuthClientCredentialFile string) (*TestOwnershipLoader, error) {
	client, err := bigquery.NewClient(ctx, googleServiceAccountCredentialFile, googleOAuthClientCredentialFile)
	if err != nil {
		return nil, err
	}
	mappingTableMgr := bigquery.NewMappingTableManager(ctx, client)

	return &TestOwnershipLoader{
		ctx:             ctx,
		dbc:             dbc,
		mappingTableMgr: mappingTableMgr,
	}, nil
}

func (tol *TestOwnershipLoader) Name() string {
	return "test ownership"
}

func (tol *TestOwnershipLoader) Load() {
	st := time.Now()
	mappings, err := tol.mappingTableMgr.ListMappings()
	if err != nil {
		tol.errors = append(tol.errors, err)
		return
	}
	log.WithFields(log.Fields{
		"mappings": len(mappings),
		"elapsed":  time.Since(st),
	}).Info("fetched test ownership mappings from BigQuery")

	if err := tol.loadMappings(mappings); err != nil {
		tol.errors = append(tol.errors, err)
	}
}

// loadMappings runs all database operations on a single pgx connection so the
// temp table remains visible across CREATE, COPY, upsert, and delete steps.
func (tol *TestOwnershipLoader) loadMappings(mappings []v1.TestOwnership) error {
	sqlDB, err := tol.dbc.DB.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB: %w", err)
	}
	conn, err := stdlib.AcquireConn(sqlDB)
	if err != nil {
		return fmt.Errorf("acquiring pgx conn: %w", err)
	}
	defer func() {
		if err := stdlib.ReleaseConn(sqlDB, conn); err != nil {
			log.WithError(err).Error("failed to release pgx conn")
		}
	}()

	st := time.Now()
	cleanup, err := db.CopyToTempTable(tol.ctx, conn, "tmp_test_ownerships", mappings,
		[]db.TempColumn[v1.TestOwnership]{
			{Name: "api_version", Type: "text NOT NULL DEFAULT ''", Value: func(m *v1.TestOwnership) any { return m.APIVersion }},
			{Name: "unique_id", Type: "text NOT NULL DEFAULT ''", Value: func(m *v1.TestOwnership) any { return m.ID }},
			{Name: "name", Type: "text NOT NULL DEFAULT ''", Value: func(m *v1.TestOwnership) any { return m.Name }},
			{Name: "suite", Type: "text NOT NULL DEFAULT ''", Value: func(m *v1.TestOwnership) any { return m.Suite }},
			{Name: "product", Type: "text NOT NULL DEFAULT ''", Value: func(m *v1.TestOwnership) any { return m.Product }},
			{Name: "priority", Type: "integer NOT NULL DEFAULT 0", Value: func(m *v1.TestOwnership) any { return m.Priority }},
			{Name: "staff_approved_obsolete", Type: "boolean NOT NULL DEFAULT false", Value: func(m *v1.TestOwnership) any { return m.StaffApprovedObsolete }},
			{Name: "component", Type: "text NOT NULL DEFAULT ''", Value: func(m *v1.TestOwnership) any { return m.Component }},
			{Name: "capabilities", Type: "text[]", Value: func(m *v1.TestOwnership) any { return m.Capabilities }},
			{Name: "jira_component", Type: "text NOT NULL DEFAULT ''", Value: func(m *v1.TestOwnership) any { return m.JIRAComponent }},
		},
	)
	defer cleanup()
	if err != nil {
		return err
	}
	log.WithField("elapsed", time.Since(st)).Info("COPY into temp table complete")

	st = time.Now()
	upsertTag, err := conn.Exec(tol.ctx, `
		INSERT INTO test_ownerships (
			api_version, unique_id, name, test_id, suite, suite_id,
			product, priority, staff_approved_obsolete, component,
			capabilities, jira_component, jira_component_id,
			created_at, updated_at
		)
		SELECT DISTINCT ON (tmp.name, tmp.suite)
			tmp.api_version, tmp.unique_id, tmp.name, t.id, tmp.suite, s.id,
			tmp.product, tmp.priority, tmp.staff_approved_obsolete, tmp.component,
			tmp.capabilities, tmp.jira_component, jc.id,
			NOW(), NOW()
		FROM tmp_test_ownerships tmp
		INNER JOIN tests t ON t.name = tmp.name AND t.deleted_at IS NULL
		LEFT JOIN suites s ON s.name = tmp.suite AND s.deleted_at IS NULL
		LEFT JOIN jira_components jc ON jc.name = tmp.jira_component
		ORDER BY tmp.name, tmp.suite, tmp.priority DESC
		ON CONFLICT (name, suite) DO UPDATE SET
			api_version             = EXCLUDED.api_version,
			unique_id               = EXCLUDED.unique_id,
			test_id                 = EXCLUDED.test_id,
			suite_id                = EXCLUDED.suite_id,
			product                 = EXCLUDED.product,
			priority                = EXCLUDED.priority,
			staff_approved_obsolete = EXCLUDED.staff_approved_obsolete,
			component               = EXCLUDED.component,
			capabilities            = EXCLUDED.capabilities,
			jira_component          = EXCLUDED.jira_component,
			jira_component_id       = EXCLUDED.jira_component_id,
			updated_at              = NOW()
	`)
	if err != nil {
		return fmt.Errorf("upserting test_ownerships: %w", err)
	}
	known := upsertTag.RowsAffected()
	log.WithFields(log.Fields{
		"rows":    known,
		"elapsed": time.Since(st),
	}).Info("upsert into test_ownerships complete")

	st = time.Now()
	deleteTag, err := conn.Exec(tol.ctx, `
		DELETE FROM test_ownerships
		WHERE NOT EXISTS (
			SELECT 1 FROM tmp_test_ownerships tmp
			WHERE tmp.name = test_ownerships.name AND tmp.suite = test_ownerships.suite
		)
		AND EXISTS (
			SELECT 1 FROM tests t
			WHERE t.id = test_ownerships.test_id AND t.deleted_at IS NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("deleting obsolete test_ownerships: %w", err)
	}

	unknown := int64(len(mappings)) - known
	if unknown > 0 {
		log.WithField("unknown", unknown).Warn("some test ownership mappings had no matching test in sippy")
	}

	var nullComponents []string
	if err := tol.dbc.DB.Raw(`
		SELECT DISTINCT jira_component FROM test_ownerships
		WHERE jira_component_id IS NULL AND jira_component != ''
	`).Scan(&nullComponents).Error; err != nil {
		log.WithError(err).Warn("failed to query unresolved jira components")
	}
	if len(nullComponents) > 0 {
		log.WithField("components", nullComponents).Warn("test ownership rows have unresolved jira_component_id")
	}

	log.WithFields(log.Fields{
		"known":    known,
		"unknown":  unknown,
		"obsolete": deleteTag.RowsAffected(),
		"elapsed":  time.Since(st),
	}).Info("component loading complete")

	return nil
}

func (tol *TestOwnershipLoader) Errors() []error {
	return tol.errors
}
