package util

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/redis.v5"

	"github.com/openshift/sippy/pkg/api/componentreadiness"
	"github.com/openshift/sippy/pkg/apis/api/componentreport"
	"github.com/openshift/sippy/pkg/apis/api/componentreport/crtest"
	log "github.com/sirupsen/logrus"
)

// E2ECacheManipulator provides utilities for manipulating values in the Redis cache
type E2ECacheManipulator struct {
	release string
	client  *redis.Client
}

func NewE2ECacheManipulator(release string) (*E2ECacheManipulator, error) {
	client, err := connectToRedis()
	if err != nil {
		return nil, err
	}
	return &E2ECacheManipulator{
		release: release,
		client:  client,
	}, nil
}

func (c *E2ECacheManipulator) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// connectToRedis creates a Redis client connection
func connectToRedis() (*redis.Client, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:23479" // Default for e2e tests
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Test Redis connection
	if err := client.Ping().Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return client, nil
}

// AddTestRegressionsToReport adds test regressions to the component report in cache
// so they can be found by GetTriagePotentialMatches function
func (c *E2ECacheManipulator) AddTestRegressionsToReport(testRegressions []componentreport.ReportTestSummary) error {
	// Get the current component report directly from the cache
	report, cacheKey, err := c.GetReport()
	if err != nil {
		return fmt.Errorf("failed to get component report from cache: %w", err)
	}

	// Add each test regression to the appropriate place in the report
	for _, testRegression := range testRegressions {
		if testRegression.Regression == nil {
			continue // Skip if no regression data
		}

		// Find or create the appropriate row for this component
		var targetRowIndex = -1
		for i, row := range report.Rows {
			if row.Component == testRegression.Component {
				targetRowIndex = i
				break
			}
		}

		// If no row exists for this component, create one
		if targetRowIndex == -1 {
			newRow := componentreport.ReportRow{
				RowIdentification: crtest.RowIdentification{
					Component:  testRegression.Component,
					Capability: testRegression.Capability,
				},
				Columns: []componentreport.ReportColumn{
					{
						ColumnIdentification: crtest.ColumnIdentification{
							Variants: testRegression.Variants,
						},
						Status:         crtest.SignificantRegression,
						RegressedTests: []componentreport.ReportTestSummary{testRegression},
					},
				},
			}
			report.Rows = append(report.Rows, newRow)
		} else {
			// Add to existing row - find or create a matching column
			row := &report.Rows[targetRowIndex]
			var targetColumnIndex = -1

			// Look for a column with matching variants
			for i, col := range row.Columns {
				if variantsMatch(col.Variants, testRegression.Variants) {
					targetColumnIndex = i
					break
				}
			}

			if targetColumnIndex == -1 {
				// Create new column
				newColumn := componentreport.ReportColumn{
					ColumnIdentification: crtest.ColumnIdentification{
						Variants: testRegression.Variants,
					},
					Status:         crtest.SignificantRegression,
					RegressedTests: []componentreport.ReportTestSummary{testRegression},
				}
				row.Columns = append(row.Columns, newColumn)
			} else {
				// Add to existing column
				row.Columns[targetColumnIndex].RegressedTests = append(row.Columns[targetColumnIndex].RegressedTests, testRegression)
			}
		}
	}

	// Update the cached component report so GetTriagePotentialMatches can find the test regressions
	err = c.updateReportWithKey(report, cacheKey)
	if err != nil {
		log.WithError(err).Warn("Failed to update cached component report, test may not work as expected")
	}

	return nil
}

// GetReport retrieves the component report directly from Redis cache
func (c *E2ECacheManipulator) GetReport() (componentreport.ComponentReport, string, error) {
	// Find the ComponentReport cache key by scanning for keys that start with "ComponentReport~"
	keyPattern := "_SIPPY_*ComponentReport~*"
	keys, err := c.client.Keys(keyPattern).Result()
	if err != nil {
		return componentreport.ComponentReport{}, "", fmt.Errorf("failed to scan for ComponentReport keys: %w", err)
	}

	var cacheKey string
	for _, key := range keys {
		// Strip the prefixes to get the JSON part
		// Key format: "_SIPPY_cc:ComponentReport~{JSON}" or "_SIPPY_ComponentReport~{JSON}"
		jsonPart := key

		// Remove "_SIPPY_" prefix if present
		jsonPart = strings.TrimPrefix(jsonPart, "_SIPPY_")

		// Remove "cc:" prefix if present (compressed cache prefix)
		jsonPart = strings.TrimPrefix(jsonPart, "cc:")

		// Remove "ComponentReport~" prefix
		if !strings.HasPrefix(jsonPart, "ComponentReport~") {
			log.Warnf("Unexpected cache key format, missing ComponentReport~ prefix: %s", key)
			continue
		}
		jsonPart = jsonPart[len("ComponentReport~"):]

		gk := &componentreadiness.GeneratorCacheKey{}
		if err := json.Unmarshal([]byte(jsonPart), gk); err != nil {
			log.Warnf("Failed to unmarshal ComponentReport key JSON '%s' from key '%s': %v", jsonPart, key, err)
			continue
		}
		if gk.SampleRelease.Name == Release {
			cacheKey = key
			break
		}
	}
	if cacheKey == "" {
		return componentreport.ComponentReport{}, "", fmt.Errorf("failed to find proper ComponentReport key")
	}
	log.Debugf("Found ComponentReport cache key: %s", cacheKey)

	// Get the cached data
	cachedData, err := c.client.Get(cacheKey).Bytes()
	if err != nil {
		return componentreport.ComponentReport{}, "", fmt.Errorf("failed to get cached data for key %s: %w", cacheKey, err)
	}

	// Unmarshal the component report
	var report componentreport.ComponentReport
	err = json.Unmarshal(cachedData, &report)
	if err != nil {
		return componentreport.ComponentReport{}, "", fmt.Errorf("failed to unmarshal component report: %w", err)
	}

	log.Debugf("Retrieved cached component report with %d rows", len(report.Rows))
	return report, cacheKey, nil
}

// updateReportWithKey updates the Redis cache with the modified component report using the exact cache key
func (c *E2ECacheManipulator) updateReportWithKey(report componentreport.ComponentReport, cacheKey string) error {
	if cacheKey == "" {
		return fmt.Errorf("cache key is empty, cannot update cache")
	}

	// Marshal the modified report to JSON
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal component report: %w", err)
	}

	// Store plain JSON in Redis cache with a reasonable expiration (1 hour)
	err = c.client.Set(cacheKey, reportJSON, time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to store in Redis cache: %w", err)
	}

	log.Debugf("Updated cached component report with key %s, %d rows", cacheKey, len(report.Rows))
	return nil
}

// variantsMatch checks if two variant maps are equivalent
func variantsMatch(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
