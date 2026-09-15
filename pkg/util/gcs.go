package util

import (
	"net/url"
	"regexp"
	"strings"
)

// gcsPathStrip removes the prow/gcsweb prefix through the bucket name, leaving the object path.
// It matches any bucket, including origin-ci-test, test-platform-results, and test-platform-results-public.
var gcsPathStrip = regexp.MustCompile(`.*/gs/[^/]+/`)

// GCSObjectPathFromURL returns the GCS object path from a prow job URL (logs/... or pr-logs/...).
// Empty if the URL is not a prow gs URL. Trailing slashes from the input are preserved.
func GCSObjectPathFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" {
		return ""
	}
	stripped := gcsPathStrip.ReplaceAllString(parsed.Path, "")
	if stripped == "" || stripped == parsed.Path {
		return ""
	}
	return stripped
}

// GCSObjectPrefixFromURL returns the GCS object prefix for listing a job run, always with a trailing slash.
func GCSObjectPrefixFromURL(rawURL string) string {
	path := GCSObjectPathFromURL(rawURL)
	if path == "" {
		return ""
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}
