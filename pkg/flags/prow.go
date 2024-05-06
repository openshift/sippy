package flags

import (
	"net/url"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

type ProwFlags struct {
	URL string
}

func NewProwFlags() *ProwFlags {
	return &ProwFlags{
		URL: "https://prow.ci.openshift.org",
	}
}

func (f *ProwFlags) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&f.URL, "prow-url", f.URL, "URL for Prow")
}

func (f *ProwFlags) Validate() error {
	_, err := url.ParseRequestURI(f.URL)
	if err != nil {
		return errors.WithMessage(err, "Prow URL must be valid")
	}
	return nil
}
