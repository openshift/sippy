#!/usr/bin/env python
#
# This script can be used to generate the code block for component readiness resolved incidents. (see resolved_4.15_issues.go)
#
# Fill out the variants at the start of the script per the comments. Then run the script and pipe the output to a file.
# Once finished, open that file and copy paste the resulting go code (at the end) into the resolved_issues.go file.
# Formatting may be off, let the IDE and gofmt handle that.

import os
import requests
import argparse
import uuid
from datetime import datetime,timedelta,timezone

# pip install --upgrade google-cloud-bigquery
from google.cloud import bigquery
from google.cloud.exceptions import NotFound


# The main test report, taken from top level component readiness by monitoring the dev console in firefox to fetch the API request
# made to component readiness. We'll parse through everything in this report looking for the specific test we're flagging for mass
# attribution.
# TEST_REPORT = "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2023-10-31T23:59:59Z&baseRelease=4.14&baseStartTime=2023-10-04T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-02-28T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-02-22T00:00:00Z"

# the test we're mass attributing to a known issue.
# TEST_ID = "openshift-tests:c1f54790201ec8f4241eca902f854b79"

# Template for the regression we're ignoring failed job runs for. Update the Description, Jira, and ResolutionDate below.
REGRESSION_TEMPLATE = '''
	mustAddResolvedIssue(release416, ResolvedIssue{
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

PROJECT_ID="openshift-ci-data-analysis"
DATASET_ID="fsbabcock_test"
TABLE_ID="api_resolved_incidents"

# long term
# PROJECT_ID="openshift-gce-devel"
# DATASET_ID="ci_analysis_us"
# TABLE_ID="triaged_incidents"

TABLE_KEY=f"{PROJECT_ID}.{DATASET_ID}.{TABLE_ID}"


#https://sippy.dptools.openshift.org/api/component_readiness/test_details?baseEndTime=2023-10-31T23:59:59Z&baseRelease=4.14&baseStartTime=2023-10-04T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-02-27T23:59:59Z&sampleRelease=4.15&sampleStartTime=2024-02-21T00:00:00Z&component=Monitoring&capability=Other&testId=openshift-tests-upgrade:c1f54790201ec8f4241eca902f854b79&environment=ovn%20upgrade-minor%20amd64%20metal-ipi%20standard&network=ovn&upgrade=upgrade-minor&arch=amd64&platform=metal-ipi&variant=standard

def hack_for_rfc_3339(inputTime):
    # nasty hack since python doesn't support RFC-3339
    if inputTime.endswith('Z'):
        inputTime = inputTime[:-1] + '+00:00'
    
    return inputTime

def ensure_resolved_incidents_table_exists():
 client = bigquery.Client(project=PROJECT_ID)

 try:
    client.get_table(TABLE_KEY)  # Make an API request.
    print("Table {} exists.".format(TABLE_KEY))
 except NotFound:
    print("Table {} not found.".format(TABLE_KEY))

    schema = [
        bigquery.SchemaField("release", "STRING", mode="REQUIRED"),
        bigquery.SchemaField("test_id", "STRING", mode="REQUIRED"),
        bigquery.SchemaField("test_name", "STRING", mode="REQUIRED"),
        bigquery.SchemaField("incident_id", "STRING", mode="REQUIRED"),
        bigquery.SchemaField("modified_time", "TIMESTAMP", mode="REQUIRED"),

        bigquery.SchemaField("variant", "RECORD", mode="NULLABLE", fields=[
        bigquery.SchemaField("network", "STRING", mode="NULLABLE"),
        bigquery.SchemaField("upgrade", "STRING", mode="NULLABLE"),
        bigquery.SchemaField("arch", "STRING", mode="NULLABLE"),
        bigquery.SchemaField("platform", "STRING", mode="NULLABLE"),
        bigquery.SchemaField("variant", "STRING", mode="NULLABLE")]),

        bigquery.SchemaField("issue", "RECORD", mode="REQUIRED", fields=[
        bigquery.SchemaField("type", "STRING", mode="REQUIRED"),
        bigquery.SchemaField("description", "STRING", mode="NULLABLE"),
        bigquery.SchemaField("url", "STRING", mode="NULLABLE"),
        bigquery.SchemaField("start_date", "TIMESTAMP", mode="REQUIRED"),
        bigquery.SchemaField("resolution_date", "TIMESTAMP", mode="NULLABLE")]),

        bigquery.SchemaField("job_runs", "RECORD", mode="REPEATED", fields=[
        bigquery.SchemaField("url", "STRING", mode="REQUIRED"),
        bigquery.SchemaField("start_time", "TIMESTAMP", mode="REQUIRED")]),

        bigquery.SchemaField("attributions", "RECORD", mode="REPEATED", fields=[
        bigquery.SchemaField("id", "STRING", mode="REQUIRED"),
        bigquery.SchemaField("update_time", "TIMESTAMP", mode="REQUIRED")])

    
    ]

    table = bigquery.Table(TABLE_KEY, schema=schema)
    table.time_partitioning = bigquery.TimePartitioning(type_=bigquery.TimePartitioningType.DAY, field="modified_time")

    table_ref = client.create_table(table)

    print(f"Created table {table_ref.project}.{table_ref.dataset_id}.{table_ref.table_id}")

def match_existing_incident(existing_results, new_job_runs):
    
    complete_jobs = []
    
    for job in new_job_runs:
        complete_jobs.append({"url": job["URL"], "start_time": hack_for_rfc_3339(job["StartTime"])})
    

    updateIncidentID = ""
    matchedRow = None
    if existing_results.total_rows > 0:
        for row in existing_results:
            # we need to find an overlapping job run to confirm this is a record we want to update
            for jobRun in row["job_runs"]:
                for newJobRun in new_job_runs:
                    if newJobRun["url"] == jobRun["url"]:
                            if len(updateIncidentID) > 0 and updateIncidentID != row["incident_id"]:
                                print("Error: Found job_runs overlapping with multiple IncidentIDs")
                                return "invalid", None, None
                            updateIncidentID = row["incident_id"]
                            matchedRow = row
                            print("Matched IncidentID: " + updateIncidentID)

                       

        if len(updateIncidentID) > 0 and matchedRow != None:
            
            # go through the rows again and match only that incidentID
            # get the jobs we don't already have in our list            
            for jobRun in matchedRow["job_runs"]:
                    found = False
                    for existingRun in complete_jobs:
                        if existingRun["url"] == jobRun["url"]:
                            found = True
                            break

                    if not found :        
                        complete_jobs.append({"url": jobRun["URL"], "start_time": hack_for_rfc_3339(jobRun["StartTime"])})

            return updateIncidentID, complete_jobs, matchedRow["attributions"], matchedRow["modified_time"]

    return None,None,None,None                    
                            
                   

def write_incident_record (release, test_id, test_name, network, upgrade, arch, platform, variant, job_runs, issue_type, issue_url, issue_description, modified_time, target_modified_time):

 # make sure the table we are updating exists
 ensure_resolved_incidents_table_exists()


 client = bigquery.Client(project=PROJECT_ID)

 query_job = client.query(f"SELECT * FROM `{TABLE_KEY}` WHERE release='{release}' AND test_id='{test_id}' AND variant.network='{network}' \
                          AND variant.upgrade='{upgrade}' AND variant.arch='{arch}' AND variant.platform='{platform}' AND variant.variant='{variant}' \
                          AND modified_time > TIMESTAMP_ADD(TIMESTAMP('{target_modified_time}'), INTERVAL -14 DAY) AND modified_time < TIMESTAMP('{target_modified_time}')" )
 results = query_job.result()

# do we have overlapping job runs
# if yes then update
# when we update we need to set all the known job runs
# update description, resolution date if provided
# and modified date

# get the email from the client._credentials for now
# add array record of modifications by who and when
# add the modified date to the query between target - 2 weeks and target

 updateIncidentID, complete_jobs, attributions, lastModification = match_existing_incident(results, job_runs)

 if updateIncidentID != None and complete_jobs == None:
    print("Invalid incident id match, skipping")
    return
 
 if updateIncidentID != None:
    if len(complete_jobs) == 0:
        print(f"Matching IncidentID {updateIncidentID} is missing jobs")
        return
    
    #generate common where clause that includes modifiedTime
    where = f" WHERE incident_id='{updateIncidentID}' AND modified_time=TIMESTAMP('{lastModification}')"   

    
    earliestAttribution = None
    updateAttributions = f"SELECT '{client._credentials.service_account_email}' AS id, TIMESTAMP('{modified_time}') AS update_time"
    # might use a select statement from the table instead of the static list of existing attributions, if I can make it work...
    # updateAttributions += f" UNION ALL SELECT AS STRUCT ta.ID, ta.update_time FROM {TABLE_KEY}, UNNEST(Attribution) ta {where}"
    for attribution in attributions:
        attributionID = attribution["id"]
        attributionUpdateTime = attribution["update_time"]
        updateAttributions += f" UNION ALL SELECT '{attributionID}' AS id, TIMESTAMP('{attributionUpdateTime}') AS update_time"
        
        if earliestAttribution == None or attributionUpdateTime < earliestAttribution:
            earliestAttribution = attributionUpdateTime
        

    # once the record is persisted we can update multiple times
    # but when originally written it is not available for modification right away
    # look for the first attribution time (create) and fall back to lastModification if needed
    if earliestAttribution == None:
        earliestAttribution = lastModification

    t = client.get_table(TABLE_KEY)
    if t.streaming_buffer != None:
        timeDelta = earliestAttribution + timedelta(minutes=95)
        if timeDelta > t.streaming_buffer.oldest_entry_time:
            # print(f"Oldest Streaming Buffer Item {t.streaming_buffer.oldest_entry_time}, existing record last modification time {lastModification}. Time Delta: {timeDelta}")
            print(f"Found existing incident {updateIncidentID} with a pending modification {lastModification}.  Please try the update again later")
            return


    if len(updateIncidentID) > 0:
        earliestStartTime = None
        latestStartTime = None
        updateJobRuns = ""
        for jr in complete_jobs:
            try:
                startTime = datetime.fromisoformat(hack_for_rfc_3339(jr["start_time"]))
                if earliestStartTime == None or startTime < earliestStartTime:
                    earliestStartTime = startTime
                if latestStartTime == None or latestStartTime < startTime:
                    latestStartTime = startTime

            except ValueError:
                print("Error parsing start time: " + jr["start_time"])

            
            if len(updateJobRuns) > 0 :
                updateJobRuns += " UNION ALL  "
            
            jobURL = jr["url"] 
            jobStartTime = jr["start_time"]
            updateJobRuns += f"SELECT '{jobURL}' AS url, TIMESTAMP('{jobStartTime}') AS start_time"

       

        q = f"UPDATE `{TABLE_KEY}` SET job_runs=ARRAY(SELECT AS STRUCT * FROM ({updateJobRuns})), attributions=ARRAY(SELECT AS STRUCT * FROM({updateAttributions})), modified_time='{modified_time}'"
        #q = "UPDATE `" + TABLE_KEY +"` SET JobRuns=ARRAY(SELECT AS STRUCT * FROM (" +  updateJobRuns  +")), attributions=ARRAY(SELECT AS STRUCT * FROM("+ updateAttributions + ")), modified_time='" + modified_time + "'"
        
        q += f", issue.start_date=TIMESTAMP('{earliestStartTime}'), issue.resolution_date=TIMESTAMP('{latestStartTime}')"

        if len(issue_description) > 0 :
            q += f", issue.description='{issue_description}'"

        if len(issue_type) > 0:
            q += f", issue.type='{issue_type}'"

        if len(issue_url) > 0:
            q += f", issue.url='{issue_url}'"    
        
        q += where
    

        query_job = client.query(q)
        results = query_job.result()
        print(results.num_dml_affected_rows)
 else :
    incidentID =  str(uuid.uuid4())

    save_jobs = []
            
    earliestStartTime = None
    latestStartTime = None
    for jr in job_runs:
        startTime = datetime.fromisoformat(hack_for_rfc_3339(jr["StartTime"]))
        save_jobs.append({"url": jr["URL"], "start_time": startTime})
        
        if earliestStartTime == None or startTime < earliestStartTime:
            earliestStartTime = startTime
        if latestStartTime == None or latestStartTime < startTime:
                    latestStartTime = startTime    


    if earliestStartTime == None:
        earliestStartTime = modified_time

    issue = {"type": issue_type, "url": issue_url, "start_date": earliestStartTime, "resolution_date": latestStartTime}
    if len(issue_description) > 0 :
        issue["description"] = issue_description

    row = {"release": release, "test_id": test_id, "test_name": test_name, "incident_id": incidentID, "modified_time": modified_time,
            "variant":{"network": network, "upgrade": upgrade, "arch": arch, "platform": platform, "variant": variant}, 
            "job_runs": save_jobs,
            "issue": issue,
            "attributions": [{"id": client._credentials.service_account_email, "update_time": modified_time}]
            }


    rows = [row]
        
    table_ref = client.dataset(DATASET_ID).table(TABLE_ID)
    table = client.get_table(table_ref)
    errors = client.insert_rows(table, rows) 
    
    if len(errors) > 0:
        print(f"Errors creating the incident: {errors}")
    else :
        print("Incident created")

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
job_runs = []

if __name__ == '__main__':

    parser = argparse.ArgumentParser("Generate Incident Regression Records")
    parser.add_argument("--issue-type", choices=['Infrastructure', 'Product'], help="The type of regression.", required=True)
    parser.add_argument("--issue-description", help="A short description of the regression.")
    parser.add_argument("--issue-url", help="The URL (JIRA / PR) associated with the regression.")
    parser.add_argument("--test-id", help="The internal id of the test.", required=True)
    parser.add_argument("--test-name", help="The test name.", required=True)
    parser.add_argument("--test-report-url", help="The component readiness url for the test regression.", required=True)
    parser.add_argument("--test-release", help="The release the test is running against.", required=True)
    parser.add_argument("--target-modified-time", help="The target date to query for existing record (range: target-2weeks - target).")
    
    args = parser.parse_args()

    print("\n\nTestID: " + args.test_id)
    print("\nTestReport URL: " + args.test_report_url)
    print("\nIssue Type: " + args.issue_type)
    print("\nTest Release: " + args.test_release)
    

    issue_url = ""
    if args.issue_url:
        issue_url = args.issue_url.strip("'")

    description = ""
    if args.issue_description:
        description = args.issue_description

    modified_time = datetime.now(tz=timezone.utc)
    
    target_modified_time = f"{modified_time}"
    if args.target_modified_time:
        try:
            targetModifiedTime = datetime.fromisoformat(hack_for_rfc_3339(args.target_modified_time))   
        except ValueError:
            print("Invalid target modified Date: " + args.target_modified_time)
            exit()

        # keep the string format for now...     
        target_modified_time = args.target_modified_time

    print(f"\nIssue URL: {issue_url}")
    print(f"\nIssue Description: {description}")
    print(f"\nModified Time: {modified_time}")
    print(f"\nTargetModifiedTime: {target_modified_time}")
    

    # Fetch the top level regressed component + tests report:
    top_lvl_report = fetch_json_data(args.test_report_url)

    for row in top_lvl_report['rows']:
        for col in row['columns']:
            if 'regressed_tests' not in col:
                continue
            regressed_tests = col['regressed_tests']
            for rt in regressed_tests:
                if rt["test_id"] != args.test_id:
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
                test_details_url = args.test_report_url.replace('/api/component_readiness?', '/api/component_readiness/test_details?')
                environment = '%20s'.join([network, upgrade, arch, platform, variant])
                test_details_url += '&component=%s&capability=%s&testId=%s&environment=%s&network=%s&upgrade=%s&arch=%s&platform=%s&variant=%s' % (component, capability, args.test_id, environment, network, upgrade, arch, platform, variant)
                print("  Querying test details: %s" % test_details_url)

                # Call the function to fetch JSON data from the API
                json_data = fetch_json_data(test_details_url)

                if json_data is None:
                    print("Error: no json data returned from %s" % test_details_url)

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

                        job_runs.append({"URL": prow_url, "StartTime": start_time})

                # write the record to bigquery
                write_incident_record(args.test_release, args.test_id, test_name, network, upgrade, arch, platform, variant, job_runs, args.issue_type, issue_url, description, modified_time, target_modified_time)




