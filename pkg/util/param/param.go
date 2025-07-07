package param

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Cleanse removes unexpected characters from an input parameter value.
// This is useful for sanitizing dynamic SQL queries built from user input.
func Cleanse(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ':' || r == ' ' {
			return r
		}
		return -1
	}, name)
}

// when requesting a param, also validate it against a regexp to ensure it is what we expect
var wordRegexp = regexp.MustCompile(`^\w+$`)
var uintRegexp = regexp.MustCompile(`^\d+$`)
var nameRegexp = regexp.MustCompile(`^[-.\w]+$`)
var releaseRegexp = regexp.MustCompile(`^\d+\.\d+$`)
var nonEmptyRegex = regexp.MustCompile(`^.+$`)
var paramRegexp = map[string]*regexp.Regexp{
	// sippy classic params
	"release":         regexp.MustCompile(`^(Presubmits|\d+\.\d+)$`),
	"period":          wordRegexp,
	"stream":          wordRegexp,
	"arch":            wordRegexp,
	"payload":         nameRegexp,
	"fromPayload":     nameRegexp,
	"toPayload":       nameRegexp,
	"job":             nameRegexp,
	"job_name":        nameRegexp,
	"test":            regexp.MustCompile(`^.+$`), // tests can be anything, so always parameterize in sql
	"prow_job_run_id": uintRegexp,
	"file":            nameRegexp,
	"repo_info":       nameRegexp,
	"pull_number":     uintRegexp,
	"sort":            wordRegexp,
	"sortField":       wordRegexp,
	// component readiness params
	"baseRelease":      releaseRegexp,
	"sampleRelease":    releaseRegexp,
	"testBasisRelease": releaseRegexp,
	"samplePROrg":      nameRegexp,
	"samplePRRepo":     nameRegexp,
	"samplePRNumber":   uintRegexp,
	// jobartifacts params
	"prowJobRuns":        regexp.MustCompile(`^\d+(,\d+)*$`), // comma-separated integers
	"pathGlob":           nonEmptyRegex,                      // a glob can be anything
	"maxJobFilesToScan ": uintRegexp,
	"textContains":       nonEmptyRegex, // text search can look for anything
	"textRegex":          nonEmptyRegex, // regex could be just about anything
	"maxFileMatches":     uintRegexp,
	"beforeContext":      uintRegexp,
	"afterContext":       uintRegexp,
}

// SafeRead returns the value of a query parameter only if it matches the given regexp.
// this should be used to validate query parameters that are not otherwise validated.
func SafeRead(req *http.Request, name string) string {
	re, ok := paramRegexp[name]
	if !ok {
		log.Fatalf("code BUG: request for unknown param %s", name) // revive:disable-line:deep-exit
	}
	value := req.URL.Query().Get(name)
	if value == "" || re.MatchString(value) {
		return value
	}
	log.Warnf("invalid value for %s param: %q", name, value)
	return ""
}

// ReadUint returns the value of a query parameter only if it is an unsigned int.
// If the param is not present, it returns 0 and nil.
// If limit is positive then values greater than the limit are considered invalid.
// If the value is invalid, it returns 0 and an error.
// If the param is present and valid, it returns the value and nil.
func ReadUint(req *http.Request, name string, limit int) (int, error) {
	intStr := req.URL.Query().Get(name)
	if intStr == "" {
		return 0, nil
	}

	if !uintRegexp.MatchString(intStr) {
		err := fmt.Errorf("invalid value for %q param: %q", name, intStr)
		log.Warn(err)
		return 0, err
	}

	intValue, err := strconv.Atoi(intStr)
	if err != nil {
		log.WithError(err).Warnf("invalid value for %q param: %q", name, intStr)
		return 0, err
	}

	if limit > 0 && intValue > limit {
		err = fmt.Errorf("value %d for %q param exceeds limit %d", intValue, name, limit)
		log.Warn(err)
		return 0, err
	}
	return intValue, nil
}
