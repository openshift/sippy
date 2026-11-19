package flags

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIFlagsTrustedDelegators(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr string
	}{
		{name: "absent allowlist enables no delegation", args: nil, want: nil},
		{
			name: "repeated flag keeps each exact principal",
			args: []string{"--trusted-delegator", "system:serviceaccount:ns:mcp", "--trusted-delegator", "other-bot"},
			want: []string{"system:serviceaccount:ns:mcp", "other-bot"},
		},
		{
			name: "commas are not split so principals stay exact",
			args: []string{"--trusted-delegator", "a,b"},
			want: []string{"a,b"},
		},
		{name: "blank principal is rejected", args: []string{"--trusted-delegator", "  "}, wantErr: "must not be blank"},
		{name: "empty principal is rejected", args: []string{"--trusted-delegator="}, wantErr: "must not be blank"},
		{
			name:    "surrounding whitespace is rejected",
			args:    []string{"--trusted-delegator", " chai-bot"},
			wantErr: "surrounding whitespace",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NewAPIFlags()
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			f.BindFlags(fs)
			require.NoError(t, fs.Parse(tc.args))

			err := f.Validate()
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, f.TrustedDelegators)
		})
	}
}
