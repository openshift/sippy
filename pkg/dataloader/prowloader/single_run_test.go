package prowloader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"github.com/openshift/sippy/pkg/apis/junit"
	"github.com/openshift/sippy/pkg/apis/prow"
	sippyprocessingv1 "github.com/openshift/sippy/pkg/apis/sippyprocessing/v1"
	"github.com/openshift/sippy/pkg/dataloader/prowloader/pgwriter"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
	"github.com/openshift/sippy/pkg/synthetictests"
	"github.com/openshift/sippy/pkg/testidentification"
)

var singleRunNow = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

type recordingSyntheticTestManager struct {
	called bool
}

func (m *recordingSyntheticTestManager) CreateSyntheticTests(result *sippyprocessingv1.RawJobRunResult) *junit.TestSuite {
	m.called = true
	result.OverallResult = sippyprocessingv1.JobSucceeded
	return &junit.TestSuite{Name: testidentification.SippySuiteName}
}

func validSingleRunProw(start time.Time, completion *time.Time) prow.ProwJob {
	return prow.ProwJob{
		Spec: prow.ProwJobSpec{
			Job: "periodic-e2e", Cluster: "build01",
			DecorationConfig: prow.DecorationConfig{GCSConfiguration: prow.GCSConfiguration{Bucket: "test-platform-results"}},
			Refs:             &prow.Refs{Org: "openshift", Repo: "origin", Pulls: []prow.Pull{{Number: 42, SHA: "abc", Title: "change"}}},
		},
		Status: prow.ProwJobStatus{
			BuildID: "123", StartTime: start, CompletionTime: completion,
			State: prow.SuccessState,
			URL:   "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/periodic-e2e/123",
		},
		Annotations: map[string]string{"releaseJobName": "periodic-e2e"},
	}
}

func testSingleRunImporter(t *testing.T, pj prow.ProwJob) (*SingleRunImporter, *pgwriter.JobRunResult, *[]string) {
	t.Helper()
	prowJSON, err := json.Marshal(pj)
	require.NoError(t, err)
	var written pgwriter.JobRunResult
	var calls []string
	i := &SingleRunImporter{
		configuredBucket: "test-platform-results",
		syntheticManager: synthetictests.NewEmptySyntheticTestManager(),
		now:              func() time.Time { return singleRunNow },
		exists: func(context.Context, uint) (bool, error) {
			calls = append(calls, "exists")
			return false, nil
		},
		readArtifact: func(_ context.Context, _, object string) ([]byte, error) {
			calls = append(calls, "read:"+object)
			if strings.HasSuffix(object, prowJobFilename) {
				return prowJSON, nil
			}
			return []byte(`{"timestamp":1787835600}`), nil
		},
		loadJUnit: func(context.Context, string, string) (*junit.TestSuites, []string, error) {
			calls = append(calls, "junit")
			return &junit.TestSuites{Suites: []*junit.TestSuite{{
				Name: "openshift-tests", TestCases: []*junit.TestCase{{Name: "test one", Duration: 1}},
			}}}, []string{"logs/periodic-e2e/123/artifacts/junit.xml"}, nil
		},
		loadLabels: func(context.Context, string, civil.Date) ([]string, error) {
			calls = append(calls, "labels")
			return []string{"InfraFailure", "KnownIssue"}, nil
		},
		write: func(_ context.Context, _ *db.DB, current civil.Date, result pgwriter.JobRunResult) error {
			calls = append(calls, "write")
			assert.Equal(t, civil.DateOf(singleRunNow), current)
			written = result
			return nil
		},
	}
	i.lookupDefinition = func(_ context.Context, name string) (*models.ProwJob, error) {
		calls = append(calls, "definition")
		return &models.ProwJob{Model: gorm.Model{ID: 7}, Name: name, Release: "4.20", Variants: []string{"Platform:aws"}}, nil
	}
	return i, &written, &calls
}

func validRequest() SingleRunImportRequest {
	return SingleRunImportRequest{ProwJobRunID: "123", Bucket: "test-platform-results", JobPrefix: "logs/periodic-e2e/123"}
}

func TestSingleRunImportRequestHasOnlyPublicContractFields(t *testing.T) {
	typ := reflect.TypeOf(SingleRunImportRequest{})
	require.Equal(t, 3, typ.NumField())
	assert.Equal(t, "prow_job_run_id", typ.Field(0).Tag.Get("json"))
	assert.Equal(t, "bucket", typ.Field(1).Tag.Get("json"))
	assert.Equal(t, "job_prefix", typ.Field(2).Tag.Get("json"))
}

func TestSingleRunUsesExplicitSyntheticTestManager(t *testing.T) {
	completion := singleRunNow.Add(-time.Hour)
	i, _, _ := testSingleRunImporter(t, validSingleRunProw(singleRunNow.Add(-2*time.Hour), &completion))
	manager := &recordingSyntheticTestManager{}
	i.syntheticManager = manager

	result, err := i.Import(context.Background(), validRequest())
	require.NoError(t, err)
	assert.True(t, manager.called)
	assert.Equal(t, SingleRunStatusImported, result.Status)
}

func TestSingleRunProwJobArtifactReadClassification(t *testing.T) {
	completion := singleRunNow.Add(-time.Hour)
	pj := validSingleRunProw(singleRunNow.Add(-2*time.Hour), &completion)
	for _, tc := range []struct {
		name    string
		readErr error
		kind    SingleRunImportErrorKind
	}{
		{"missing prowjob.json", fmt.Errorf("wrapped storage error: %w", storage.ErrObjectNotExist), SingleRunNotFound},
		{"other GCS read failure", errors.New("GCS read failed"), SingleRunArtifactFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i, _, calls := testSingleRunImporter(t, pj)
			i.readArtifact = func(context.Context, string, string) ([]byte, error) {
				return nil, tc.readErr
			}

			_, err := i.Import(context.Background(), validRequest())
			assertImportKind(t, err, tc.kind)
			require.ErrorIs(t, err, tc.readErr)
			if tc.kind == SingleRunNotFound {
				require.ErrorIs(t, err, storage.ErrObjectNotExist)
			}
			assert.Equal(t, []string{"exists"}, *calls)
		})
	}
}

func TestCanonicalJobPrefixLayouts(t *testing.T) {
	tests := []struct {
		name, prefix string
		valid        bool
	}{
		{"periodic", "logs/job/123", true},
		{"legacy PR", "pr-logs/pull/42/job/123", true},
		{"current PR", "pr-logs/pull/openshift_origin/42/job/123", true},
		{"trailing slash normalized", "logs/job/123/", true},
		{"step prefix ending in ID", "logs/job/123/artifacts/123", false},
		{"artifact prefix", "logs/job/123/artifacts", false},
		{"traversal", "logs/job/../123", false},
		{"wrong ID", "logs/job/124", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := canonicalJobPrefix(tc.prefix, "123")
			assert.Equal(t, tc.valid, err == nil, "%v", err)
		})
	}
}

func TestParseProwJobURLLocation(t *testing.T) {
	for _, prefix := range []string{
		"logs/job/123",
		"pr-logs/pull/42/job/123",
		"pr-logs/pull/openshift_origin/42/job/123",
	} {
		bucket, got, err := parseProwJobURLLocation("https://prow.ci.openshift.org/view/gs/test-platform-results/"+prefix+"/", "123")
		require.NoError(t, err)
		assert.Equal(t, "test-platform-results", bucket)
		assert.Equal(t, prefix, got)
	}
	_, _, err := parseProwJobURLLocation("https://prow/view/gs/test-platform-results/logs/job/123/artifacts/123", "123")
	assert.Error(t, err)
}

func TestSingleRunAgeBoundsStopDownstreamWork(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		valid bool
	}{
		{"exactly 14 days accepted", singleRunNow.Add(-SingleRunMaximumAge), true},
		{"older than 14 days rejected", singleRunNow.Add(-SingleRunMaximumAge - time.Nanosecond), false},
		{"future rejected", singleRunNow.Add(time.Nanosecond), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			completion := tc.start.Add(time.Hour)
			i, _, calls := testSingleRunImporter(t, validSingleRunProw(tc.start, &completion))
			_, err := i.Import(context.Background(), validRequest())
			if tc.valid {
				require.NoError(t, err)
				assert.Contains(t, *calls, "write")
			} else {
				assertImportKind(t, err, SingleRunInvalidProwJob)
				assert.NotContains(t, *calls, "junit")
				assert.NotContains(t, *calls, "labels")
				assert.NotContains(t, *calls, "write")
			}
		})
	}
}

func TestSingleRunCompletionPrecedenceAndMarkerFallback(t *testing.T) {
	start := singleRunNow.Add(-time.Hour)
	t.Run("Prow completion is authoritative", func(t *testing.T) {
		completion := start.Add(20 * time.Minute)
		i, written, calls := testSingleRunImporter(t, validSingleRunProw(start, &completion))
		_, err := i.Import(context.Background(), validRequest())
		require.NoError(t, err)
		assert.Equal(t, 20*time.Minute, written.Run.Duration)
		assert.NotContains(t, strings.Join(*calls, "|"), finishedFilename)
	})
	t.Run("top-level marker supplies missing completion", func(t *testing.T) {
		i, written, calls := testSingleRunImporter(t, validSingleRunProw(start, nil))
		markerTime := start.Add(30 * time.Minute).Unix()
		originalRead := i.readArtifact
		i.readArtifact = func(ctx context.Context, bucket, object string) ([]byte, error) {
			if strings.HasSuffix(object, finishedFilename) {
				*calls = append(*calls, "marker:"+object)
				return []byte(fmt.Sprintf(`{"timestamp":%d}`, markerTime)), nil
			}
			return originalRead(ctx, bucket, object)
		}
		_, err := i.Import(context.Background(), validRequest())
		require.NoError(t, err)
		assert.Equal(t, 30*time.Minute, written.Run.Duration)
		assert.Contains(t, *calls, "marker:logs/periodic-e2e/123/finished.json")
	})
}

func TestSingleRunInvalidMarkerAndReadFailure(t *testing.T) {
	start := singleRunNow.Add(-time.Hour)
	for _, tc := range []struct {
		name    string
		content string
		readErr error
		kind    SingleRunImportErrorKind
	}{
		{"missing timestamp", `{}`, nil, SingleRunInvalidProwJob},
		{"zero timestamp", `{"timestamp":0}`, nil, SingleRunInvalidProwJob},
		{"malformed timestamp", `{"timestamp":"later"}`, nil, SingleRunInvalidProwJob},
		{"before start", fmt.Sprintf(`{"timestamp":%d}`, start.Add(-time.Second).Unix()), nil, SingleRunInvalidProwJob},
		{"missing marker", "", storage.ErrObjectNotExist, SingleRunArtifactFailure},
		{"marker read failure", "", errors.New("GCS read failed"), SingleRunArtifactFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i, _, calls := testSingleRunImporter(t, validSingleRunProw(start, nil))
			originalRead := i.readArtifact
			i.readArtifact = func(ctx context.Context, bucket, object string) ([]byte, error) {
				if strings.HasSuffix(object, finishedFilename) {
					return []byte(tc.content), tc.readErr
				}
				return originalRead(ctx, bucket, object)
			}
			_, err := i.Import(context.Background(), validRequest())
			assertImportKind(t, err, tc.kind)
			assert.NotContains(t, *calls, "write")
		})
	}
}

func TestSingleRunJUnitToleranceAndReadFailure(t *testing.T) {
	start := singleRunNow.Add(-2 * time.Hour)
	completion := start.Add(time.Hour)
	pj := validSingleRunProw(start, &completion)
	t.Run("multiple suites preserve conversion lifecycle and failure output", func(t *testing.T) {
		i, written, _ := testSingleRunImporter(t, pj)
		i.loadJUnit = func(context.Context, string, string) (*junit.TestSuites, []string, error) {
			return &junit.TestSuites{Suites: []*junit.TestSuite{
				{Name: "openshift-tests", TestCases: []*junit.TestCase{{Name: "passing", Lifecycle: "informing", Duration: 1}}},
				{Name: "openshift-tests", TestCases: []*junit.TestCase{{Name: "failing", FailureOutput: &junit.FailureOutput{Output: "failure detail"}, Duration: 2}}},
			}}, []string{"one-junit.xml", "two-junit.xml"}, nil
		}
		result, err := i.Import(context.Background(), validRequest())
		require.NoError(t, err)
		assert.Equal(t, 2, result.JUnitFiles)
		assert.Equal(t, 2, result.Tests)
		require.Len(t, written.Tests, 2)
		byName := map[string]pgwriter.TestRow{}
		for _, test := range written.Tests {
			byName[test.TestName] = test
		}
		assert.Equal(t, "informing", byName["passing"].Lifecycle)
		require.NotNil(t, byName["failing"].Output)
		assert.Equal(t, "failure detail", *byName["failing"].Output)
		assert.Equal(t, 1, written.Run.TestFailures)
	})
	t.Run("empty or malformed contribution may import zero tests", func(t *testing.T) {
		i, written, _ := testSingleRunImporter(t, pj)
		i.loadJUnit = func(context.Context, string, string) (*junit.TestSuites, []string, error) {
			return &junit.TestSuites{}, []string{"empty-junit.xml", "malformed-junit.xml"}, nil
		}
		result, err := i.Import(context.Background(), validRequest())
		require.NoError(t, err)
		assert.Equal(t, 2, result.JUnitFiles)
		assert.Zero(t, result.Tests)
		assert.Empty(t, written.Tests)
	})
	t.Run("JUnit read failure aborts before labels and write", func(t *testing.T) {
		i, _, calls := testSingleRunImporter(t, pj)
		i.loadJUnit = func(context.Context, string, string) (*junit.TestSuites, []string, error) {
			return nil, nil, storage.ErrObjectNotExist
		}
		_, err := i.Import(context.Background(), validRequest())
		assertImportKind(t, err, SingleRunArtifactFailure)
		assert.NotContains(t, *calls, "labels")
		assert.NotContains(t, *calls, "write")
	})
}

func TestSingleRunEarlyDuplicateAndLookupError(t *testing.T) {
	completion := singleRunNow.Add(-time.Hour)
	pj := validSingleRunProw(singleRunNow.Add(-2*time.Hour), &completion)
	t.Run("committed duplicate skips artifacts and labels", func(t *testing.T) {
		i, _, calls := testSingleRunImporter(t, pj)
		i.exists = func(context.Context, uint) (bool, error) { return true, nil }
		result, err := i.Import(context.Background(), validRequest())
		require.NoError(t, err)
		assert.Equal(t, SingleRunStatusAlreadyImported, result.Status)
		assert.Empty(t, *calls)
	})
	t.Run("lookup error never false succeeds", func(t *testing.T) {
		i, _, calls := testSingleRunImporter(t, pj)
		i.exists = func(context.Context, uint) (bool, error) { return false, errors.New("database unavailable") }
		_, err := i.Import(context.Background(), validRequest())
		assertImportKind(t, err, SingleRunPersistenceFailure)
		assert.Empty(t, *calls)
	})
}

func TestSingleRunNoBigQueryJobsRowLabelsPRsAndRaceLoser(t *testing.T) {
	start := singleRunNow.Add(-2 * time.Hour)
	completion := start.Add(time.Hour)
	i, written, calls := testSingleRunImporter(t, validSingleRunProw(start, &completion))
	i.write = func(_ context.Context, _ *db.DB, _ civil.Date, result pgwriter.JobRunResult) error {
		*calls = append(*calls, "write")
		*written = result
		return pgwriter.ErrProwJobRunAlreadyExists
	}
	result, err := i.Import(context.Background(), validRequest())
	require.NoError(t, err)
	assert.Equal(t, SingleRunStatusAlreadyImported, result.Status)
	assert.Equal(t, "periodic-e2e", result.ProwJobName)
	assert.Equal(t, "4.20", result.Release)
	assert.Equal(t, uint(7), written.Run.ProwJobID, "the existing classified ProwJob definition is reused")
	assert.Equal(t, []string{"InfraFailure", "KnownIssue"}, written.Run.Labels)
	require.Len(t, written.PullRequests, 1)
	assert.Equal(t, "https://github.com/openshift/origin/pull/42", written.PullRequests[0].Link)
	assert.Nil(t, written.PullRequests[0].MergedAt)
	assert.Equal(t, []string{"exists", "read:logs/periodic-e2e/123/prowjob.json", "definition", "junit", "labels", "write"}, *calls,
		"the API import path must not prepare database partitions")
}

func TestSingleRunWriteFailureIsPersistenceFailure(t *testing.T) {
	start := singleRunNow.Add(-2 * time.Hour)
	completion := start.Add(time.Hour)
	i, _, calls := testSingleRunImporter(t, validSingleRunProw(start, &completion))
	i.write = func(context.Context, *db.DB, civil.Date, pgwriter.JobRunResult) error {
		*calls = append(*calls, "write")
		return errors.New("database write failed")
	}

	result, err := i.Import(context.Background(), validRequest())
	assert.Nil(t, result)
	assertImportKind(t, err, SingleRunPersistenceFailure)
	assert.Contains(t, err.Error(), "database write failed")
}

func TestSingleRunDefinitionAndLabelFailuresPreventWrite(t *testing.T) {
	pj := validSingleRunProw(singleRunNow.Add(-2*time.Hour), nil)
	t.Run("missing definition is ignored without writes", func(t *testing.T) {
		i, _, calls := testSingleRunImporter(t, pj)
		i.lookupDefinition = func(context.Context, string) (*models.ProwJob, error) {
			*calls = append(*calls, "definition")
			return nil, nil
		}
		result, err := i.Import(context.Background(), validRequest())
		require.NoError(t, err)
		assert.Equal(t, SingleRunStatusIgnored, result.Status)
		assert.Equal(t, SingleRunReasonNotTracked, result.Reason)
		assert.Equal(t, "periodic-e2e", result.ProwJobName)
		assert.Equal(t, []string{"exists", "read:logs/periodic-e2e/123/prowjob.json", "definition"}, *calls)
	})
	t.Run("definition lookup error remains a persistence failure", func(t *testing.T) {
		i, _, calls := testSingleRunImporter(t, pj)
		i.lookupDefinition = func(context.Context, string) (*models.ProwJob, error) {
			*calls = append(*calls, "definition")
			return nil, errors.New("database unavailable")
		}
		result, err := i.Import(context.Background(), validRequest())
		assert.Nil(t, result)
		assertImportKind(t, err, SingleRunPersistenceFailure)
		assert.Equal(t, []string{"exists", "read:logs/periodic-e2e/123/prowjob.json", "definition"}, *calls)
	})
	t.Run("tracked definition without release remains not found", func(t *testing.T) {
		i, _, calls := testSingleRunImporter(t, pj)
		i.lookupDefinition = func(context.Context, string) (*models.ProwJob, error) {
			*calls = append(*calls, "definition")
			return &models.ProwJob{Name: "periodic-e2e"}, nil
		}
		result, err := i.Import(context.Background(), validRequest())
		assert.Nil(t, result)
		assertImportKind(t, err, SingleRunNotFound)
		assert.Equal(t, []string{"exists", "read:logs/periodic-e2e/123/prowjob.json", "definition"}, *calls)
	})
	t.Run("label source failure is strict", func(t *testing.T) {
		i, _, calls := testSingleRunImporter(t, pj)
		i.loadLabels = func(context.Context, string, civil.Date) ([]string, error) { return nil, errors.New("query failed") }
		_, err := i.Import(context.Background(), validRequest())
		assertImportKind(t, err, SingleRunArtifactFailure)
		assert.NotContains(t, *calls, "write")
	})
	t.Run("absent label row is an explicit empty label set", func(t *testing.T) {
		i, written, _ := testSingleRunImporter(t, pj)
		i.loadLabels = func(context.Context, string, civil.Date) ([]string, error) { return []string{}, nil }
		_, err := i.Import(context.Background(), validRequest())
		require.NoError(t, err)
		assert.Empty(t, written.Run.Labels)
	})
}

func TestValidateSingleRunProwJobMetadata(t *testing.T) {
	start := singleRunNow.Add(-time.Hour)
	completion := start.Add(30 * time.Minute)
	base := validSingleRunProw(start, &completion)
	for _, tc := range []struct {
		name   string
		mutate func(*prow.ProwJob)
	}{
		{"nonterminal", func(p *prow.ProwJob) { p.Status.State = prow.PendingState }},
		{"build ID mismatch", func(p *prow.ProwJob) { p.Status.BuildID = "124" }},
		{"artifact bucket mismatch", func(p *prow.ProwJob) { p.Spec.DecorationConfig.GCSConfiguration.Bucket = "other-bucket" }},
		{"URL prefix mismatch", func(p *prow.ProwJob) { p.Status.URL = "https://prow/view/gs/test-platform-results/logs/other/123" }},
		{"step URL", func(p *prow.ProwJob) {
			p.Status.URL = "https://prow/view/gs/test-platform-results/logs/periodic-e2e/123/artifacts/123"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pj := base
			tc.mutate(&pj)
			err := validateSingleRunProwJob(&pj, validRequest(), validRequest().JobPrefix, singleRunNow)
			assertImportKind(t, err, SingleRunInvalidProwJob)
		})
	}
	t.Run("missing status URL remains valid", func(t *testing.T) {
		pj := base
		pj.Status.URL = ""
		assert.NoError(t, validateSingleRunProwJob(&pj, validRequest(), validRequest().JobPrefix, singleRunNow))
	})
}

func TestSingleRunLabelsQueryUsesExactDatePartition(t *testing.T) {
	date := civil.Date{Year: 2026, Month: 8, Day: 27}
	sql, params := singleRunLabelsQuery("ci_analysis", "123", date)
	assert.Contains(t, sql, "prowjob_build_id = @BuildID")
	assert.Contains(t, sql, "DATE(prowjob_start) = @StartDate")
	assert.NotContains(t, sql, ">= @StartDate")
	require.Len(t, params, 2)
	assert.Equal(t, "123", params[0].Value)
	assert.Equal(t, date, params[1].Value)
}

func TestValidateSingleRunRequestBucketRestriction(t *testing.T) {
	request := validRequest()
	_, _, err := validateSingleRunRequest(request, "other-bucket")
	assertImportKind(t, err, SingleRunInvalidRequest)
	request.Bucket = "INVALID"
	_, _, err = validateSingleRunRequest(request, "INVALID")
	assertImportKind(t, err, SingleRunInvalidRequest)
}

func TestSingleRunDependencyErrorClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want SingleRunImportErrorKind
	}{
		{name: "configured sentinel", err: fmt.Errorf("storage client: %w", ErrDependencyNotConfigured), want: SingleRunDependencyFailure},
		{name: "connection refused errno", err: fmt.Errorf("dialing PostgreSQL: %w", syscall.ECONNREFUSED), want: SingleRunDependencyFailure},
		{name: "context deadline", err: context.DeadlineExceeded, want: SingleRunDependencyFailure},
		{name: "network timeout", err: &net.DNSError{IsTimeout: true}, want: SingleRunDependencyFailure},
		{name: "Google API unavailable", err: &googleapi.Error{Code: 429, Message: "throttled"}, want: SingleRunDependencyFailure},
		{name: "gRPC unavailable", err: status.Error(codes.Unavailable, "unavailable"), want: SingleRunDependencyFailure},
		{name: "PostgreSQL connection status", err: &pgconn.PgError{Code: "08006"}, want: SingleRunDependencyFailure},
		{name: "matching configuration text is not typed", err: errors.New("storage client is not configured"), want: SingleRunArtifactFailure},
		{name: "matching connection text is not typed", err: errors.New("dial tcp: connection refused"), want: SingleRunArtifactFailure},
		{name: "ordinary artifact failure", err: errors.New("object missing"), want: SingleRunArtifactFailure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyImportError(SingleRunArtifactFailure, "reading artifact", tc.err)
			assertImportKind(t, err, tc.want)
		})
	}
}

func TestSingleRunUnconfiguredDependenciesUseSentinel(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{name: "Prow job run existence database", call: func() error {
			_, err := ProwJobRunExists(context.Background(), nil, 123)
			return err
		}},
		{name: "Prow job run existence database handle", call: func() error {
			_, err := ProwJobRunExists(context.Background(), &db.DB{}, 123)
			return err
		}},
		{name: "GCS artifact client", call: func() error {
			_, err := (&SingleRunImporter{}).readGCSArtifact(context.Background(), "bucket", "object")
			return err
		}},
		{name: "GCS JUnit client", call: func() error {
			_, _, err := (&SingleRunImporter{}).loadGCSJUnit(context.Background(), "bucket", "prefix")
			return err
		}},
		{name: "ProwJob definition database", call: func() error {
			_, err := (&SingleRunImporter{}).findDefinition(context.Background(), "job")
			return err
		}},
		{name: "ProwJob definition database handle", call: func() error {
			_, err := (&SingleRunImporter{dbc: &db.DB{}}).findDefinition(context.Background(), "job")
			return err
		}},
		{name: "BigQuery labels client", call: func() error {
			_, err := GatherLabelsForRunFromBQ(context.Background(), nil, "123", civil.Date{})
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			require.ErrorIs(t, err, ErrDependencyNotConfigured)
		})
	}
}

func assertImportKind(t *testing.T, err error, want SingleRunImportErrorKind) {
	t.Helper()
	require.Error(t, err)
	var importErr *SingleRunImportError
	require.ErrorAs(t, err, &importErr)
	assert.Equal(t, want, importErr.Kind)
}
