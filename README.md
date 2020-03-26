# ci-investigator

A tool to process the job results from https://testgrid.k8s.io/

Analyzes any job with a status of `FLAKY` or `FAILING` as reported on the following dashboards:

```
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.3-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.2-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.1-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.3-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.2-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.1-blocking
```

For each job, finds the top N test failures as sorted by other flakiness or failures, based on command line arguments.

For each top failing test, attempts to find a bugzilla that includes the test name.

Final report is sorted by number of jobs that have experienced the particular test failure.

Data reported, for each top failing test:

* The test name
* The sig that owns the test
* The number of jobs that are reporting this as a top failure/flake
* The names of the jobs that reported it as a top failure/flake
* The associated bug, if one was found

Example output:
```
[
  {
    "testName": "operator.Create the release image containing all images built by this job",
    "owningSig": "sig-unknown",
    "jobsFailedCount": 5,
    "jobsFailedNames": [
      "release-openshift-origin-installer-e2e-aws-serial-4.5",
      "release-openshift-ocp-installer-e2e-aws-serial-4.5",
      "release-openshift-ocp-installer-e2e-azure-serial-4.3",
      "release-openshift-ocp-installer-e2e-aws-upi-4.2",
      "release-openshift-origin-installer-e2e-aws-4.2"
    ],
    "associatedBug": "no bug found"
  }
]
```
