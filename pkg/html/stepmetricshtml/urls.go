package stepmetricshtml

import (
	"net/url"
	"path/filepath"
	"regexp"
)

const (
	StepMetricsUIPath string = "/stepmetrics"
)

type URLGenerator interface {
	URL() *url.URL
}

type StepRegistryURL struct {
	Reference string
	Search    string
}

var _ URLGenerator = (*StepRegistryURL)(nil)

func (s StepRegistryURL) URL() *url.URL {
	u := &url.URL{
		Scheme: "https",
		Host:   "steps.ci.openshift.org",
		Path:   s.getPath(),
	}

	if u.Path == "/search" {
		values := url.Values{}
		values.Add("job", s.Search)
		u.RawQuery = values.Encode()
	}

	return u
}

func (s StepRegistryURL) getPath() string {
	if s.Reference != "" {
		return filepath.Join("/reference", s.Reference)
	}

	return "/search"
}

type SippyURL struct {
	Release           string
	MultistageJobName string
	StepName          string
	Variant           string
}

var _ URLGenerator = (*SippyURL)(nil)

func (s SippyURL) URL() *url.URL {
	values := mapToURLValues(map[string]string{
		"release":           s.Release,
		"multistageJobName": s.MultistageJobName,
		"stepName":          s.StepName,
		"variant":           s.Variant,
	})

	return &url.URL{
		Path:     StepMetricsUIPath,
		RawQuery: values.Encode(),
	}
}

var _ URLGenerator = (*CISearchURL)(nil)

type CISearchURL struct {
	Release string
	Search  string
}

func (c CISearchURL) URL() *url.URL {
	values := mapToURLValues(map[string]string{
		"maxAge":     "168h",
		"context":    "1",
		"type":       `bug+junit`,
		"name":       c.Release,
		"maxMatches": "5",
		"maxBytes":   "20971520",
		"groupBy":    "job",
		"search":     regexp.QuoteMeta(c.Search),
	})

	return &url.URL{
		Scheme:   "https",
		Host:     "search.ci.openshift.org",
		Path:     "/",
		RawQuery: values.Encode(),
	}
}

func mapToURLValues(inMap map[string]string) url.Values {
	values := url.Values{}

	for k, v := range inMap {
		if v != "" {
			values.Add(k, v)
		}
	}

	return values
}
