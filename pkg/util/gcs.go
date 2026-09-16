package util

import (
	"net/url"
	"regexp"
	"strings"
)

// gcsPathStrip removes the prow prefix through the bucket name, leaving the object path.
// It matches /gs/<bucket>/ at the start of the path, or /view/gs/<bucket>/ for prow job URLs.
var gcsPathStrip = regexp.MustCompile(`^/(?:view/)?gs/[^/]+/`)

// gcsBucketFromPath captures the bucket name from a prow gs URL path.
var gcsBucketFromPath = regexp.MustCompile(`^/(?:view/)?gs/([^/]+)/`)

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

// GCSBucketFromURL returns the GCS bucket name from a prow job URL.
// Empty if the URL is not a prow gs URL.
func GCSBucketFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" {
		return ""
	}
	m := gcsBucketFromPath.FindStringSubmatch(parsed.Path)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ResolveGCSBucket prefers the bucket recorded on the job, then the bucket in the
// prow URL, then fallback. Used so historical jobs keep reading/writing the bucket
// they were ingested from instead of a process-wide default.
func ResolveGCSBucket(stored, jobURL, fallback string) string {
	if stored != "" {
		return stored
	}
	if b := GCSBucketFromURL(jobURL); b != "" {
		return b
	}
	return fallback
}
