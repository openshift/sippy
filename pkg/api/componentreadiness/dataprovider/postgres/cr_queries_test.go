package postgres

import (
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestPROrPayloadSampleSpecAggregatesOnlySelectedRuns(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	filter := "EXISTS (SELECT 1 FROM matching_runs WHERE run_id = pjr.id AND name = ?)"
	spec := prOrPayloadSampleSpec(reqopts.Release{Start: start, End: end}, filter, []any{"selected"}, []string{"blocking"})
	query, args := buildInnerAggregation(spec, "JOIN prow_jobs pj ON pj.id = e.prow_job_id JOIN vg ON vg.vcid = pj.variant_combination_id", nil, drilldownFilters{})

	for _, fragment := range []string{
		"FROM prow_job_run_tests e",
		"pjr.id = e.prow_job_run_id",
		"pjr.prow_job_release = e.prow_job_run_release",
		"pjr.timestamp = e.prow_job_run_timestamp",
		"COALESCE(e.suite_id, 0) AS suite_id",
		"COUNT(*) AS total_count",
		"COUNT(*) FILTER (WHERE e.status = 1) AS success_count",
		"COUNT(*) FILTER (WHERE e.status = 13) AS flake_count",
		"MAX(e.prow_job_run_timestamp) FILTER (WHERE e.status = 12) AS last_failure",
		"pjr.labels @> ARRAY['InfraFailure']",
		"e.lifecycle = ANY(?)",
		filter,
	} {
		if !strings.Contains(query, fragment) {
			t.Errorf("filtered query missing %q: %s", fragment, query)
		}
	}
	if strings.Contains(query, "e.release = ?") || strings.Contains(query, "pj.release = ?") {
		t.Errorf("filtered sample must not constrain the ordinary release: %s", query)
	}
	wantArgs := []any{start, end, start, end, "selected"}
	if len(args) != len(wantArgs)+1 || !reflect.DeepEqual(args[:len(wantArgs)], wantArgs) {
		t.Errorf("arguments = %#v, want date bounds and selector %#v followed by lifecycles", args, wantArgs)
	}
}
