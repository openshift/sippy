package param

import (
	"net/http"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Cleanse removes unexpected characters from an input parameter value.
// This is useful for sanitizing dynamic SQL queries built from user input.
func Cleanse(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, name)
}

// when requesting a param, also validate it against a regexp to ensure it is what we expect
var wordRegexp = regexp.MustCompile(`^[\w]+$`)
var numRegexp = regexp.MustCompile(`^[\d]+$`)
var nameRegexp = regexp.MustCompile(`^[-.\w]+$`)
var releaseRegexp = regexp.MustCompile(`^[\d]+\.[\d]+$`)
var paramRegexp = map[string]*regexp.Regexp{
	// sippy classic params
	"release":         regexp.MustCompile(`^(Presubmits|[\d]+\.[\d]+)$`),
	"period":          wordRegexp,
	"stream":          wordRegexp,
	"arch":            wordRegexp,
	"payload":         nameRegexp,
	"fromPayload":     nameRegexp,
	"toPayload":       nameRegexp,
	"job":             nameRegexp,
	"job_name":        nameRegexp,
	"test":            regexp.MustCompile(`^.+$`), // tests can be anything, so always parameterize in sql
	"prow_job_run_id": numRegexp,
	"file":            nameRegexp,
	"repo_info":       nameRegexp,
	"pull_number":     numRegexp,
	"sort":            wordRegexp,
	"sortField":       wordRegexp,
	// component readiness params
	"baseRelease":      releaseRegexp,
	"sampleRelease":    releaseRegexp,
	"testBasisRelease": releaseRegexp,
	"samplePROrg":      nameRegexp,
	"samplePRRepo":     nameRegexp,
	"samplePRNumber":   numRegexp,
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
