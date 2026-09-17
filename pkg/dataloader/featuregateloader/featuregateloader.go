package featuregateloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	gh "github.com/google/go-github/v45/github"
	"github.com/jackc/pgx/v4/stdlib"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

const (
	repoOwner       = "openshift"
	repoName        = "api"
	featureGatePath = "payload-manifests/featuregates"
)

type FeatureGateLoader struct {
	ctx            context.Context
	dbc            *db.DB
	ghClient       *gh.Client
	errs           []error
	releaseConfigs []v1.Release
}

func New(ctx context.Context, dbc *db.DB, ghClient *gh.Client, configs []v1.Release) *FeatureGateLoader {
	return &FeatureGateLoader{
		ctx:            ctx,
		dbc:            dbc,
		ghClient:       ghClient,
		releaseConfigs: configs,
	}
}

func (l *FeatureGateLoader) Name() string {
	return "feature_gates"
}

func (l *FeatureGateLoader) Load() {
	releases := l.getTargetReleases()
	dbFeatureGates := l.getFeatureGatesFromGitHub(releases)

	if err := l.upsertFeatureGates(dbFeatureGates); err != nil {
		l.errs = append(l.errs, fmt.Errorf("error upserting feature gates: %w", err))
	}
}

func (l *FeatureGateLoader) Errors() []error {
	return l.errs
}

func (l *FeatureGateLoader) getTargetReleases() []string {
	var targetReleases []string
	for _, release := range l.releaseConfigs {
		if release.Capabilities[v1.FeatureGatesCap] {
			targetReleases = append(targetReleases, release.Release)
		}
	}

	log.WithFields(log.Fields{
		"releases": strings.Join(targetReleases, ","),
		"count":    len(targetReleases),
	}).Info("found target releases for feature gates")
	return targetReleases
}

func (l *FeatureGateLoader) getFeatureGatesFromGitHub(releases []string) []models.FeatureGate {
	if len(releases) == 0 {
		log.Info("no releases found to load feature gates")
		return nil
	}

	var dbFeatureGates []models.FeatureGate
	for _, release := range releases {
		rLog := log.WithField("release", release)
		rLog.Info("fetching feature gates from GitHub")

		branch := fmt.Sprintf("release-%s", release)
		fgs, err := l.fetchFeatureGatesForBranch(release, branch)
		if err != nil {
			l.errs = append(l.errs, fmt.Errorf("failed to fetch feature gates for release %s: %w", release, err))
			continue
		}
		dbFeatureGates = append(dbFeatureGates, fgs...)
		rLog.WithField("count", len(fgs)).Info("finished processing release")
	}

	return dbFeatureGates
}

func (l *FeatureGateLoader) fetchFeatureGatesForBranch(release, branch string) ([]models.FeatureGate, error) {
	opts := &gh.RepositoryContentGetOptions{Ref: branch}
	_, entries, _, err := l.ghClient.Repositories.GetContents(l.ctx, repoOwner, repoName, featureGatePath, opts)
	if err != nil {
		return nil, fmt.Errorf("listing directory for branch %s: %w", branch, err)
	}

	var dbFeatureGates []models.FeatureGate
	for _, entry := range entries {
		if entry.GetType() != "file" {
			continue
		}

		topology, featureSet, valid := parseFeatureGateFilename(entry.GetName())
		if !valid {
			continue
		}

		downloadURL := entry.GetDownloadURL()
		if downloadURL == "" {
			l.errs = append(l.errs, fmt.Errorf("feature gate file %s on branch %s has no download URL", entry.GetName(), branch))
			continue
		}

		data, err := l.downloadFile(downloadURL)
		if err != nil {
			l.errs = append(l.errs, fmt.Errorf("failed to download %s: %w", entry.GetName(), err))
			continue
		}

		var fg FeatureGate
		if err := yaml.Unmarshal(data, &fg); err != nil {
			l.errs = append(l.errs, fmt.Errorf("failed to unmarshal YAML from %s: %w", entry.GetName(), err))
			continue
		}

		dbFeatureGates = append(dbFeatureGates, convertAPIToDB(fg, release, topology, featureSet, downloadURL)...)
	}

	return dbFeatureGates, nil
}

func (l *FeatureGateLoader) downloadFile(downloadURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(l.ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", downloadURL, err)
	}
	resp, err := l.ghClient.Client().Do(req) //nolint:gosec // URL comes from GitHub Contents API response
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s from %s", resp.Status, downloadURL)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

var featureGateTempCols = []db.TempColumn[models.FeatureGate]{
	{Name: "release", Type: "text NOT NULL", Value: func(fg *models.FeatureGate) any { return fg.Release }},
	{Name: "topology", Type: "text NOT NULL", Value: func(fg *models.FeatureGate) any { return fg.Topology }},
	{Name: "feature_set", Type: "text NOT NULL", Value: func(fg *models.FeatureGate) any { return fg.FeatureSet }},
	{Name: "feature_gate", Type: "text NOT NULL", Value: func(fg *models.FeatureGate) any { return fg.FeatureGate }},
	{Name: "status", Type: "text NOT NULL", Value: func(fg *models.FeatureGate) any { return fg.Status }},
}

func (l *FeatureGateLoader) upsertFeatureGates(featureGates []models.FeatureGate) error {
	if len(featureGates) == 0 {
		return nil
	}

	st := time.Now()
	sqlDB, err := l.dbc.DB.DB()
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

	cleanup, err := db.CopyToTempTable(l.ctx, conn, "tmp_feature_gates", featureGates, featureGateTempCols)
	if err != nil {
		return fmt.Errorf("COPY tmp_feature_gates: %w", err)
	}
	defer cleanup()

	log.WithFields(log.Fields{
		"rows":    len(featureGates),
		"elapsed": time.Since(st),
	}).Info("COPY into temp table complete")

	st = time.Now()
	upsertTag, err := conn.Exec(l.ctx, `
		INSERT INTO feature_gates (release, topology, feature_set, feature_gate, status, created_at, updated_at)
		SELECT release, topology, feature_set, feature_gate, status, NOW(), NOW()
		FROM tmp_feature_gates
		ON CONFLICT (release, topology, feature_set, feature_gate) DO UPDATE SET
			status     = EXCLUDED.status,
			updated_at = NOW()
	`)
	if err != nil {
		return fmt.Errorf("upserting feature_gates: %w", err)
	}
	log.WithFields(log.Fields{
		"rows":    upsertTag.RowsAffected(),
		"elapsed": time.Since(st),
	}).Info("upsert into feature_gates complete")

	return nil
}

var featureGateFilenameRe = regexp.MustCompile(
	`^featureGate-(?:\d+-\d+-)?(?P<topology>[^-]+)-(?P<featureSet>[^-]+)\.yaml$`,
)

// parseFeatureGateFilename extracts topology and feature set from the filename.
// Handles both old format (featureGate-{topology}-{featureSet}.yaml) and
// new versioned format (featureGate-{majorStart}-{majorEnd}-{topology}-{featureSet}.yaml).
func parseFeatureGateFilename(filename string) (string, string, bool) {
	m := featureGateFilenameRe.FindStringSubmatch(filename)
	if m == nil {
		return "", "", false
	}
	return m[featureGateFilenameRe.SubexpIndex("topology")],
		m[featureGateFilenameRe.SubexpIndex("featureSet")], true
}

func convertAPIToDB(fg FeatureGate, release, topology, featureSet, path string) []models.FeatureGate {
	var dbFeatureGates []models.FeatureGate
	fgLogger := log.WithFields(log.Fields{
		"release":    release,
		"topology":   topology,
		"featureSet": featureSet,
		"path":       path,
	})

	fgLogger.Info("found feature gate configuration file")

	for _, entry := range fg.Status.FeatureGates {
		for _, enabled := range entry.Enabled {
			fgLogger.WithField("featureGate", enabled.Name).Debug("found enabled feature gate")
			dbFeatureGates = append(dbFeatureGates, models.FeatureGate{
				Release:     release,
				Topology:    topology,
				FeatureSet:  featureSet,
				FeatureGate: enabled.Name,
				Status:      "enabled",
			})
		}
		for _, disabled := range entry.Disabled {
			fgLogger.WithField("featureGate", disabled.Name).Debug("found disabled feature gate")
			dbFeatureGates = append(dbFeatureGates, models.FeatureGate{
				Release:     release,
				Topology:    topology,
				FeatureSet:  featureSet,
				FeatureGate: disabled.Name,
				Status:      "disabled",
			})
		}
	}
	return dbFeatureGates
}
