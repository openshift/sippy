#!/usr/bin/env python
#
# This script can be used to generate the code block for component readiness resolved incidents. (see resolved_4.15_issues.go)
#
# Fill out the variants at the start of the script per the comments. Then run the script and pipe the output to a file.
# Once finished, open that file and copy paste the resulting go code (at the end) into the resolved_issues.go file.
# Formatting may be off, let the IDE and gofmt handle that.

import os
import requests

# The main test report, taken from top level component readiness by monitoring the dev console in firefox to fetch the API request
# made to component readiness. We'll parse through everything in this report looking for the specific test we're flagging for mass
# attribution.
TEST_REPORT = "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2023-10-31T23:59:59Z&baseRelease=4.14&baseStartTime=2023-10-04T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-02-28T23:59:59Z&sampleRelease=4.15&sampleStartTime=2024-02-22T00:00:00Z"

# the test we're mass attributing to a known issue.
TEST_ID = "openshift-tests:c1f54790201ec8f4241eca902f854b79"

# Template for the regression we're ignoring failed job runs for. Update the Description, Jira, and ResolutionDate below.
REGRESSION_TEMPLATE = '''
	mustAddResolvedIssue(release415, ResolvedIssue{
		TestID:   "%s",
		TestName: "%s",
		Variant: apitype.ComponentReportColumnIdentification{
			Network:  "%s",
			Upgrade:  "%s",
			Arch:     "%s",
			Platform: "%s",
            Variant: "%s",
		},
		Issue: Issue{
			IssueType: "Infrastructure",
			Infrastructure: &InfrastructureIssue{
				Description:    "Loki outage caused ci logging pods to never go ready and eventually a DaemonSetRolloutStuck alert to fire",
				JiraURL:        "https://issues.redhat.com/browse/TRT-1537",
				ResolutionDate: mustTime("2024-02-28T13:00:00Z"),
			},
			PayloadBug: nil,
		},
		ImpactedJobRuns: []JobRun{
%s
    },
})
'''

OUTPUT_FILE = "resolved_issues.txt"

# The template for each entry in ImpactedJobRuns above. You shouldn't need to modify this.
JOB_TEMPLATE = '''			{
				URL:       "%s",
				StartTime: mustTime("%s"),
			},
'''

#https://sippy.dptools.openshift.org/api/component_readiness/test_details?baseEndTime=2023-10-31T23:59:59Z&baseRelease=4.14&baseStartTime=2023-10-04T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-02-27T23:59:59Z&sampleRelease=4.15&sampleStartTime=2024-02-21T00:00:00Z&component=Monitoring&capability=Other&testId=openshift-tests-upgrade:c1f54790201ec8f4241eca902f854b79&environment=ovn%20upgrade-minor%20amd64%20metal-ipi%20standard&network=ovn&upgrade=upgrade-minor&arch=amd64&platform=metal-ipi&variant=standard



def fetch_json_data(api_url):
    try:
        response = requests.get(api_url)
        # Check if the request was successful (status code 200)
        if response.status_code == 200:
            # Return JSON data
            return response.json()
        else:
            # If the request was not successful, print error message
            print("Error: Unable to fetch data. Status code:", response.status_code)
            return None
    except requests.exceptions.RequestException as e:
        # Handle any exceptions that may occur during the request
        print("Error: An exception occurred during the request:", e)
        return None

all_resolved_issues = '' # we'll append each nurp regression

if __name__ == '__main__':

    # Fetch the top level regressed component + tests report:
    top_lvl_report = fetch_json_data(TEST_REPORT)

    for row in top_lvl_report['rows']:
        for col in row['columns']:
            if 'regressed_tests' not in col:
                continue
            regressed_tests = col['regressed_tests']
            for rt in regressed_tests:
                if rt["test_id"] != TEST_ID:
                    continue
                print
                print("REGRESSION: %s" % rt["test_name"])
                print
                component = rt['component']
                capability = rt['capability']
                test_name = rt['test_name']
                network = rt['network']
                upgrade = rt['upgrade']
                arch = rt['arch']
                platform = rt['platform']
                variant = rt['variant']

# TODO: These are the url params we need to add onto those in the TEST_REPORT URL when we go to get the test details (job run data):
# &component=Monitoring&capability=Other&testId=openshift-tests-upgrade:c1f54790201ec8f4241eca902f854b79&environment=ovn%20upgrade-minor%20amd64%20metal-ipi%20standard&network=ovn&upgrade=upgrade-minor&arch=amd64&platform=metal-ipi&variant=standard

                # Build out the url for full test_details api call so we can get the job runs. Adjust the endpoint, re-use the original request params, and add some more that are needed.
                test_details_url = TEST_REPORT.replace('/api/component_readiness?', '/api/component_readiness/test_details?')
                environment = '%20s'.join([network, upgrade, arch, platform, variant])
                test_details_url += '&component=%s&capability=%s&testId=%s&environment=%s&network=%s&upgrade=%s&arch=%s&platform=%s&variant=%s' % (component, capability, TEST_ID, environment, network, upgrade, arch, platform, variant)
                print("  Querying test details: %s" % test_details_url)

                # Call the function to fetch JSON data from the API
                json_data = fetch_json_data(test_details_url)

                if json_data is None:
                    print("Error: no json data returned from %s" % api_endpoint)

                golang_job_list = ''

                for job in json_data["job_stats"]:
                    if not 'sample_job_run_stats' in job:
                        continue
                    for sjr in job["sample_job_run_stats"]:
                        if sjr["test_stats"]["failure_count"] == 0:
                            continue
                        prow_url = sjr["job_url"]
                        print("  Failed job run: %s" % prow_url)

                        # TODO: it would be ideal to check the actual test failure output for some search string or regex to make sure it's
                        # the issue we're mass attributing. This however would require parsing junit XML today.

                        # grab prowjob.json from artifacts with some assumptions about paths:
                        url_tokens = prow_url.split('/')
                        job_name, job_run = url_tokens[-2], url_tokens[-1]
                        artifacts_dir = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/logs/%s/%s/" % (job_name, job_run)

                        # grab prowjob.json for the start time:
                        prow_job_json = fetch_json_data(artifacts_dir + 'prowjob.json')
                        start_time = prow_job_json["status"]["startTime"]
                        print("    Prow job start time: %s" % start_time)

                        golang_job_list += JOB_TEMPLATE % (prow_url, start_time)


                all_resolved_issues += REGRESSION_TEMPLATE % (TEST_ID, test_name, network, upgrade, arch, platform, variant, golang_job_list)

    f = open(OUTPUT_FILE, "w")
    f.write(all_resolved_issues)
    f.close()
    print("Go code written to %s, open this file and copy/paste it's contents into the appropriate file in component readiness. (i.e. resolve_4.15_issues.go)" % OUTPUT_FILE)





