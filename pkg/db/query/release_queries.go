package query

import (
	"fmt"

	"github.com/openshift/sippy/pkg/db"
)

// CurrentActiveRelease returns the most relevant active release. It prefers the
// in-development release with the oldest development_start_date (most mature).
// If no in-development release exists (all have a GA date), it falls back to
// the release with the most recent GA date. Used as a fallback for
// partition-scoped queries when no specific release is available from the caller.
func CurrentActiveRelease(dbc *db.DB) (string, error) {
	var release string
	err := dbc.DB.Table("release_definitions").
		Where("ga_date IS NULL").
		Where("product = ?", "OCP").
		Order("development_start_date ASC").
		Limit(1).
		Pluck("release", &release).Error
	if err != nil {
		return "", fmt.Errorf("query in-development release: %w", err)
	}
	if release != "" {
		return release, nil
	}
	err = dbc.DB.Table("release_definitions").
		Where("product = ?", "OCP").
		Order("ga_date DESC").
		Limit(1).
		Pluck("release", &release).Error
	if err != nil {
		return "", fmt.Errorf("query latest GA release: %w", err)
	}
	return release, nil
}
