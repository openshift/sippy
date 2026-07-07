package releasedefloader

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm/clause"

	"github.com/openshift/sippy/pkg/api"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

// ReleaseDefinitionLoader fetches release metadata from BigQuery and syncs it to PostgreSQL.
type ReleaseDefinitionLoader struct {
	ctx      context.Context
	dbc      *db.DB
	bqClient *bqcachedclient.Client
	errs     []error
}

func NewReleaseDefinitionLoader(ctx context.Context, dbc *db.DB, bqClient *bqcachedclient.Client) *ReleaseDefinitionLoader {
	return &ReleaseDefinitionLoader{
		ctx:      ctx,
		dbc:      dbc,
		bqClient: bqClient,
	}
}

func (l *ReleaseDefinitionLoader) Name() string {
	return "release-definitions"
}

func (l *ReleaseDefinitionLoader) Load() {
	releaseRows, err := api.GetReleaseRowsFromBigQuery(l.ctx, l.bqClient)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("fetching releases from bigquery: %w", err))
		return
	}
	defs := make([]models.ReleaseDefinition, 0, len(releaseRows))
	for _, row := range releaseRows {
		defs = append(defs, ReleaseRowToDefinition(row))
	}
	if err := syncReleaseDefinitions(l.dbc, defs); err != nil {
		l.errs = append(l.errs, fmt.Errorf("syncing release definitions: %w", err))
	}
}

func (l *ReleaseDefinitionLoader) Errors() []error {
	return l.errs
}

func syncReleaseDefinitions(dbc *db.DB, defs []models.ReleaseDefinition) error {
	for _, def := range defs {
		err := dbc.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "release"}},
			DoUpdates: clause.AssignmentColumns([]string{"major", "minor", "patch", "previous_release", "ga_date", "development_start_date", "product", "status", "capabilities", "updated_at"}),
		}).Create(&def).Error
		if err != nil {
			return fmt.Errorf("upserting release definition %s: %w", def.Release, err)
		}
	}
	log.WithField("count", len(defs)).Info("synced release definitions to postgres")
	return nil
}

// ReleaseRowToDefinition converts a BigQuery ReleaseRow directly to a ReleaseDefinition DB model.
func ReleaseRowToDefinition(r v1.ReleaseRow) models.ReleaseDefinition {
	caps := make(pq.StringArray, 0, len(r.Capabilities))
	for _, cap := range r.Capabilities {
		caps = append(caps, string(cap))
	}
	sort.Strings(caps)

	def := models.ReleaseDefinition{
		Release:         r.Release,
		Major:           r.Major,
		Minor:           r.Minor,
		PreviousRelease: r.PreviousRelease.StringVal,
		Product:         r.Product.StringVal,
		Status:          r.ReleaseStatus.StringVal,
		Capabilities:    caps,
	}
	if r.Patch.Valid {
		p := int(r.Patch.Int64)
		def.Patch = &p
	}
	if r.GADate.Valid {
		ga := r.GADate.Date.In(time.UTC)
		def.GADate = &ga
	}
	if r.DevelStartDate.IsValid() {
		ds := r.DevelStartDate.In(time.UTC)
		def.DevelopmentStartDate = &ds
	}
	return def
}
