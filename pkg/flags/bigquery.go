package flags

import (
	"context"
	"fmt"

	"github.com/openshift/sippy/pkg/bigquery/bqlabel"
	"github.com/spf13/pflag"

	"github.com/openshift/sippy/pkg/apis/cache"
	bqcachedclient "github.com/openshift/sippy/pkg/bigquery"
)

// BigQueryFlags contain auth information for Google cloud-related services.
type BigQueryFlags struct {
	BigQueryProject string
	BigQueryDataset string
	ReleasesTable   string
}

func NewBigQueryFlags() *BigQueryFlags {
	return &BigQueryFlags{
		ReleasesTable: "openshift-ci-data-analysis.ci_data.Releases",
	}
}

func (f *BigQueryFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.BigQueryProject, "bigquery-project", "openshift-gce-devel", "BigQuery project to use")
	fs.StringVar(&f.BigQueryDataset, "bigquery-dataset", "ci_analysis_us", "Dataset to use")
	fs.StringVar(&f.ReleasesTable, "bigquery-releases-table", f.ReleasesTable, "BigQuery table containing release information")
}

func (f *BigQueryFlags) GetBigQueryClient(ctx context.Context, opCtx bqlabel.OperationalContext, cacheClient cache.Cache, googleServiceAccountCredentialFile string) (*bqcachedclient.Client, error) {
	if googleServiceAccountCredentialFile == "" {
		return nil, fmt.Errorf("service account required")
	}

	return bqcachedclient.New(
		ctx, opCtx, cacheClient,
		googleServiceAccountCredentialFile,
		f.BigQueryProject, f.BigQueryDataset, f.ReleasesTable,
	)
}
