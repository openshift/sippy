import {
  cancelledReport,
  capabilitiesReport,
  componentReport,
  ALT_COMPONENT,
  ALT_SAMPLE_RELEASE,
  mainReport,
  releaseCatalog,
  singleTestReport,
  testDetailsReport,
  testsReport,
  GENERATED_AT,
  PINNED_DATE,
  views,
} from './data.js'

const cors = {
  'access-control-allow-origin': '*',
  'access-control-allow-methods': 'GET,POST,PUT,DELETE,OPTIONS',
  'access-control-allow-headers': '*',
}

export function createApi() {
  return {
    readOnly: false,
    reportMode: 'ok',
    testDetailsMode: 'ok',
    failNextMutation: false,
    requests: [],
    triages: [],
    nextTriageId: 7,
    _reportQueue: [],
    _hold: null,
    _releaseHold: null,

    queueReport(body) {
      this._reportQueue.push(body)
    },

    holdCurrentReport() {
      this._hold = new Promise((resolve) => {
        this._releaseHold = resolve
      })
    },

    releaseHeldReport() {
      if (this._releaseHold) {
        this._releaseHold()
        this._releaseHold = null
        this._hold = null
      }
    },

    reportRequests() {
      return this.requests.filter(
        (request) =>
          request.method === 'GET' && isComponentReport(request.pathname)
      )
    },

    mutationRequests() {
      return this.requests.filter((request) =>
        ['POST', 'PUT', 'DELETE'].includes(request.method)
      )
    },
  }
}

export async function installApi(page, api) {
  await page.route('**/api/**', async (route) => {
    const request = route.request()
    if (request.method() === 'OPTIONS') {
      await route.fulfill({ status: 204, headers: cors })
      return
    }
    const url = new URL(request.url())
    const bodyText = request.postData() || ''
    api.requests.push({
      method: request.method(),
      url: request.url(),
      pathname: url.pathname,
      search: url.search,
      body: bodyText ? JSON.parse(bodyText) : null,
    })
    try {
      const response = await responseFor(api, request.method(), url, bodyText)
      if (response.hold) {
        await response.hold
      }
      await route.fulfill({
        status: response.status,
        contentType: 'application/json',
        headers: cors,
        body: JSON.stringify(response.body),
      })
    } catch (error) {
      await route.fulfill({
        status: 500,
        contentType: 'text/plain',
        headers: cors,
        body: String(error),
      })
      throw error
    }
  })
}

async function responseFor(api, method, url, bodyText) {
  const path = url.pathname

  if (path.endsWith('/api/releases')) {
    return ok(releaseCatalog())
  }
  if (path.endsWith('/api/capabilities')) {
    const caps = ['local_db', 'component_readiness', 'openshift_releases']
    if (!api.readOnly) {
      caps.push('write_endpoints')
    }
    return ok(caps)
  }
  if (path.endsWith('/api/report_date')) {
    return ok({ pinnedDateTime: PINNED_DATE })
  }
  if (path.endsWith('/api/component_readiness/views')) {
    return ok(views())
  }
  if (path.endsWith('/api/job_variants')) {
    return ok({
      variants: {
        Platform: ['aws', 'gcp'],
        Architecture: ['amd64'],
        Network: ['ovn'],
      },
    })
  }
  if (path.endsWith('/api/tests/capabilities')) {
    return ok(['sdn', 'storage'])
  }
  if (path.endsWith('/api/tests/lifecycles')) {
    return ok(['blocking', 'informing'])
  }
  if (path.endsWith('/api/tests/bugs')) {
    return ok([])
  }
  if (path.includes('/matches')) {
    return ok([])
  }
  if (path.endsWith('/force_close_preview')) {
    if (api.reportMode === 'empty-preview') {
      return ok({ would_close: [], would_not_close: [] })
    }
    return ok({
      would_close: [
        {
          regression_id: 101,
          test_name: 'should maintain pod network',
        },
      ],
      would_not_close: [],
    })
  }
  if (path.endsWith('/force_close_regressions') && method === 'POST') {
    if (api.failNextMutation) {
      api.failNextMutation = false
      return fail(400, 'rejected')
    }
    return ok({ closed_regression_ids: [101] })
  }
  if (path.includes('/api/component_readiness/triages')) {
    return triageResponse(api, method, url, bodyText)
  }
  if (path.endsWith('/api/component_readiness/test_details')) {
    if (api.testDetailsMode === 'empty') {
      return ok({})
    }
    if (api.testDetailsMode === 'error') {
      return fail(500, 'test details failed')
    }
    return ok(
      testDetailsReport({
        historical: true,
        stale: true,
        lifecycle: 'informing',
      })
    )
  }
  if (isComponentReport(path) && method === 'GET') {
    return componentReportResponse(api, url)
  }
  throw new Error(`unexpected API ${method} ${url.pathname}${url.search}`)
}

function componentReportResponse(api, url) {
  let body
  if (api._reportQueue.length > 0) {
    body = api._reportQueue.shift()
  } else if (api.reportMode === 'empty') {
    body = { generated_at: GENERATED_AT, rows: [] }
  } else if (api.reportMode === 'cancelled') {
    body = cancelledReport()
  } else if (api.reportMode === 'error') {
    return fail(500, 'report failed')
  } else {
    body = reportForQuery(url)
  }
  // The view query stays pending, including a repeat of that same query, until
  // the test releases it. The replacement sample is the newer selection.
  if (
    api._hold &&
    url.searchParams.get('sampleRelease') !== ALT_SAMPLE_RELEASE
  ) {
    return { status: 200, body, hold: api._hold }
  }
  return ok(body)
}

function reportForQuery(url) {
  const component = url.searchParams.get('component')
  const capability = url.searchParams.get('capability')
  const testId = url.searchParams.get('testId')
  const sample = url.searchParams.get('sampleRelease')
  if (testId) {
    return singleTestReport()
  }
  if (capability) {
    return testsReport()
  }
  if (component) {
    return capabilitiesReport()
  }
  if (sample === ALT_SAMPLE_RELEASE) {
    return componentReport(ALT_COMPONENT, -400, [])
  }
  return mainReport()
}

function triageResponse(api, method, url, bodyText) {
  const idMatch = url.pathname.match(/\/triages\/(\d+)/)
  if (method === 'POST') {
    if (api.failNextMutation) {
      api.failNextMutation = false
      return fail(400, 'rejected')
    }
    const payload = JSON.parse(bodyText || '{}')
    const created = {
      id: api.nextTriageId++,
      url: payload.url,
      type: payload.type,
      description: payload.description,
      regressions: payload.regressions || [],
      resolved: { Time: null, Valid: false },
      links: {
        self: `/api/component_readiness/triages/${api.nextTriageId - 1}`,
      },
    }
    api.triages.push(created)
    return ok(created)
  }
  if (idMatch && method === 'GET') {
    const found = api.triages.find((triage) => String(triage.id) === idMatch[1])
    if (!found) {
      return fail(404, 'triage not found')
    }
    return ok(found)
  }
  if (idMatch && method === 'PUT') {
    if (api.failNextMutation) {
      api.failNextMutation = false
      return fail(400, 'rejected')
    }
    const payload = JSON.parse(bodyText || '{}')
    const index = api.triages.findIndex(
      (triage) => String(triage.id) === idMatch[1]
    )
    if (index === -1) {
      return fail(404, 'triage not found')
    }
    api.triages[index] = {
      ...api.triages[index],
      ...payload,
      id: api.triages[index].id,
      links: api.triages[index].links,
    }
    return ok(api.triages[index])
  }
  if (idMatch && method === 'DELETE') {
    if (api.failNextMutation) {
      api.failNextMutation = false
      return fail(400, 'rejected')
    }
    api.triages = api.triages.filter(
      (triage) => String(triage.id) !== idMatch[1]
    )
    return ok({})
  }
  if (method === 'GET') {
    return ok(api.triages)
  }
  throw new Error(`unexpected API ${method} ${url.pathname}`)
}

function isComponentReport(pathname) {
  return pathname.endsWith('/api/component_readiness')
}

function ok(body) {
  return { status: 200, body }
}

function fail(status, message) {
  return { status, body: { code: status, message } }
}
