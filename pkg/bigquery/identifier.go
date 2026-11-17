package bigquery

import (
	"fmt"
	"regexp"
)

var datasetIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+){0,2}$`)

// ValidateDatasetIdentifier permits only simple identifier segments because
// dataset paths are interpolated into BigQuery SQL rather than passed as values.
func ValidateDatasetIdentifier(dataset string) error {
	if !datasetIdentifierPattern.MatchString(dataset) {
		return fmt.Errorf("invalid BigQuery dataset %q", dataset)
	}
	return nil
}
