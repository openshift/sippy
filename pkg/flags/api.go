package flags

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// APIFlags holds configuration information for Sippy API servers.
type APIFlags struct {
	EnableWriteEndpoints bool
	ListenAddr           string
	MetricsAddr          string
	// TrustedDelegators lists exact authenticated principals (X-Forwarded-User values) that may act
	// for another user through the X-Acting-For header. Empty enables no delegation.
	TrustedDelegators []string
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
	fs.StringArrayVar(&f.TrustedDelegators, "trusted-delegator", f.TrustedDelegators,
		"Exact authenticated principal (X-Forwarded-User value) allowed to act for another user via the X-Acting-For header (one principal per arg instance; omit to disable delegation)")
}

// Validate rejects allowlist entries that could never match exactly or would be ambiguous, so a
// misconfigured allowlist fails at startup instead of silently enabling or disabling delegation.
func (f *APIFlags) Validate() error {
	for _, principal := range f.TrustedDelegators {
		if strings.TrimSpace(principal) == "" {
			return fmt.Errorf("--trusted-delegator must not be blank")
		}
		if strings.TrimSpace(principal) != principal {
			return fmt.Errorf("--trusted-delegator %q must not have surrounding whitespace", principal)
		}
	}
	return nil
}
