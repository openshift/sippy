package releaseoverride

import (
	"fmt"
	"regexp"
)

type regexpRelease struct {
	pattern *regexp.Regexp
	release string
}

// SyntheticReleaseOverrides holds both exact job-name overrides and compiled
// regexp patterns from synthetic releases. Use New to construct one.
type SyntheticReleaseOverrides struct {
	exactMatches  map[string]string
	regexpMatches []regexpRelease
}

// New creates an empty SyntheticReleaseOverrides.
func New() *SyntheticReleaseOverrides {
	return &SyntheticReleaseOverrides{exactMatches: map[string]string{}}
}

// AddExact registers an exact job-name → release mapping and returns an error
// if the job was already claimed by a different release.
func (s *SyntheticReleaseOverrides) AddExact(jobName, release string) error {
	if existing, conflict := s.exactMatches[jobName]; conflict {
		return fmt.Errorf(
			"job %q is claimed by synthetic releases %q and %q",
			jobName, existing, release,
		)
	}
	s.exactMatches[jobName] = release
	return nil
}

// AddRegexp compiles the pattern and associates it with the given release.
func (s *SyntheticReleaseOverrides) AddRegexp(pattern, release string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	s.regexpMatches = append(s.regexpMatches, regexpRelease{pattern: re, release: release})
	return nil
}

// Lookup returns the synthetic release a job belongs to, checking exact names
// first and then regexp patterns.
func (s *SyntheticReleaseOverrides) Lookup(jobName string) (string, bool) {
	if s == nil {
		return "", false
	}
	if release, ok := s.exactMatches[jobName]; ok {
		return release, true
	}
	for _, rr := range s.regexpMatches {
		if rr.pattern.MatchString(jobName) {
			return rr.release, true
		}
	}
	return "", false
}
