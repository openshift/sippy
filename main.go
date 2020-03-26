package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

/*
{
  "release-openshift-ocp-installer-console-aws-4.4": {
    "alert": "",
    "last_run_timestamp": 1585175980000,
    "last_update_timestamp": 1585180645,
    "latest_green": "1",
    "overall_status": "PASSING",
    "overall_status_icon": "done",
    "status": "10 of 10 (100.0%) recent columns passed (3670 of 3670 or 100.0% cells)",
    "tests": [],
    "dashboard_name": ""
  },
*/

var (
	dashboard_urls []string = []string{
		//"https://testgrid.k8s.io/redhat-openshift-informing",
		"https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-informing",
	}
)

type FailureMeta struct {
	count int
	jobs  map[string]interface{}
	sig   string
}

type Job struct {
	OverallStatus string `json:"overall_status"`
}

type JobDetails struct {
	Tests []Test `json:"tests"`
}

type Test struct {
	Name string `json:"name"`
}

func badStatus(status string) bool {
	switch status {
	case "FAILING", "FLAKY":
		return true
	}
	return false
}

func fetchJobs(dashboard_url string) (map[string]Job, error) {
	resp, err := http.Get(dashboard_url + "/summary")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Non-200 response code fetching dashboard page: %v", resp)
	}

	var jobs map[string]Job
	err = json.NewDecoder(resp.Body).Decode(&jobs)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func fetchJobDetails(dashboard_url, jobName, sortBy string) (JobDetails, error) {
	//https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/table?tab=release-openshift-ocp-installer-e2e-aws-4.4&width=10&exclude-filter-by-regex=Monitor%5Cscluster&exclude-filter-by-regex=%5Eoperator.Run%20template.*container%20test%24&dashboard=redhat-openshift-ocp-release-4.4-informing
	// sort-by-flakiness=
	url := fmt.Sprintf("%s/table?tab=%s&exclude-filter-by-regex=Monitor%%5Cscluster&exclude-filter-by-regex=%%5Eoperator.Run%%20template.*container%%20test%%24&%s", dashboard_url, jobName, sortBy)
	//fmt.Printf("fetching job details: %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return JobDetails{}, err
	}
	if resp.StatusCode != 200 {
		return JobDetails{}, fmt.Errorf("Non-200 response code fetching job details: %v", resp)
	}

	var details JobDetails
	err = json.NewDecoder(resp.Body).Decode(&details)
	if err != nil {
		return JobDetails{}, err
	}
	return details, nil
}

func main() {
	failures := make(map[string]FailureMeta)
	sigRegex := regexp.MustCompile(`\[(sig-.*?)\]`)
	//	i := 0

	for _, dashboard_url := range dashboard_urls {
		jobs, err := fetchJobs(dashboard_url)
		if err != nil {
			fmt.Printf("Error fetching dashboard page %s: %v\n", dashboard_url, err)
			continue
		}

		for jobName, job := range jobs {
			if badStatus(job.OverallStatus) {
				fmt.Printf("Job %s has bad status %s\n", jobName, job.OverallStatus)
				details, err := fetchJobDetails(dashboard_url, jobName, "sort-by-flakiness=")
				if err != nil {
					fmt.Printf("Error fetching job details for %s: %v\n", jobName, err)
				}
				for _, test := range details.Tests {
					if test.Name == "Overall" {
						continue
					}
					fmt.Printf("found top failing test: %q\n", test.Name)
					meta, ok := failures[test.Name]
					if !ok {
						meta = FailureMeta{
							jobs: make(map[string]interface{}),
						}
					}
					meta.count++
					if _, ok := meta.jobs[jobName]; !ok {
						meta.jobs[jobName] = struct{}{}
					}
					match := sigRegex.FindStringSubmatch(test.Name)
					if len(match) > 1 {
						meta.sig = match[1]
					} else {
						meta.sig = "sig-unknown"
					}
					failures[test.Name] = meta
					break
				}
				fmt.Println("\n\n================================================================================")
			}
			/*
				i++
				if i > 5 {
					break
				}
			*/
		}
	}

	sigCount := make(map[string]int)
	for t, f := range failures {
		fmt.Printf("Test: %s\nCount: %d\nSig: %s\nJobs: %v\n\n", t, f.count, f.sig, f.jobs)
		if _, ok := sigCount[f.sig]; !ok {
			sigCount[f.sig] = 0
		}
		sigCount[f.sig] += f.count
	}
	for s, c := range sigCount {
		fmt.Printf("Sig %s is responsible for the top flake in %d job definitions\n", s, c)
	}
}

/*
https://testgrid.k8s.io/redhat-openshift-informing


release page:
https://testgrid.k8s.io/redhat-openshift-informing/summary

job:
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing/table?tab=release-openshift-ocp-installer-e2e-aws-4.4&width=10&exclude-filter-by-regex=Monitor%5Cscluster&exclude-filter-by-regex=%5Eoperator.Run%20template.*container%20test%24&dashboard=redhat-openshift-ocp-release-4.4-informing
*/
