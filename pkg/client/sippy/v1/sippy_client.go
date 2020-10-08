package v1

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"

	sippyv1 "github.com/openshift/sippy/pkg/apis/sippy/v1"
)

const defaultSippyAPIEndpoint = "https://sippy.ci.openshift.org/json"

type ReleaseReportClient interface {
	Release(name string) ReleaseReportInterface
}

type ReleaseReportInterface interface {
	Report(ctx context.Context) (*sippyv1.Report, error)
	ReportAll(ctx context.Context) (sippyv1.ReleaseReportsMap, error)
}

type releaseReportClient struct {
	http.Client
}

// New returns a new Sippy (https://sippy.ci.openshift.org/json) API client
func New() ReleaseReportClient {
	return &releaseReportClient{http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}}
}

// Release returns an interface that allow to fetch report for specific openshift release.
// The name format is "4.6", "4.5", etc...
func (c *releaseReportClient) Release(name string) ReleaseReportInterface {
	releaseUrl, _ := url.Parse(defaultSippyAPIEndpoint)
	releaseQuery := releaseUrl.Query()
	releaseQuery.Add("release", name)
	releaseUrl.RawQuery = releaseQuery.Encode()
	return &releaseReportInterface{
		releaseUrl:          *releaseUrl,
		releaseName:         name,
		releaseReportClient: c,
	}
}

type releaseReportInterface struct {
	releaseUrl  url.URL
	releaseName string
	*releaseReportClient
}

func (c *releaseReportInterface) ReportAll(ctx context.Context) (sippyv1.ReleaseReportsMap, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.releaseUrl.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	report := sippyv1.ReleaseReportsMap{}
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return nil, err
	}
	return report, nil
}

// Report returns a single release report or HTTP error.
func (c *releaseReportInterface) Report(ctx context.Context) (*sippyv1.Report, error) {
	result, err := c.ReportAll(ctx)
	if err != nil {
		return nil, err
	}
	return result[c.releaseName], nil
}
