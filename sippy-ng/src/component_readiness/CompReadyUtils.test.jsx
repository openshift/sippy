//import HelloWorld from './HelloWorld'
import {
  convertApiUrlToUiUrl,
  createLatestRequest,
  dateEndFormat,
  dateFormat,
  endTimeControlValue,
  endTimeParam,
  formatHistoricalDateRange,
  formatLongDate,
  formatUrlDateRange,
  generateTestDetailsReportLink,
  getForceClosePreviewUrl,
  getForceCloseRegressionsUrl,
  getTestDetailsLink,
  getTriagesAPIUrl,
  historicalClosureScope,
  historicalWindowFromLink,
  parseEnvironmentVariants,
  triagedRegressionCellRank,
  triagedRegressionViewCell,
  triagedRegressionViewNames,
  validateHistoricalRegression,
  viewControlDefaults,
} from './CompReadyUtils'

test('createLatestRequest keeps only the newest id current', () => {
  const requests = createLatestRequest()
  const first = requests.start()
  const second = requests.start()
  expect(requests.isCurrent(first)).toBe(false)
  expect(requests.isCurrent(second)).toBe(true)
})

test('parses a query param start time without timezone', () => {
  let result = formatLongDate('2024-09-05 00:00:00', dateFormat)
  expect(result).toBe('2024-09-05 00:00:00')
})

test('parses a query param start time eod without timezone', () => {
  let result = formatLongDate('2024-09-05 23:59:59', dateFormat)
  expect(result).toBe('2024-09-05 00:00:00')
})

test('parses a query param end time without timezone', () => {
  let result = formatLongDate('2024-09-05 23:59:59', dateEndFormat)
  expect(result).toBe('2024-09-05 23:59:59')
})

test('parses an ISO8601 start time', () => {
  let result = formatLongDate('2024-08-06T00:00:00Z', dateFormat)
  expect(result).toBe('2024-08-06 00:00:00')
})

test('parses an ISO8601 end time mid-day', () => {
  let result = formatLongDate('2024-08-06T08:00:00Z', dateEndFormat)
  expect(result).toBe('2024-08-06 23:59:59')
})

test('parses an ISO8601 end time', () => {
  let result = formatLongDate('2024-08-06T23:59:59Z', dateEndFormat)
  expect(result).toBe('2024-08-06 23:59:59')
})

describe('getTestDetailsLink', () => {
  test('returns null for null links', () => {
    expect(getTestDetailsLink(null)).toBeNull()
  })

  test('returns plain test_details key when present', () => {
    const links = { test_details: '/api/test/details' }
    expect(getTestDetailsLink(links)).toBe('/api/test/details')
  })

  test('returns exact view key when viewName matches', () => {
    const links = {
      'test_details:4.22-main': '/api/main',
      'test_details:4.22-arm64': '/api/arm64',
    }
    expect(getTestDetailsLink(links, '4.22-arm64')).toBe('/api/arm64')
  })

  test('returns null when viewName is specified but key is missing', () => {
    const links = {
      'test_details:4.22-main': '/api/main',
    }
    expect(getTestDetailsLink(links, '4.22-arm64')).toBeNull()
  })

  test('falls back to first test_details: key when no viewName', () => {
    const links = {
      'test_details:4.22-main': '/api/main',
    }
    expect(getTestDetailsLink(links)).toBe('/api/main')
  })

  test('returns null when no matching keys exist', () => {
    const links = { self: '/api/self' }
    expect(getTestDetailsLink(links)).toBeNull()
  })

  test('plain test_details takes priority over view-specific keys', () => {
    const links = {
      test_details: '/api/plain',
      'test_details:4.22-main': '/api/main',
    }
    expect(getTestDetailsLink(links, '4.22-main')).toBe('/api/plain')
    expect(getTestDetailsLink(links)).toBe('/api/plain')
  })
})

describe('convertApiUrlToUiUrl', () => {
  test('rewrites /api/component_readiness/ to /sippy-ng/component_readiness/', () => {
    expect(
      convertApiUrlToUiUrl(
        'http://localhost:8080/api/component_readiness/test_details?testId=foo'
      )
    ).toBe('/sippy-ng/component_readiness/test_details?testId=foo')
  })

  test('rewrites generic /api/ prefix to /sippy-ng/', () => {
    expect(
      convertApiUrlToUiUrl('https://sippy.dptools.openshift.org/api/other/path')
    ).toBe('/sippy-ng/other/path')
  })

  test('returns non-/api/ URLs unchanged', () => {
    expect(
      convertApiUrlToUiUrl('/sippy-ng/component_readiness/test_details?x=1')
    ).toBe('/sippy-ng/component_readiness/test_details?x=1')
  })

  test('handles relative /api/ paths without a host', () => {
    expect(
      convertApiUrlToUiUrl('/api/component_readiness/test_details?a=b')
    ).toBe('/sippy-ng/component_readiness/test_details?a=b')
  })
})

describe('getTriagesAPIUrl', () => {
  test('returns collection URL without view when no arguments', () => {
    const result = getTriagesAPIUrl()
    expect(result).toMatch(/\/api\/component_readiness\/triages$/)
  })

  test('returns collection URL with view query parameter', () => {
    const result = getTriagesAPIUrl(null, '4.18-main')
    expect(result).toMatch(
      /\/api\/component_readiness\/triages\?view=4\.18-main$/
    )
  })

  test('encodes view value in query parameter', () => {
    const result = getTriagesAPIUrl(null, '4.18 beta')
    expect(result).toContain('view=4.18%20beta')
  })

  test('returns ID-specific URL without view query parameter', () => {
    const result = getTriagesAPIUrl(42, '4.18-main')
    expect(result).toMatch(/\/api\/component_readiness\/triages\/42$/)
  })

  test('returns ID-specific URL when only ID is provided', () => {
    const result = getTriagesAPIUrl(7)
    expect(result).toMatch(/\/api\/component_readiness\/triages\/7$/)
  })
})

describe('force-close API URLs', () => {
  test('returns the force-close preview URL for a triage', () => {
    expect(getForceClosePreviewUrl(42)).toBe(
      'http://localhost:8080/api/component_readiness/triages/42/force_close_preview'
    )
  })

  test('returns the force-close submission URL for a triage', () => {
    expect(getForceCloseRegressionsUrl(42)).toBe(
      'http://localhost:8080/api/component_readiness/triages/42/force_close_regressions'
    )
  })
})

describe('generateTestDetailsReportLink', () => {
  test('returns server link converted to UI URL when HATEOAS link is present', () => {
    const test = {
      test_id: 'test-123',
      test_name: 'my test',
      component: 'Networking',
      capability: 'cap1',
      variants: { Architecture: 'amd64' },
      base_stats: { release: '4.18' },
      links: {
        test_details:
          'http://localhost:8080/api/component_readiness/test_details?testId=test-123&component=Networking',
      },
    }
    const result = generateTestDetailsReportLink(test)
    expect(result).toBe(
      '/sippy-ng/component_readiness/test_details?testId=test-123&component=Networking'
    )
  })

  test('returns null when no HATEOAS link exists', () => {
    const test = {
      test_id: 'test-123',
      test_name: 'my test',
      component: 'Networking',
      capability: 'cap1',
      variants: { Architecture: 'amd64' },
      base_stats: { release: '4.18' },
      links: {},
    }
    const result = generateTestDetailsReportLink(test)
    expect(result).toBeNull()
  })

  test('uses viewName to find view-specific link', () => {
    const test = {
      test_id: 'test-456',
      links: {
        'test_details:4.18-main':
          'http://localhost:8080/api/component_readiness/test_details?testId=test-456&view=4.18-main',
      },
    }
    const result = generateTestDetailsReportLink(test, '4.18-main')
    expect(result).toBe(
      '/sippy-ng/component_readiness/test_details?testId=test-456&view=4.18-main'
    )
  })
})

describe('historical closed regression helpers', () => {
  const historicalLink =
    'http://localhost:8080/api/component_readiness/test_details?view=5.0-main&testId=t1' +
    '&sampleStartTime=2026-09-22T00%3A00%3A00Z&sampleEndTime=2026-09-30T00%3A00%3A00Z&regressionId=49971'
  const closedRegression = {
    id: 49971,
    test_id: 't1',
    release: '5.0',
    variants: ['Platform:azure', 'Network:ovn'],
    closed: { Time: '2026-10-02T08:00:00Z', Valid: true },
    last_failure: { Time: '2026-09-29T01:33:26Z', Valid: true },
    views: [{ view_name: '5.0-main', active: false }],
    links: { 'test_details:5.0-main': historicalLink },
  }

  test.each([
    ['closed regression', closedRegression, '5.0-main', 'regression'],
    [
      'closed only in this view',
      {
        ...closedRegression,
        closed: { Valid: false },
        views: [
          { view_name: '5.0-main', active: false },
          { view_name: '5.0-azure', active: true },
        ],
      },
      '5.0-main',
      'view',
    ],
    [
      'open in this view',
      {
        ...closedRegression,
        closed: { Valid: false },
        views: [{ view_name: '5.0-main', active: true }],
      },
      '5.0-main',
      null,
    ],
    ['never seen in the view', closedRegression, '5.0-other', null],
  ])('historicalClosureScope: %s', (_name, regression, view, expected) => {
    expect(historicalClosureScope(regression, view)).toBe(expected)
  })

  test('view columns include closed views without current report data', () => {
    expect(triagedRegressionViewNames({}, [closedRegression])).toEqual([
      '5.0-main',
    ])
  })

  test('view columns skip open views without report data and keep main first', () => {
    const openElsewhere = {
      ...closedRegression,
      id: 2,
      closed: { Valid: false },
      views: [
        { view_name: '5.0-azure', active: false },
        { view_name: '5.0-other', active: true },
      ],
      links: {},
    }
    expect(
      triagedRegressionViewNames({ '5.0-main': [] }, [openElsewhere])
    ).toEqual(['5.0-main', '5.0-azure'])
  })

  test('historicalWindowFromLink reads the concrete sample window', () => {
    const window = historicalWindowFromLink(historicalLink)
    expect(window.start.toISOString()).toBe('2026-09-22T00:00:00.000Z')
    expect(window.endExclusive.toISOString()).toBe('2026-09-30T00:00:00.000Z')
    expect(historicalWindowFromLink('/api/x?testId=1')).toBeNull()
    expect(historicalWindowFromLink(undefined)).toBeNull()
  })

  test.each([
    ['2026-09-22T00:00:00Z', '2026-09-30T00:00:00Z', 'September 22–29, 2026'],
    [
      '2026-09-28T00:00:00Z',
      '2026-10-06T00:00:00Z',
      'September 28 – October 5, 2026',
    ],
    [
      '2026-12-28T00:00:00Z',
      '2027-01-05T00:00:00Z',
      'December 28, 2026 – January 4, 2027',
    ],
    ['2026-09-29T00:00:00Z', '2026-09-30T00:00:00Z', 'September 29, 2026'],
    ['2026-05-10T00:00:00Z', '2026-06-09T23:59:59Z', 'May 10 – June 9, 2026'],
  ])('formatHistoricalDateRange(%s, %s)', (start, end, expected) => {
    expect(formatHistoricalDateRange(new Date(start), new Date(end))).toBe(
      expected
    )
  })

  test('closed regression cell links to the historical report', () => {
    const cell = triagedRegressionViewCell(closedRegression, '5.0-main', {})
    expect(cell.kind).toBe('historical')
    expect(cell.scope).toBe('regression')
    expect(cell.url).toBe(
      '/sippy-ng/component_readiness/test_details?view=5.0-main&testId=t1' +
        '&sampleStartTime=2026-09-22T00%3A00%3A00Z&sampleEndTime=2026-09-30T00%3A00%3A00Z&regressionId=49971'
    )
    expect(cell.tooltip).toBe(
      'Closed regression. View historical test details for September 22–29, 2026.'
    )
  })

  test('closure in only one view is labelled as such', () => {
    const regression = {
      ...closedRegression,
      closed: { Valid: false },
      views: [
        { view_name: '5.0-main', active: false },
        { view_name: '5.0-azure', active: true },
      ],
    }
    const cell = triagedRegressionViewCell(regression, '5.0-main', {})
    expect(cell.tooltip).toMatch(/^Closed in this view\. View historical/)
  })

  test('current report data keeps the severity cell', () => {
    const current = {
      regression: { id: 49971 },
      status: -400,
      explanations: ['regressed'],
      links: { test_details: '/api/component_readiness/test_details?x=1' },
    }
    const cell = triagedRegressionViewCell(closedRegression, '5.0-main', {
      '5.0-main': [current],
    })
    expect(cell).toEqual({
      kind: 'current',
      status: -400,
      explanations: ['regressed'],
      url: '/sippy-ng/component_readiness/test_details?x=1',
    })
  })

  test('closed regression without a link explains why', () => {
    const noFailure = {
      ...closedRegression,
      last_failure: { Valid: false },
      links: {},
    }
    expect(triagedRegressionViewCell(noFailure, '5.0-main', {})).toEqual({
      kind: 'unavailable',
      scope: 'regression',
      tooltip:
        'Closed regression. Historical test details are unavailable because no last failure was recorded.',
    })
    const removedView = { ...closedRegression, links: {} }
    expect(
      triagedRegressionViewCell(removedView, '5.0-main', {}).tooltip
    ).toMatch(/no longer configured/)
  })

  test('status cells sort current, historical, unavailable, then empty', () => {
    const cells = [
      null,
      { kind: 'unavailable' },
      { kind: 'historical' },
      { kind: 'current', status: 0 },
      { kind: 'current', status: -400 },
    ]
    const sorted = [...cells].sort(
      (a, b) => triagedRegressionCellRank(a) - triagedRegressionCellRank(b)
    )
    expect(sorted).toEqual([
      { kind: 'current', status: -400 },
      { kind: 'current', status: 0 },
      { kind: 'historical' },
      { kind: 'unavailable' },
      null,
    ])
  })

  test('open regression without report data shows nothing', () => {
    const open = {
      ...closedRegression,
      closed: { Valid: false },
      views: [{ view_name: '5.0-main', active: true }],
    }
    expect(triagedRegressionViewCell(open, '5.0-main', {})).toBeNull()
  })

  test.each([
    ['2026-09-30T00:00:00Z', '2026-09-29 23:59:59'],
    ['2026-09-30 00:00:00', '2026-09-29 23:59:59'],
    ['2026-06-09T23:59:59Z', '2026-06-09T23:59:59Z'],
    [undefined, undefined],
  ])('endTimeControlValue(%s)', (value, expected) => {
    expect(endTimeControlValue(value)).toBe(expected)
  })

  test('endTimeParam keeps an unchanged exclusive end and formats edits', () => {
    const url = '2026-09-30T00:00:00Z'
    expect(endTimeParam('2026-09-29 23:59:59', url)).toBe(url)
    expect(endTimeParam('2026-09-28 23:59:59', url)).toBe('2026-09-28 23:59:59')
    expect(endTimeParam('2026-06-09T23:59:59Z', '2026-06-09T23:59:59Z')).toBe(
      '2026-06-09 23:59:59'
    )
  })

  const view = {
    name: '5.0-main',
    base_release: {
      release: '4.22',
      start: '2026-05-10T00:00:00Z',
      end: '2026-06-09T23:59:59Z',
    },
    sample_release: {
      release: '5.0',
      start: '2026-11-10T00:00:00Z',
      end: '2026-11-17T12:00:00Z',
    },
    variant_options: {
      column_group_by: { Platform: {} },
      db_group_by: { Platform: {}, Network: {} },
      include_variants: { Platform: ['aws'] },
    },
    advanced_options: { confidence: 95, pity_factor: 5 },
    test_filters: { lifecycles: ['blocking'] },
  }

  test('viewControlDefaults uses the view when the URL has no overrides', () => {
    expect(viewControlDefaults(view, { view: '5.0-main' })).toEqual({
      baseRelease: '4.22',
      baseStartTime: '2026-05-10 00:00:00',
      baseEndTime: '2026-06-09 00:00:00',
      sampleRelease: '5.0',
      sampleStartTime: '2026-11-10 00:00:00',
      sampleEndTime: '2026-11-17 00:00:00',
      columnGroupBy: ['Platform'],
      dbGroupBy: ['Platform', 'Network'],
      includeVariants: { Platform: ['aws'] },
      testLifecycles: ['blocking'],
      testCapabilities: [],
      confidence: 95,
      pity: 5,
    })
  })

  test('viewControlDefaults lets explicit URL parameters win', () => {
    const defaults = viewControlDefaults(view, {
      view: '5.0-main',
      sampleStartTime: '2026-09-22T00:00:00Z',
      sampleEndTime: '2026-09-30T00:00:00Z',
      baseStartTime: '2026-05-10T00:00:00Z',
      baseEndTime: '2026-06-09T23:59:59Z',
      confidence: 90,
      includeVariant: ['Platform:azure'],
      testLifecycles: [],
    })
    expect(defaults).not.toHaveProperty('sampleStartTime')
    expect(defaults).not.toHaveProperty('sampleEndTime')
    expect(defaults).not.toHaveProperty('baseStartTime')
    expect(defaults).not.toHaveProperty('baseEndTime')
    expect(defaults).not.toHaveProperty('confidence')
    expect(defaults).not.toHaveProperty('includeVariants')
    expect(defaults.testLifecycles).toEqual(['blocking'])
    expect(defaults.sampleRelease).toBe('5.0')
  })

  test.each([
    ['2026-09-22T00:00:00Z', '2026-09-30T00:00:00Z', 'September 22–29, 2026'],
    ['2026-09-22 00:00:00', '2026-09-28 23:59:59', 'September 22–28, 2026'],
    ['2026-05-10T00:00:00Z', '2026-06-09T23:59:59Z', 'May 10 – June 9, 2026'],
    [undefined, '2026-09-30T00:00:00Z', null],
    ['not a date', '2026-09-30T00:00:00Z', null],
  ])('formatUrlDateRange(%s, %s)', (start, end, expected) => {
    expect(formatUrlDateRange(start, end)).toBe(expected)
  })

  test('parseEnvironmentVariants splits key:value pairs', () => {
    expect(parseEnvironmentVariants('Platform:azure Network:ovn')).toEqual({
      Platform: 'azure',
      Network: 'ovn',
    })
    expect(parseEnvironmentVariants(undefined)).toEqual({})
  })

  const request = {
    testId: 't1',
    sampleRelease: '5.0',
    variants: { Platform: 'azure', Network: 'ovn', Topology: 'ha' },
    view: '5.0-main',
  }

  test('validateHistoricalRegression accepts the matching record', () => {
    expect(validateHistoricalRegression(closedRegression, request)).toEqual({
      valid: true,
      mismatches: [],
      scope: 'regression',
    })
  })

  test.each([
    ['test ID', { test_id: 'other' }],
    ['release', { release: '4.22' }],
    ['variants', { variants: ['Platform:aws', 'Network:ovn'] }],
    ['view', { views: [{ view_name: '5.0-azure', active: false }] }],
  ])(
    'validateHistoricalRegression rejects a mismatched %s',
    (field, change) => {
      const result = validateHistoricalRegression(
        { ...closedRegression, ...change },
        request
      )
      expect(result.valid).toBe(false)
      expect(result.mismatches).toEqual([field])
    }
  )

  test('validateHistoricalRegression reports closure in only this view', () => {
    const result = validateHistoricalRegression(
      {
        ...closedRegression,
        closed: { Valid: false },
        views: [{ view_name: '5.0-main', active: false }],
      },
      request
    )
    expect(result.scope).toBe('view')
  })

  test('validateHistoricalRegression handles a missing record', () => {
    expect(validateHistoricalRegression(null, request).valid).toBe(false)
  })
})
