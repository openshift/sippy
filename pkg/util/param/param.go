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
var dateRegexp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var nonEmptyRegex = regexp.MustCompile(`^.+$`)
var boolRegexp = regexp.MustCompile(`^(true|false)$`)
var paramRegexp = map[string]*regexp.Regexp{
	// sippy classic params
	"release":          regexp.MustCompile(`^[\w.-]+$`), // usually 4.x or Presubmit, but allow any "word"
	"period":           wordRegexp,
	"stream":           wordRegexp,
	"arch":             wordRegexp,
	"payload":          nameRegexp,
	"fromPayload":      nameRegexp,
	"toPayload":        nameRegexp,
	"job":              nameRegexp,
	"job_name":         nameRegexp,
	"test":             regexp.MustCompile(`^.+$`),       // tests can be anything, so always parameterize in sql
	"test_id":          regexp.MustCompile(`^[\w:.-]+$`), // test IDs like "openshift-tests-upgrade:af8a62c596e5c2b5448a5d308f4989a6"
	"prow_job_run_id":  uintRegexp,
	"prow_job_run_ids": regexp.MustCompile(`^\d+(,\d+)*$`), // comma-separated integers
	"org":              nameRegexp,
	"repo":             nameRegexp,
	"pr_number":        uintRegexp,
	"file":             nameRegexp,
	"repo_info":        nameRegexp,
	"pull_number":      uintRegexp,
	"sort":             wordRegexp,
	"sortField":        wordRegexp,
	"start_date":       dateRegexp, // YYYY-MM-DD format
	"end_date":         dateRegexp, // YYYY-MM-DD format
	"include_success":  boolRegexp, // true or false
	// component readiness params
	"baseRelease":      releaseRegexp,
	"sampleRelease":    releaseRegexp,
	"testBasisRelease": releaseRegexp,
	"samplePROrg":      nameRegexp,
	"samplePRRepo":     nameRegexp,
	"samplePRNumber":   uintRegexp,
	"samplePayloadTag": nameRegexp,
	"view":             nameRegexp, // component readiness view name
	// jobartifacts params
	"prowJobRuns":        regexp.MustCompile(`^\d+(,\d+)*$`), // comma-separated integers
	"pathGlob":           nonEmptyRegex,                      // a glob can be anything
	"maxJobFilesToScan ": uintRegexp,
	"textContains":       nonEmptyRegex, // text search can look for anything
	"textRegex":          nonEmptyRegex, // regex could be just about anything
	"maxFileMatches":     uintRegexp,
	"beforeContext":      uintRegexp,
	"afterContext":       uintRegexp,
	// recent test failures params
	"previousPeriod": wordRegexp,
	"includeOutputs": boolRegexp,
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

// ReadBool returns the boolean value of a query parameter.
// Accepts "true" or "false" (case-sensitive).
// If the param is not present or empty, it returns the provided default value and nil.
// If the value is invalid, it returns false and an error.
func ReadBool(req *http.Request, name string, defaultValue bool) (bool, error) {
	value := req.URL.Query().Get(name)
	if value == "" {
		return defaultValue, nil
	}

	if !boolRegexp.MatchString(value) {
		err := fmt.Errorf("invalid value for %q param: %q (expected true or false)", name, value)
		log.Warn(err)
		return false, err
	}

	return value == "true", nil
}
