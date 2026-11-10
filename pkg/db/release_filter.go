package db

import "k8s.io/apimachinery/pkg/util/sets"

// ExcludeReleases removes explicitly excluded releases while preserving the supplied order.
func ExcludeReleases(releases, excluded []string) []string {
	if len(excluded) == 0 {
		return releases
	}

	excludedSet := sets.New(excluded...)
	selected := make([]string, 0, len(releases))
	for _, release := range releases {
		if !excludedSet.Has(release) {
			selected = append(selected, release)
		}
	}
	return selected
}
