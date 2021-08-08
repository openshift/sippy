# Sippy API

Sippy has a simple REST API at `/api`. There is an older API
available at `/json` as well, with a single endpoint that displays
multiple reports.

Note that where the responses include a top-level ID, these are synthetic
and may change across API calls. These are only used by the frontend
data tables. Other ID's when provided  such as Bugzilla, Prow ID's, etc
are accurate.

## Release Health

Endpoint: `/api/health`

Returns a summary of overall release health, including the percentage of successful runs of each,
as well as a summary of variant success rates.

<details>
<summary>Example response</summary>

```json
{
  "indicators": {
    "infrastructure": {
      "current": {
        "percentage": 88.88888888888889,
        "runs": 1998
      },
      "previous": {
        "percentage": 95.31914893617022,
        "runs": 1880
      }
    },
    "install": {
      "current": {
        "percentage": 96.53083700440529,
        "runs": 3632
      },
      "previous": {
        "percentage": 98.8409703504043,
        "runs": 3710
      }
    },
    "upgrade": {
      "current": {
        "percentage": 98.50299401197606,
        "runs": 334
      },
      "previous": {
        "percentage": 99.52941176470588,
        "runs": 425
      }
    }
  },
  "variants": {
    "current": {
      "success": 2,
      "unstable": 1,
      "failed": 17
    },
    "previous": {
      "success": 3,
      "unstable": 6,
      "failed": 11
    }
  },
  "last_updated": "2021-08-09T14:12:09.319089659Z"
}
```

</details>

### Parameters 
| Option   | Type           | Description                                                                                                              | Acceptable values                        |
|----------|----------------|--------------------------------------------------------------------------------------------------------------------------|------------------------------------------|
| release* | String         | The OpenShift release to return results from (e.g., 4.9)                                                                 | N/A                                      |

`*` indicates a required value.

### Install

| Option   | Type           | Description                                                                                                              | Acceptable values                        |
|----------|----------------|--------------------------------------------------------------------------------------------------------------------------|------------------------------------------|
| release* | String         | The OpenShift release to return results from (e.g., 4.9)                                                                 | N/A                                      |

`*` indicates a required value.

<details>
<summary>Example response</summary>

```json
{
  "column_names": [
    "All",
    "aws",
  ],
  "description": "Install Rates by Operator by Variant",
  "tests": {
    "Overall": {
      "All": {
        "id": 0,
        "name": "All",
        "current_successes": 4045,
        "current_failures": 166,
        "current_flakes": 0,
        "current_pass_percentage": 96.05794348135834,
        "current_runs": 4211,
        "previous_successes": 4260,
        "previous_failures": 54,
        "previous_flakes": 0,
        "previous_pass_percentage": 98.74826147426981,
        "previous_runs": 4314,
        "net_improvement": 0,
        "bugs": null,
        "associated_bugs": null
      },
      "aws": {
        "id": 0,
        "name": "aws",
        "current_successes": 361,
        "current_failures": 6,
        "current_flakes": 0,
        "current_pass_percentage": 98.36512261580381,
        "current_runs": 367,
        "previous_successes": 371,
        "previous_failures": 4,
        "previous_flakes": 0,
        "previous_pass_percentage": 98.93333333333332,
        "previous_runs": 375,
        "net_improvement": 0,
        "bugs": null,
        "associated_bugs": null
      }
    }
  },
  "title": "Install Rates by Operator"
}
```

</details>

### Upgrade

| Option   | Type           | Description                                                                                                              | Acceptable values                        |
|----------|----------------|--------------------------------------------------------------------------------------------------------------------------|------------------------------------------|
| release* | String         | The OpenShift release to return results from (e.g., 4.9)                                                                 | N/A                                      |

`*` indicates a required value.

## Jobs

Endpoint: `/api/jobs`

<details>
<summary>Example response</summary>

```json
[
  {
    "id": 51,
    "name": "periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade",
    "brief_name": "e2e-gcp-upgrade",
    "variants": [
      "gcp",
      "upgrade"
    ],
    "current_pass_percentage": 10.030395136778116,
    "current_projected_pass_percentage": 10.784313725490197,
    "current_runs": 329,
    "previous_pass_percentage": 35.78274760383386,
    "previous_projected_pass_percentage": 37.45819397993311,
    "previous_runs": 313,
    "net_improvement": -25.752352467055744,
    "test_grid_url": "https://testgrid.k8s.io/redhat-openshift-ocp-release-4.9-informing#periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade",
    "bugs": [],
    "associated_bugs": [
      {
        "id": 1983758,
        "status": "NEW",
        "last_change_time": "2021-07-27T16:59:31Z",
        "summary": "gcp upgrades are failing on \"Cluster frontend ingress remain available\"",
        "target_release": [
          "---"
        ],
        "component": [
          "Routing"
        ],
        "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1983758"
      }
    ]
  }
]
```

</details>

### Parameters

| Option   | Type           | Description                                                                                                              | Acceptable values                                   |
|----------|----------------|--------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------|
| release* | String         | The OpenShift release to return results from (e.g., 4.9)                                                                 | N/A                                                 |
| filterBy | String / Array | Filters the results by the specified value. Can be specified multiple times, e.g. filterBy=hasBug&filterBy=name&job=aws  | "job", "bug", "noBug", "upgrade", "runs", "variant" |
| job      | String         | Filters the results by job names only containing this value                                                              | N/A                                                 |
| variant  | String         | Filters the results for jobs only with this variant                                                                      | N/A                                                 |
| sortBy   | String         | Sorts the results                                                                                                        | "regression", "improvement"                         |
| limit    | Integer        | The maximum amount of results to return                                                                                  | N/A                                                 |
| runs     | Integer        | When specified with filterBy=runs, filter by the minimum number of runs a job should have                                | N/A                                                 |

`*` indicates a required value.

## Job Details

Endpoint: `/api/jobs/details`

A summary of runs for job(s). Results contains of the following values
for each job:

  - S success
  - F failure (e2e )
  - f failure (other tests)
  - U upgrade failure
  - I setup failure (installer)
  - N setup failure (infra)
  - n failure before setup (infra)
  - R running


<details>
<Summary>Example response</Summary>

```json
{
  "jobs": [
    {
      "name": "periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6",
      "results": [
        {
          "timestamp": 1628207039000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423429598720299008"
        },
        {
          "timestamp": 1628045973000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1422754032564310016"
        },
        {
          "timestamp": 1628198644000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423394362347229184"
        },
        {
          "timestamp": 1628485392000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1424597097709047808"
        },
        {
          "timestamp": 1628343908000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1424003666343366656"
        },
        {
          "timestamp": 1628325313000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423925674229370880"
        },
        {
          "timestamp": 1628289649000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423776089259380736"
        },
        {
          "timestamp": 1628277370000,
          "result": "S",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423724523844276224"
        },
        {
          "timestamp": 1628358891000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1424066513538650112"
        },
        {
          "timestamp": 1628190532000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423360364472438784"
        },
        {
          "timestamp": 1628274962000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423714481237659648"
        },
        {
          "timestamp": 1627391095000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420007279679246336"
        },
        {
          "timestamp": 1627473363000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420352338517823488"
        },
        {
          "timestamp": 1627617630000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420957438630170624"
        },
        {
          "timestamp": 1627515377000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420528516700573696"
        },
        {
          "timestamp": 1627396851000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420031423921786880"
        },
        {
          "timestamp": 1627363991000,
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1419893597473345536"
        }
      ]
    }
  ],
  "start": 1627317573000,
  "end": 1628508950000
}
```

</details>

### Parameters

| Option   | Type           | Description                                                                                                              | Acceptable values                        |
|----------|----------------|--------------------------------------------------------------------------------------------------------------------------|------------------------------------------|
| release* | String         | The OpenShift release to return results from (e.g., 4.9)                                                                 | N/A                                      |
| filterBy | String / Array | Filters the results by the specified value. Can be specified multiple times, e.g. filterBy=hasBug&filterBy=name&job=aws  | "job", "bug", "noBug", "upgrade", "runs" |
| job      | String         | Filters the results by jobs only containing this value                                                                   | N/A                                      |
| limit    | Integer        | The maximum amount of results to return                                                                                  | N/A                                      |

## Tests

Endpoint: `/api/tests`

### Parameters

| Option   | Type           | Description                                                                               | Acceptable values                                           |
|----------|----------------|-------------------------------------------------------------------------------------------|-------------------------------------------------------------|
| release* | String         | The OpenShift release to return results from (e.g., 4.9)                                  | N/A                                                         |
| filterBy | String / Array | Filters the results in the specified way. Can be specified multiple times.                | "test", "bug", "noBug", "install", "upgrade", "runs", "trt" |
| test     | String         | Filters the results by jobs only containing this value                                    | N/A                                                         |
| sortBy   | String         | Sorts the results                                                                         | "regression", "improvement"                                 |
| limit    | Integer        | The maximum amount of results to return                                                   | N/A                                                         |
| runs     | Integer        | When specified with filterBy=runs, filter by the minimum number of runs a job should have | N/A                                                         |

<details>
<summary>Example response</summary>

```json
[
  {
    "id": 253,
    "name": "[sig-network-edge] Cluster frontend ingress remain available",
    "current_successes": 554,
    "current_failures": 31,
    "current_flakes": 201,
    "current_pass_percentage": 94.70085470085469,
    "current_runs": 786,
    "previous_successes": 734,
    "previous_failures": 25,
    "previous_flakes": 242,
    "previous_pass_percentage": 96.70619235836627,
    "previous_runs": 1001,
    "net_improvement": -2.005337657511575,
    "bugs": [
      {
        "id": 1980141,
        "status": "POST",
        "last_change_time": "2021-08-03T14:02:12Z",
        "summary": "NetworkPolicy e2e tests are flaky in 4.9, especially in stress",
        "target_release": [
          "4.9.0"
        ],
        "component": [
          "Networking"
        ],
        "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1980141"
      },
      {
        "id": 1983829,
        "status": "NEW",
        "last_change_time": "0001-01-01T00:00:00Z",
        "summary": "ovn-kubernetes upgrade jobs are failing disruptive tests",
        "target_release": [
          "4.9.0"
        ],
        "component": [
          "Networking"
        ],
        "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1983829"
      },
      {
        "id": 1981872,
        "status": "NEW",
        "last_change_time": "2021-08-03T17:13:35Z",
        "summary": "SDN networking failures during GCP upgrades",
        "target_release": [
          "4.9.0"
        ],
        "component": [
          "Networking"
        ],
        "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1981872"
      }
    ],
    "associated_bugs": [
      {
        "id": 1983758,
        "status": "NEW",
        "last_change_time": "2021-07-27T16:59:31Z",
        "summary": "gcp upgrades are failing on \"Cluster frontend ingress remain available\"",
        "target_release": [
          "---"
        ],
        "component": [
          "Routing"
        ],
        "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1983758"
      },
      {
        "id": 1943334,
        "status": "POST",
        "last_change_time": "2021-07-23T10:58:19Z",
        "summary": "[ovnkube] node pod should taint NoSchedule on termination; clear on startup",
        "target_release": [
          "---"
        ],
        "component": [
          "Networking"
        ],
        "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1943334"
      },
      {
        "id": 1987046,
        "status": "POST",
        "last_change_time": "2021-07-30T07:02:22Z",
        "summary": "periodic ci-4.8-upgrade-from-stable-4.7-e2e-*-ovn-upgrade are permafailing on service/ingress disruption",
        "target_release": [
          "4.8.z"
        ],
        "component": [
          "Networking"
        ],
        "url": "https://bugzilla.redhat.com/show_bug.cgi?id=1987046"
      }
    ]
  }
]
```

</details>
