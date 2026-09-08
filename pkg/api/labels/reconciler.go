package labels

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/infrafailure"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/openshift/sippy/pkg/db/query"
)

var (
	reconciliationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sippy_label_reconciliation_total",
		Help: "Total job label reconciliation attempts by outcome.",
	}, []string{"outcome"})
	reconciliationChanges = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sippy_label_reconciliation_changes_total",
		Help: "Total labels changed by reconciliation operation.",
	}, []string{"operation"})
)

// ReconcileRequest identifies the job whose authoritative label set must be
// reconciled. The bucket and GCS path are loaded from Sippy's run metadata.
type ReconcileRequest struct {
	RunID string `json:"run_id"`
}

// ReconcileOutcome describes the externally relevant result of a reconciliation.
type ReconcileOutcome int

const (
	ReconcileOutcomeError ReconcileOutcome = iota
	ReconcileOutcomeApplied
	ReconcileOutcomeNoOp
	ReconcileOutcomeRunNotFound
)

// ReconcileResult reports the full-set changes made by one reconciliation.
type ReconcileResult struct {
	RunID   string `json:"run_id"`
	Added   int    `json:"added"`
	Updated int    `json:"updated"`
	Removed int    `json:"removed"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Reconciler reads the authoritative GCS label set and applies it transactionally.
type Reconciler struct {
	dbc       *db.DB
	gcsClient *storage.Client
	read      func(context.Context, *storage.Client, string, string) (map[string]DiscoveredLabel, error)
}

// NewReconciler constructs a GCS-backed label reconciler.
func NewReconciler(dbc *db.DB, gcsClient *storage.Client) *Reconciler {
	return &Reconciler{dbc: dbc, gcsClient: gcsClient, read: ReadLabels}
}

// ValidateReconcileRequest validates the job identity supplied by the caller.
func ValidateReconcileRequest(request ReconcileRequest) error {
	if request.RunID == "" {
		return fmt.Errorf("run_id is required")
	}
	return nil
}

const missingRunMessage = "job run not found; labels will be read from GCS when the run is loaded"

// Reconcile reads GCS before opening the database transaction. Any GCS list,
// read, or parse failure therefore cannot be mistaken for an empty set.
func (r *Reconciler) Reconcile(ctx context.Context, request ReconcileRequest) (ReconcileResult, ReconcileOutcome) {
	reconciliationTotal.WithLabelValues("attempt").Inc()
	result := ReconcileResult{RunID: request.RunID}
	if err := ValidateReconcileRequest(request); err != nil {
		reconciliationTotal.WithLabelValues("malformed").Inc()
		result.Error = err.Error()
		return result, ReconcileOutcomeError
	}
	runID, err := strconv.ParseInt(request.RunID, 10, 64)
	if err != nil {
		result.Error = fmt.Sprintf("invalid run ID %q: %v", request.RunID, err)
		return result, ReconcileOutcomeError
	}
	if r == nil || r.dbc == nil || r.dbc.DB == nil || r.gcsClient == nil || r.read == nil {
		result.Error = "label reconciliation is not configured"
		return result, ReconcileOutcomeError
	}
	runMetadata, found, err := r.loadRunMetadata(ctx, runID)
	if err != nil {
		result.Error = err.Error()
		return result, ReconcileOutcomeError
	}
	if !found {
		result.Message = missingRunMessage
		return result, ReconcileOutcomeRunNotFound
	}
	desired, err := r.readAuthoritativeLabels(ctx, request.RunID, runMetadata)
	if err != nil {
		result.Error = err.Error()
		return result, ReconcileOutcomeError
	}

	reconciliation, outcome, err := r.reconcileTransaction(ctx, runID, request.RunID, desired)
	if err != nil {
		reconciliationTotal.WithLabelValues("failure").Inc()
		result.Error = err.Error()
		log.WithError(err).WithField("run_id", request.RunID).Error("label reconciliation failed")
		return result, ReconcileOutcomeError
	}
	result.Added = reconciliation.Added
	result.Updated = reconciliation.Updated
	result.Removed = reconciliation.Removed
	return r.finishReconciliation(result, outcome)
}

func (r *Reconciler) loadRunMetadata(ctx context.Context, runID int64) (models.ProwJobRun, bool, error) {
	partitionKeys, err := query.LookupProwJobRunPartitionKeys(r.dbc.DB.WithContext(ctx), runID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			reconciliationTotal.WithLabelValues("missing_run").Inc()
			return models.ProwJobRun{}, false, nil
		}
		return models.ProwJobRun{}, false, fmt.Errorf("lookup partition keys for job run %d: %w", runID, err)
	}

	var runMetadata models.ProwJobRun
	err = r.dbc.DB.WithContext(ctx).
		Where("id = ? AND prow_job_release = ? AND timestamp = ?", runID, partitionKeys.ProwJobRelease, partitionKeys.Timestamp).
		First(&runMetadata).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		reconciliationTotal.WithLabelValues("missing_run").Inc()
		return models.ProwJobRun{}, false, nil
	}
	if err != nil {
		return models.ProwJobRun{}, false, fmt.Errorf("load job run %d metadata: %w", runID, err)
	}
	return runMetadata, true, nil
}

func (r *Reconciler) readAuthoritativeLabels(ctx context.Context, requestRunID string, runMetadata models.ProwJobRun) (map[string]DiscoveredLabel, error) {
	jobRunPath, err := DeriveJobRunPathFromURL(runMetadata.URL)
	if err != nil {
		reconciliationTotal.WithLabelValues("failure").Inc()
		return nil, err
	}
	if runMetadata.GCSBucket == "" {
		return nil, fmt.Errorf("job run %s has no GCS bucket", requestRunID)
	}
	desired, err := r.read(ctx, r.gcsClient, runMetadata.GCSBucket, jobRunPath)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{"run_id": requestRunID, "bucket": runMetadata.GCSBucket, "job_run_path": jobRunPath}).Error("failed to read authoritative GCS labels")
		return nil, err
	}
	return desired, nil
}

func (r *Reconciler) reconcileTransaction(ctx context.Context, runID int64, requestRunID string, desired map[string]DiscoveredLabel) (ReconcileResult, ReconcileOutcome, error) {
	result := ReconcileResult{RunID: requestRunID}

	var outcome ReconcileOutcome
	err := r.dbc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, partKeys, found, err := lockRunForReconciliation(tx, runID)
		if err != nil {
			return err
		}
		if !found {
			reconciliationTotal.WithLabelValues("missing_run").Inc()
			outcome = ReconcileOutcomeRunNotFound
			return nil
		}

		current, err := loadCurrentLabels(tx, runID, run.Labels)
		if err != nil {
			return err
		}
		diff := DiffLabels(current, desired)
		result.Added = len(diff.Add)
		result.Updated = len(diff.Update)
		result.Removed = len(diff.Remove)
		if err := reconcileInfraFailure(tx, runID, partKeys, run.Labels, desired); err != nil {
			return err
		}

		if len(diff.Add) == 0 && len(diff.Update) == 0 && len(diff.Remove) == 0 {
			outcome = ReconcileOutcomeNoOp
			return nil
		}
		if err := persistLabelDefinitions(tx, desired); err != nil {
			return err
		}
		if err := updateRunLabels(tx, runID, partKeys, desired); err != nil {
			return err
		}
		outcome = ReconcileOutcomeApplied
		return nil
	})
	return result, outcome, err
}

func lockRunForReconciliation(tx *gorm.DB, runID int64) (models.ProwJobRun, query.ProwJobRunPartitionKeys, bool, error) {
	partKeys, err := query.LookupProwJobRunPartitionKeys(tx, runID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ProwJobRun{}, query.ProwJobRunPartitionKeys{}, false, nil
	}
	if err != nil {
		return models.ProwJobRun{}, query.ProwJobRunPartitionKeys{}, false, fmt.Errorf("lookup partition keys for prow job run %d: %w", runID, err)
	}

	var run models.ProwJobRun
	row := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND prow_job_release = ? AND timestamp = ?", runID, partKeys.ProwJobRelease, partKeys.Timestamp).
		Take(&run)
	if errors.Is(row.Error, gorm.ErrRecordNotFound) {
		return models.ProwJobRun{}, query.ProwJobRunPartitionKeys{}, false, nil
	}
	if row.Error != nil {
		return models.ProwJobRun{}, query.ProwJobRunPartitionKeys{}, false, fmt.Errorf("load prow job run %d with partition keys: %w", runID, row.Error)
	}
	return run, partKeys, true, nil
}

func loadCurrentLabels(tx *gorm.DB, runID int64, labelIDs pq.StringArray) (map[string]DiscoveredLabel, error) {
	current := make(map[string]DiscoveredLabel, len(labelIDs))
	for _, id := range labelIDs {
		current[id] = DiscoveredLabel{ID: id}
	}
	if len(current) == 0 {
		return current, nil
	}

	var definitions []jobrunscan.Label
	if err := tx.Where("id IN ?", mapKeys(current)).Find(&definitions).Error; err != nil {
		return nil, fmt.Errorf("load label definitions for prow job run %d: %w", runID, err)
	}
	for _, definition := range definitions {
		current[definition.ID] = DiscoveredLabel{ID: definition.ID, Title: definition.LabelTitle, Explanation: definition.Explanation}
	}
	return current, nil
}

func reconcileInfraFailure(tx *gorm.DB, runID int64, partKeys query.ProwJobRunPartitionKeys, current pq.StringArray, desired map[string]DiscoveredLabel) error {
	oldInfra := containsLabel(current, infrafailure.LabelInfraFailure)
	newInfra := containsDesired(desired, infrafailure.LabelInfraFailure)
	if !oldInfra && newInfra {
		if err := infrafailure.SubtractInfraFailureFromSummaries(tx, runID, partKeys); err != nil {
			return fmt.Errorf("subtract InfraFailure summaries for run %d: %w", runID, err)
		}
	}
	if oldInfra && !newInfra {
		if err := infrafailure.RestoreInfraFailureToSummaries(tx, runID, partKeys); err != nil {
			return fmt.Errorf("restore InfraFailure summaries for run %d: %w", runID, err)
		}
	}
	return nil
}

func persistLabelDefinitions(tx *gorm.DB, desired map[string]DiscoveredLabel) error {
	for _, label := range desired {
		var definition jobrunscan.Label
		definitionResult := tx.Where("id = ?", label.ID).First(&definition)
		if errors.Is(definitionResult.Error, gorm.ErrRecordNotFound) {
			if err := tx.Create(&jobrunscan.Label{LabelContent: jobrunscan.LabelContent{
				ID: label.ID, LabelTitle: label.Title, Explanation: label.Explanation,
			}}).Error; err != nil {
				return fmt.Errorf("create label definition %q: %w", label.ID, err)
			}
			continue
		}
		if definitionResult.Error != nil {
			return fmt.Errorf("load label definition %q: %w", label.ID, definitionResult.Error)
		}
		if err := tx.Model(&jobrunscan.Label{}).Where("id = ?", label.ID).Updates(map[string]any{
			"label_title": label.Title, "explanation": label.Explanation,
		}).Error; err != nil {
			return fmt.Errorf("update label definition %q: %w", label.ID, err)
		}
	}
	return nil
}

func updateRunLabels(tx *gorm.DB, runID int64, partKeys query.ProwJobRunPartitionKeys, desired map[string]DiscoveredLabel) error {
	ids := make([]string, 0, len(desired))
	for id := range desired {
		ids = append(ids, id)
	}
	sortStrings(ids)
	if err := tx.Model(&models.ProwJobRun{}).
		Where("id = ? AND prow_job_release = ? AND timestamp = ?", runID, partKeys.ProwJobRelease, partKeys.Timestamp).
		Update("labels", pq.StringArray(ids)).Error; err != nil {
		return fmt.Errorf("update labels for prow job run %d: %w", runID, err)
	}
	return nil
}

func (r *Reconciler) finishReconciliation(result ReconcileResult, outcome ReconcileOutcome) (ReconcileResult, ReconcileOutcome) {
	if outcome == ReconcileOutcomeApplied {
		reconciliationChanges.WithLabelValues("add").Add(float64(result.Added))
		reconciliationChanges.WithLabelValues("update").Add(float64(result.Updated))
		reconciliationChanges.WithLabelValues("remove").Add(float64(result.Removed))
	}
	if outcome == ReconcileOutcomeRunNotFound {
		result.Message = missingRunMessage
		return result, outcome
	}
	if outcome == ReconcileOutcomeNoOp {
		reconciliationTotal.WithLabelValues("noop").Inc()
		result.Message = "labels already reconciled"
	} else {
		reconciliationTotal.WithLabelValues("applied").Inc()
		result.Message = "labels reconciled"
	}
	log.WithFields(log.Fields{"run_id": result.RunID, "added": result.Added, "updated": result.Updated, "removed": result.Removed, "outcome": outcome}).Info("reconciled job labels")
	return result, outcome
}

// ReadLabels lists and parses the current GCS label files exactly once.
func ReadLabels(ctx context.Context, client *storage.Client, bucketName, jobRunPath string) (map[string]DiscoveredLabel, error) {
	if client == nil {
		return nil, fmt.Errorf("GCS client is required")
	}
	labels := make(map[string]DiscoveredLabel)
	jobRunPath = strings.TrimSuffix(jobRunPath, "/") + "/"
	it := client.Bucket(bucketName).Objects(ctx, &storage.Query{Prefix: jobRunPath + bucketLabelsPrefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			return labels, nil
		}
		if err != nil {
			reconciliationTotal.WithLabelValues("failure").Inc()
			return nil, fmt.Errorf("list GCS labels: %w", err)
		}
		if !strings.HasSuffix(attrs.Name, ".json") {
			continue
		}
		reader, err := client.Bucket(bucketName).Object(attrs.Name).NewReader(ctx)
		if err != nil {
			reconciliationTotal.WithLabelValues("failure").Inc()
			return nil, fmt.Errorf("read GCS label %q: %w", attrs.Name, err)
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			reconciliationTotal.WithLabelValues("failure").Inc()
			return nil, fmt.Errorf("read GCS label %q: %w", attrs.Name, readErr)
		}
		if closeErr != nil {
			reconciliationTotal.WithLabelValues("failure").Inc()
			return nil, fmt.Errorf("close GCS label %q: %w", attrs.Name, closeErr)
		}
		label, err := ParseLabelFile(data)
		if err != nil {
			reconciliationTotal.WithLabelValues("malformed").Inc()
			return nil, fmt.Errorf("parse GCS label %q: %w", attrs.Name, err)
		}
		if previous, ok := labels[label.ID]; ok && !sameLabel(previous, label) {
			reconciliationTotal.WithLabelValues("malformed").Inc()
			return nil, fmt.Errorf("conflicting metadata for label %q", label.ID)
		}
		labels[label.ID] = label
	}
}

func containsLabel(labels pq.StringArray, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

func containsDesired(labels map[string]DiscoveredLabel, want string) bool {
	_, ok := labels[want]
	return ok
}

func mapKeys(values map[string]DiscoveredLabel) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return keys
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
