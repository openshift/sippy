package postgres

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/reqopts"
)

func TestTestFailuresExpression(t *testing.T) {
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name                 string
		lifecycles           []string
		sampleReleases       []string
		wantArgs             int
		wantSubq             bool
		wantReleasePredicate string
	}{
		{
			name: "no lifecycles uses pre-computed column",
		},
		{
			name:       "empty lifecycles uses pre-computed column",
			lifecycles: []string{},
		},
		{
			name:                 "with lifecycles keeps explicit release pruning",
			lifecycles:           []string{"blocking"},
			wantArgs:             5,
			wantSubq:             true,
			wantReleasePredicate: "lf.prow_job_run_release = ?",
		},
		{
			name:                 "filtered sample bounds releases and correlates to outer run",
			lifecycles:           []string{"blocking", "informing"},
			sampleReleases:       []string{"4.16", "4.17"},
			wantArgs:             5,
			wantSubq:             true,
			wantReleasePredicate: "lf.prow_job_run_release = ANY(?) AND lf.prow_job_run_release = pjr.prow_job_release",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, args := testFailuresExpression("4.16", start, end, tt.lifecycles, tt.sampleReleases)
			if len(args) != tt.wantArgs {
				t.Fatalf("len(args) = %d, want %d", len(args), tt.wantArgs)
			}
			hasSubquery := strings.Contains(expr, "prow_job_run_tests lf")
			if hasSubquery != tt.wantSubq {
				t.Fatalf("has subquery = %t, want %t", hasSubquery, tt.wantSubq)
			}
			if tt.wantSubq {
				if !strings.Contains(expr, "lf.lifecycle = ANY(?)") {
					t.Fatal("subquery should filter by lifecycle")
				}
				if !strings.Contains(expr, "lf.status = ?") {
					t.Fatal("subquery should use placeholder for failure status")
				}
				if !strings.Contains(expr, tt.wantReleasePredicate) {
					t.Fatalf("subquery release predicate should be %q", tt.wantReleasePredicate)
				}
				if tt.sampleReleases == nil && args[0] != "4.16" {
					t.Fatalf("first subquery argument = %v, want release 4.16", args[0])
				}
				if tt.sampleReleases != nil {
					releaseArray, ok := args[0].(*pq.StringArray)
					if !ok || !reflect.DeepEqual([]string(*releaseArray), tt.sampleReleases) {
						t.Fatalf("first subquery argument = %v, want releases %v", args[0], tt.sampleReleases)
					}
				}
				if !strings.Contains(expr, "lf.prow_job_run_timestamp >= ?") {
					t.Fatal("subquery should include explicit time range filter")
				}
			} else if !strings.Contains(expr, "COALESCE(pjr.test_failures, 0)") {
				t.Fatal("should use pre-computed column when no lifecycles")
			}
		})
	}
}

func TestTestDetailsJobNameSQL(t *testing.T) {
	start := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name            string
		useReleaseName  bool
		wantExpression  string
		wantJoinPresent bool
		wantArgs        []any
	}{
		{
			name:           "without PR filter uses Prow job name",
			wantExpression: "pj.name",
		},
		{
			name:            "PR filter uses releaseJobName with Prow fallback",
			useReleaseName:  true,
			wantExpression:  "COALESCE(NULLIF(split_part(release_job_name.value, ',', 1), ''), pj.name)",
			wantJoinPresent: true,
			wantArgs:        []any{start, end},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expression, join, args := testDetailsJobNameSQL(tt.useReleaseName, start, end)
			if expression != tt.wantExpression {
				t.Errorf("expression = %q, want %q", expression, tt.wantExpression)
			}
			if got := join != ""; got != tt.wantJoinPresent {
				t.Errorf("join present = %t, want %t", got, tt.wantJoinPresent)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("arguments = %#v, want %#v", args, tt.wantArgs)
			}
			if tt.wantJoinPresent {
				for _, fragment := range []string{
					"prow_job_run_annotations release_job_name",
					"release_job_name.key = 'releaseJobName'",
					"release_job_name.prow_job_run_timestamp = pjr.timestamp",
					"release_job_name.prow_job_run_timestamp >= ?",
					"release_job_name.prow_job_run_timestamp < ?",
				} {
					if !strings.Contains(join, fragment) {
						t.Errorf("join missing %q: %s", fragment, join)
					}
				}
			}
		})
	}
}

func TestBuildTestDetailsRunScope(t *testing.T) {
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	tests := []struct {
		name             string
		runFilter        string
		runArgs          []any
		wantReleaseScope bool
		wantArgs         []any
	}{
		{
			name:             "ordinary sample uses release and time",
			wantReleaseScope: true,
			wantArgs:         []any{start, end, start, end, "4.20", "4.20", "4.20"},
		},
		{
			name:      "selected sample uses run membership and time",
			runFilter: "EXISTS (SELECT 1 FROM selected_runs WHERE run_id = pjr.id AND tag = ?)",
			runArgs:   []any{"payload-1"},
			wantArgs:  []any{start, end, start, end, "payload-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, args := buildTestDetailsRunScope("4.20", start, end, tt.runFilter, tt.runArgs)
			for _, fragment := range []string{
				"pjr.timestamp >= ? AND pjr.timestamp < ?",
				"pjrt.prow_job_run_timestamp >= ? AND pjrt.prow_job_run_timestamp < ?",
			} {
				if !strings.Contains(clause, fragment) {
					t.Errorf("run scope missing %q: %s", fragment, clause)
				}
			}
			if got := strings.Contains(clause, "pj.release = ?"); got != tt.wantReleaseScope {
				t.Errorf("release scope present = %t, want %t: %s", got, tt.wantReleaseScope, clause)
			}
			if tt.runFilter != "" && !strings.Contains(clause, tt.runFilter) {
				t.Errorf("run scope missing sample selector: %s", clause)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("arguments = %#v, want %#v", args, tt.wantArgs)
			}
		})
	}
}

func TestAppendTestDetailsLifecycleFilter(t *testing.T) {
	tests := []struct {
		name           string
		lifecycles     []string
		wantFilter     bool
		wantAdditional int
	}{
		{
			name:           "configured lifecycle is filtered",
			lifecycles:     []string{"blocking"},
			wantFilter:     true,
			wantAdditional: 1,
		},
		{
			name:       "empty lifecycle is unfiltered",
			lifecycles: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalArgs := []any{"existing"}
			query, args := appendTestDetailsLifecycleFilter("SELECT 1 WHERE true", originalArgs, tt.lifecycles)

			if got := strings.Contains(query, "pjrt.lifecycle = ANY(?)"); got != tt.wantFilter {
				t.Fatalf("lifecycle filter present = %t, want %t", got, tt.wantFilter)
			}
			if got, want := len(args), len(originalArgs)+tt.wantAdditional; got != want {
				t.Fatalf("argument count = %d, want %d", got, want)
			}
			if tt.wantAdditional == 1 {
				got, ok := args[len(args)-1].(*pq.StringArray)
				if !ok {
					t.Fatalf("lifecycle argument has type %T, want *pq.StringArray", args[len(args)-1])
				}
				if strings.Join(*got, ",") != strings.Join(tt.lifecycles, ",") {
					t.Fatalf("lifecycle argument = %v, want %v", got, tt.lifecycles)
				}
			}
		})
	}
}

func TestParseVariants(t *testing.T) {
	tests := []struct {
		name     string
		variants pq.StringArray
		want     map[string]string
	}{
		{
			name:     "empty array",
			variants: pq.StringArray{},
			want:     map[string]string{},
		},
		{
			name:     "single variant",
			variants: pq.StringArray{"Platform:aws"},
			want:     map[string]string{"Platform": "aws"},
		},
		{
			name:     "multiple variants",
			variants: pq.StringArray{"Platform:aws", "Network:ovn", "Architecture:amd64"},
			want:     map[string]string{"Platform": "aws", "Network": "ovn", "Architecture": "amd64"},
		},
		{
			name:     "value containing colon uses first colon only",
			variants: pq.StringArray{"Suite:openshift-tests:sig-auth"},
			want:     map[string]string{"Suite": "openshift-tests:sig-auth"},
		},
		{
			name:     "entry without colon is skipped",
			variants: pq.StringArray{"malformed", "Platform:aws"},
			want:     map[string]string{"Platform": "aws"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVariants(tc.variants)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			for k, wantV := range tc.want {
				if gotV, ok := got[k]; !ok {
					t.Errorf("missing key %q", k)
				} else if gotV != wantV {
					t.Errorf("got[%q] = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestFilterByDBGroupBy(t *testing.T) {
	tests := []struct {
		name      string
		variants  map[string]string
		dbGroupBy sets.Set[string]
		want      map[string]string
	}{
		{
			name:      "empty variants",
			variants:  map[string]string{},
			dbGroupBy: sets.New[string]("Platform"),
			want:      map[string]string{},
		},
		{
			name:      "keeps matching keys",
			variants:  map[string]string{"Platform": "aws", "Network": "ovn", "Architecture": "amd64"},
			dbGroupBy: sets.New[string]("Platform", "Network"),
			want:      map[string]string{"Platform": "aws", "Network": "ovn"},
		},
		{
			name:      "no matching keys",
			variants:  map[string]string{"Platform": "aws"},
			dbGroupBy: sets.New[string]("Network"),
			want:      map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterByDBGroupBy(tc.variants, tc.dbGroupBy)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			for k, wantV := range tc.want {
				if gotV := got[k]; gotV != wantV {
					t.Errorf("got[%q] = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestMatchesIncludeVariants(t *testing.T) {
	tests := []struct {
		name            string
		variants        map[string]string
		includeVariants map[string][]string
		want            bool
	}{
		{
			name:            "empty filter matches everything",
			variants:        map[string]string{"Platform": "aws"},
			includeVariants: map[string][]string{},
			want:            true,
		},
		{
			name:            "matching single key",
			variants:        map[string]string{"Platform": "aws", "Network": "ovn"},
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			want:            true,
		},
		{
			name:            "value not in allowed list",
			variants:        map[string]string{"Platform": "azure"},
			includeVariants: map[string][]string{"Platform": {"aws", "gcp"}},
			want:            false,
		},
		{
			name:            "required key missing from variants",
			variants:        map[string]string{"Platform": "aws"},
			includeVariants: map[string][]string{"Network": {"ovn"}},
			want:            false,
		},
		{
			name:            "all keys must match",
			variants:        map[string]string{"Platform": "aws", "Network": "sdn"},
			includeVariants: map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			want:            false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesIncludeVariants(tc.variants, tc.includeVariants)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSpotCheckMatchesTestFilters(t *testing.T) {
	scalingID := crtest.SpotCheckTestID("spotcheck-30d", "Etcd", "Scaling")
	recoveryID := crtest.SpotCheckTestID("spotcheck-30d", "Etcd", "Recovery")

	tests := []struct {
		name       string
		reqOptions reqopts.RequestOptions
		testID     string
		capability string
		want       bool
	}{
		{
			name:       "no filters keeps everything",
			testID:     scalingID,
			capability: "Scaling",
			want:       true,
		},
		{
			name:       "capability filter keeps a match",
			reqOptions: reqopts.RequestOptions{TestFilters: reqopts.TestFilters{Capabilities: []string{"Scaling"}}},
			testID:     scalingID,
			capability: "Scaling",
			want:       true,
		},
		{
			name:       "capability filter matches case-insensitively",
			reqOptions: reqopts.RequestOptions{TestFilters: reqopts.TestFilters{Capabilities: []string{"cpu partitioning"}}},
			testID:     crtest.SpotCheckTestID("spotcheck-30d", "Node / Kubelet", "CPU Partitioning"),
			capability: "CPU Partitioning",
			want:       true,
		},
		{
			name:       "capability filter drops other capabilities",
			reqOptions: reqopts.RequestOptions{TestFilters: reqopts.TestFilters{Capabilities: []string{"Scaling"}}},
			testID:     crtest.SpotCheckTestID("spotcheck-30d", "Node / Kubelet", "CPU Partitioning"),
			capability: "CPU Partitioning",
			want:       false,
		},
		{
			name:       "an empty test identification filters nothing",
			reqOptions: reqopts.RequestOptions{TestIDOptions: []reqopts.TestIdentification{{}}},
			testID:     scalingID,
			capability: "Scaling",
			want:       true,
		},
		{
			name:       "exact test ID keeps its own result",
			reqOptions: reqopts.RequestOptions{TestIDOptions: []reqopts.TestIdentification{{TestID: scalingID}}},
			testID:     scalingID,
			capability: "Scaling",
			want:       true,
		},
		{
			name:       "exact test ID drops sibling spot-check tests",
			reqOptions: reqopts.RequestOptions{TestIDOptions: []reqopts.TestIdentification{{TestID: scalingID}}},
			testID:     recoveryID,
			capability: "Recovery",
			want:       false,
		},
		{
			name:       "exact test ID of a regular test drops spot-check results",
			reqOptions: reqopts.RequestOptions{TestIDOptions: []reqopts.TestIdentification{{TestID: "openshift-tests:abc123"}}},
			testID:     scalingID,
			capability: "Scaling",
			want:       false,
		},
		{
			name:       "requested capability drops other capabilities",
			reqOptions: reqopts.RequestOptions{TestIDOptions: []reqopts.TestIdentification{{Component: "Etcd", Capability: "Scaling"}}},
			testID:     recoveryID,
			capability: "Recovery",
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spotCheckMatchesTestFilters(tt.reqOptions, tt.testID, tt.capability); got != tt.want {
				t.Errorf("spotCheckMatchesTestFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpotCheckSampleVariants(t *testing.T) {
	tier := map[string][]string{"JobTier": {"spotcheck-30d"}}

	t.Run("without cross-compare the request's variants are kept", func(t *testing.T) {
		reqOptions := reqopts.RequestOptions{VariantOption: reqopts.Variants{
			IncludeVariants: map[string][]string{"Platform": {"aws"}},
		}}
		got, ok := spotCheckSampleVariants(reqOptions, tier)
		if !ok {
			t.Fatal("expected variants to match")
		}
		want := map[string][]string{"Platform": {"aws"}, "JobTier": {"spotcheck-30d"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("cross-compare selects the sample-side values", func(t *testing.T) {
		reqOptions := reqopts.RequestOptions{VariantOption: reqopts.Variants{
			IncludeVariants:     map[string][]string{"Platform": {"aws"}, "Network": {"ovn"}},
			CompareVariants:     map[string][]string{"Platform": {"gcp"}},
			VariantCrossCompare: []string{"Platform"},
		}}
		got, ok := spotCheckSampleVariants(reqOptions, tier)
		if !ok {
			t.Fatal("expected variants to match")
		}
		want := map[string][]string{"Platform": {"gcp"}, "Network": {"ovn"}, "JobTier": {"spotcheck-30d"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("the request's variants are not modified", func(t *testing.T) {
		reqOptions := reqopts.RequestOptions{VariantOption: reqopts.Variants{
			IncludeVariants: map[string][]string{"Platform": {"aws"}},
		}}
		spotCheckSampleVariants(reqOptions, tier)
		if _, ok := reqOptions.VariantOption.IncludeVariants["JobTier"]; ok {
			t.Error("sample variants leaked into the request's IncludeVariants")
		}
	})
}

func TestSpotCheckSampleVariantsNarrowing(t *testing.T) {
	tests := []struct {
		name          string
		request       map[string][]string
		sample        map[string][]string
		want          map[string][]string
		wantSomeMatch bool
	}{
		{
			name:          "sample tier replaces the request's tiers",
			request:       map[string][]string{"JobTier": {"standard", "blocking"}},
			sample:        map[string][]string{"JobTier": {"spotcheck-30d"}},
			want:          map[string][]string{"JobTier": {"spotcheck-30d"}},
			wantSomeMatch: true,
		},
		{
			name:          "sample variant the request does not set is applied",
			request:       map[string][]string{"Network": {"ovn"}},
			sample:        map[string][]string{"Platform": {"aws"}},
			want:          map[string][]string{"Network": {"ovn"}, "Platform": {"aws"}},
			wantSomeMatch: true,
		},
		{
			name:          "overlapping values are intersected",
			request:       map[string][]string{"Platform": {"aws", "gcp"}},
			sample:        map[string][]string{"Platform": {"aws", "azure"}},
			want:          map[string][]string{"Platform": {"aws"}},
			wantSomeMatch: true,
		},
		{
			name:    "disjoint values match nothing",
			request: map[string][]string{"Platform": {"gcp"}},
			sample:  map[string][]string{"Platform": {"aws"}},
		},
		{
			name:          "an empty request value does not restrict",
			request:       map[string][]string{"Platform": {}},
			sample:        map[string][]string{"Platform": {"aws"}},
			want:          map[string][]string{"Platform": {"aws"}},
			wantSomeMatch: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqOptions := reqopts.RequestOptions{VariantOption: reqopts.Variants{IncludeVariants: tt.request}}
			got, ok := spotCheckSampleVariants(reqOptions, tt.sample)
			if ok != tt.wantSomeMatch {
				t.Fatalf("ok = %v, want %v", ok, tt.wantSomeMatch)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
