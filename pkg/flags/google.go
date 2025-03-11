package flags

import (
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

// GoogleCloudFlags contain configuration information for Google cloud-related services.
type GoogleCloudFlags struct {
	ServiceAccountCredentialFile string
	OAuthClientCredentialFile    string
}

func NewGoogleCloudFlags() *GoogleCloudFlags {
	return &GoogleCloudFlags{
		ServiceAccountCredentialFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
	}
}

func (f *GoogleCloudFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.ServiceAccountCredentialFile,
		"google-service-account-credential-file",
		f.ServiceAccountCredentialFile,
		"location of a credential file described by https://cloud.google.com/docs/authentication/production")

	fs.StringVar(&f.OAuthClientCredentialFile,
		"google-oauth-credential-file",
		f.OAuthClientCredentialFile,
		"location of a credential file described by https://developers.google.com/people/quickstart/go, setup from https://cloud.google.com/bigquery/docs/authentication/end-user-installed#client-credentials")
}

func (f *GoogleCloudFlags) Validate() error {
	if f.ServiceAccountCredentialFile == "" && f.OAuthClientCredentialFile == "" {
		return errors.New("must specify one of 'google-service-account-credential-file' or 'google-oauth-credential-file'")
	}

	return nil
}
