// Package verify provides read-only storage adapters and pure comparisons for
// checking the integrity of Sippy's daily data pipeline.
package verify

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"cloud.google.com/go/civil"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Check identifies a daily data integrity check.
type Check string

const (
	CheckBQCompleteness      Check = "bq-completeness"
	CheckDailyTotals         Check = "daily-totals"
	CheckCumulativeSummaries Check = "cumulative-summaries"
)

// AllChecks lists supported checks in their canonical execution order.
var AllChecks = []Check{CheckBQCompleteness, CheckDailyTotals, CheckCumulativeSummaries}

// Options selects the date, checks, and optional release for a run.
type Options struct {
	Date    civil.Date
	Checks  []Check
	Release string
}

// Scope is the normalized date and release set passed to each verifier.
type Scope struct {
	Date     civil.Date
	Releases []string
}

// Verifier runs one integrity check over the complete selected date and
// release scope.
type Verifier interface {
	Verify(context.Context, Scope) Result
}

func ParseChecks(values []string) ([]Check, error) {
	if len(values) == 0 {
		return append([]Check(nil), AllChecks...), nil
	}
	valid := sets.New[Check](AllChecks...)
	selected := sets.New[Check]()
	for _, value := range values {
		check := Check(value)
		if !valid.Has(check) {
			allowed := make([]string, len(AllChecks))
			for i := range AllChecks {
				allowed[i] = string(AllChecks[i])
			}
			return nil, fmt.Errorf("invalid --check %q: must be one of %s", value, strings.Join(allowed, ", "))
		}
		selected.Insert(check)
	}
	checks := make([]Check, 0, len(selected))
	for _, check := range AllChecks {
		if selected.Has(check) {
			checks = append(checks, check)
		}
	}
	return checks, nil
}

func ContainsCheck(checks []Check, wanted Check) bool {
	return sets.New[Check](checks...).Has(wanted)
}

// BuildID is the normalized unsigned Prow build identifier.
type BuildID uint64

// Summary is the bounded outcome record for one check, release, and date.
type Summary struct {
	Check         Check
	Release       string
	Date          civil.Date
	Passed        bool
	ExpectedRows  int
	ActualRows    int
	Discrepancies int
	Error         string
}

func (s Summary) Fields() log.Fields {
	return log.Fields{
		"check":         s.Check,
		"release":       s.Release,
		"date":          s.Date.String(),
		"passed":        s.Passed,
		"expectedRows":  s.ExpectedRows,
		"actualRows":    s.ActualRows,
		"discrepancies": s.Discrepancies,
		"error":         s.Error,
	}
}

// Discrepancy describes one deterministic difference found by a check.
type Discrepancy struct {
	Check    Check
	Release  string
	Date     civil.Date
	Kind     string
	Key      string
	Field    string
	Expected string
	Actual   string
	Detail   string
}

func (d Discrepancy) Fields() log.Fields {
	return log.Fields{
		"check":    d.Check,
		"release":  d.Release,
		"date":     d.Date.String(),
		"kind":     d.Kind,
		"key":      d.Key,
		"field":    d.Field,
		"expected": d.Expected,
		"actual":   d.Actual,
		"detail":   d.Detail,
	}
}

// Result contains all summary and discrepancy records from a run.
type Result struct {
	Summaries     []Summary
	Discrepancies []Discrepancy
}

func (r Result) Passed() bool {
	if len(r.Summaries) == 0 {
		return false
	}
	for _, summary := range r.Summaries {
		if !summary.Passed {
			return false
		}
	}
	return true
}

func (r *Result) Sort() {
	sort.SliceStable(r.Summaries, func(i, j int) bool {
		a, b := r.Summaries[i], r.Summaries[j]
		if a.Check != b.Check {
			return a.Check < b.Check
		}
		if a.Release != b.Release {
			return a.Release < b.Release
		}
		return a.Date.Before(b.Date)
	})
	sort.SliceStable(r.Discrepancies, func(i, j int) bool {
		a, b := r.Discrepancies[i], r.Discrepancies[j]
		av := []string{string(a.Check), a.Release, a.Date.String(), a.Kind, a.Key, a.Field, a.Expected, a.Actual, a.Detail}
		bv := []string{string(b.Check), b.Release, b.Date.String(), b.Kind, b.Key, b.Field, b.Expected, b.Actual, b.Detail}
		for k := range av {
			if av[k] != bv[k] {
				return av[k] < bv[k]
			}
		}
		return false
	})
}

func (r Result) Log(logger log.FieldLogger) {
	for _, summary := range r.Summaries {
		entry := logger.WithFields(summary.Fields())
		if summary.Passed {
			entry.Info("verification summary")
		} else {
			entry.Error("verification summary")
		}
	}
	for _, discrepancy := range r.Discrepancies {
		logger.WithFields(discrepancy.Fields()).Error("verification discrepancy")
	}
}

// SummaryKey uniquely identifies a daily or cumulative test summary row.
type SummaryKey struct {
	Release   string
	Date      civil.Date
	TestID    uint64
	ProwJobID uint64
	SuiteID   uint64
	Lifecycle string
}

func (k SummaryKey) String() string {
	return fmt.Sprintf("release=%q,date=%s,test_id=%d,prow_job_id=%d,suite_id=%d,lifecycle=%q", k.Release, k.Date, k.TestID, k.ProwJobID, k.SuiteID, k.Lifecycle)
}

// Counts contains the status counters compared by summary checks.
type Counts struct {
	Successes int64
	Failures  int64
	Flakes    int64
	Runs      int64
}

func (c Counts) Add(other Counts) Counts {
	return Counts{
		Successes: c.Successes + other.Successes,
		Failures:  c.Failures + other.Failures,
		Flakes:    c.Flakes + other.Flakes,
		Runs:      c.Runs + other.Runs,
	}
}

// DailyRow associates a summary key with its counters.
type DailyRow struct {
	Key    SummaryKey
	Counts Counts
}

// CumulativeRows contains the three inputs needed for cumulative validation.
type CumulativeRows struct {
	// Previous contains cumulative rows from the day before the target date.
	Previous []DailyRow
	// Daily contains daily total rows from the target date.
	Daily []DailyRow
	// Target contains stored cumulative rows from the target date.
	Target []DailyRow
}

// BQJob contains the BigQuery Prow job fields needed for release attribution and ID comparison.
type BQJob struct {
	// BuildID is the raw BigQuery build ID and may be blank or malformed.
	BuildID string
	// JobName is the Prow job name used for release attribution.
	JobName string
	// Annotations contains parsed Prow job annotations used for attribution.
	Annotations map[string]string
	// HasRefs reports whether the source job has pull request refs.
	HasRefs bool
}
