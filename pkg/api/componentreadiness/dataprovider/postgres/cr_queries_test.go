package postgres

import (
	"testing"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
)

func TestShouldUseAllTestBranches(t *testing.T) {
	tests := []struct {
		name       string
		reqOptions reqopts.RequestOptions
		want       bool
	}{
		{name: "unscoped request", reqOptions: reqopts.RequestOptions{}},
		{
			name: "normalized empty test identification",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{}},
			},
		},
		{
			name: "include all tests",
			reqOptions: reqopts.RequestOptions{
				IncludeAllTests: true,
			},
			want: true,
		},
		{
			name: "component drill-down",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{Component: "Etcd"}},
			},
			want: true,
		},
		{
			name: "capability drill-down",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{Capability: "Quorum"}},
			},
			want: true,
		},
		{
			name: "component and capability drill-down",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{Component: "Etcd", Capability: "Quorum"}},
			},
			want: true,
		},
		{
			name: "exact test request",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{TestID: "test-id"}},
			},
		},
		{
			name: "scoped exact test request",
			reqOptions: reqopts.RequestOptions{
				TestIDOptions: []reqopts.TestIdentification{{Component: "Etcd", Capability: "Quorum", TestID: "test-id"}},
			},
		},
		{
			name: "include all exact test request",
			reqOptions: reqopts.RequestOptions{
				IncludeAllTests: true,
				TestIDOptions:   []reqopts.TestIdentification{{Component: "Etcd", Capability: "Quorum", TestID: "test-id"}},
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldUseAllTestBranches(test.reqOptions); got != test.want {
				t.Fatalf("shouldUseAllTestBranches() = %t, want %t", got, test.want)
			}
		})
	}
}
