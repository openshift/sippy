# Sippy API

Sippy has a REST API at `/api`. The API is used by the front-end and is also
available as a self-describing endpoint — visiting `/api` returns a JSON list of
all available endpoints with their paths, descriptions, HTTP methods, and
required capabilities.

> **Note:** The API surface changes over time as features are added.
> Use the `/api` endpoint to discover the current set of available endpoints.
> We try not to break backwards compatibility where possible.

For exact API usage, you can use your browser's web developer tools to
examine the requests we make.

## Capabilities

Not every Sippy deployment exposes every endpoint. Endpoints are gated by
server **capabilities** that depend on which backends are configured:

| Capability             | Description                                        |
|------------------------|----------------------------------------------------|
| `openshift_releases`   | OpenShift release data is loaded                   |
| `local_db`             | Local PostgreSQL database is available              |
| `build_clusters`       | Build cluster health data is available              |
| `component_readiness`  | BigQuery component readiness data is configured     |
| `write_endpoints`      | Mutating (write) endpoints are enabled              |
| `chat`                 | Sippy chat service is available                     |

Use `GET /api/capabilities` to see which capabilities the current deployment
has enabled.

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

## Common Parameters

Many endpoints accept some or all of the following query parameters:

| Parameter   | Type    | Description                                                    |
|-------------|---------|----------------------------------------------------------------|
| `release`   | String  | The OpenShift release to query (e.g., `4.18`)                  |
| `filter`    | Filter  | Filters the results (see Filtering above)                      |
| `sortField` | String  | Sort by this field                                             |
| `sort`      | String  | Sort direction: `asc` or `desc`                                |
| `limit`     | Integer | Maximum number of results to return                            |

## Endpoint Reference

The following sections group endpoints by functional area.

### Release Health

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api/health`                  | Overall release health summary from DB                     |
| `GET /api/health/build_cluster`    | Build cluster health report                                |
| `GET /api/health/build_cluster/analysis` | Build cluster health analysis                        |
| `GET /api/releases`                | Reports on releases                                        |
| `GET /api/releases/health`         | Release health report                                      |
| `GET /api/releases/tags`           | Lists release tags                                         |
| `GET /api/releases/tags/events`    | Lists events for release tags                              |
| `GET /api/releases/pull_requests`  | Pull requests included in releases                         |
| `GET /api/releases/job_runs`       | Job runs for releases                                      |
| `GET /api/releases/test_failures`  | Analysis of test failures for releases                     |
| `GET /api/variants`                | Reports on variants                                        |

### Jobs

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api/jobs`                    | Returns a list of jobs                                     |
| `GET /api/jobs/runs`               | Returns a report of job runs                               |
| `GET /api/jobs/runs/risk_analysis` | Analyzes risks of job runs                                 |
| `GET /api/jobs/runs/intervals`     | Reports intervals of job runs                              |
| `GET /api/jobs/runs/events`        | Returns Kubernetes events from job run artifacts           |
| `GET /api/jobs/analysis`           | Analyzes jobs from the database                            |
| `GET /api/jobs/details`            | Reports details of jobs                                    |
| `GET /api/jobs/bugs`               | Reports bugs related to jobs                               |
| `GET /api/jobs/artifacts`          | Queries job artifacts and their contents                   |
| `GET /api/job/run/summary`         | Raw job run summary data including test failures           |
| `GET /api/job/run/payload`         | Returns the payload a job run was using                    |
| `GET /api/job_variants`            | Reports all job variants defined in BigQuery               |

### Job Run Labels (CRUD)

| Endpoint                           | Method   | Description                                     |
|------------------------------------|----------|-------------------------------------------------|
| `/api/jobs/labels`                 | `GET`    | List all job run label definitions               |
| `/api/jobs/labels`                 | `POST`   | Create a new job run label definition            |
| `/api/jobs/labels/{id}`            | `GET`    | Get a specific job run label definition          |
| `/api/jobs/labels/{id}`            | `PUT`    | Update a job run label definition                |
| `/api/jobs/labels/{id}`            | `DELETE` | Delete a job run label definition                |

### Job Run Symptoms (CRUD)

| Endpoint                           | Method   | Description                                     |
|------------------------------------|----------|-------------------------------------------------|
| `/api/jobs/symptoms`               | `GET`    | List all job run symptom definitions             |
| `/api/jobs/symptoms`               | `POST`   | Create a new job run symptom definition          |
| `/api/jobs/symptoms/{id}`          | `GET`    | Get a specific job run symptom definition        |
| `/api/jobs/symptoms/{id}`          | `PUT`    | Update a job run symptom definition              |
| `/api/jobs/symptoms/{id}`          | `DELETE` | Delete a job run symptom definition              |

### Tests

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api/tests`                   | Reports on tests                                           |
| `GET /api/tests/v2`                | Reports on tests (v2)                                      |
| `GET /api/tests/v2/runs`           | Test runs from BigQuery with optional filtering            |
| `GET /api/tests/details`           | Details of tests                                           |
| `GET /api/tests/analysis/overall`  | Overall analysis of tests                                  |
| `GET /api/tests/analysis/variants` | Analysis of tests by variants                              |
| `GET /api/tests/analysis/jobs`     | Analysis of tests by job                                   |
| `GET /api/tests/bugs`              | Reports bugs in tests                                      |
| `GET /api/tests/outputs`           | Outputs of tests                                           |
| `GET /api/tests/recent_failures`   | Lists tests that recently started failing                  |
| `GET /api/tests/durations`         | Durations of tests                                         |
| `GET /api/tests/capabilities`      | Returns list of available test capabilities                |
| `GET /api/tests/lifecycles`        | Returns list of available test lifecycles                  |

### Install & Upgrade

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api/install`                 | Reports on installations                                   |
| `GET /api/upgrade`                 | Reports on upgrades                                        |

### Component Readiness

| Endpoint                                               | Description                                                         |
|--------------------------------------------------------|---------------------------------------------------------------------|
| `GET /api/component_readiness`                         | Reports component readiness from BigQuery                           |
| `GET /api/component_readiness/test_details`            | Test details for component readiness from BigQuery                  |
| `GET /api/component_readiness/variants`                | Test variants for component readiness from BigQuery                 |
| `GET /api/component_readiness/views`                   | Lists predefined server-side views                                  |
| `GET /api/component_readiness/regressions`             | List test regressions (supports `view` OR `release` params)         |
| `GET /api/component_readiness/regressions/{id}`        | Get a specific regression record                                    |
| `GET /api/component_readiness/regressions/{id}/matches`| List potential matching regressions for a given triage               |
| `POST /api/component_readiness/bugs`                   | Create Jira bugs from component readiness                           |

### Component Readiness Triages (CRUD)

| Endpoint                                               | Method   | Description                                          |
|--------------------------------------------------------|----------|------------------------------------------------------|
| `/api/component_readiness/triages`                     | `GET`    | List regression triage records                        |
| `/api/component_readiness/triages`                     | `POST`   | Create regression triage record                       |
| `/api/component_readiness/triages/{id}`                | `GET`    | Get specific triage record                            |
| `/api/component_readiness/triages/{id}`                | `PUT`    | Update triage record                                  |
| `/api/component_readiness/triages/{id}`                | `DELETE` | Delete triage record                                  |
| `/api/component_readiness/triages/{id}/matches`        | `GET`    | List potential matching regressions for a triage      |
| `/api/component_readiness/triages/{id}/audit`          | `GET`    | Get audit logs for a triage                           |

### Pull Requests & Repositories

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api/pull_requests`           | Reports on pull requests                                   |
| `GET /api/pull_requests/test_results` | Test failures for a specific PR from BigQuery           |
| `GET /api/repositories`            | Reports on repositories                                    |

### Payloads

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api/payloads/test_failures`  | Analysis of test failures in payloads                      |
| `GET /api/payloads/diff`           | Pull requests that differ between payloads                 |

### Incidents

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api/incidents`               | Reports incident events                                    |

### Feature Gates

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api/feature_gates`           | Feature gates and their test counts for a release          |

### Chat

These endpoints proxy to the sippy-chat service when the `chat` capability
is enabled.

| Endpoint                           | Method   | Description                                          |
|------------------------------------|----------|------------------------------------------------------|
| `/api/chat`                        | various  | HTTP proxy for REST API requests to sippy-chat       |
| `/api/chat/stream`                 | various  | WebSocket proxy for chat API requests                |
| `/api/chat/personas`               | `GET`    | List available personas                               |
| `/api/chat/models`                 | `GET`    | List available models                                 |
| `/api/chat/prompts`                | `GET`    | List available prompt templates                       |
| `/api/chat/prompts/render`         | `GET`    | Render prompt templates                               |
| `/api/chat/ratings`                | `POST`   | Create a chat rating record                           |
| `/api/chat/conversations`          | `POST`   | Create a new chat conversation                        |
| `/api/chat/conversations/{id}`     | `GET`    | Get a specific chat conversation by ID                |

### Utility

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `GET /api`                         | Returns this API endpoint listing as JSON                  |
| `GET /api/autocomplete/{field}`    | Autocompletes queries from the database                    |
| `GET /api/capabilities`            | Lists available API capabilities                           |
| `GET /api/canary`                  | Displays canary report from database                       |
| `GET /api/report_date`             | Displays report date                                       |

### MCP (Model Context Protocol)

| Endpoint                           | Description                                                |
|------------------------------------|------------------------------------------------------------|
| `/mcp/v1/`                         | Handles MCP requests                                       |
