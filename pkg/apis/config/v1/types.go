package v1

import "time"

type SippyConfig struct {
	Prow     ProwConfig               `yaml:"prow"`
	Releases map[string]ReleaseConfig `yaml:"releases"`
}

type ProwConfig struct {
	// URL to the prowjob.js endpoint of the prow instance. This endpoint contains
	// a JSON file with all the ProwJob resources from the prow cluster.
	URL string `yaml:"url"`
}

type ReleaseConfig struct {
	// GADate contains the GA date for this release, if any.
	GADate *time.Time

	// Jobs is a set of jobs that should be considered part of the release.
	Jobs map[string]bool `yaml:"jobs,omitempty"`

	// Regexp is a list of regular expressions that match a job to a release.
	Regexp []string `yaml:"regexp,omitempty"`

	// BlockingJobs is the list of blocking payload jobs
	BlockingJobs []string `yaml:"blockingJobs,omitempty"`

	// InformingJobs is the list of informing payload jobs
	InformingJobs []string `yaml:"informingJobs,omitempty"`
}
