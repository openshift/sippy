package jobrunannotator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

const BucketLabelsPrefix = "artifacts/job_labels/"

// JobRunBucketLabelContainer is a schema-specifying container for writing a label file to the prow bucket.
// for incompatible schema changes just add another version with a different type and tag
type JobRunBucketLabelContainer struct {
	V1 *JobRunBucketLabel `json:"symptom_label_v1,omitempty"`
}

// JobRunBucketLabel represents a single label applied to a job run per a symptom.
type JobRunBucketLabel struct {
	Symptom jobrunscan.SymptomContent `json:"symptom"`
	Label   jobrunscan.LabelContent   `json:"label"`
	// path of the matched file relative to the job directory in the bucket
	FileMatch string `json:"file_match"`
	// the first file text that matched the symptom, if relevant for the matcher (otherwise empty)
	TextMatch string `json:"text_match,omitempty"`
	// metadata for where to write this
	Bucket     string `json:"-"`
	JobRunPath string `json:"-"`
}

func (x JobRunBucketLabel) WriteJSONToBucket(ctx context.Context, client *storage.Client) error {
	baseName := path.Base(x.FileMatch)
	hasher := sha256.New()
	hasher.Write([]byte(x.FileMatch))
	labelFile := x.Label.ID + "-" + baseName + "-" + hex.EncodeToString(hasher.Sum(nil)) + ".json"

	bucketWriter := client.Bucket(x.Bucket).Object(x.JobRunPath + BucketLabelsPrefix + labelFile).NewWriter(ctx)
	encoder := json.NewEncoder(bucketWriter)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(JobRunBucketLabelContainer{V1: &x}); err != nil {
		_ = bucketWriter.Close()
		return err
	}

	return bucketWriter.Close()
}
