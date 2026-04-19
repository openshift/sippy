package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	if os.Getenv("DEBUG_LOGGING") != "" {
		logrus.SetLevel(logrus.DebugLevel)
	}
}

// mockCache is a test cache that properly returns errors on miss,
// records durations passed to Set, and can simulate errors.
type mockCache struct {
	store        map[string][]byte
	setDurations map[string]time.Duration
	setCalls     int
	getCalls     int
	getErr       error
	setErr       error
}

func newMockCache() *mockCache {
	return &mockCache{
		store:        make(map[string][]byte),
		setDurations: make(map[string]time.Duration),
	}
}

func (m *mockCache) Get(_ context.Context, key string, duration time.Duration) ([]byte, error) {
	m.getCalls++
	if m.getErr != nil {
		return nil, m.getErr
	}
	val, ok := m.store[key]
	if !ok {
		return nil, fmt.Errorf("cache miss")
	}
	return val, nil
}

func (m *mockCache) Set(_ context.Context, key string, content []byte, duration time.Duration) error {
	m.setCalls++
	if m.setErr != nil {
		return m.setErr
	}
	m.store[key] = content
	m.setDurations[key] = duration
	return nil
}

type testResult struct {
	Value string `json:"value"`
}

type testCacheKey struct {
	Query string
}

func makeGenerateFn(result testResult, callCount *int) func(context.Context) (testResult, []error) {
	return func(_ context.Context) (testResult, []error) {
		*callCount++
		return result, nil
	}
}

func makeFailingGenerateFn(callCount *int) func(context.Context) (testResult, []error) {
	return func(_ context.Context) (testResult, []error) {
		*callCount++
		return testResult{}, []error{fmt.Errorf("generate failed")}
	}
}

// TestGetDataFromCacheOrGenerate_NilCache verifies that with no cache, generateFn is always called directly.
func TestGetDataFromCacheOrGenerate_NilCache(t *testing.T) {
	var generateCalls int
	expected := testResult{Value: "generated"}
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "prefix~", nil)

	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), nil, cache.RequestOptions{}, spec,
		makeGenerateFn(expected, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, expected, result)
	assert.Equal(t, 1, generateCalls)
}

// TestGetDataFromCacheOrGenerate_CacheMiss verifies that on a cache miss, generateFn is called and the result is stored.
func TestGetDataFromCacheOrGenerate_CacheMiss(t *testing.T) {
	mc := newMockCache()
	var generateCalls int
	expected := testResult{Value: "generated"}
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "prefix~", nil)

	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, cache.RequestOptions{}, spec,
		makeGenerateFn(expected, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, expected, result)
	assert.Equal(t, 1, generateCalls)
	assert.Equal(t, 1, mc.getCalls, "should attempt cache read")
	assert.Equal(t, 1, mc.setCalls, "should store result in cache")
}

// TestGetDataFromCacheOrGenerate_CacheHit verifies that on a cache hit, the cached value is returned and generateFn is skipped.
func TestGetDataFromCacheOrGenerate_CacheHit(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "prefix~", nil)
	cached := testResult{Value: "cached"}

	// Pre-populate the cache
	cacheKey, err := spec.GetCacheKey()
	require.NoError(t, err)
	data, err := json.Marshal(cached)
	require.NoError(t, err)
	mc.store[string(cacheKey)] = data

	var generateCalls int
	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, cache.RequestOptions{}, spec,
		makeGenerateFn(testResult{Value: "fresh"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, cached, result, "should return cached value")
	assert.Equal(t, 0, generateCalls, "should not call generateFn on cache hit")
}

// TestGetDataFromCacheOrGenerate_ForceRefresh verifies that ForceRefresh skips cache read entirely, regenerates, and writes the new value.
func TestGetDataFromCacheOrGenerate_ForceRefresh(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "prefix~", nil)

	// Pre-populate the cache
	cacheKey, err := spec.GetCacheKey()
	require.NoError(t, err)
	data, _ := json.Marshal(testResult{Value: "stale"})
	mc.store[string(cacheKey)] = data

	var generateCalls int
	expected := testResult{Value: "fresh"}
	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, cache.RequestOptions{ForceRefresh: true}, spec,
		makeGenerateFn(expected, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, expected, result, "should return freshly generated value")
	assert.Equal(t, 1, generateCalls, "should call generateFn despite cache hit")
	assert.Equal(t, 0, mc.getCalls, "should skip cache read entirely")
	assert.Equal(t, 1, mc.setCalls, "should write new value to cache")
}

// TestGetDataFromCacheOrGenerate_SkipCacheWrites verifies that SkipCacheWrites generates data but does not store it in the cache.
func TestGetDataFromCacheOrGenerate_SkipCacheWrites(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "prefix~", nil)

	var generateCalls int
	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, cache.RequestOptions{SkipCacheWrites: true}, spec,
		makeGenerateFn(testResult{Value: "generated"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, "generated", result.Value)
	assert.Equal(t, 1, generateCalls)
	assert.Equal(t, 0, mc.setCalls, "should not write to cache")
}

// TestGetDataFromCacheOrGenerate_ForceRefreshWithSkipCacheWrites verifies that both flags together regenerate data with no cache read or write, leaving existing entries untouched.
func TestGetDataFromCacheOrGenerate_ForceRefreshWithSkipCacheWrites(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "prefix~", nil)

	// Pre-populate
	cacheKey, _ := spec.GetCacheKey()
	data, _ := json.Marshal(testResult{Value: "stale"})
	mc.store[string(cacheKey)] = data

	var generateCalls int
	opts := cache.RequestOptions{ForceRefresh: true, SkipCacheWrites: true}
	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, opts, spec,
		makeGenerateFn(testResult{Value: "fresh"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, "fresh", result.Value)
	assert.Equal(t, 1, generateCalls)
	assert.Equal(t, 0, mc.getCalls, "should skip cache read")
	assert.Equal(t, 0, mc.setCalls, "should skip cache write")

	// Original cached value should still be there
	assert.Equal(t, data, mc.store[string(cacheKey)], "original cache entry should be untouched")
}

// TestGetDataFromCacheOrGenerate_GenerateErrorSkipsCacheWrite verifies that errors from generateFn are not cached.
func TestGetDataFromCacheOrGenerate_GenerateErrorSkipsCacheWrite(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "prefix~", nil)

	var generateCalls int
	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, cache.RequestOptions{}, spec,
		makeFailingGenerateFn(&generateCalls), testResult{},
	)

	assert.Len(t, errs, 1)
	assert.Equal(t, testResult{}, result)
	assert.Equal(t, 1, generateCalls)
	assert.Equal(t, 0, mc.setCalls, "should not cache error results")
}

// TestGetDataFromCacheOrGenerate_StableData_UsesLongerExpiry verifies that when queryEndDate is older than StableAge, StableExpiry is used as the cache duration.
func TestGetDataFromCacheOrGenerate_StableData_UsesLongerExpiry(t *testing.T) {
	mc := newMockCache()
	stableAge := 24 * time.Hour
	stableExpiry := 7 * 24 * time.Hour
	// queryEndDate is well in the past (older than stableAge)
	oldDate := time.Now().Add(-3 * 24 * time.Hour)
	spec := NewCacheSpec(testCacheKey{Query: "stable"}, "prefix~", &oldDate)

	opts := cache.RequestOptions{
		StableAge:    stableAge,
		StableExpiry: stableExpiry,
	}

	var generateCalls int
	_, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, opts, spec,
		makeGenerateFn(testResult{Value: "data"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	cacheKey, _ := spec.GetCacheKey()
	assert.Equal(t, stableExpiry, mc.setDurations[string(cacheKey)], "should use StableExpiry for old data")
}

// TestGetDataFromCacheOrGenerate_RecentData_UsesDefaultExpiry verifies that when queryEndDate is recent (within StableAge), the default 8h cache duration is used instead of StableExpiry.
func TestGetDataFromCacheOrGenerate_RecentData_UsesDefaultExpiry(t *testing.T) {
	mc := newMockCache()
	stableAge := 7 * 24 * time.Hour
	stableExpiry := 7 * 24 * time.Hour
	// queryEndDate is recent (not older than stableAge)
	recentDate := time.Now().Add(-1 * time.Hour)
	spec := NewCacheSpec(testCacheKey{Query: "recent"}, "prefix~", &recentDate)

	opts := cache.RequestOptions{
		StableAge:    stableAge,
		StableExpiry: stableExpiry,
	}

	var generateCalls int
	_, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, opts, spec,
		makeGenerateFn(testResult{Value: "data"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	cacheKey, _ := spec.GetCacheKey()
	duration := mc.setDurations[string(cacheKey)]
	assert.NotEqual(t, stableExpiry, duration, "should NOT use StableExpiry for recent data")
	assert.Equal(t, defaultCacheDuration, duration, "should use default cache duration")
}

// TestGetDataFromCacheOrGenerate_RefreshRecent_RefreshesRecentData verifies that RefreshRecent forces regeneration for data with a recent queryEndDate, skipping the cache read.
func TestGetDataFromCacheOrGenerate_RefreshRecent_RefreshesRecentData(t *testing.T) {
	mc := newMockCache()
	stableAge := 7 * 24 * time.Hour
	recentDate := time.Now().Add(-1 * time.Hour)
	spec := NewCacheSpec(testCacheKey{Query: "recent"}, "prefix~", &recentDate)

	// Pre-populate cache
	cacheKey, _ := spec.GetCacheKey()
	data, _ := json.Marshal(testResult{Value: "stale"})
	mc.store[string(cacheKey)] = data

	opts := cache.RequestOptions{
		RefreshRecent: true,
		StableAge:     stableAge,
	}

	var generateCalls int
	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, opts, spec,
		makeGenerateFn(testResult{Value: "fresh"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, "fresh", result.Value, "should regenerate recent data")
	assert.Equal(t, 1, generateCalls)
	assert.Equal(t, 0, mc.getCalls, "should skip cache read for recent data")
}

// TestGetDataFromCacheOrGenerate_RefreshRecent_DoesNotRefreshStableData verifies that RefreshRecent does not refresh stable (old) data — the cached value is returned.
func TestGetDataFromCacheOrGenerate_RefreshRecent_DoesNotRefreshStableData(t *testing.T) {
	mc := newMockCache()
	stableAge := 24 * time.Hour
	// queryEndDate is old enough to be stable
	oldDate := time.Now().Add(-3 * 24 * time.Hour)
	spec := NewCacheSpec(testCacheKey{Query: "old"}, "prefix~", &oldDate)

	// Pre-populate cache
	cacheKey, _ := spec.GetCacheKey()
	cached := testResult{Value: "cached-stable"}
	data, _ := json.Marshal(cached)
	mc.store[string(cacheKey)] = data

	opts := cache.RequestOptions{
		RefreshRecent: true,
		StableAge:     stableAge,
	}

	var generateCalls int
	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, opts, spec,
		makeGenerateFn(testResult{Value: "fresh"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, "cached-stable", result.Value, "should return cached value for stable data")
	assert.Equal(t, 0, generateCalls, "should not regenerate stable data")
}

// TestGetDataFromCacheOrGenerate_RefreshRecent_NoQueryEndDate verifies that RefreshRecent refreshes data when no queryEndDate is set (treated as non-stable).
func TestGetDataFromCacheOrGenerate_RefreshRecent_NoQueryEndDate(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "nodate"}, "prefix~", nil)

	// Pre-populate cache
	cacheKey, _ := spec.GetCacheKey()
	cached := testResult{Value: "cached"}
	data, _ := json.Marshal(cached)
	mc.store[string(cacheKey)] = data

	opts := cache.RequestOptions{
		RefreshRecent: true,
		StableAge:     24 * time.Hour,
	}

	var generateCalls int
	result, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, opts, spec,
		makeGenerateFn(testResult{Value: "fresh"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	// With no queryEndDate, HasStableData returns false, so refreshRecent = true
	assert.Equal(t, "fresh", result.Value, "should refresh when no queryEndDate is set")
	assert.Equal(t, 1, generateCalls)
}

// TestGetDataFromCacheOrGenerate_CRTimeRoundingFactor verifies that CRTimeRoundingFactor bounds the cache duration to between 0 and the rounding factor.
func TestGetDataFromCacheOrGenerate_CRTimeRoundingFactor(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "rounded"}, "prefix~", nil)

	roundingFactor := 1 * time.Hour
	opts := cache.RequestOptions{
		CRTimeRoundingFactor: roundingFactor,
	}

	var generateCalls int
	_, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, opts, spec,
		makeGenerateFn(testResult{Value: "data"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	cacheKey, _ := spec.GetCacheKey()
	duration := mc.setDurations[string(cacheKey)]
	// Should be between 0 and the rounding factor
	assert.Greater(t, duration, time.Duration(0), "rounded duration should be positive")
	assert.LessOrEqual(t, duration, roundingFactor, "rounded duration should not exceed rounding factor")
}

// TestGetDataFromCacheOrGenerate_CRTimeRoundingFactor_OverriddenByStableExpiry verifies that StableExpiry takes precedence over CRTimeRoundingFactor for stable (old) data.
func TestGetDataFromCacheOrGenerate_CRTimeRoundingFactor_OverriddenByStableExpiry(t *testing.T) {
	mc := newMockCache()
	stableAge := 24 * time.Hour
	stableExpiry := 7 * 24 * time.Hour
	oldDate := time.Now().Add(-3 * 24 * time.Hour)
	spec := NewCacheSpec(testCacheKey{Query: "stable-rounded"}, "prefix~", &oldDate)

	opts := cache.RequestOptions{
		CRTimeRoundingFactor: 1 * time.Hour,
		StableAge:            stableAge,
		StableExpiry:         stableExpiry,
	}

	var generateCalls int
	_, errs := GetDataFromCacheOrGenerate(
		context.Background(), mc, opts, spec,
		makeGenerateFn(testResult{Value: "data"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	cacheKey, _ := spec.GetCacheKey()
	assert.Equal(t, stableExpiry, mc.setDurations[string(cacheKey)],
		"StableExpiry should override CRTimeRoundingFactor for stable data")
}

// TestHasStableData verifies the stability determination based on queryEndDate and StableAge combinations.
func TestHasStableData(t *testing.T) {
	tests := []struct {
		name         string
		queryEndDate *time.Time
		stableAge    time.Duration
		expected     bool
	}{
		{
			name:         "nil queryEndDate",
			queryEndDate: nil,
			stableAge:    24 * time.Hour,
			expected:     false,
		},
		{
			name:         "zero StableAge",
			queryEndDate: timePtr(time.Now().Add(-48 * time.Hour)),
			stableAge:    0,
			expected:     false,
		},
		{
			name:         "recent data is not stable",
			queryEndDate: timePtr(time.Now().Add(-1 * time.Hour)),
			stableAge:    24 * time.Hour,
			expected:     false,
		},
		{
			name:         "old data is stable",
			queryEndDate: timePtr(time.Now().Add(-48 * time.Hour)),
			stableAge:    24 * time.Hour,
			expected:     true,
		},
		{
			name:         "boundary - just barely within StableAge",
			queryEndDate: timePtr(time.Now().Add(-24*time.Hour + time.Minute)),
			stableAge:    24 * time.Hour,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewCacheSpec(testCacheKey{Query: "test"}, "", tt.queryEndDate)
			opts := cache.RequestOptions{StableAge: tt.stableAge}
			assert.Equal(t, tt.expected, spec.HasStableData(opts))
		})
	}
}

// TestCalculateRoundedCacheDuration verifies that the cache duration defaults to 8h without a rounding factor, and is bounded by the factor when set.
func TestCalculateRoundedCacheDuration(t *testing.T) {
	t.Run("zero rounding factor uses default", func(t *testing.T) {
		duration := CalculateRoundedCacheDuration(cache.RequestOptions{})
		assert.Equal(t, defaultCacheDuration, duration)
	})

	t.Run("rounding factor produces duration within bounds", func(t *testing.T) {
		factor := 2 * time.Hour
		duration := CalculateRoundedCacheDuration(cache.RequestOptions{CRTimeRoundingFactor: factor})
		assert.Greater(t, duration, time.Duration(0))
		assert.LessOrEqual(t, duration, factor)
	})
}

// TestNewCacheSpec_PanicsOnUnexportedFields verifies that NewCacheSpec panics when given a struct with no exported fields, since it can't be serialized as a cache key.
func TestNewCacheSpec_PanicsOnUnexportedFields(t *testing.T) {
	type badKey struct {
		unexported string
	}
	assert.Panics(t, func() {
		NewCacheSpec(badKey{unexported: "nope"}, "prefix~", nil)
	})
}

// TestNewCacheSpec_Prefix verifies that the prefix distinguishes cache keys for different queries that share the same key struct.
func TestNewCacheSpec_Prefix(t *testing.T) {
	withPrefix := NewCacheSpec(testCacheKey{Query: "q1"}, "pfx~", nil)
	withoutPrefix := NewCacheSpec(testCacheKey{Query: "q1"}, "", nil)

	keyWith, err := withPrefix.GetCacheKey()
	require.NoError(t, err)
	keyWithout, err := withoutPrefix.GetCacheKey()
	require.NoError(t, err)

	assert.NotEqual(t, keyWith, keyWithout, "prefix should make keys different")
	assert.Contains(t, string(keyWith), "pfx~")
	assert.NotContains(t, string(keyWithout), "pfx~")
}

func timePtr(t time.Time) *time.Time {
	return &t
}

const testMatview = "prow_test_report_7d_matview"

// helper to pre-populate the cache with a matview-style cached value (val + timestamp)
func seedMatviewCache(t *testing.T, mc *mockCache, spec CacheSpec, val testResult, cachedAt time.Time) {
	t.Helper()
	cacheKey, err := spec.GetCacheKey()
	require.NoError(t, err)
	entry := struct {
		Val       testResult
		Timestamp time.Time
	}{Val: val, Timestamp: cachedAt}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	mc.store[string(cacheKey)] = data
}

func seedRefreshTimestamp(mc *mockCache, matview string, refreshedAt time.Time) {
	mc.store[RefreshMatviewKey(matview)] = []byte(refreshedAt.UTC().Format(time.RFC3339))
}

// TestGetDataFromCacheOrMatview_NilCache verifies that with no cache, generateFn is always called.
func TestGetDataFromCacheOrMatview_NilCache(t *testing.T) {
	var generateCalls int
	expected := testResult{Value: "generated"}
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "mv~", nil)

	result, errs := GetDataFromCacheOrMatview(
		context.Background(), nil, spec, testMatview, time.Hour,
		makeGenerateFn(expected, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, expected, result)
	assert.Equal(t, 1, generateCalls)
}

// TestGetDataFromCacheOrMatview_CacheMiss verifies that on a miss, generateFn is called and the result is stored.
func TestGetDataFromCacheOrMatview_CacheMiss(t *testing.T) {
	mc := newMockCache()
	var generateCalls int
	expected := testResult{Value: "generated"}
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "mv~", nil)

	result, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec, testMatview, time.Hour,
		makeGenerateFn(expected, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, expected, result)
	assert.Equal(t, 1, generateCalls)
	assert.Equal(t, 1, mc.setCalls, "should store result in cache")
}

// TestGetDataFromCacheOrMatview_CacheHit_NoRefresh verifies that a cached value is returned
// when no matview refresh timestamp exists in the cache.
func TestGetDataFromCacheOrMatview_CacheHit_NoRefresh(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "mv~", nil)
	cached := testResult{Value: "cached"}
	seedMatviewCache(t, mc, spec, cached, time.Now().UTC())

	var generateCalls int
	result, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec, testMatview, time.Hour,
		makeGenerateFn(testResult{Value: "fresh"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, cached, result, "should return cached value when no refresh timestamp exists")
	assert.Equal(t, 0, generateCalls, "should not call generateFn")
}

// TestGetDataFromCacheOrMatview_CacheHit_RefreshBeforeCacheTime verifies that a cached value
// is returned when the matview was refreshed before the data was cached.
func TestGetDataFromCacheOrMatview_CacheHit_RefreshBeforeCacheTime(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "mv~", nil)
	cachedAt := time.Now().UTC()
	cached := testResult{Value: "cached"}
	seedMatviewCache(t, mc, spec, cached, cachedAt)
	// Matview was refreshed 10 minutes before the data was cached
	seedRefreshTimestamp(mc, testMatview, cachedAt.Add(-10*time.Minute))

	var generateCalls int
	result, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec, testMatview, time.Hour,
		makeGenerateFn(testResult{Value: "fresh"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, cached, result, "should return cached value when refresh predates cache entry")
	assert.Equal(t, 0, generateCalls, "should not call generateFn")
}

// TestGetDataFromCacheOrMatview_CacheInvalidated_RefreshAfterCacheTime verifies that when the
// matview was refreshed after the data was cached, the cached value is invalidated and
// generateFn is called to produce fresh data.
func TestGetDataFromCacheOrMatview_CacheInvalidated_RefreshAfterCacheTime(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "mv~", nil)
	cachedAt := time.Now().UTC().Add(-5 * time.Minute)
	seedMatviewCache(t, mc, spec, testResult{Value: "stale"}, cachedAt)
	// Matview was refreshed 1 minute ago, after the data was cached
	seedRefreshTimestamp(mc, testMatview, time.Now().UTC().Add(-1*time.Minute))

	var generateCalls int
	expected := testResult{Value: "fresh"}
	result, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec, testMatview, time.Hour,
		makeGenerateFn(expected, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, expected, result, "should regenerate data after matview refresh")
	assert.Equal(t, 1, generateCalls, "should call generateFn when cache is invalidated")
	assert.GreaterOrEqual(t, mc.setCalls, 1, "should store the fresh result")
}

// TestGetDataFromCacheOrMatview_GenerateErrorSkipsCacheWrite verifies that errors from
// generateFn are not cached.
func TestGetDataFromCacheOrMatview_GenerateErrorSkipsCacheWrite(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "mv~", nil)

	var generateCalls int
	result, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec, testMatview, time.Hour,
		makeFailingGenerateFn(&generateCalls), testResult{},
	)

	assert.Len(t, errs, 1)
	assert.Equal(t, testResult{}, result)
	assert.Equal(t, 1, generateCalls)
	assert.Equal(t, 0, mc.setCalls, "should not cache error results")
}

// TestGetDataFromCacheOrMatview_InvalidRefreshTimestamp verifies that an unparseable refresh
// timestamp in the cache is treated like no refresh (cache is still used).
func TestGetDataFromCacheOrMatview_InvalidRefreshTimestamp(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "mv~", nil)
	cached := testResult{Value: "cached"}
	seedMatviewCache(t, mc, spec, cached, time.Now().UTC())
	// Store garbage as the refresh timestamp
	mc.store[RefreshMatviewKey(testMatview)] = []byte("not-a-timestamp")

	var generateCalls int
	result, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec, testMatview, time.Hour,
		makeGenerateFn(testResult{Value: "fresh"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	assert.Equal(t, cached, result, "should return cached value when refresh timestamp is unparseable")
	assert.Equal(t, 0, generateCalls)
}

// TestGetDataFromCacheOrMatview_CacheDurationPassedToSet verifies that the specified
// cacheDuration is used when writing to the cache.
func TestGetDataFromCacheOrMatview_CacheDurationPassedToSet(t *testing.T) {
	mc := newMockCache()
	spec := NewCacheSpec(testCacheKey{Query: "q1"}, "mv~", nil)
	expectedDuration := 2 * time.Hour

	var generateCalls int
	_, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec, testMatview, expectedDuration,
		makeGenerateFn(testResult{Value: "data"}, &generateCalls), testResult{},
	)

	assert.Empty(t, errs)
	cacheKey, _ := spec.GetCacheKey()
	assert.Equal(t, expectedDuration, mc.setDurations[string(cacheKey)], "should use specified cache duration")
}

// TestGetDataFromCacheOrMatview_DifferentMatviews verifies that cache entries for different
// matviews are invalidated independently.
func TestGetDataFromCacheOrMatview_DifferentMatviews(t *testing.T) {
	mc := newMockCache()
	matview7d := "prow_test_report_7d_matview"
	matview2d := "prow_test_report_2d_matview"
	spec7d := NewCacheSpec(testCacheKey{Query: "7d"}, "mv~", nil)
	spec2d := NewCacheSpec(testCacheKey{Query: "2d"}, "mv~", nil)

	cachedAt := time.Now().UTC().Add(-5 * time.Minute)
	seedMatviewCache(t, mc, spec7d, testResult{Value: "cached-7d"}, cachedAt)
	seedMatviewCache(t, mc, spec2d, testResult{Value: "cached-2d"}, cachedAt)

	// Only refresh the 7d matview (after caching)
	seedRefreshTimestamp(mc, matview7d, time.Now().UTC().Add(-1*time.Minute))
	// 2d matview was refreshed before caching
	seedRefreshTimestamp(mc, matview2d, cachedAt.Add(-10*time.Minute))

	var gen7dCalls, gen2dCalls int

	// 7d should be invalidated
	result7d, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec7d, matview7d, time.Hour,
		makeGenerateFn(testResult{Value: "fresh-7d"}, &gen7dCalls), testResult{},
	)
	assert.Empty(t, errs)
	assert.Equal(t, "fresh-7d", result7d.Value, "7d cache should be invalidated")
	assert.Equal(t, 1, gen7dCalls)

	// 2d should still be cached
	result2d, errs := GetDataFromCacheOrMatview(
		context.Background(), mc, spec2d, matview2d, time.Hour,
		makeGenerateFn(testResult{Value: "fresh-2d"}, &gen2dCalls), testResult{},
	)
	assert.Empty(t, errs)
	assert.Equal(t, "cached-2d", result2d.Value, "2d cache should not be invalidated")
	assert.Equal(t, 0, gen2dCalls)
}
