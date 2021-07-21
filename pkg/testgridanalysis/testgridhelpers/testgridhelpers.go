package testgridhelpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	gourl "net/url"
	"os"
	"regexp"
	"time"

	testgridv1 "github.com/openshift/sippy/pkg/apis/testgrid/v1"
	"github.com/openshift/sippy/pkg/util"
	"k8s.io/klog"
)

func DownloadData(dashboards []string, filter, storagePath string) {
	var jobFilter *regexp.Regexp
	if len(filter) > 0 {
		jobFilter = regexp.MustCompile(filter)
	}

	for _, dashboard := range dashboards {
		err := downloadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error fetching dashboard page %s: %v\n", dashboard, err)
			continue
		}
		jobs, _, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error loading dashboard page %s: %v\n", dashboard, err)
			continue
		}

		for jobName, job := range jobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				err := downloadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error fetching job details for %s: %v\n", jobName, err)
				}
			}
		}
	}
}

// LoadTestGridDataFromDisk reads the requested testgrid data from disk and returns the details and the timestamp of the last
// modification on disk
func LoadTestGridDataFromDisk(storagePath string, dashboards []string, jobFilter *regexp.Regexp) ([]testgridv1.JobDetails, time.Time) {
	testGridJobDetails := []testgridv1.JobDetails{}

	lastUpdateTime := time.Time{}

	for _, dashboard := range dashboards {
		jobs, ts, err := loadJobSummaries(dashboard, storagePath)
		if err != nil {
			klog.Errorf("Error loading dashboard page %s: %v\n", dashboard, err)
			continue
		}
		if ts.After(lastUpdateTime) {
			lastUpdateTime = ts
		}

		for jobName, job := range jobs {
			if util.RelevantJob(jobName, job.OverallStatus, jobFilter) {
				klog.V(4).Infof("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := loadJobDetails(dashboard, jobName, storagePath)
				if err != nil {
					klog.Errorf("Error loading job details for %s: %v\n", jobName, err)
				} else {
					testGridJobDetails = append(testGridJobDetails, details)
				}
			}
		}
	}

	return testGridJobDetails, lastUpdateTime
}

func loadJobDetails(dashboard, jobName, storagePath string) (testgridv1.JobDetails, error) {
	details := testgridv1.JobDetails{
		Name: jobName,
	}

	url := URLForJobDetails(dashboard, jobName)

	var buf *bytes.Buffer
	filename := storagePath + "/" + normalizeURL(url.String())
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return details, fmt.Errorf("could not read local data file %s: %v", filename, err)
	}
	buf = bytes.NewBuffer(b)

	err = json.NewDecoder(buf).Decode(&details)
	if err != nil {
		return details, err
	}
	details.TestGridUrl = URLForJob(dashboard, jobName).String()
	return details, nil
}

func loadJobSummaries(dashboard, storagePath string) (map[string]testgridv1.JobSummary, time.Time, error) {
	jobs := make(map[string]testgridv1.JobSummary)
	url := URLForJobSummary(dashboard)

	var buf *bytes.Buffer
	filename := storagePath + "/" + normalizeURL(url.String())
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return jobs, time.Time{}, fmt.Errorf("could not read local data file %s (holds %s): %v", filename, url.String(), err)
	}
	buf = bytes.NewBuffer(b)
	f, _ := os.Stat(filename)
	f.ModTime()

	err = json.NewDecoder(buf).Decode(&jobs)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("could not parse local data file %s (holds %s): %v", filename, url.String(), err)
	}

	return jobs, f.ModTime(), nil
}

func normalizeURL(url string) string {
	return replaceChars(url, `/":?`, '-')
}

func replaceChars(s, needles string, by rune) string {
	out := make([]rune, len(s))
NextChar:
	for i, c := range s {
		for _, r := range needles {
			if c == r {
				out[i] = by
				continue NextChar
			}
		}
		out[i] = c
	}
	return string(out)
}

func downloadJobSummaries(dashboard, storagePath string) error {
	url := URLForJobSummary(dashboard)

	resp, err := http.Get(url.String())
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("non-200 response code fetching job details from %v: %v", url, resp)
	}
	filename := storagePath + "/" + normalizeURL(url.String())
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer([]byte{})
	io.Copy(buf, resp.Body)

	_, err = f.Write(buf.Bytes())
	return err
}

// https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/table?&show-stale-tests=&tab=release-openshift-origin-installer-e2e-azure-compact-4.4

// https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing#release-openshift-origin-installer-e2e-azure-compact-4.4&show-stale-tests=&sort-by-failures=

func URLForJobDetails(dashboard, jobName string) *gourl.URL {
	url := &gourl.URL{
		Scheme: "https",
		Host:   "testgrid.k8s.io",
		Path:   fmt.Sprintf("/%s/table", gourl.PathEscape(dashboard)),
	}
	query := url.Query()
	query.Set("show-stale-tests", "")
	query.Set("tab", jobName)
	url.RawQuery = query.Encode()

	return url
}
func URLForJobSummary(dashboard string) *gourl.URL {
	url := &gourl.URL{
		Scheme: "https",
		Host:   "testgrid.k8s.io",
		Path:   fmt.Sprintf("/%s/summary", gourl.PathEscape(dashboard)),
	}

	return url
}

func URLForJob(dashboard, jobName string) *gourl.URL {
	url := &gourl.URL{
		Scheme: "https",
		Host:   "testgrid.k8s.io",
		Path:   fmt.Sprintf("/%s", gourl.PathEscape(dashboard)),
	}
	// this is a non-standard fragment honored by test-grid
	url.Fragment = gourl.PathEscape(jobName)

	return url
}

func downloadJobDetails(dashboard, jobName, storagePath string) error {
	url := URLForJobDetails(dashboard, jobName)

	resp, err := http.Get(url.String())
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		responseDump, _ := httputil.DumpResponse(resp, true)
		fmt.Fprintf(os.Stderr, "response dump\n%v\n", string(responseDump))
		return fmt.Errorf("non-200 response code fetching job details from %v: %v", url, resp)
	}

	filename := storagePath + "/" + normalizeURL(url.String())
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer([]byte{})
	io.Copy(buf, resp.Body)

	_, err = f.Write(buf.Bytes())
	return err

}
