package flags

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

type goldenFile struct {
	Metadata struct {
		AsOf      time.Time `json:"as_of"`
		Release   string    `json:"release"`
		Generated time.Time `json:"generated_at"`
	} `json:"metadata"`
	Results map[string]validationSnapshot `json:"results"`
}

func allQueryCases() []queryCase {
	cases := getQueryCases()
	for _, qc := range getIndividualQueryCases() {
		cases = append(cases, qc)
	}
	return cases
}

func Test_GenerateGoldenFile(t *testing.T) {
	dbc, _ := getBenchmarkDBClient(t)
	goldenPath := os.Getenv("golden_file_path")
	if goldenPath == "" {
		t.Fatal("golden_file_path env var is required")
	}

	asOf := time.Now().UTC()
	cases := allQueryCases()

	gf := goldenFile{
		Results: make(map[string]validationSnapshot),
	}
	gf.Metadata.AsOf = asOf
	gf.Metadata.Release = benchmarkRelease
	gf.Metadata.Generated = time.Now().UTC()

	for _, vc := range cases {
		t.Run(vc.name, func(t *testing.T) {
			snap, err := vc.fn(dbc, asOf)
			if err != nil {
				t.Fatalf("%s failed: %v", vc.name, err)
			}
			gf.Results[vc.name] = snap
			t.Logf("%s: row_count=%d", vc.name, snap.RowCount)
		})
	}

	data, err := json.MarshalIndent(gf, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal golden file: %v", err)
	}
	if err := os.WriteFile(goldenPath, data, 0o600); err != nil {
		t.Fatalf("failed to write golden file: %v", err)
	}
	t.Logf("golden file written to %s with %d cases", goldenPath, len(gf.Results))
}

func Test_ValidateGoldenFile(t *testing.T) {
	dbc, _ := getBenchmarkDBClient(t)
	goldenPath := os.Getenv("golden_file_path")
	if goldenPath == "" {
		t.Fatal("golden_file_path env var is required")
	}

	data, err := os.ReadFile(goldenPath) // #nosec G304
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	var gf goldenFile
	if err := json.Unmarshal(data, &gf); err != nil {
		t.Fatalf("failed to unmarshal golden file: %v", err)
	}

	asOf := gf.Metadata.AsOf
	t.Logf("validating against golden file (asOf=%s, generated=%s, release=%s)",
		asOf.Format(time.RFC3339), gf.Metadata.Generated.Format(time.RFC3339), gf.Metadata.Release)

	cases := allQueryCases()
	var mismatches []string

	for _, vc := range cases {
		t.Run(vc.name, func(t *testing.T) {
			expected, ok := gf.Results[vc.name]
			if !ok {
				t.Logf("SKIP: case %s not found in golden file", vc.name)
				return
			}

			got, err := vc.fn(dbc, asOf)
			if err != nil {
				t.Errorf("%s failed: %v", vc.name, err)
				mismatches = append(mismatches, vc.name)
				return
			}

			if got.RowCount != expected.RowCount {
				t.Errorf("row_count: expected %d, got %d", expected.RowCount, got.RowCount)
				mismatches = append(mismatches, vc.name)
			}

			for key, expectedVal := range expected.SpotChecks {
				gotVal, ok := got.SpotChecks[key]
				if !ok {
					t.Errorf("spot_check %q: expected %q, got (missing)", key, expectedVal)
					mismatches = append(mismatches, fmt.Sprintf("%s.%s", vc.name, key))
					continue
				}
				if gotVal != expectedVal {
					t.Errorf("spot_check %q: expected %q, got %q", key, expectedVal, gotVal)
					mismatches = append(mismatches, fmt.Sprintf("%s.%s", vc.name, key))
				}
			}

			t.Logf("row_count=%d (expected %d)", got.RowCount, expected.RowCount)
		})
	}

	if len(mismatches) > 0 {
		t.Logf("total mismatches: %d — %s", len(mismatches), strings.Join(mismatches, ", "))
	}
}
