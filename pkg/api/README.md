# Sippy API

Sippy has a REST API at `/api`. The API is used by the front-end and is also
available for programmatic access. The docs here may not be fully up-to-date,
although we do try not to break backwards compatibility where possible.

For exact API usage, you can use your browser's web developer tools to
examine the requests we make.

> **Tip**: You can also query `/api` (GET) at runtime to get a JSON list of all
> available endpoints and their descriptions, filtered by the server's
> active capabilities.

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

## Capabilities

Not all endpoints are available on every Sippy deployment. Endpoints are gated
behind *capabilities* that the server advertises based on its configuration:

| Capability              | Description |
|-------------------------|-------------|
| `local_db`              | PostgreSQL database is available |
| `component_readiness`   | BigQuery component-readiness data is available |
| `build_cluster`         | Build cluster health data is available |
| `write_endpoints`       | Mutating (POST/PUT/DELETE) endpoints are enabled |
| `chat`                  | Sippy-chat AI service proxy is available |

Use `GET /api/capabilities` to discover which capabilities the current server
supports.

## Common Parameters

Many endpoints share the following query parameters:

| Option    | Type       | Description                                              |
|-----------|------------|----------------------------------------------------------|
| release   | String     | The OpenShift release to query (e.g. `4.17`)             |
| filter    | Filter     | URI-encoded JSON filter (see Filtering above)            |
| sortField | Field name | Sort results by this field                               |
| sort      | String     | Sort direction: `asc` or `desc`                          |
| limit     | Integer    | Maximum number of results to return                      |

`*` indicates a required value in the per-endpoint tables below.

---

## Endpoints

### Release Health

`GET /api/health` — Reports general release health from the database.

**Required capability:** `local_db`

| Option    | Type   | Description                                       |
|-----------|--------|---------------------------------------------------|
| release*  | String | The OpenShift release to return results from       |

Returns a summary of overall release health including the percentage of
successful runs for infrastructure, install, and upgrade, as well as a summary
of variant success rates.

---

### Install

`GET /api/install` — Reports on installations.

**Required capability:** `local_db`

| Option    | Type   | Description                                       |
|-----------|--------|---------------------------------------------------|
| release*  | String | The OpenShift release to return results from       |

---

### Upgrade

`GET /api/upgrade` — Reports on upgrades.

**Required capability:** `local_db`

| Option    | Type   | Description                                       |
|-----------|--------|---------------------------------------------------|
| release*  | String | The OpenShift release to return results from       |

---

### Jobs

`GET /api/jobs` — Returns a list of jobs.

**Required capability:** `local_db`

| Option    | Type       | Description                                       |
|-----------|------------|---------------------------------------------------|
| release*  | String     | The OpenShift release to return results from       |
| filter    | Filter     | Filters the results (see Filtering)                |
| sortField | Field name | Sort by this field                                 |
| sort      | String     | `asc` or `desc`                                    |
| limit     | Integer    | Maximum number of results to return                |

---

### Job Details

`GET /api/jobs/details` — Reports details of jobs.

**Required capability:** `local_db`

| Option    | Type   | Description                                       |
|-----------|--------|---------------------------------------------------|
| release*  | String | The OpenShift release to return results from       |
| job       | String | Return only jobs containing this value in their name |
| limit     | Integer | Maximum number of results to return               |

---

### Job Runs

`GET /api/jobs/runs` — Returns a report of job runs.

**Required capability:** `local_db`

---

### Job Run Risk Analysis

`GET /api/jobs/runs/risk_analysis` — Analyzes risks of job runs.

**Required capability:** `local_db`

---

### Job Run Intervals

`GET /api/jobs/runs/intervals` — Reports intervals of job runs.

**Required capability:** `local_db`

---

### Job Run Events

`GET /api/jobs/runs/events` — Returns Kubernetes events from job run artifacts (events.json).

**Required capability:** `local_db`

---

### Job Run Summary

`GET /api/job/run/summary` — Returns raw job run summary data including test failures and cluster operators.

**Required capability:** `local_db`

---

### Job Run Payload

`GET /api/job/run/payload` — Returns the payload a job run was using.

**Required capability:** `component_readiness`

---

### Job Analysis

`GET /api/jobs/analysis` — Analyzes jobs from the database.

**Required capability:** `local_db`

---

### Job Bugs

`GET /api/jobs/bugs` — Reports bugs related to jobs.

**Required capability:** `local_db`

---

### Job Artifacts

`GET /api/jobs/artifacts` — Queries job artifacts and their contents.

**Required capability:** `local_db`

---

### Job Run Labels

CRUD operations for job run label definitions.

| Method | Endpoint                  | Description                           | Capabilities              |
|--------|---------------------------|---------------------------------------|---------------------------|
| GET    | `/api/jobs/labels`        | List all job run label definitions    | `local_db`                |
| POST   | `/api/jobs/labels`        | Create a new label definition         | `local_db`, `write_endpoints` |
| GET    | `/api/jobs/labels/{id}`   | Get a specific label definition       | `local_db`                |
| PUT    | `/api/jobs/labels/{id}`   | Update a label definition             | `local_db`, `write_endpoints` |
| DELETE | `/api/jobs/labels/{id}`   | Delete a label definition             | `local_db`, `write_endpoints` |

---

### Job Run Symptoms

CRUD operations for job run symptom definitions.

| Method | Endpoint                    | Description                           | Capabilities              |
|--------|-----------------------------|---------------------------------------|---------------------------|
| GET    | `/api/jobs/symptoms`        | List all symptom definitions          | `local_db`                |
| POST   | `/api/jobs/symptoms`        | Create a new symptom definition       | `local_db`, `write_endpoints` |
| GET    | `/api/jobs/symptoms/{id}`   | Get a specific symptom definition     | `local_db`                |
| PUT    | `/api/jobs/symptoms/{id}`   | Update a symptom definition           | `local_db`, `write_endpoints` |
| DELETE | `/api/jobs/symptoms/{id}`   | Delete a symptom definition           | `local_db`, `write_endpoints` |

---

### Job Variants

`GET /api/job_variants` — Reports all job variants defined in BigQuery.

**Required capability:** `component_readiness`

---

### Tests

`GET /api/tests` — Reports on tests.

**Required capability:** `local_db`

| Option    | Type       | Description                                       |
|-----------|------------|---------------------------------------------------|
| release*  | String     | The OpenShift release to return results from       |
| filter    | Filter     | Filters the results (see Filtering)                |
| sortField | Field name | Sort by this field                                 |
| sort      | String     | `asc` or `desc`                                    |
| limit     | Integer    | Maximum number of results to return                |

---

### Tests (v2)

`GET /api/tests/v2` — Reports on tests (BigQuery-backed).

**Required capability:** `local_db`

---

### Test Details

`GET /api/tests/details` — Details of tests.

**Required capability:** `local_db`

---

### Test Analysis

| Endpoint                        | Description                    | Capability |
|---------------------------------|--------------------------------|------------|
| `GET /api/tests/analysis/overall`  | Overall analysis of tests      | `local_db` |
| `GET /api/tests/analysis/variants` | Analysis of tests by variants  | `local_db` |
| `GET /api/tests/analysis/jobs`     | Analysis of tests by job       | `local_db` |

---

### Test Bugs

`GET /api/tests/bugs` — Reports bugs in tests.

**Required capability:** `local_db`

---

### Test Outputs

`GET /api/tests/outputs` — Outputs of tests.

**Required capability:** `local_db`

---

### Recent Test Failures

`GET /api/tests/recent_failures` — Lists tests that recently started failing with configurable time windows.

**Required capability:** `local_db`

---

### Test Runs (v2)

`GET /api/tests/v2/runs` — Test runs from BigQuery with optional filtering by prow job run IDs and job names.

**Required capability:** `component_readiness`

---

### Test Durations

`GET /api/tests/durations` — Durations of tests.

**Required capability:** `local_db`

---

### Test Capabilities

`GET /api/tests/capabilities` — Returns list of available test capabilities.

**Required capability:** `component_readiness`

---

### Test Lifecycles

`GET /api/tests/lifecycles` — Returns list of available test lifecycles.

**Required capability:** `component_readiness`

---

### Pull Requests

`GET /api/pull_requests` — Reports on pull requests.

**Required capability:** `local_db`

---

### Pull Request Test Results

`GET /api/pull_requests/test_results` — Fetches test failures for a specific pull request from BigQuery (presubmits and /payload jobs). Optional: `include_successes` param to also return successes for matching test names.

**Required capability:** `component_readiness`

---

### Repositories

`GET /api/repositories` — Reports on repositories.

**Required capability:** `local_db`

---

### Releases

`GET /api/releases` — Reports on releases.

No special capabilities required.

---

### Release Health (detailed)

`GET /api/releases/health` — Reports health of releases.

**Required capability:** `local_db`

---

### Release Tags

| Endpoint                          | Description                        | Capability |
|-----------------------------------|------------------------------------|------------|
| `GET /api/releases/tags`          | Lists release tags                 | `local_db` |
| `GET /api/releases/tags/events`   | Lists events for release tags      | `local_db` |

---

### Release Pull Requests

`GET /api/releases/pull_requests` — Reports pull requests for releases.

**Required capability:** `local_db`

---

### Release Job Runs

`GET /api/releases/job_runs` — Lists job runs for releases.

**Required capability:** `local_db`

---

### Release Test Failures

`GET /api/releases/test_failures` — Analysis of test failures for releases.

**Required capability:** `local_db`

---

### Payload Test Failures

`GET /api/payloads/test_failures` — Analysis of test failures in payloads.

**Required capability:** `local_db`

---

### Payload Diff

`GET /api/payloads/diff` — Reports pull requests that differ between payloads.

**Required capability:** `local_db`

---

### Health – Build Cluster

| Endpoint                                | Description                     | Capabilities                  |
|-----------------------------------------|---------------------------------|-------------------------------|
| `GET /api/health/build_cluster`         | Reports health of build cluster | `local_db`, `build_cluster`   |
| `GET /api/health/build_cluster/analysis`| Analyzes build cluster health   | `local_db`, `build_cluster`   |

---

### Variants

`GET /api/variants` — Reports on variants.

**Required capability:** `local_db`

---

### Incidents

`GET /api/incidents` — Reports incident events.

**Required capability:** `local_db`

---

### Feature Gates

`GET /api/feature_gates` — Reports feature gates and their test counts for a particular release.

**Required capability:** `local_db`

---

### Canary

`GET /api/canary` — Displays canary report from database.

**Required capability:** `local_db`

---

### Report Date

`GET /api/report_date` — Displays report date.

No special capabilities required.

---

### Autocomplete

`GET /api/autocomplete/{field}` — Autocompletes queries from database.

**Required capability:** `local_db`

---

### Capabilities (meta)

`GET /api/capabilities` — Lists available API capabilities on this server.

No special capabilities required.

---

## Component Readiness

These endpoints are powered by BigQuery and provide component readiness
analysis.

| Endpoint                                                     | Method | Description                                                                           |
|--------------------------------------------------------------|--------|---------------------------------------------------------------------------------------|
| `/api/component_readiness`                                   | GET    | Reports component readiness from BigQuery                                             |
| `/api/component_readiness/test_details`                      | GET    | Reports test details for component readiness from BigQuery                            |
| `/api/component_readiness/variants`                          | GET    | Reports test variants for component readiness from BigQuery                           |
| `/api/component_readiness/views`                             | GET    | Lists all predefined server-side views over ComponentReadiness data                   |
| `/api/component_readiness/bugs`                              | POST   | Create Jira Bugs from component readiness                                             |

**Required capability:** `component_readiness` (and `local_db` / `write_endpoints` for some)

### Component Readiness – Triages

CRUD for regression triage records.

| Method | Endpoint                                          | Description                                         |
|--------|---------------------------------------------------|-----------------------------------------------------|
| GET    | `/api/component_readiness/triages`                | List regression triage records                      |
| POST   | `/api/component_readiness/triages`                | Create a triage record                              |
| GET    | `/api/component_readiness/triages/{id}`           | Get specific triage record                          |
| PUT    | `/api/component_readiness/triages/{id}`           | Update a triage record                              |
| DELETE | `/api/component_readiness/triages/{id}`           | Delete a triage record                              |
| GET    | `/api/component_readiness/triages/{id}/matches`   | List potential matching regressions for a triage     |
| GET    | `/api/component_readiness/triages/{id}/audit`     | Get audit logs for a triage                          |

### Component Readiness – Regressions

| Method | Endpoint                                              | Description                                         |
|--------|-------------------------------------------------------|-----------------------------------------------------|
| GET    | `/api/component_readiness/regressions`                | List test regressions (supports view or release params) |
| GET    | `/api/component_readiness/regressions/{id}`           | Get specific regression record                      |
| GET    | `/api/component_readiness/regressions/{id}/matches`   | List potential matching triages for a regression     |

---

## Chat (AI Assistant)

These endpoints proxy to the sippy-chat AI service.

| Endpoint                           | Method | Description                                          |
|------------------------------------|--------|------------------------------------------------------|
| `/api/chat`                        | *      | HTTP proxy for REST API requests to sippy-chat        |
| `/api/chat/stream`                 | *      | WebSocket proxy for chat (supports HTTP and WebSocket)|
| `/api/chat/personas`               | GET    | Lists available personas                              |
| `/api/chat/models`                 | GET    | Lists available models                                |
| `/api/chat/prompts`               | GET    | Lists available prompt templates                      |
| `/api/chat/prompts/render`        | POST   | Renders a prompt template                             |
| `/api/chat/ratings`               | POST   | Create a chat rating record                           |
| `/api/chat/conversations`         | POST   | Create a new chat conversation                        |
| `/api/chat/conversations/{id}`    | GET    | Get a specific chat conversation by ID                |

**Required capability:** `chat` (and `local_db` / `write_endpoints` for some)

---

## MCP (Model Context Protocol)

`/mcp/v1/` — Handles MCP requests.

This endpoint serves the Model Context Protocol interface.
