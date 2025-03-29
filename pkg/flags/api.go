package flags

import (
	"github.com/spf13/pflag"
)

// APIFlags holds configuration information for Sippy API servers.
type APIFlags struct {
	EnableWriteEndpoints bool
	ListenAddr           string
	MetricsAddr          string
	RedisURL             string
}

func NewAPIFlags() *APIFlags {
	return &APIFlags{
		ListenAddr:  ":8080",
		MetricsAddr: ":2112",
	}
}

func (f *APIFlags) BindFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&f.EnableWriteEndpoints, "enable-write-endpoints", false, "Enable write-endpoints for triage etc")
	fs.StringVar(&f.ListenAddr, "listen", f.ListenAddr, "The address to serve analysis reports on (default :8080)")
	fs.StringVar(&f.MetricsAddr, "listen-metrics", f.MetricsAddr, "The address to serve prometheus metrics on (default :2112)")
}
