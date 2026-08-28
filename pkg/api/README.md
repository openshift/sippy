# Sippy API

Sippy has a simple REST API at `/api`. The API is used by the front-end.
The docs here may not be fully up-to-date, although we do try not to
break backwards compatability where possible.

For exact API usage, you can use your browser's web developer tools to
examine the requests we make.

## Filtering and sorting

### Filtering

The API's that support filtering, as indicated in their docs below, use a filtering format as follows. The format is
similar to the filtering options used by Material UI's data tables internally.

An individual filter is JSON, in the following format:

```json
{
  "columnName": "name",
  "operatorValue": "contains",
  "value": "aws"
}
```

- String operators are: contains, starts with, ends with, equals, is empty, is not empty.
- Numerical operators are: =, !=, <, <=, >, >=
- Array operators are: contains

An optional 'not' field may be specified which inverts the operator. For example, the below filter means name does not
contain aws:

```json
{
  "columnName": "name",
  "not": true,
  "operatorValue": "contains",
  "value": "aws"
}
```

A composed filter consists of one or more filters, along with a link operator. A link operator is either `and` or `or`.

Example:

```json
{
  "linkOperator": "and",
  "items": [
    {
      "columnName": "name",
      "operatorValue": "contains",
      "value": "aws"
    },
    {
      "columnName": "name",
      "not": true,
      "operatorValue": "contains",
      "value": "upgrade"
    }
  ]
}
```

The filter should be URI encoded json in the `filter` parameter.

### Sorting

You may sort results by any sortable field in the item by specifying `sortField`, as well `sort` with the value
`asc` or `desc`.

## Release Health

Endpoint: `/api/health`

Returns a summary of overall release health, including the percentage of successful runs of each, as well as a summary
of variant success rates.

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
    "aws"
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

### Import one completed Prow job run

`POST /api/jobs/runs/import` imports a single completed run directly from its
GCS artifacts. The route is enabled only when the server has both the
`local_db` and `write_endpoints` capabilities. Production authentication is
provided by the deployment OAuth proxy, and the handler additionally requires
a nonempty `X-Forwarded-User` header before it invokes the importer.

The JSON request body is limited to 1 MiB, must contain exactly one JSON value,
and rejects unknown fields. Its public fields are exactly:

```json
{
  "prow_job_run_id": "1234567890",
  "bucket": "test-platform-results",
  "job_prefix": "logs/periodic-ci-example/1234567890"
}
```

The bucket must equal the Sippy server's configured Google storage bucket.
`job_prefix` must be a canonical top-level Prow job path ending in the requested
run ID. Accepted layouts are `logs/<job>/<id>`, legacy
`pr-logs/pull/<pull-number>/<job>/<id>`, and current
`pr-logs/pull/<org>_<repo>/<pull-number>/<job>/<id>`. Nested artifact or step
paths are rejected even if their last component is the run ID. When present,
the GCS location in `prowjob.json`'s `status.url` must agree with the request.

The importer first checks PostgreSQL for a real committed parent run; a matching
orphan ID-map row is not enough. A committed duplicate returns without reading
GCS or BigQuery. Otherwise it reads `<job_prefix>/prowjob.json`, determines the
duration, reads and combines every matching `**junit**.xml` object, resolves the
existing ProwJob definition (including its release and variants), reads labels,
prepares partitions, and finally performs the permanent write. The start time
must not be in the future or more than 14 days old; exactly 14 days is accepted.

`status.completionTime` is authoritative when present and no completion marker
is read. When it is absent, the importer reads only the top-level
`<job_prefix>/finished.json` and uses its positive numeric Unix-seconds
`timestamp`. Duration is completion or marker time minus `status.startTime`;
request receipt time and GCS object metadata are never used. Missing, malformed,
zero, or pre-start marker timing makes the ProwJob invalid.

JUnit behavior intentionally matches the batch loader. Artifact listing or
read errors abort the import, while malformed or empty JUnit files contribute
no native test rows. Synthetic results may still be produced, and a run with
zero tests may still be imported. All files are downloaded and converted before
the database transaction.

Labels come from the authoritative BigQuery `job_labels` table using build ID
and the exact `DATE(prowjob_start)` partition; the BigQuery `jobs` candidate
table is not queried. A successful query with no row means an empty label set.
An unavailable or failed label query aborts without writing rather than silently
persisting incomplete labels. Labels are persisted unchanged, including
`InfraFailure`. Runs already carrying `InfraFailure` retain their parent and
test rows but are excluded from daily and cumulative summary deltas in both the
single-run and batch writers. Existing late-label handling continues to
subtract a run that was summarized before `InfraFailure` was applied.

Before the permanent write, the importer calls the existing partition preparer
for the resolved release from the Prow start time through two days after the
single captured current time. The 14-day validation bound limits historical
work but does not replace partition preparation. The unique parent insert
(`ON CONFLICT (id) DO NOTHING RETURNING id`) is the race-ownership gate. Only
the winner writes ID maps, annotations, pull-request associations, tests,
outputs, and summaries in one transaction; concurrent losers return a
successful idempotent no-op. Pull-request identities from `prowjob.json` are
stored without live GitHub enrichment or risk-comment side effects.

A new import returns `201 Created`; an early or concurrent duplicate returns
`200 OK`. Both use this response shape (definition-derived fields may be absent
and counts are zero for an early duplicate):

```json
{
  "prow_job_run_id": "1234567890",
  "status": "imported",
  "prow_job_name": "periodic-ci-example",
  "release": "4.20",
  "bucket": "test-platform-results",
  "job_prefix": "logs/periodic-ci-example/1234567890",
  "gcs_location": "gs://test-platform-results/logs/periodic-ci-example/1234567890",
  "junit_files": 2,
  "tests": 347,
  "links": {
    "self": "https://sippy.example/api/jobs/runs/import",
    "prow_job": "https://prow.ci.openshift.org/view/gs/..."
  }
}
```

Errors have `{"code": <HTTP status>, "message": "<detail>"}`. Statuses are
`400` for malformed requests or invalid/mismatched locations, `401` for missing
forwarded identity, `404` for a missing ProwJob definition or release, `422` for
invalid Prow metadata, age, state, or completion timing, `502` for ordinary
artifact or label-query failures, `503` for recognized unavailable,
unconfigured, unauthorized, throttled, or timed-out dependencies, and `500`
for partition or transactional persistence failures. Error details are retained
for the trusted intra-team callers behind the OAuth proxy.

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
| filter   | Filter         | Filters the results by the specified value. Can be specified multiple times, e.g. filterBy=hasBug&filterBy=name&job=aws  | See filtering                                       |
| sortField| Field name     | Sort by this field                                                                                                       |                                                     |
| sort     | asc / desc     | Sort type, ascending or descending                                                                                       | "asc" or "desc"                                     |
| limit    | Integer        | The maximum amount of results to return                                                                                  | N/A                                                 |

`*` indicates a required value.

## Job Details

Endpoint: `/api/jobs/details`

A summary of runs for job(s). Results contains of the following values for each job:

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
          "timestamp": "2021-08-06T03:43:59Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423429598720299008"
        },
        {
          "timestamp": "2021-08-04T03:59:33Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1422754032564310016"
        },
        {
          "timestamp": "2021-08-05T21:24:04Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423394362347229184"
        },
        {
          "timestamp": "2021-08-09T05:03:12Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1424597097709047808"
        },
        {
          "timestamp": "2021-08-07T14:25:08Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1424003666343366656"
        },
        {
          "timestamp": "2021-08-07T09:15:13Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423925674229370880"
        },
        {
          "timestamp": "2021-08-06T23:20:49Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423776089259380736"
        },
        {
          "timestamp": "2021-08-06T19:56:10Z",
          "result": "S",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423724523844276224"
        },
        {
          "timestamp": "2021-08-07T18:34:51Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1424066513538650112"
        },
        {
          "timestamp": "2021-08-05T19:08:52Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423360364472438784"
        },
        {
          "timestamp": "2021-08-06T19:16:02Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1423714481237659648"
        },
        {
          "timestamp": "2021-07-27T13:24:55Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420007279679246336"
        },
        {
          "timestamp": "2021-07-28T12:16:03Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420352338517823488"
        },
        {
          "timestamp": "2021-07-30T04:20:30Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420957438630170624"
        },
        {
          "timestamp": "2021-07-29T00:56:17Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420528516700573696"
        },
        {
          "timestamp": "2021-07-27T15:00:51Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1420031423921786880"
        },
        {
          "timestamp": "2021-07-27T05:53:11Z",
          "result": "F",
          "url": "https://prow.ci.openshift.org/view/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-e2e-metal-ipi-ovn-ipv6/1419893597473345536"
        }
      ]
    }
  ],
  "start": "2021-07-26",
  "end": "2021-08-09"
}
```

</details>

### Parameters

| Option   | Type           | Description                                                                                                              | Acceptable values                        |
|----------|----------------|--------------------------------------------------------------------------------------------------------------------------|------------------------------------------|
| release* | String         | The OpenShift release to return results from (e.g., 4.9)                                                                 | N/A                                      |
| job      | String         | Return only jobs containing only containing this value in their name                                                     | N/A                                      |
| limit    | Integer        | The maximum amount of results to return                                                                                  | N/A                                      |

## Re-evaluate Job Run Symptoms

Endpoint: `POST /api/jobs/runs/reevaluate`

Re-runs all symptom definitions against the artifacts for specified job runs and updates
BigQuery, GCS, and PostgreSQL with the results. Requires `--enable-write-endpoints`.

### Request

```json
{
  "prow_job_build_ids": ["1234567890", "0987654321"],
  "dry_run": false
}
```

Maximum 50 job run IDs per request. IDs must be numeric strings.

### Response (200 OK)

```json
{
  "results": [
    {
      "prow_job_build_id": "1234567890",
      "status": "success",
      "symptoms_evaluated": 42,
      "symptoms_matched": ["CreatePodSandboxForPodFailedInJournal"],
      "labels_applied": ["InfraFailure", "NodeProblem"],
      "bq_entries_written": 2,
      "gcs_artifacts_written": 2,
      "postgres_updated": true,
      "links": {
        "job_run": "https://prow.ci.openshift.org/view/gs/test-platform-results/logs/.../1234567890",
        "symptom:CreatePodSandboxForPodFailedInJournal": "http://localhost:8080/api/jobs/symptoms/SomeSymptom"
      }
    },
    {
      "prow_job_build_id": "0987654321",
      "status": "missing_error",
      "error": "job run 0987654321 not found in database"
    }
  ],
  "links": {
    "self": "http://localhost:8080/api/jobs/runs/reevaluate"
  }
}
```

### Status Values

- `success` - re-evaluation completed and all backends updated.
- `missing_error` - the job run ID was not found in the database.
- `eval_error` - artifact scanning failed (timeout, GCS error, database error).
- `rewrite_error` - scanning succeeded but writing to BQ/GCS/PostgreSQL failed.

## Tests

Endpoint: `/api/tests`

### Parameters

| Option   | Type           | Description                                                                               | Acceptable values                                   |
|----------|----------------|-------------------------------------------------------------------------------------------|-----------------------------------------------------|
| release* | String         | The OpenShift release to return results from (e.g., 4.9)                                  | N/A                                                 |
| filter   | Filter         | Filters the results by the specified value.                                               | See filtering                                       |
| sortField| Field name     | Sort by this field                                                                        |                                                     |
| sort     | asc / desc     | Sort type, ascending or descending                                                        | "asc" or "desc"                                     |
| limit    | Integer        | The maximum amount of results to return                                                   | N/A                                                 |

`filter` supports a `lifecycle` field (`equals` or `!=` operators only; other operators return a
400) to restrict results to a test lifecycle (`blocking` or `informing`). It narrows which
underlying test runs are aggregated; it is not returned as a field on results, and
blocking/informing runs for the same test are combined into a single row when no lifecycle filter
is applied. This filter is only supported against the Postgres-backed report (`/api/tests`); using
it against `/api/tests/v2` (BigQuery) returns a 400, since the underlying BigQuery comparison
tables don't carry a lifecycle column.

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

## Feature Gates

### List Feature Gates

Endpoint: `/api/feature_gates`

Returns all feature gates and their test counts for a release. Each gate includes
lightweight HATEOAS links (`ui_detail` and `api_detail`) for navigation.

| Option   | Type   | Description                                              |
|----------|--------|----------------------------------------------------------|
| release* | String | The OpenShift release to return results from (e.g., 5.0) |
| filter   | Filter | Filters the results. See filtering above.                |

### Feature Gate Detail

Endpoint: `/api/feature_gates/{feature_gate}`

Returns a single feature gate with full HATEOAS links for test queries
(`gate_tests`, `install_tests`, `gate_job_tests`, `ui_detail`).
The `install_tests` link is only present for gates whose name contains "Install".

The response includes a `promotion` object with promotion readiness data:
per-variant test pass rates, overall sufficiency, warnings, and errors.
The promotion evaluation is computed from the same data that the `gate_tests`
and `install_tests` HATEOAS links point to. Both the links and the promotion
logic use canonical filter definitions from
`pkg/api/featuregatepromotion/filters.go`, ensuring they always stay in sync.

| Option        | Type   | Description                                              |
|---------------|--------|----------------------------------------------------------|
| release*      | String | The OpenShift release to return results from (e.g., 5.0) |
| feature_gate  | Path   | The feature gate name (in the URL path)                  |

## Component Readiness Triages

Endpoint: `GET /api/component_readiness/triages`

Lists triage records. Supports an optional `view` query parameter to filter triages
to those associated with regressions active in the specified component readiness view.
When `view` is omitted, all triages are returned (original behavior).

### Parameters

| Option | Type   | Description                                                                 | Acceptable values |
|--------|--------|-----------------------------------------------------------------------------|-------------------|
| view   | String | Filter triages to those linked to regressions active in this view (e.g., 4.18-main). Optional; omit to return all triages. | N/A               |

Endpoint: `GET /api/component_readiness/triages/{id}`

Returns a single triage record by ID.

Endpoint: `POST /api/component_readiness/triages`

Creates a new triage record.

Endpoint: `PUT /api/component_readiness/triages/{id}`

Updates an existing triage record.

Endpoint: `DELETE /api/component_readiness/triages/{id}`

Deletes a triage record.
