package featuregateloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v4/stdlib"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

const (
	githubAPIBase   = "https://api.github.com"
	repoOwner       = "openshift"
	repoName        = "api"
	featureGatePath = "payload-manifests/featuregates"
)

type FeatureGateLoader struct {
	ctx            context.Context
	dbc            *db.DB
	httpClient     *http.Client
	githubToken    string
	errs           []error
	releaseConfigs []v1.Release
}

func New(ctx context.Context, dbc *db.DB, configs []v1.Release) *FeatureGateLoader {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	return &FeatureGateLoader{
		ctx:            ctx,
		dbc:            dbc,
		httpClient:     httpClient,
		githubToken:    os.Getenv("GITHUB_TOKEN"),
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

// githubContent represents a file entry from the GitHub Contents API.
type githubContent struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
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
	entries, err := l.listDirectory(branch)
	if err != nil {
		return nil, err
	}

	var dbFeatureGates []models.FeatureGate
	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}

		topology, featureSet, valid := parseFeatureGateFilename(entry.Name)
		if !valid {
			continue
		}

		if entry.DownloadURL == "" {
			l.errs = append(l.errs, fmt.Errorf("feature gate file %s on branch %s has no download URL", entry.Name, branch))
			continue
		}

		data, err := l.downloadFile(entry.DownloadURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download %s: %w", entry.Name, err)
		}

		var fg FeatureGate
		if err := yaml.Unmarshal(data, &fg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal YAML from %s: %w", entry.Name, err)
		}

		dbFeatureGates = append(dbFeatureGates, convertAPIToDB(fg, release, topology, featureSet, entry.DownloadURL)...)
	}

	return dbFeatureGates, nil
}

func (l *FeatureGateLoader) listDirectory(branch string) ([]githubContent, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s",
		githubAPIBase, repoOwner, repoName, featureGatePath, url.QueryEscape(branch))

	body, err := l.doGet(apiURL, map[string]string{"Accept": "application/vnd.github.v3+json"})
	if err != nil {
		return nil, fmt.Errorf("listing directory for branch %s: %w", branch, err)
	}

	var entries []githubContent
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decoding directory listing for branch %s: %w", branch, err)
	}

	return entries, nil
}

func (l *FeatureGateLoader) downloadFile(downloadURL string) ([]byte, error) {
	if !isAllowedDownloadURL(downloadURL) {
		return nil, fmt.Errorf("download URL not from an allowed host: %s", downloadURL)
	}
	return l.doGet(downloadURL, nil)
}

var allowedDownloadHosts = sets.New[string](
	"raw.githubusercontent.com",
	"objects.githubusercontent.com",
)

func isAllowedDownloadURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && allowedDownloadHosts.Has(u.Host)
}

// doGet performs an authenticated GET request and returns the response body.
func (l *FeatureGateLoader) doGet(targetURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(l.ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if l.githubToken != "" {
		req.Header.Set("Authorization", "token "+l.githubToken)
	}

	resp, err := l.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %s: %w", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found: %s", targetURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s from %s", resp.Status, targetURL)
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
