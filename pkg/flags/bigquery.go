package flags

import (
	"context"
	"fmt"

	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
)

// BigQueryFlags contain auth information for Google cloud-related services.
type BigQueryFlags struct {
	BigQueryProject string
	BigQueryDataset string
}

func NewBigQueryFlags() *BigQueryFlags {
	return &BigQueryFlags{}
}

func (f *BigQueryFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.BigQueryProject, "bigquery-project", "openshift-gce-devel", "BigQuery project to use")
	fs.StringVar(&f.BigQueryDataset, "bigquery-dataset", "ci_analysis_us", "Dataset to use")
}

func (f *BigQueryFlags) GetBigQueryClient(ctx context.Context, cacheClient cache.Cache, googleServiceAccountCredentialFile string) (*bqcachedclient.Client, error) {
	if googleServiceAccountCredentialFile == "" {
		return nil, fmt.Errorf("service account required")
	}

	return bqcachedclient.New(ctx, googleServiceAccountCredentialFile, f.BigQueryProject, f.BigQueryDataset, cacheClient)
}
