#!/usr/bin/env python
#
# This script can be used to generate JSON output or DB records for component readiness resolved incidents. 
# By default it will output JSON to stdout.  The output can be reviewed and after verification specify --output-type=DB
# to persist the record(s).  Prior to persisting the DB record you can write the JSON to a file by specifying --output-file
# review and modify the JSON to remove any extraneous job runs, etc and then specify the file using --input-file to control
# the records created.  Additionally an input file can be hand crafted specifying multiple TestId records as well as Variant
# data and / or JobRun StartTime filtering.  See test_matches and job_matches for more details. 
#
# Specify the test-report-url by navigating through component readiness to get to the most focused view of the test(s) in question
# and copying the URL from the developer tools network view.  That URL will form the base query for looking up failed tests based
# on the test-id parameter, which can be copied from the test details report, or list of TestId records in the input-file.
# Specify the test-release, issue-type and issue-url related to the incident if applicable. 


import argparse
from datetime import datetime,timedelta,timezone
import io
import json
import os
import requests
import sys
import uuid

# pip install --upgrade google-cloud-bigquery
from google.cloud import bigquery
from google.cloud.exceptions import NotFound


# The main test report, taken from top level component readiness by monitoring the dev console in firefox to fetch the API request
# made to component readiness. We'll parse through everything in this report looking for the specific test we're flagging for mass
# attribution.
# TEST_REPORT = "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2023-10-31T23:59:59Z&baseRelease=4.14&baseStartTime=2023-10-04T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-02-28T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-02-22T00:00:00Z"

# the test we're mass attributing to a known issue.
# TEST_ID = "openshift-tests:c1f54790201ec8f4241eca902f854b79"


PROJECT_ID="openshift-gce-devel"
DATASET_ID="ci_analysis_us"
TABLE_ID="triaged_incidents"

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
        bigquery.SchemaField("incident_group_id", "STRING", mode="NULLABLE"),
        bigquery.SchemaField("modified_time", "TIMESTAMP", mode="REQUIRED"),

        bigquery.SchemaField("variants", "RECORD", mode="REPEATED", fields=[
        bigquery.SchemaField("key", "STRING", mode="REQUIRED"),
        bigquery.SchemaField("value", "STRING", mode="REQUIRED"),]),

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

# For any existing_results passed in look to see if either the issue_url matches
# or there is at least 1 job run in common. If so we will return the record so it
# can be updated with any new job runs instead of creating a new incident
def match_existing_incident(variants, issue_type, issue_url, existing_results, new_job_runs):
    
    complete_jobs = []
    
    for job in new_job_runs:
        complete_jobs.append({"url": job["URL"], "start_time": hack_for_rfc_3339(job["StartTime"])})
    

    vKey = key_from_variants(variants)
    updateIncidentID = ""
    matchedRow = None
    if existing_results.total_rows > 0:
        for row in existing_results:

            # do we match all variants?  If not then continue
            rowKey = key_from_variants(row["variants"])

            if rowKey != vKey:
                continue
            

            existing_issue = row["issue"]
            # issue type is required but we have to check to see if the existing issue has a url or not
            if len(issue_url) > 0 and "url" in existing_issue and issue_url == existing_issue["url"] and issue_type == existing_issue["type"] :
                if len(updateIncidentID) > 0 and updateIncidentID != row["incident_id"]:
                    print("Error: Found multiple matching incidents")
                    return "invalid", None, None
                updateIncidentID = row["incident_id"]
                matchedRow = row
                print("Matched IncidentID: " + updateIncidentID)
            else :
                # we need to find an overlapping job run to confirm this is a record we want to update
                for jobRun in row["job_runs"]:
                    for newJobRun in new_job_runs:
                        if newJobRun["URL"] == jobRun["url"]:
                                if len(updateIncidentID) > 0 and updateIncidentID != row["incident_id"]:
                                    print("Error: Found job_runs overlapping with multiple IncidentIDs")
                                    return "invalid", None, None
                                updateIncidentID = row["incident_id"]
                                matchedRow = row
                                print("Matched IncidentID: " + updateIncidentID)
                       

        if len(updateIncidentID) > 0 and matchedRow != None:
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
                            
                   

def write_incident_record (triaged_incident, modified_time, target_modified_time):

    # make sure the table we are updating exists
    # does not attempt any updates or migration
    ensure_resolved_incidents_table_exists()


    client = bigquery.Client(project=PROJECT_ID)

    release     = triaged_incident["Release"]
    test_id     = triaged_incident["TestId"]
    test_name   = triaged_incident["TestName"]
    job_runs    = triaged_incident["JobRuns"]

    variants = triaged_incident["Variants"]

    issue_type  = triaged_incident["Issue"]["Type"]

    if "URL" in triaged_incident["Issue"]:
        issue_url   = triaged_incident["Issue"]["URL"]
    if "Description" in triaged_incident["Issue"]:
        issue_description   = triaged_incident["Issue"]["Description"]
    
    # build a query to check for exising incidents
    query_job = client.query(f"SELECT * FROM `{TABLE_KEY}` WHERE release='{release}' AND test_id='{test_id}' \
                            AND modified_time > TIMESTAMP_ADD(TIMESTAMP('{target_modified_time}'), INTERVAL -14 DAY) AND modified_time < TIMESTAMP('{target_modified_time}')" )
    results = query_job.result()

    # from our query see if we have any existing incidents that match
    # if yes then update
    # when we update we need to set all the known job runs
    # update description, resolution date if provided
    # and modified date

    # get the email from the client._credentials for now
    # add array record of modifications by who and when
    # add the modified date to the query between target - 2 weeks and target

    updateIncidentID, complete_jobs, attributions, lastModification = match_existing_incident(variants, issue_type, issue_url, results, job_runs)

    if updateIncidentID != None and complete_jobs == None:
        print("Invalid incident id match, skipping")
        return
    
    # if we have a valid updateIncidentID we will attempt to update the existing record
    if updateIncidentID != None and len(updateIncidentID) > 0:
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
                print(f"Found existing incident {updateIncidentID} with a pending modification {lastModification}.  Please try the update again later")
                return

      
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
    
    # we didn't match an existing record so create a new one
    else:

        # we might update an existing record without any new job runs
        # but if we don't have any here then bail
        if len(job_runs) == 0:
            print("Missing job runs, skipping new record")
            return

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
                "variants": variants, 
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

def validate_parameters(triage_data, issue_type, test_id):
    # currently we require a TestId for each record
    # if we are missing an IssueType then we require
    # an issue_type to have been provided via arguments and passed in
    
    valid_inputs = True
    validated_record_counts = {}
    validated_record_counts["TotalRecords"] = 0
    validated_record_counts["IssueType"] = 0
    validated_record_counts["TestId"] = 0
    if triage_data:
        for record in triage_data:
            validated_record_counts["TotalRecords"] += 1
                
            if  "Issue" in record:
                if "Type" in record["Issue"] and len(record["Issue"]["Type"]) > 0:
                     if record["Issue"]["Type"] == "Product" or record["Issue"]["Type"] == "Infrastructure":
                         validated_record_counts["IssueType"]  += 1
            if "TestId" in record and len(record["TestId"]) > 0:
                validated_record_counts["TestId"] += 1

  
    # we have to have values for issue_type and test_id
    if len(issue_type) == 0:
       if validated_record_counts["TotalRecords"] == 0 or validated_record_counts["TotalRecords"] >  validated_record_counts["IssueType"]:   
            print("Missing input parameter for Issue Type")
            valid_inputs = False
    
    # at a minimum we have to have a TestId for each record in triage_data
    if (validated_record_counts["TotalRecords"] == 0 and len(test_id) == 0) or (validated_record_counts["TotalRecords"] > 0 and validated_record_counts["TotalRecords"] >  validated_record_counts["TestId"]):
        print("Missing input parameter for Test Id")
        valid_inputs = False
    
    if not valid_inputs:
        exit()

def variant_key(variant):
    return variant["key"]

def key_from_variants(variants):
    variants.sort(key=variant_key)
    key = ""
    for v in variants:
        if len(key) > 0:
            key += "_"
        key += v["key"] + "_" + v["value"]
    
    return key

def match_variant(variants, key, value):
    for variant in variants:
        if variant["key"] == key and variant["value"] == value:
            return True
    return False

# Compare against the target_test_id from command line or triage_data passed in via JSON file
# The JSON file allows for more test_ids to be targeted along with more refined matching
# via variants and in job matches job start_time and URL
def test_matches(triage_data, target_test_id, test_id, variants):
    
    # if we have a target via argument and it matches then we always match
    if target_test_id and target_test_id == test_id:
        return True, None
    
    # otherwise do we have an entry for this test id in triage_data
    # do we need to support multiples?  Currently first match wins
    if triage_data:
        for record in triage_data:
            if record["TestId"] == test_id:
                # does our record specify any variants
                # if so check those for matches
                matches = True
                if "Variants" in record:
                    # see if we have each key / value present in the variants passed in
                    record_variants = record["Variants"]
                    for record in record_variants:
                        if not match_variant(variants, record["key"], record["value"]):
                            matches = False
                if matches:
                    return True, record

    return False, None
    
def compare_time_is_greater(base, check):
    # check if either is a string
    # if so get time object
    # then compare
    base_time = None
    if isinstance(base,str):
        base_time = datetime.fromisoformat(hack_for_rfc_3339(base))
    else:
        base_time = base

    check_time = None
    if isinstance(check,str):
        check_time = datetime.fromisoformat(hack_for_rfc_3339(check))
    else:
        check_time = check

    return check > base  

def job_matches(record, prow_url, start_time):
    if record:        
        if "StartTimeMax" in record:
            mx = record["StartTimeMax"]
            if compare_time_is_greater(mx, start_time):
                return False
        if "StartTimeMin" in record:
            mx = record["StartTimeMin"]
            if compare_time_is_greater(start_time, mx):
                return False
        if "JobRuns" in record:
            for run in record["JobRuns"]:
                if "URL" in run:
                    if run["URL"] == prow_url:
                        return True
            # we didn't match the URL
            return False
    # either no record or no JobRuns in the record
    return True

def issue_details(record, issue_type, issue_description, issue_url):
    if record:
        if "Issue" in record:
            if "Type" in record["Issue"]:
                if len(record["Issue"]["Type"]) > 0:
                    if record["Issue"]["Type"] == "Product" or record["Issue"]["Type"] == "Infrastructure":
                        issue_type = record["Issue"]["Type"] 
            if "Description" in record["Issue"]:
                    if len(record["Issue"]["Description"]) > 0:
                        issue_description = record["Issue"]["Description"]
            if "URL" in record["Issue"]:
                    if len(record["Issue"]["URL"]) > 0:
                        issue_url = record["Issue"]["URL"]

    return issue_type, issue_description, issue_url 
                    

if __name__ == '__main__':

    parser = argparse.ArgumentParser("Generate Incident Regression Records")
    parser.add_argument("--issue-type", choices=['Infrastructure', 'Product'], help="The type of regression.")
    parser.add_argument("--issue-description", help="A short description of the regression.")
    parser.add_argument("--issue-url", help="The URL (JIRA / PR) associated with the regression.")
    parser.add_argument("--test-id", help="The internal id of the test.")
    parser.add_argument("--test-report-url", help="The component readiness url for the test regression.", required=True)
    parser.add_argument("--test-release", help="The release the test is running against.", required=True)
    parser.add_argument("--target-modified-time", help="The target date to query for existing record (range: target-2weeks - target).")
    parser.add_argument("--output-file", help="Write JSON output to the specified file instead of DB.")
    parser.add_argument("--input-file", help="JSON input file containing test criteria for creating incidents.")
    parser.add_argument("--output-type", choices=['JSON', 'DB'], help="Write the incident record(s) as JSON or as DB record", default='JSON')
    
    args = parser.parse_args()

    if args.test_id:
        print("\n\nTestID: " + args.test_id)
    
    if args.issue_type:
        print("\nIssue Type: " + args.issue_type)

    print("\nTestReport URL: " + args.test_report_url)
    print("\nTest Release: " + args.test_release)

    # if we don't have an input file then we must have issue-type, test-id and test-name
    # if we don't have issue-type or test-id or test-name then they must exist in the input-file
    triage_data = None
    if args.input_file:
        filename = args.input_file.strip("'")
        try:
            with open(filename, 'r') as incident_file:
                triage_data = json.load(incident_file)
        except Exception as e:
            print(f"Failed to load input file {filename}: {e}")
            exit()

    validate_parameters(triage_data, args.issue_type, args.test_id)
    
    if args.output_type == "DB":
        if None == os.environ.get('GOOGLE_APPLICATION_CREDENTIALS'):
            print("Missing 'GOOGLE_APPLICATION_CREDENTIALS' env variable for DB output")
            exit()

    output_file = ""
    if args.output_file:
        output_file = args.output_file.strip("'")    

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
    triaged_incidents = []

    for row in top_lvl_report['rows']:
        for col in row['columns']:
            if 'regressed_tests' not in col:
                continue
            regressed_tests = col['regressed_tests']
            for rt in regressed_tests:

                # test_ids can span variants
                # if we input a file we have to check to see if any variants are included with the test_id
                # and match only them

                # we always pull variants from the test but will match against any json input
                test_id = rt['test_id']

                variants = []
                variants.append({"key": "Network", "value": rt['network']})
                variants.append({"key": "Upgrade", "value": rt['upgrade']})
                variants.append({"key": "Architecture", "value": rt['arch']})
                variants.append({"key": "Platform", "value": rt['platform']})
                # don't see VariantVariant in https://github.com/openshift/sippy/pull/1531/files#diff-3f72919066e1ec3ae4b037dfc91c09ef6d6eac0488762ef35c5a116f73ff1637R237
                # worth review
                variants.append({"key": "Variant", "value": rt['variant']})

                # do we have an input file, if so check to see if we have an entry
                # for this test id
                # if we do see if we have a variant section
                # make sure any specified variants match
                # then check to see if we have specified jobruns
                # or if we specified JobRunStartRangeBegin / JobRunStartRangeEnd
                matches, record = test_matches(triage_data, args.test_id, test_id, variants)
                

                if not matches:
                    continue

                # if we have a record use the
                # issue type, description, url if provided
                issue_type, issue_description, issue_url = issue_details(record, args.issue_type, description, issue_url)

                print
                print("REGRESSION: %s" % rt["test_name"])
                print
                component = rt['component']
                capability = rt['capability']
                test_name = rt['test_name']
                

# TODO: These are the url params we need to add onto those in the TEST_REPORT URL when we go to get the test details (job run data):
# &component=Monitoring&capability=Other&testId=openshift-tests-upgrade:c1f54790201ec8f4241eca902f854b79&environment=ovn%20upgrade-minor%20amd64%20metal-ipi%20standard&network=ovn&upgrade=upgrade-minor&arch=amd64&platform=metal-ipi&variant=standard

                # Build out the url for full test_details api call so we can get the job runs. Adjust the endpoint, re-use the original request params, and add some more that are needed.
                test_details_url = args.test_report_url.replace('/api/component_readiness?', '/api/component_readiness/test_details?')
                environment = '%20s'.join([rt['network'], rt['upgrade'],  rt['arch'], rt['platform'], rt['variant']])
                test_details_url += '&component=%s&capability=%s&testId=%s&environment=%s&network=%s&upgrade=%s&arch=%s&platform=%s&variant=%s' % (component, capability, test_id, environment, rt['network'], rt['upgrade'], rt['arch'], rt['platform'], rt['variant'])
                print("  Querying test details: %s" % test_details_url)

                # Call the function to fetch JSON data from the API
                json_data = fetch_json_data(test_details_url)

                if json_data is None:
                    print("Error: no json data returned from %s" % test_details_url)
                
                triaged_incident = {}
                # test_release can't change since we only input one test_report_url
                # currently require it as part of input args, though it could also be pulled out of json if provided
                triaged_incident["Release"] = args.test_release
                triaged_incident["TestId"] = test_id
                triaged_incident["TestName"] = test_name
                issue = {}
                issue["Type"] = issue_type
                if len(issue_description) > 0:
                    issue["Description"] = issue_description
                if len(issue_url) > 0:
                    issue["URL"] = issue_url
                triaged_incident["Issue"] = issue
                triaged_incident["Variants"] = variants
                triaged_incident["JobRuns"] = []

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

                        if job_matches(record, prow_url, start_time):
                            triaged_incident["JobRuns"].append({"URL": prow_url, "StartTime": start_time})

                
                if args.output_type == 'JSON':
                    triaged_incidents.append(triaged_incident)
                    # add that to a list of incidents that we write in the end
                else:
                    # write the record to bigquery
                    write_incident_record(triaged_incident, modified_time, target_modified_time)

    if args.output_type == 'JSON':
        if len(output_file) > 0:      
            with open(output_file, 'w') as incident_file:
                json.dump(triaged_incidents, incident_file)
                print(f"output data to {output_file}")
        else:
            print("Specify --output-type=DB to persist record\n\n")
            json.dump(triaged_incidents, sys.stdout, indent=4)
            print("\n\n")





