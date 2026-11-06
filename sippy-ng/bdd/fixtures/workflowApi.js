import * as data from './workflowData.js'

// Apply only operators exercised by these browser scenarios. Unknown operators
// are errors, so a changed frontend contract cannot silently return every row.
function matches(row, item) {
  const actual = row[item.field]
  let result
  switch (item.operator) {
    case 'contains':
      result = String(actual || '').includes(item.value)
      break
    case 'equals':
    case '=':
      result = String(actual) === String(item.value)
      break
    case 'starts with':
      result = String(actual || '').startsWith(item.value)
      break
    case 'has entry containing':
      result = (actual || []).some((value) => value.includes(item.value))
      break
    case '>=':
      result = actual >= item.value
      break
    case 'has entry':
      result = (actual || []).includes(item.value)
      break
    case 'is not empty':
      result = actual !== null && actual !== undefined && actual !== ''
      break
    case '>':
      result = actual > item.value
      break
    default:
      throw new Error(`Unsupported workflow fixture operator: ${item.operator}`)
  }
  return item.not ? !result : result
}

function selected(rows, url) {
  const model = JSON.parse(url.searchParams.get('filter') || '{"items":[]}')
  const items = model.items || []
  let result = rows.filter(
    (row) =>
      items.length === 0 ||
      (model.logicOperator === 'or'
        ? items.some((item) => matches(row, item))
        : items.every((item) => matches(row, item)))
  )
  const field = url.searchParams.get('sortField')
  if (field) {
    const direction = url.searchParams.get('sort') === 'asc' ? 1 : -1
    result = result.sort(
      (a, b) =>
        direction * (a[field] > b[field] ? 1 : a[field] < b[field] ? -1 : 0)
    )
  }
  const limit = Number(url.searchParams.get('limit'))
  return limit ? result.slice(0, limit) : result
}

const ok = (body) => ({ status: 200, body })
const fail = (message) => ({ status: 500, body: { message } })

// Returning null leaves the existing strict router in charge of unknown paths.
export function workflowResponse(api, method, url, bodyText) {
  const path = url.pathname
  if (method === 'PUT' || method === 'DELETE') {
    const label = api.labels.find(
      (label) => path === `/api/jobs/labels/${label.id}`
    )
    if (!label) return null
    if (api.readOnly) return { status: 403, body: { message: 'read only' } }
    if (api.failNextMutation) {
      api.failNextMutation = false
      return fail('Synthetic mutation rejected')
    }
    if (method === 'DELETE') {
      api.labels = api.labels.filter((item) => item.id !== label.id)
      return ok({})
    }
    const updated = { ...label, ...JSON.parse(bodyText) }
    api.labels = api.labels.map((item) =>
      item.id === label.id ? updated : item
    )
    return ok(updated)
  }
  if (method !== 'GET') return null
  const mode = api.workflowModes[path] || 'ok'
  if (mode === 'error') return fail(`Synthetic request failed: ${path}`)
  const empty = mode === 'empty'
  switch (path) {
    case '/api/jobs':
      return ok(selected(empty ? [] : data.jobs(), url))
    case '/api/tests':
      return ok(selected(empty ? [] : data.tests(), url))
    case '/api/jobs/runs': {
      const rows = selected(empty ? [] : data.runs(), url)
      const size = Number(url.searchParams.get('perPage') || 25)
      const page = Number(url.searchParams.get('page') || 0)
      return ok({
        rows: rows.slice(page * size, (page + 1) * size),
        total_rows: rows.length,
      })
    }
    case '/api/jobs/analysis':
      return ok({
        by_period: {
          '2026-10-25': {
            total_runs: 20,
            test_count: { [data.TEST]: 3 },
            result_count: { S: 16, F: 4 },
          },
        },
      })
    case '/api/jobs/bugs':
      return ok([])
    case '/api/tests/analysis/overall':
      return ok({ overall: [] })
    case '/api/tests/analysis/jobs':
    case '/api/tests/analysis/variants':
      return ok({})
    case '/api/tests/durations':
    case '/api/tests/outputs':
      return ok([])
    case '/api/component_readiness/regressions':
      return ok([])
    case '/api/tests/recent_failures':
      return ok({ rows: [], total_rows: 0 })
    case '/api/health':
      return ok(data.health(empty))
    case '/api/install':
    case '/api/upgrade':
      return ok(data.operatorResults(empty))
    case '/api/health/build_cluster':
      return ok(
        empty
          ? []
          : [
              { ...data.statistics(), id: 1, cluster: data.CLUSTER },
              {
                ...data.statistics(),
                id: 2,
                cluster: 'build-synthetic-b',
                current_pass_percentage: 25,
              },
            ]
      )
    case '/api/health/build_cluster/analysis':
      return ok(
        empty
          ? {}
          : {
              [data.CLUSTER]: {
                by_period: { '2026-10-25': data.statistics() },
              },
            }
      )
    case '/api/feature_gates':
      return ok(
        selected(
          [
            {
              id: 1,
              feature_gate: data.GATE,
              unique_test_count: 1,
              enabled: ['TechPreviewNoUpgrade:SelfManagedHA'],
              first_seen_in_major: '5',
              first_seen_in_minor: '0',
              first_seen_in: '4.22',
              links: {
                details: `/api/feature_gates/${data.GATE}?release=${data.RELEASE}`,
              },
            },
            {
              id: 2,
              feature_gate: 'OtherGate',
              unique_test_count: 9,
              enabled: ['TechPreviewNoUpgrade:SelfManagedHA'],
              first_seen_in_major: '5',
              first_seen_in_minor: '0',
              first_seen_in: '4.22',
            },
          ],
          url
        )
      )
    case `/api/feature_gates/${data.GATE}`:
      return ok(data.gateDetail())
    case '/api/repositories':
      return ok(
        selected(
          [
            {
              id: 1,
              org: data.ORG,
              repo: data.REPO,
              job_count: 2,
              revert_count: 1,
              worst_premerge_job_failures: 1.2,
            },
            {
              id: 2,
              org: data.ORG,
              repo: 'another-repo',
              job_count: 7,
              revert_count: 0,
              worst_premerge_job_failures: 2,
            },
          ],
          url
        )
      )
    case '/api/pull_requests':
      return ok(
        selected(
          [
            {
              id: 1,
              org: data.ORG,
              repo: data.REPO,
              number: 98765,
              title: 'Synthetic selected change',
              author: 'synthetic-author',
              merged_at: '2026-10-25T12:00:00Z',
              link: `https://github.example.invalid/${data.ORG}/${data.REPO}/pull/98765`,
            },
            {
              id: 2,
              org: data.ORG,
              repo: 'another-repo',
              number: 98766,
              title: 'Other repository change',
              author: 'other-author',
              merged_at: '2026-10-25T11:00:00Z',
            },
          ],
          url
        )
      )
    case '/api/jobs/runs/events':
    case '/api/jobs/runs/intervals': {
      const run = url.searchParams.get('prow_job_run_id')
      // A different run has no data; never reuse the selected run's evidence.
      if (empty) return ok({})
      if (run !== data.RUN) return ok(null)
      if (url.searchParams.get('file') === 'missing-intervals.json') {
        return ok({
          jobRunURL: data.GCS,
          items: [],
          intervalFilesAvailable: [
            'synthetic-intervals.json',
            'missing-intervals.json',
          ],
        })
      }
      return ok({
        jobRunURL: data.GCS,
        items: path.endsWith('events') ? data.events() : data.intervals(),
        intervalFilesAvailable: [
          'synthetic-intervals.json',
          'missing-intervals.json',
        ],
      })
    }
    default:
      return null
  }
}
