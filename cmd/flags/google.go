package flags

import (
	"os"

	"github.com/spf13/pflag"
)

// GoogleCloudCredentialFlags contain auth information for Google cloud-related services.
type GoogleCloudCredentialFlags struct {
	ServiceAccountCredentialFile string
	OAuthClientCredentialFile    string
}

func NewGoogleCloudCredentialFlags() *GoogleCloudCredentialFlags {
	return &GoogleCloudCredentialFlags{
		OAuthClientCredentialFile: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
	}
}

func (f *GoogleCloudCredentialFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.ServiceAccountCredentialFile,
		"google-service-account-credential-file",
		f.ServiceAccountCredentialFile,
		"location of a credential file described by https://cloud.google.com/docs/authentication/production")

	fs.StringVar(&f.OAuthClientCredentialFile,
		"google-oauth-credential-file",
		f.OAuthClientCredentialFile,
		"location of a credential file described by https://developers.google.com/people/quickstart/go, setup from https://cloud.google.com/bigquery/docs/authentication/end-user-installed#client-credentials")
}
