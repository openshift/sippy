package featuregateloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	v1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm/clause"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

type FeatureGateLoader struct {
	dbc            *db.DB
	errs           []error
	releaseConfigs []v1.Release
}

func New(dbc *db.DB, configs []v1.Release) *FeatureGateLoader {
	return &FeatureGateLoader{
		dbc:            dbc,
		releaseConfigs: configs,
	}
}

func (l *FeatureGateLoader) Name() string {
	return "feature_gates"
}

func (l *FeatureGateLoader) Load() {
	dbFeatureGates, err := getFeatureGatesForReleases(l.getTargetReleases())
	if err != nil {
		l.errs = append(l.errs, errors.Wrapf(err, "error querying feature gates for releases"))
		return
	}

	tx := l.dbc.DB.Clauses(clause.OnConflict{
		// Composite key
		Columns: []clause.Column{
			{Name: "release"},
			{Name: "topology"},
			{Name: "feature_set"},
			{Name: "feature_gate"},
		},
		// Only update the "status" field:
		DoUpdates: clause.AssignmentColumns([]string{"status"}),
	}).
		CreateInBatches(dbFeatureGates, 500)

	if tx.Error != nil {
		l.errs = append(l.errs, errors.Wrap(tx.Error, "error loading feature gates"))
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

	log.Infof("Found %d target releases from db: %s", len(targetReleases), strings.Join(targetReleases, ","))
	return targetReleases
}

func getFeatureGatesForReleases(releases []string) ([]models.FeatureGate, error) {
	if len(releases) == 0 {
		log.Infof("no releases found to load feature gates")
		return nil, nil
	}

	tempDir, err := os.MkdirTemp("", "openshift-api-*")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp directory")
	}
	defer os.RemoveAll(tempDir)

	cloneStart := time.Now()
	log.Infof("Cloning API repo into: %s", tempDir)
	// Clone the repository to the temporary directory
	repo, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL:      "https://github.com/openshift/api.git",
		Progress: os.Stdout,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to clone repo")
	}
	// Fetch remotes
	err = repo.Fetch(&git.FetchOptions{
		RefSpecs: []config.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch remotes")
	}
	log.Infof("Successfully cloned repo into %s after %+v", tempDir, time.Since(cloneStart))

	var dbFeatureGates []models.FeatureGate
	for _, release := range releases {
		log.Infof("Processing release %s", release)
		if err := switchBranch(repo, fmt.Sprintf("release-%s", release)); err != nil {
			return nil, errors.Wrapf(err, "couldn't switch to release-%s branch", release)
		}
		releaseFGs, err := listFeatureGates(release, tempDir)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list feature gates for %s", release)
		}
		dbFeatureGates = append(dbFeatureGates, releaseFGs...)
		log.Infof("Finished with %s", release)
	}

	return dbFeatureGates, nil
}

func listFeatureGates(release, tmpDir string) ([]models.FeatureGate, error) {
	targetDir := filepath.Join(tmpDir, "payload-manifests/featuregates")
	var dbFeatureGates []models.FeatureGate

	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if isFeatureGateFile(info) {
			featureGates, err := processFeatureGateFile(path, release, info.Name())
			if err != nil {
				return err
			}
			dbFeatureGates = append(dbFeatureGates, featureGates...)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get feature gates: %w", err)
	}
	return dbFeatureGates, nil
}

// isFeatureGateFile checks if the file follows the expected naming pattern
func isFeatureGateFile(info os.FileInfo) bool {
	return !info.IsDir() && strings.HasPrefix(info.Name(), "featureGate-") && strings.HasSuffix(info.Name(), ".yaml")
}

func processFeatureGateFile(path, release, filename string) ([]models.FeatureGate, error) {
	topology, featureSet, valid := parseFeatureGateFilename(filename)
	if !valid {
		log.Infof("Skipping malformed filename: %s", filename)
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	var fg FeatureGate
	if err := yaml.Unmarshal(data, &fg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML from %s: %w", path, err)
	}

	return convertAPIToDB(fg, release, topology, featureSet, path), nil
}

// parseFeatureGateFilename extracts topology and feature set from the filename
func parseFeatureGateFilename(filename string) (string, string, bool) {
	parts := strings.Split(strings.TrimSuffix(filename, ".yaml"), "-")
	if len(parts) < 3 {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// convertAPIToDB converts the parsed feature gate data into db models
func convertAPIToDB(fg FeatureGate, release, topology, featureSet, path string) []models.FeatureGate {
	var dbFeatureGates []models.FeatureGate
	fgLogger := log.WithField("release", release).
		WithField("topology", topology).
		WithField("featureSet", featureSet).
		WithField("path", path)

	fgLogger.Info("Found feature gate configuration file")

	for _, entry := range fg.Status.FeatureGates {
		for _, enabled := range entry.Enabled {
			fgLogger.WithField("featureGate", enabled.Name).Debugf("Found enabled feature gate")
			dbFeatureGates = append(dbFeatureGates, models.FeatureGate{
				Release:     release,
				Topology:    topology,
				FeatureSet:  featureSet,
				FeatureGate: enabled.Name,
				Status:      "enabled",
			})
		}
		for _, disabled := range entry.Disabled {
			fgLogger.WithField("featureGate", disabled.Name).Debugf("Found disabled feature gate")
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

func switchBranch(repo *git.Repository, branchName string) error {
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branchName)),
		Force:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branchName, err)
	}
	return nil
}
