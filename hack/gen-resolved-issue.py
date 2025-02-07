#!/usr/bin/env python3
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
import re
import sys
import time
import uuid

# pip install --upgrade google-cloud-bigquery
from google.cloud import bigquery
from google.cloud.exceptions import NotFound
from urllib.parse import urlsplit, urlencode, parse_qs

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

def rfc_3339_start_end_times(start, end):
   start = start.replace(hour=0, minute=0, second=0)
   s = start.strftime("%Y-%m-%dT%H:%M:%S") + 'Z'
   end = end.replace(hour=23, minute=59, second=59)
   e = end.strftime("%Y-%m-%dT%H:%M:%S") + 'Z'
   return s,e

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
        bigquery.SchemaField("completion_time", "TIMESTAMP", mode="NULLABLE"),
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
        newJob = {"url": job["URL"], "start_time": hack_for_rfc_3339(job["StartTime"])}
        if "CompletionTime" in job and job["CompletionTime"] != None:
            newJob["completion_time"] = hack_for_rfc_3339(job["CompletionTime"])

        complete_jobs.append(newJob)


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
                    print("Error: Found multiple matching incidents: " + updateIncidentID + ", " + row["incident_id"])
                    return "invalid", None, None, None
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
                                    return "invalid", None, None, None
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

                    if not found:
                        st = jobRun["start_time"]
                        ct = None
                        if "completion_time" in jobRun and jobRun["completion_time"] != None:
                            ct = jobRun["completion_time"]
                            ct = hack_for_rfc_3339(f"{ct}")

                        complete_jobs.append({"url": jobRun["url"], "start_time":  hack_for_rfc_3339(f"{st}"), "completion_time": ct})

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
    issue_url = ""

    if "URL" in triaged_incident["Issue"]:
        issue_url   = triaged_incident["Issue"]["URL"]
    if "Description" in triaged_incident["Issue"]:
        issue_description   = triaged_incident["Issue"]["Description"]

    resolution_date = None
    validate_resolution_date = None
    if "ResolutionDate" in triaged_incident["Issue"]:
        resolution_date = triaged_incident["Issue"]["ResolutionDate"]
        validate_resolution_date = datetime.fromisoformat(hack_for_rfc_3339(resolution_date))

    # build a query to check for exising incidents
    query_job = client.query(f"SELECT * FROM `{TABLE_KEY}` WHERE release='{release}' AND test_id='{test_id}' \
                            AND modified_time > TIMESTAMP_ADD(TIMESTAMP('{target_modified_time}'), INTERVAL -14 DAY) AND modified_time < TIMESTAMP_ADD(TIMESTAMP('{target_modified_time}'), INTERVAL 1 DAY)" )
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
        updateJobRuns = ""
        for jr in complete_jobs:
            try:
                startTime = datetime.fromisoformat(hack_for_rfc_3339(jr["start_time"]))

                if validate_resolution_date != None and startTime > validate_resolution_date:
                    continue

                if earliestStartTime == None or startTime < earliestStartTime:
                    earliestStartTime = startTime

            except ValueError:
                print("Error parsing start time: " + jr["start_time"])


            if len(updateJobRuns) > 0 :
                updateJobRuns += " UNION ALL  "

            jobURL = jr["url"]
            jobStartTime = jr["start_time"]
            updateJobRuns += f"SELECT '{jobURL}' AS url, TIMESTAMP('{jobStartTime}') AS start_time"

            if jr["completion_time"] != None:
                jobCompletionTime = jr["completion_time"]
                updateJobRuns += f", TIMESTAMP('{jobCompletionTime}') AS completion_time"

        q = f"UPDATE `{TABLE_KEY}` SET job_runs=ARRAY(SELECT AS STRUCT * FROM ({updateJobRuns})), attributions=ARRAY(SELECT AS STRUCT * FROM({updateAttributions})), modified_time='{modified_time}'"

        q += f", issue.start_date=TIMESTAMP('{earliestStartTime}')"
        if resolution_date != None:
            q += f", issue.resolution_date=TIMESTAMP('{resolution_date}')"
        else:
            q += f", issue.resolution_date=NULL"

        if len(issue_description) > 0 :
            q += f", issue.description='{issue_description}'"

        if len(issue_type) > 0:
            q += f", issue.type='{issue_type}'"

        if len(issue_url) > 0:
            q += f", issue.url='{issue_url}'"

        if triaged_incident["GroupId"] != None:
            incident_group_id = triaged_incident["GroupId"]
            q += f", incident_group_id='{incident_group_id}'"

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
        for jr in job_runs:
            startTime = datetime.fromisoformat(hack_for_rfc_3339(jr["StartTime"]))

            completionTime = None
            if "CompletionTime" in jr and jr["CompletionTime"] != None:
                completionTime = jr["CompletionTime"]
                completionTime = datetime.fromisoformat(hack_for_rfc_3339(completionTime))

            if validate_resolution_date != None and startTime > validate_resolution_date:
                continue

            save_jobs.append({"url": jr["URL"], "start_time": startTime, "completion_time": completionTime})

            if earliestStartTime == None or startTime < earliestStartTime:
                earliestStartTime = startTime


        if earliestStartTime == None:
            earliestStartTime = modified_time

        issue = {"type": issue_type, "start_date": earliestStartTime, "resolution_date": resolution_date}
        if len(issue_url) > 0:
             issue["url"] = issue_url

        if len(issue_description) > 0 :
            issue["description"] = issue_description

        row = {"release": release, "test_id": test_id, "test_name": test_name, "incident_id": incidentID, "modified_time": modified_time,
                "variants": variants,
                "job_runs": save_jobs,
                "issue": issue,
                "attributions": [{"id": client._credentials.service_account_email, "update_time": modified_time}]
                }

        if triaged_incident["GroupId"] != None:
            row["incident_group_id"] = triaged_incident["GroupId"]

        rows = [row]
        table_ref = client.dataset(DATASET_ID).table(TABLE_ID)
        table = client.get_table(table_ref)
        errors = client.insert_rows(table, rows)

        if len(errors) > 0:
            print(f"Errors creating the incident: {errors}")
        else :
            print("Incident created")

def files_match(artifacts_dir, file_matches):
    # file matches will be a relative filePath with a list of regexes to compare
    # may have limitations for large files...
    if not file_matches or not file_matches["MatchDefinitions"]:
        return True

    match_definitions = file_matches["MatchDefinitions"]
    and_gate = False
    if "MatchGate" in match_definitions:
        gate = match_definitions["MatchGate"]
        if gate.lower() == "and":
            and_gate = True


    if not match_definitions["Files"]:
         print("Error: Invalid MatchDefinitions files")

    for file in match_definitions["Files"]:
        # get the file path, prepend the artifacts_dir
        # load the file
        # search for each Match
        file_url = artifacts_dir + file["FilePath"]
        contentType = None
        if "ContentType" in file:
            contentType = file["ContentType"]

        file_and_gate = False
        if "MatchGate" in file:
            gate = file["MatchGate"]
            if gate.lower() == "and":
                file_and_gate = True

        patterns = []
        for match in file["Matches"]:
            pattern = re.compile(match)
            patterns.append(pattern)

        if not find_file_match(file_url, patterns, contentType, file_and_gate):
            # if we are and'ing all file matches (not just this file) then return false
            if and_gate:
                return False
        else:
            if not and_gate:
                return True

    # if we aren't an and gate and we didn't hit a match then return false
    # if we are and and gate and didn't hit a miss then true
    if not and_gate:
        return False
    else:
        return True

def find_file_match(file_url, patterns, contentType, and_gate):
    try:
        response = requests.get(file_url)
        # Check if the request was successful (status code 200)
        if response.status_code == 200:
            # when we request a file that doesn't exist the response is
            # html, so we want to know the expected content-type
            # to validate the response
            if contentType and contentType not in response.headers["content-type"]:
                print("Skipping response content type: " + response.headers["content-type"] )
                return False

            # if large files become an issue we can look to download them to a temp file
            # and use https://pymotw.com/3/mmap/#regular-expressions
            # if necessary
            text = response.text
            for pattern in patterns:
                if None == re.search(pattern, text):
                    if and_gate:
                        return False
                else:
                    if not and_gate:
                        return True

            # if we got here and we are and gated then we didn't have any misses
            if and_gate:
                return True
            # if we aren't and gated and we didn't get a match return false
            return False

        else:
            # If the request was not successful, print error message
            print("Error: Unable to fetch data. Status code:", response.status_code)
            return False
    except requests.exceptions.RequestException as e:
        # Handle any exceptions that may occur during the request
        print("Error: An exception occurred during the request:", e)
        return False

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

def match_test_id_or_wildcard(target_test_id, test_id):
    if target_test_id == "*":
        return True
    return test_id == target_test_id

# Compare against the target_test_id from command line or triage_data passed in via JSON file
# The JSON file allows for more test_ids to be targeted along with more refined matching
# via variants and in job matches job start_time and URL
def test_matches(triage_data, target_test_id, test_id, variants):

    # if we have a target via argument and it matches then we always match
    if target_test_id and match_test_id_or_wildcard(target_test_id, test_id):
        return True, None

    # otherwise do we have an entry for this test id in triage_data
    # do we need to support multiples?  Currently first match wins
    if triage_data:
        for record in triage_data:
            if match_test_id_or_wildcard(record["TestId"], test_id):
                # does our record specify any variants
                # if so check those for matches
                matches = True
                if "Variants" in record:
                    # see if we have each key / value present in the variants passed in
                    record_variants = record["Variants"]
                    for variant in record_variants:
                        if not match_variant(variants, variant["key"], variant["value"]):
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

def job_matches(record, prow_url, start_time, issue_resolution_date):
    if record:
        if "JobRunStartTimeMax" in record:
            mx = record["JobRunStartTimeMax"]
            if compare_time_is_greater(mx, start_time):
                return False
        if "JobRunStartTimeMin" in record:
            mx = record["JobRunStartTimeMin"]
            if compare_time_is_greater(start_time, mx):
                return False
        if issue_resolution_date != None:
            if compare_time_is_greater(issue_resolution_date, start_time):
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

def issue_details(record, issue_type, issue_description, issue_url, resolution_date):
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
            if "ResolutionDate" in  record["Issue"]:
                if len(record["Issue"]["ResolutionDate"]) > 0:
                        resolution_date = record["Issue"]["ResolutionDate"]

    return issue_type, issue_description, issue_url, resolution_date

def find_matching_job_run_ids(incidents):
    # take the first test entry and get the set of job runs
    # enumerate the remaining test entries and job runs and remove any that don't match from the first set
    # we need to get the variant list and match against the other tests that have the same variants
    # filter that list down and then update the runs for only that variant match
    # once we process a variant match, don't process again for the next test id

    processed_variant_keys = {}
    unique_testIDs = []
    updated_incidents = []

    for test in incidents:
        testID = test["TestId"]
        vKey = key_from_variants(test["Variants"])

        # if we have seen the vKey already then
        # we have gone through all of the tests to match jobs
        # for that vKey
        if vKey in processed_variant_keys:
            continue

        matchedTests = 1
        match_jobs = test["JobRuns"]
        drop_jobs = []

        for match_test in incidents:
            match_testID = match_test["TestId"]
            match_vKey = key_from_variants(match_test["Variants"])

            if match_testID not in unique_testIDs:
                unique_testIDs.append(match_testID)

            if match_testID == testID or not match_vKey == vKey:
                continue

            matchedTests += 1
            for job in match_jobs:
                match = False
                for job_run in match_test["JobRuns"]:
                    if job_run["URL"] == job["URL"]:
                        match = True

                if not match:
                    drop_jobs.append(job)

        processed_variant_keys[vKey] = matchedTests

        # we have gone through all of the tests by this point
        # and have a count of the unique ids
        # if our variant key doesn't have the same count
        # then we didn't find all of the test failures
        # in any of the runs
        if not len(unique_testIDs) == processed_variant_keys[vKey]:
            return

        keep_jobs = []
        for job in match_jobs:
            if job not in drop_jobs:
                keep_jobs.append(job)

        for match_test in incidents:
            match_vKey = key_from_variants(match_test["Variants"])

            if not match_vKey == vKey:
                continue

            match_test["JobRuns"] = keep_jobs
            updated_incidents.append(match_test)

    return updated_incidents

def triage_regressions(regressed_tests, triaged_incidents, issue_url, currrent_cell, total_cells):
    i = 0
    for rt in regressed_tests:
        i += 1

        # test_ids can span variants
        # if we input a file we have to check to see if any variants are included with the test_id
        # and match only them

        # we always pull variants from the test but will match against any json input
        test_id = rt['test_id']

        variants = []
        variants.append({"key": "Network", "value": rt['variants']['Network']})
        variants.append({"key": "Upgrade", "value": rt['variants']['Upgrade']})
        variants.append({"key": "Architecture", "value": rt['variants']['Architecture']})
        variants.append({"key": "Platform", "value": rt['variants']['Platform']})

        variants.append({"key": "FeatureSet", "value": rt['variants']['FeatureSet']})
        variants.append({"key": "Suite", "value": rt['variants']['Suite']})
        variants.append({"key": "Topology", "value": rt['variants']['Topology']})
        variants.append({"key": "Installer", "value": rt['variants']['Installer']})

        ir_variants = {"Network": rt['variants']['Network'],
                       "Upgrade": rt['variants']['Upgrade'],
                       "Architecture": rt['variants']['Architecture'],
                       "Platform": rt['variants']['Platform'],
                       "FeatureSet": rt['variants']['FeatureSet'],
                       "Suite": rt['variants']['Suite'],
                       "Topology": rt['variants']['Topology'],
                       "Installer": rt['variants']['Installer']}
        intentionalRegression = {}
        intentionalRegression["JiraComponent"] = rt['component']
        intentionalRegression["TestID"] = test_id
        intentionalRegression["TestName"] = rt["test_name"]
        intentionalRegression["JiraBug"] = "TBD BUG"
        intentionalRegression["ReasonToAllowInsteadOfFix"] = "TBD Reason"
        intentionalRegression["variant"]= {"variants": ir_variants}


        # do we have an input file, if so check to see if we have an entry
        # for this test id
        # if we do see if we have a variant section
        # make sure any specified variants match
        # then check to see if we have specified jobruns
        # or if we specified JobRunStartRangeBegin / JobRunStartRangeEnd
        matches, record = test_matches(triage_data, args.test_id, test_id, variants)

        # check for JobRunStartTimeMax and min times
        # if the record doesn't have them and we have global settings
        # then use them
        if args.job_run_start_time_max != None:
            if record == None:
                record = {}
            if "JobRunStartTimeMax" not in record:
                record["JobRunStartTimeMax"] =  args.job_run_start_time_max

        if args.job_run_start_time_min != None:
            if record == None:
                record = {}
            if "JobRunStartTimeMin" not in record:
                record["JobRunStartTimeMin"] =  args.job_run_start_time_min


        if not matches:
            continue

        # if we have a record use the
        # issue type, description, url if provided
        issue_type, issue_description, issue_url, issue_resolution_date = issue_details(record, args.issue_type, description, issue_url, args.issue_resolution_date)

        print
        print("REGRESSION: %s (%d/%d) (cell %d/%d)" % (rt["test_name"], i, len(regressed_tests), current_cell, total_cells))
        print
        component = rt['component']
        capability = rt['capability']
        test_name = rt['test_name']

        triaged_incident = {}
        triaged_incident["TestId"] = test_id
        triaged_incident["TestName"] = test_name

        if args.output_test_info_only != None and args.output_test_info_only and args.output_type == 'JSON' and not args.intentional_regressions:
            if record != None:
                if "Variants" in record:
                    triaged_incident["Variants"] = record["Variants"]

            triaged_incidents.append(triaged_incident)
            continue

            # TODO: These are the url params we need to add onto those in the TEST_REPORT URL when we go to get the test details (job run data):
    # &component=Monitoring&capability=Other&testId=openshift-tests-upgrade:c1f54790201ec8f4241eca902f854b79&environment=ovn%20upgrade-minor%20amd64%20metal-ipi%20standard&network=ovn&upgrade=upgrade-minor&arch=amd64&platform=metal-ipi&variant=standard

        # Build out the url for full test_details api call so we can get the job runs. Adjust the endpoint, re-use the original request params, and add some more that are needed.
        test_details_url = args.test_report_url.replace('/api/component_readiness?', '/api/component_readiness/test_details?')
        #DBOptions
        dbGroupBy = f"&Platform={rt['variants']['Platform']}&Architecture={rt['variants']['Architecture']}&Network={rt['variants']['Network']}&Topology={rt['variants']['Topology']}&FeatureSet={rt['variants']['FeatureSet']}&Upgrade={rt['variants']['Upgrade']}&Suite={rt['variants']['Suite']}&Installer={rt['variants']['Installer']}"

        test_details_url += '&component=%s&capability=%s&testId=%s%s' % (component, capability, test_id, dbGroupBy)
        print("  Querying test details: %s" % test_details_url)

        # Call the function to fetch JSON data from the API
        json_data = fetch_json_data(test_details_url)

        # bad response...
        if json_data is None:
            print("Error: no json data returned from %s" % test_details_url)
            # are we hammering the server? slow down for a bit...
            time.sleep(10)
            continue

        if args.intentional_regressions:
            #get the data from json_data
            # default to 95% pass rate if not
            if "base_stats" not in json_data or "sample_stats" not in json_data:
                 intentionalRegression["PreviousSuccesses"] = 95
                 intentionalRegression["PreviousFailures"] = 5
                 intentionalRegression["PreviousFlakes"] = 0
            else:
                intentionalRegression["PreviousSuccesses"] = json_data["base_stats"]["success_count"]
                intentionalRegression["PreviousFailures"] = json_data["base_stats"]["failure_count"]
                intentionalRegression["PreviousFlakes"] = json_data["base_stats"]["flake_count"]

            intentionalRegression["RegressedSuccesses"] = json_data["sample_stats"]["success_count"]
            intentionalRegression["RegressedFailures"] = json_data["sample_stats"]["failure_count"]
            intentionalRegression["RegressedFlakes"] = json_data["sample_stats"]["flake_count"]

            triaged_incidents.append(intentionalRegression)
        else:
            # test_release can't change since we only input one test_report_url
            # currently require it as part of input args, though it could also be pulled out of json if provided
            triaged_incident["Release"] = args.test_release

            triaged_incident["GroupId"] = incident_group_id
            issue = {}
            issue["Type"] = issue_type
            if len(issue_description) > 0:
                issue["Description"] = issue_description
            if len(issue_url) > 0:
                issue["URL"] = issue_url
            if issue_resolution_date != None:
                issue["ResolutionDate"] = issue_resolution_date

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

                    # TODO: it would be ideal to check the actual test failure output for some search string or regex to make sure it's
                    # the issue we're mass attributing. This however would require parsing junit XML today.

                    # grab prowjob.json from artifacts with some assumptions about paths:
                    url_tokens = prow_url.split('/')
                    job_name, job_run = url_tokens[-2], url_tokens[-1]
                    artifacts_dir = "https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/test-platform-results/logs/%s/%s/" % (job_name, job_run)

                    # grab prowjob.json for the start time:
                    prow_job_json = fetch_json_data(artifacts_dir + 'prowjob.json')
                    start_time = prow_job_json["status"]["startTime"]

                    completion_time = None
                    if "completionTime" in prow_job_json["status"]:
                        completion_time = prow_job_json["status"]["completionTime"]

                    build_cluster =  prow_job_json["spec"]["cluster"]

                    if args.target_build_cluster != None and len(args.target_build_cluster) > 0:
                        if build_cluster != args.target_build_cluster:
                            continue

                    if job_matches(record, prow_url, start_time, issue_resolution_date):
                        # do we have any file matches defined, if so validate we have matches
                        if files_match(artifacts_dir, file_matches):
                            print("  matched failed job run: %s (%s) " % (prow_url, start_time))
                            triaged_incident["JobRuns"].append({"URL": prow_url, "StartTime": start_time, "CompletionTime": completion_time})


            # don't output empty job run incidents unless we expect to have all
            # failures matched
            if len(triaged_incident["JobRuns"]) == 0 and not args.match_all_job_runs:
                continue

            if args.output_type == 'JSON':
                triaged_incidents.append(triaged_incident)
                # add that to a list of incidents that we write in the end
            else:
                # write the record to bigquery
                write_incident_record(triaged_incident, modified_time, target_modified_time)


if __name__ == '__main__':

    parser = argparse.ArgumentParser("Generate Incident Regression Records")
    # If there is a known incident-group-id specify it here
    parser.add_argument("--assign-incident-group-id", help="Assign the specified incident-group-id common among records.")

    # Issue description, type and URL
    parser.add_argument("--issue-description", help="A short description of the regression.")
    parser.add_argument("--issue-type", choices=['Infrastructure', 'Product'], help="The type of regression.")
    parser.add_argument("--issue-url", help="The URL (JIRA / PR) associated with the regression.")
    parser.add_argument("--issue-resolution-date", help="The date when the issue was fixed and no longer causing test regressions.")


    # If there is a specific time window to consider for JobRunStartTimes specify the min and max and
    # any runs outside that window will be ignored
    parser.add_argument("--job-run-start-time-max", help="The latest date time to consider a failed job run for an incident")
    parser.add_argument("--job-run-start-time-min", help="The latest date time to consider a failed job run for an incident")

    # If the JSON file specified in --input-file is complete and you don't need to use the --test-report-url
    # to look up JobRuns, etc. set this flag to true
    parser.add_argument("--load-incidents-from-file", help="Skip test report lookup and persist incidents from the specified input file, only valid with output-type DB", type=bool, default=False)

    # if you expect an existing incident but that is hasn't been updated within the last two weeks you can specify a target
    # modified time for the match incident search range to use
    parser.add_argument("--target-modified-time", help="The target date to query for existing record (range: target-2weeks - target).")

    parser.add_argument("--target-build-cluster", help="Specify a specific build cluster that the job was run from.")

    # specify the single test id to match, or use an input file for multiple tests
    parser.add_argument("--test-id", help="The internal id of the test.")
    # the release this incident is against
    parser.add_argument("--test-release", help="The release the test is running against.")
    # the url from component readiness dev tools network console that contains regression results for the test(s) specified
    parser.add_argument("--test-report-url", help="The component readiness url for the test regression.")

    parser.add_argument("--match-all-job-runs", help="Only the job runs common to all of the test ids will be preserved. Only valid with output-type == JSON.", type=bool, default=False)
    parser.add_argument("--relative-sample-times", help="Update the sample begin and end times based on the original time span but using the current date for end.", type=bool, default=False)
    parser.add_argument("--file-matches", help="JSON string listing artifact files to look for regex matches in")


    # a JSON structured file, potentially output by this tool, to be used as input for matching tests
    # creating records
    parser.add_argument("--input-file", help="JSON input file containing test criteria for creating incidents.")
    # the output file to write JSON output too
    parser.add_argument("--output-file", help="Write JSON output to the specified file instead of DB.")
    # write JSON output or persist to DB
    parser.add_argument("--output-type", choices=['JSON', 'DB'], help="Write the incident record(s) as JSON or as DB record", default='JSON')

    # capture just the test information and not job_runs or any other
    parser.add_argument("--output-test-info-only", help="When the incident record is JSON you can record only the test info and not job_runs, must be specified on the command line", type=bool, default=False)

    parser.add_argument("--intentional-regressions", type=bool, default=False)
    args = parser.parse_args()

    # if we don't have an input file then we must have issue-type, test-id and test-name
    # if we don't have issue-type or test-id or test-name then they must exist in the input-file
    triage_data_file = None
    if args.input_file:
        filename = args.input_file.strip("'")
        try:
            with open(filename, 'r') as incident_file:
                triage_data_file = json.load(incident_file)
        except Exception as e:
            print(f"Failed to load input file {filename}: {e}")
            exit()

    # always create and incident id by default but overwrite if one passed in
    incident_group_id = str(uuid.uuid4())
    if triage_data_file != None:
        if "Arguments" in triage_data_file:
            arguments = triage_data_file["Arguments"]
            if "TestRelease" in arguments and len(arguments["TestRelease"]) > 0:
                args.test_release = arguments["TestRelease"]
            if "TestReportURL" in arguments and len(arguments["TestReportURL"]) > 0:
                args.test_report_url = arguments["TestReportURL"]
            if "IssueDescription" in arguments and len(arguments["IssueDescription"]) > 0:
                args.issue_description = arguments["IssueDescription"]
            if "IssueType" in arguments and len(arguments["IssueType"]) > 0:
                args.issue_type = arguments["IssueType"]
            if "IssueURL" in arguments and len(arguments["IssueURL"]) > 0:
                args.issue_url = arguments["IssueURL"]
            if "IssueResolutionDate" in arguments and len(arguments["IssueResolutionDate"]) > 0:
                args.issue_resolution_date = arguments["IssueResolutionDate"]
            if "OutputFile" in arguments and len(arguments["OutputFile"]) > 0:
                args.output_file = arguments["OutputFile"]
            if "OutputType" in arguments and len(arguments["OutputType"]) > 0:
                args.output_type = arguments["OutputType"]
            if "TargetModifiedTime" in arguments and len(arguments["TargetModifiedTime"]) > 0:
                args.target_modified_time = arguments["TargetModifiedTime"]
            if "TargetBuildCluster" in arguments and len(arguments["TargetBuildCluster"]) > 0:
                args.target_build_cluster = arguments["TargetBuildCluster"]
            if "JobRunStartTimeMax" in arguments and len(arguments["JobRunStartTimeMax"]) > 0:
                args.job_run_start_time_max = arguments["JobRunStartTimeMax"]
            if "JobRunStartTimeMin" in arguments and len(arguments["JobRunStartTimeMin"]) > 0:
                args.job_run_start_time_min = arguments["JobRunStartTimeMin"]
            if "MatchAllJobRuns" in arguments and len(arguments["MatchAllJobRuns"]) > 0:
                args.match_all_job_runs = bool(arguments["MatchAllJobRuns"])
            if "RelativeSampleTimes" in arguments and len(arguments["RelativeSampleTimes"]) > 0:
                args.relative_sample_times = bool(arguments["RelativeSampleTimes"])
            if "FileMatches" in arguments and len(arguments["FileMatches"]) > 0:
                args.file_matches = arguments["FileMatches"]
            if "IntentionalRegressions" in arguments and len(arguments["IntentionalRegressions"]) > 0:
                args.intentional_regressions = bool(arguments["IntentionalRegressions"])

            # if we have an input file and it doesn't specify the IncidentGroupId
            # update the file with the one we are assigning
            if "IncidentGroupId" in arguments and len(arguments["IncidentGroupId"]) > 0:
                args.assign_incident_group_id = arguments["IncidentGroupId"]
            else:
                arguments["IncidentGroupId"] = incident_group_id
                filename = args.input_file.strip("'")
                with open(filename, 'w') as incident_file:
                    json.dump(triage_data_file, incident_file, indent=4)
                    print(f"Added IncidentGroupId to to {filename}")

    file_matches = None
    if args.file_matches:
        try:
            # file_matches = json.load(args.file_matches)
            file_matches = args.file_matches
        except Exception as e:
            print(f"Failed to load json file matches: {e}")
            exit()

    if args.test_report_url == None or len(args.test_report_url) == 0:
        # if we are loading incidents from a file we don't need the URL
        if args.load_incidents_from_file == False:
            print(f"test-report-url is required")
            exit()
    if args.test_release == None or len(args.test_release) == 0:
        print(f"test-release is required")
        exit()

    triage_data = None
    if triage_data_file != None and "Tests" in triage_data_file:
        triage_data = triage_data_file["Tests"]

    validate_parameters(triage_data, args.issue_type, args.test_id)

    modified_time = datetime.now(tz=timezone.utc)
    if args.relative_sample_times:
        if not args.test_report_url == None:
            o = urlsplit(args.test_report_url)
            params = parse_qs(o.query)
            startTime = None
            endTime = None
            if "sampleEndTime" in params:
                print("Original sample end time: " + params["sampleEndTime"][0])
                endTime = datetime.fromisoformat(hack_for_rfc_3339(params["sampleEndTime"][0]))
            if "sampleStartTime" in params:
                print("Original sample start time: " + params["sampleStartTime"][0])
                startTime = datetime.fromisoformat(hack_for_rfc_3339(params["sampleStartTime"][0]))

            if not startTime == None and not endTime == None:
                diff = endTime - startTime
                newSampleEndTime = modified_time.replace(hour=23, minute=59, second=59)
                newSampleStartTime = newSampleEndTime - diff
                newSampleStartTimeParam, newSampleEndTimeParam = rfc_3339_start_end_times(newSampleStartTime, newSampleEndTime)
                params["sampleStartTime"][0] = newSampleStartTimeParam
                params["sampleEndTime"][0] = newSampleEndTimeParam

                print("Updated sample end time: " + params["sampleEndTime"][0])
                print("Updated sample start time: " + params["sampleStartTime"][0])

                query_new = urlencode(params, doseq=True)
                parsed=o._replace(query=query_new)
                url_new = (parsed.geturl())
                args.test_report_url = url_new

    # if an incident id was passed in use it
    if args.assign_incident_group_id:
        incident_group_id = args.assign_incident_group_id

    if args.test_id:
        print("\n\nTestID: " + args.test_id)

    if args.issue_type:
        print("\nIssue Type: " + args.issue_type)

    print("\nTestReport URL: " + args.test_report_url)
    print("\nTest Release: " + args.test_release)

    if args.output_type == "DB":
        if None == os.environ.get('GOOGLE_APPLICATION_CREDENTIALS'):
            print("Missing 'GOOGLE_APPLICATION_CREDENTIALS' env variable for DB output")
            exit()
        if args.match_all_job_runs:
            print("Invalid specification of match-all-job-runs when output-type is DB")
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

    target_modified_time = f"{modified_time}"
    if args.target_modified_time:
        try:
            targetModifiedTime = datetime.fromisoformat(hack_for_rfc_3339(args.target_modified_time))
        except ValueError:
            print("Invalid target modified Date: " + args.target_modified_time)
            exit()

        # keep the string format for now...
        target_modified_time = args.target_modified_time

    issue_resolution_date = ""
    if args.issue_resolution_date:
        try:
            issue_resolution_date = datetime.fromisoformat(hack_for_rfc_3339(args.issue_resolution_date))
        except ValueError:
            print("Invalid issue resolution date: " + args.issue_resolution_date)
            exit()

    print(f"\nIssue URL: {issue_url}")
    print(f"\nIssue Description: {description}")
    print(f"\nIssue Resolution Date: {issue_resolution_date}")
    print(f"\nModified Time: {modified_time}")
    print(f"\nTargetModifiedTime: {target_modified_time}")


    if args.load_incidents_from_file:
        if args.output_type == 'DB':
            for triaged_incident in triage_data:
                 write_incident_record(triaged_incident, modified_time, target_modified_time)
        else:
            print("Invalid output type for load-incidents-from-file")
            exit()
    else :
        # Fetch the top level regressed component + tests report:
        top_lvl_report = fetch_json_data(args.test_report_url)
        triaged_incidents = []

        total_regressed_cells = 0
        for row in top_lvl_report['rows']:
            for col in row['columns']:
                if 'regressed_tests' in col or 'triaged_incidents' in col:
                    total_regressed_cells += 1
        print("Scanning %d total regressed cells" % total_regressed_cells)

        current_cell = 0
        for row in top_lvl_report['rows']:
            for col in row['columns']:
                if 'regressed_tests' in col or 'triaged_incidents' in col:
                    current_cell += 1
                if 'regressed_tests' in col:
                    regressed_tests = col['regressed_tests']
                    triage_regressions(regressed_tests, triaged_incidents, issue_url, current_cell, total_regressed_cells)
                if 'triaged_incidents' in col:
                    regressed_tests = col['triaged_incidents']
                    triage_regressions(regressed_tests, triaged_incidents, issue_url, current_cell, total_regressed_cells)


    if args.output_type == 'JSON':
        if args.intentional_regressions:
            triage_output = triaged_incidents
        else:
            if args.match_all_job_runs:
                triaged_incidents = find_matching_job_run_ids(triaged_incidents)
                if triaged_incidents == NotFound:
                    print("Error: no remaining triaged incidents after matching job runs")
                    exit()

            triage_output = {"Arguments": {"TestRelease": args.test_release, "TestReportURL": args.test_report_url,
                                        "IssueDescription": description, "IssueType": args.issue_type,
                                        "IssueURL": issue_url, "OutputFile": output_file,
                                        "IncidentGroupId": incident_group_id,
                                        "TargetModifiedTime": target_modified_time},
                                        "Tests": triaged_incidents}
            if  args.job_run_start_time_max != None:
                triage_output["Arguments"]["JobRunStartTimeMax"] = args.job_run_start_time_max
            if  args.job_run_start_time_min != None:
                triage_output["Arguments"]["JobRunStartTimeMin"] = args.job_run_start_time_min
            if args.issue_resolution_date != None:
                triage_output["Arguments"]["IssueResolutionDate"] = args.issue_resolution_date
            if args.target_build_cluster != None:
                triage_output["Arguments"]["TargetBuildCluster"] = args.target_build_cluster

        if len(output_file) > 0:
            with open(output_file, 'w') as incident_file:
                json.dump(triage_output, incident_file, indent=4)
                print(f"output data to {output_file}")
        else:
            print("Specify --output-type=DB to persist record\n\n")
            json.dump(triage_output, sys.stdout, indent=4)
            print("\n\n")





