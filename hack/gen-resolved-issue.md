# Installation and Setup

## Virtual Environment Setup (Optional)

For isolated dependency management, you can set up a virtual environment:
```bash
virtualenv gen-resolved-issue
source gen-resolved-issue/bin/activate
```

## Install Dependencies

Ensure you have all necessary libraries by installing them from a `requirements.txt` file:
```bash
pip3 install -r requirements.txt
```

# Usage Overview

## Command Line Arguments

Below is a list of command-line options grouped by type.

### Group Incident ID

- `--assign-incident-group-id`: Assign the specified incident-group-id common among records.  Use this to update an existing incident-group-id (e.g., to add to the existing group).  When omitted a new incident-group-id will automatically be created.

### Issue Details

- `--issue-resolution-date`: The date when the issue was fixed and no longer causing test regressions, this will clear the triaged regression icon if it is within the sample range
- `--issue-description`: A short description of the issue.
- `--issue-type`: Type of issue (options: 'Infrastructure', 'Product').
- `--issue-url`: URL associated with the issue (e.g., JIRA or PR link).

### Job Run Time Windows

If there is a specific time window to consider for JobRunStartTimes, specify the min and max, and any runs outside that window will be ignored.

- `--job-run-start-time-max`: Latest datetime to consider a job run for inclusion.
- `--job-run-start-time-min`: Earliest datetime to consider a job run for inclusion.

### Incident Data Source

If the JSON file specified in `--input-file` is complete and you don't need to use the `--test-report-url` to look up JobRuns, etc. set this flag to true.

- `--load-incidents-from-file`: Skips test report lookup and uses specified input file. Only valid with `--output-type=DB`.

### Incident Query Range

If you expect an existing incident to exist, but it hasn't been updated within the last two weeks, specify a target modified time for the match incident search range to use.

- `--target-modified-time`: Specifies the target date for querying existing records (format: target-2weeks to target).
- `--target-build-cluster`: Specifies the target build cluster a job was run on.  Generally used for cluster specific infrastructure failures.
- `--file-matches`: Specifies a set of files to check for specific regex matches within to identify a matching failure.  
  
  While possible to pass the expected JSON as an argument, consider using JSON --input-file and filling in the Arguments - FileMatches section.
This can be used with the TestId wildcard (*) to match all regressed tests returned from the TestReportURL which have JobRuns with a matching FileMatch.  In this example a second JSON file is created to be reviewed prior to persisting to the DB. 
```json
    {
    "Arguments": {
        "TestRelease": "4.17",
        "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2024-06-27T23:59:59Z&baseRelease=4.16&baseStartTime=2024-05-31T00:00:00Z&columnGroupBy=Platform,Architecture,Network&confidence=95&dbGroupBy=Platform,Architecture,Network,Topology,FeatureSet,Upgrade,Suite,Installer&ignoreDisruption=true&ignoreMissing=false&includeVariant=Architecture:amd64&includeVariant=FeatureSet:default&includeVariant=Installer:ipi&includeVariant=Installer:upi&includeVariant=Owner:eng&includeVariant=Platform:metal&includeVariant=Topology:ha&includeVariant=Network:ovn&minFail=3&pity=5&sampleEndTime=2024-07-25T23:59:59Z&sampleRelease=4.17&sampleStartTime=2024-07-20T00:00:00Z",
        "IssueDescription": "Triage metal resource issues",
        "IssueType": "Infrastructure",
        "IssueURL": "",
        "OutputFile": "component-readiness-triage/triage/active/metal_resource_review_4_17_test_regressions.json",
        "FileMatches": {
            "MatchDefinitions": {
                "MatchGate": "OR",
                "Files": [
                    {
                        "FilePath": "artifacts/e2e-metal-ipi-ovn-bm-upgrade/gather-extra/artifacts/machines.json",
                        "ContentType": "application/json",
                        "MatchGate": "AND",
                        "Matches": [
                            "errorReason[\\\":\\s]*InsufficientResources",
                            "\"phase\": \"Provisioning\"",
                            "\"errorMessage\": \"No available BareMetalHost found\""
                        ]
                    },
                    {
                        "FilePath": "build-log.txt",
                        "ContentType": "text/plain",
                        "MatchGate": "AND",
                        "Matches": [
                            "<==== OFCIR ERROR RESPONSE BODY =====",
                            "^No available resource found"
                        ]
                    },
                    {
                        "FilePath": "artifacts/e2e-metal-ipi-ovn-bm/gather-extra/artifacts/machines.json",
                        "ContentType": "application/json",
                        "MatchGate": "AND",
                        "Matches": [
                            "\"errorMessage\": \"No available BareMetalHost found\"",
                            "\"errorReason\": \"InsufficientResources\""
                        ]
                    },
                    {
                        "FilePath": "artifacts/e2e-metal-ipi-ovn-bm-upgrade/gather-extra/artifacts/machines.json",
                        "ContentType": "application/json",
                        "MatchGate": "AND",
                        "Matches": [
                            "\"message\": \"Instance has not been created\"",
                            "\"reason\": \"InstanceNotCreated\""
                        ]
                    }
                ]
            }
        },
        "IncidentGroupId": "25140332-b6da-45ed-a3f6-8456182e4df2"
    },
    "Tests": [
        {
            "TestId": "*"
        }
    ]
}
```

### Test Identification

- `--test-id`: Internal ID of the test to match.
- `--test-release`: Release version the test is running against.
- `--test-report-url`: API URL from component readiness that contains regression results (obtain this from the link on the lower right corner of a component readiness page).

### Output Control

- `--match-all-job-runs`: Only the job runs that contain failures for each of the test ids will be preserved. Only valid with `--output-type=JSON`. ['True' 'False'], default=False
- `--input-file`: Specifies a JSON file containing test criteria for creating incidents. The specificed JSON file could protentially have been produced by this tool.
- `--output-file`: Specifies the file to write JSON output to (instead of DB).
- `--output-type`: Write the incident record(s) as JSON or as DB record ['JSON', 'DB']; default='JSON'. When the output type is DB 'GOOGLE_APPLICATION_CREDENTIALS' environment variable must be specified.
- `--output-test-info-only`: When the incident record is JSON, record only the test info and not job_runs, must be specified on the command line ['True', 'False'] default=False.


### Intentional Regressions
- `--intentional-regressions`:  This will generate JSON output in the form supported for [allowed regressions](https://github.com/jupierce/enhancements/blob/openshift-tests-extension/dev-guide/component-readiness.md) at GA time as documented in the dev-guide.


## Examples

### Basic Workflow Example

This example demonstrates a basic workflow of using the script to generate an output JSON, modify it, and then persist the results to big query.

1. **Generate Initial JSON**

   Run the tool with a minimal configuration to generate initial data:

   ```bash
   ./gen-resolved-issue.py --input-file=example_input.json
   ```
   `example_input.json` content:
   ```json
    {
        "Arguments": {
            "TestRelease": "4.16",
            "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2024-02-28T23:59:59Z&baseRelease=4.15&baseStartTime=2024-02-01T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-04-05T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-03-29T00:00:00Z&component=Networking%20%2F%20cluster-network-operator&capability=Other&environment=ovn%20amd64%20metal-ipi&network=ovn&arch=amd64&platform=metal-ipi",
            "IssueDescription": "sample description for OLM Failures",
            "IssueType": "Infrastructure",
            "OutputFile": "example_output.json",
            "OutputType": "JSON"
        },
        "Tests": [
            {
                "TestId": "openshift-tests-networking:e83723b6"
            }
        ]
    }
   ```

   This will produce `example_output.json`.

2. **Review and Modify JSON**

   Manually review the `example_output.json` to ensure that the `Arguments`, `Tests`, `Variants`, and `Jobs` sections are as expected.

3. **Persist to Database**

   After reviewing and potentially modifying the output JSON, run the tool again to persist the data in bigquery:

   ```bash
   ./gen-resolved-issue.py --input-file=example_output.json --output-type=DB --load-incidents-from-file=True
   ```

4. **If you made a mistake**

   If you triaged the wrong jobs, you can always remove rows related to the `IncidentGroupId` you used from the `triaged_incidents` table in bigquery and start over.

### Using IncidentGroupId

To associate records with an existing incident group or to create a new group and maintain the association for subsequent updates:

1. **Initial Run with New Group**

   ```bash
   ./gen-resolved-issue.py --input-file=example_input.json --output-type=JSON
   cat example_output.json|grep IncidentGroupId
           "IncidentGroupId": "08406e53-435e-48d1-ae7e-e2c5a48d0398",
   ```

   Make sure to capture the `IncidentGroupId` generated in the initial run.

2. **Subsequent Run Using Existing Group**

   Use the `IncidentGroupId` from the initial output to keep the issues grouped properly:
   ```bash
   ./gen-resolved-issue.py --input-file=subsequent_input.json --assign-incident-group-id=[your-incident-group-id] --output-type=DB
   ```

### Example Input file with a list of TestIds to match
```json
{
    "Arguments": {
        "TestRelease": "4.16",
        "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2024-02-28T23:59:59Z&baseRelease=4.15&baseStartTime=2024-02-01T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-04-05T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-03-29T00:00:00Z&component=Networking%20%2F%20cluster-network-operator&capability=Other&environment=ovn%20amd64%20metal-ipi&network=ovn&arch=amd64&platform=metal-ipi",
        "IssueDescription": "CNO PODS",
        "IssueType": "Product",
        "IssueURL": "https://issues.redhat.com/browse/TRT-1555",
        "OutputFile": "test_cno_pods_output.json",
        "OutputType": "JSON",
        "IncidentGroupId": "f2dda7d3-b504-4a4f-b342-3d225a26d3e7",
        "TargetModifiedTime": "2024-04-05 20:40:48.517998+00:00",
        "JobRunStartTimeMax": "2024-04-01T00:58:50Z",
        "JobRunStartTimeMin": "2024-03-31T03:24:33Z"
    },
    "Tests": [
        { "TestId": "openshift-tests-upgrade:cc1518431a9dbf5c839f29edc86a51c0"},
        { "TestId": "openshift-tests-upgrade:07a835d0c2d8e48df6a12d8e0206d67a"},
        { "TestId": "openshift-tests-upgrade:44e2e0e6106443fef746afb65a3aaa9f"},
        { "TestId": "openshift-tests-upgrade:fbe6ebd6d5f577a21de3de9504ca242a"},
        { "TestId": "openshift-tests-upgrade:a7bca0ce3787e8bd213b32795d81bb50"},
        { "TestId": "openshift-tests-upgrade:c2f88e80fa2064a98711768d5a679735"} 
    ]
}
```

```
./gen-resolved-issue.py --input-file=test_cno_pods_input.json
```

Review `test_cno_pods_output.json`, remove the OutputType or change it to DB

Run the command to persist the entries to DB (make sure GOOGLE_APPLICATION_CREDENTIALS is defined)
```
./gen-resolved-issue.py --input-file=test_cno_pods_output.json --output-type=DB --load-incidents-from-file=True
```

### Example Pulling in Test Information Only
Start with a wildcard for the TestId and a minimal list of Variants
```json
{
    "Arguments": {
        "TestRelease": "4.15",
        "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2023-10-31T23:59:59Z&baseRelease=4.14&baseStartTime=2023-10-04T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-04-30T23:59:59Z&sampleRelease=4.15&sampleStartTime=2024-04-23T00:00:00Z",
        "IssueDescription": "CLEAR EVERYTHING",
        "IssueType": "Infrastructure",
        "IssueURL": "https://issues.redhat.com/browse/TRT-1555",
        "OutputFile": "metal_4_15_regressions.json",
        "OutputType": "JSON"
    },
    "Tests": [
        {
            "Release": "4.15",
            "TestId": "*",
            "Variants": [
                             {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        }
    ]
}
```

Run the command and specify output-test-info-only=True
This will skip adding job runs and allow the output to be used for a different release and / or time period
```
./gen-resolved-issue.py --input-file=metal_4_15_input.json --output-test-info-only=true
```

The output file can be edited and renamed (`metal_4_16_input.json `) to change the TestRelease to 4.16, update the TestReportURL and the OutputFile name.  Then rerun the command specifying the 4.16 input file and omitting the output-test-info-only flag to create the full 4.16 specific output that matches only the provided tests and variants.

```json
{
    "Arguments": {
        "TestRelease": "4.16",
        "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2024-02-28T23:59:59Z&baseRelease=4.15&baseStartTime=2024-02-01T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-04-30T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-04-23T00:00:00Z",
        "IssueDescription": "CLEAR EVERYTHING",
        "IssueType": "Infrastructure",
        "IssueURL": "https://issues.redhat.com/browse/TRT-1555",
        "OutputFile": "metal_4_16_regressions.json",
        "OutputType": "JSON",
        "IncidentGroupId": "5adf9bf4-75e8-493e-a71e-d3455d3c9f1c",
        "TargetModifiedTime": "2024-04-30 17:49:14.654868+00:00"
    },
    "Tests": [
        {
            "TestId": "Operator results:868edf33fa32b0aab132c2a676d49e3b",
            "TestName": "operator conditions cluster-autoscaler",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:ff3f4ce2ada4b853ece12306b1ef3eaf",
            "TestName": "operator conditions machine-api",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:45d55df296fbbfa7144600dce70c1182",
            "TestName": "operator conditions etcd",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:ad47fd0f8db4a5195cee022678627c9b",
            "TestName": "operator conditions image-registry",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "cluster install:0cb1bb27e418491b1ffdacab58c5c8c0",
            "TestName": "install should succeed: overall",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "cluster install:2bc0fe9de9a98831c20e569a21d7ded9",
            "TestName": "install should succeed: cluster creation",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "openshift-tests:e78644c3024c99c0a7226427e95fb8e9",
            "TestName": "[sig-arch] events should not repeat pathologically for ns/openshift-console",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:258e3ff8c9692c937596663377c10e29",
            "TestName": "operator conditions console",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:7e4c8db94dde9f957ea7d639cd29d6dd",
            "TestName": "operator conditions monitoring",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "openshift-tests:b3997eeabb330f3000872f22d6ddb618",
            "TestName": "[bz-networking][invariant] alert/OVNKubernetesResourceRetryFailure should not be at or above info",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:4b5f6af893ad5577904fbaec3254506d",
            "TestName": "operator conditions network",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:33921465a4b24f992f7e9c47b1ec9409",
            "TestName": "operator conditions ingress",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:55a75a8aa11231d0ca36a4d65644e1dd",
            "TestName": "operator conditions operator-lifecycle-manager-packageserver",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:776d244e9df7ada04b8510480fb86902",
            "TestName": "operator conditions openshift-samples",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:2bc3a57ebccf0bcb4d36d338809848c2",
            "TestName": "operator conditions kube-apiserver",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:2bc3a57ebccf0bcb4d36d338809848c2",
            "TestName": "operator conditions kube-apiserver",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "openshift-tests:b4f339155fefdaea15a98fc78a8b9177",
            "TestName": "[sig-arch] events should not repeat pathologically for ns/openshift-kube-controller-manager",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:50009b9589c6c5db8d438d0a551a4681",
            "TestName": "operator conditions kube-scheduler",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:466e1a49a33b63218495dc8201953194",
            "TestName": "operator conditions authentication",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:a4dfe6caa55e94230b4ee0ff127b6d64",
            "TestName": "operator conditions openshift-apiserver",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        }
    ]
}
```

## Triage JIRA Requests
Starting with [TRT-1657](https://issues.redhat.com/browse/TRT-1657) use the [regressedModal](https://sippy.dptools.openshift.org/sippy-ng/component_readiness/main?regressedModal=1) view to see the list of regressions and filter based on the test(s) being triaged, in this case `KubePodNotReady`.  Review the failed tests and variants, copy the testIDs to create the minimal starter file.

In Component Readiness navigate to the [Component / Capability view](https://sippy.dptools.openshift.org/sippy-ng/component_readiness/capability?baseEndTime=2024-02-28%2023%3A59%3A59&baseRelease=4.15&baseStartTime=2024-02-01%2000%3A00%3A00&capability=Alerts&component=OLM&confidence=95&excludeArches=arm64%2Cheterogeneous%2Cppc64le%2Cs390x&excludeClouds=openstack%2Cibmcloud%2Clibvirt%2Covirt%2Cunknown&excludeVariants=hypershift%2Cosd%2Cmicroshift%2Ctechpreview%2Csingle-node%2Cassisted%2Ccompact&groupBy=cloud%2Carch%2Cnetwork&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-05-08%2023%3A59%3A59&sampleRelease=4.16&sampleStartTime=2024-05-02%2000%3A00%3A00) that narrows the results down as much as possible ( you could exclude arches, platforms, networks, etc. if you needed).  From the web developer tools capture the [api URL](https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2024-02-28T23:59:59Z&baseRelease=4.15&baseStartTime=2024-02-01T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-05-08T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-05-02T00:00:00Z&component=OLM&capability=Alerts).  You should have enough data to run gen-resolved-issue.py at this point to generate a fully populated output file `trt_1657_4_16_regressions.json`.  ** You can update the TestReportURL with new time ranges and rerun using this file to pick up new failures.  If you do rerun to update incidents make sure you add the `IncidentGroupId` that is assigned the first time to keep the issues grouped properly.

```json
[
    {
    "Arguments": {
        "TestRelease": "4.16",
        "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2024-02-28T23:59:59Z&baseRelease=4.15&baseStartTime=2024-02-01T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-05-08T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-05-02T00:00:00Z&component=OLM&capability=Alerts",
        "IssueDescription": "OLM / Akamai cache problem",
        "IssueType": "Infrastructure",
        "IssueURL": "https://issues.redhat.com/browse/OCPBUGS-33052",
        "OutputFile": "/my/path/to/gen_resolved_issues/trt_1657_4_16_regressions.json"
    },
    "Tests": [
        {
            "TestId": "openshift-tests-upgrade:7dd49c583b2a86489d255d6ec262f69e"
        },
        {
            "TestId": "openshift-tests:7dd49c583b2a86489d255d6ec262f69e"
        }
    ]
}
]
```

Run your command to generate the full regressions output file:
`./hack/gen-resolved-issue.py --input-file=/my/path/to/gen_resolved_issues/trt_1657_4_16_template.json`

Review the output to confirm it looks correct, has the correct variants, etc.
Run the command with the regressions file as the input and output DB.
`./hack/gen-resolved-issue.py --input-file=my/path/to/gen_resolved_issues/trt_1657_4_16_regressions.json --output-type=DB --load-incidents-from-file=True`
Copy the `"IncidentGroupId": "xx-zz-yy",` value back to the Arguments section of your template and attach the template to the JIRA  in case you need to update again later on. 