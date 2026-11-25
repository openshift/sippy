import {
  CellKind,
  ClosureScope,
  endTimeControlValue,
  endTimeParam,
  formatHistoricalDateRange,
  formatUrlDateRange,
  historicalClosure,
  historicalWindowFromLink,
  parseUrlDate,
  preferredTestDetailsLink,
  triagedRegressionCellRank,
  triagedRegressionViewCell,
  triagedRegressionViewNames,
  validateHistoricalRegression,
} from './HistoricalRegressionUtils'

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
const closedInMainOnly = {
  ...closedRegression,
  closed: { Valid: false },
  views: [
    { view_name: '5.0-main', active: false },
    { view_name: '5.0-azure', active: true },
  ],
}

describe('historicalClosure', () => {
  test.each([
    ['closed regression', closedRegression, '5.0-main', 'regression'],
    ['closed only in this view', closedInMainOnly, '5.0-main', 'view'],
    ['open in another view', closedInMainOnly, '5.0-azure', null],
    ['never seen in the view', closedRegression, '5.0-other', null],
    [
      'link without an association',
      {
        ...closedRegression,
        links: { 'test_details:5.0-other': historicalLink },
      },
      '5.0-other',
      null,
    ],
  ])('%s', (_name, regression, view, expected) => {
    expect(historicalClosure(regression, view)?.scope ?? null).toBe(expected)
  })

  test('returns the closed association', () => {
    expect(historicalClosure(closedInMainOnly, '5.0-main')).toEqual({
      scope: ClosureScope.VIEW,
      association: { view_name: '5.0-main', active: false },
    })
  })
})

describe('triagedRegressionViewNames', () => {
  test('includes closed views without current report data', () => {
    expect(triagedRegressionViewNames({}, [closedRegression])).toEqual([
      '5.0-main',
    ])
  })

  test('skips open views without report data and keeps main first', () => {
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
})

describe('preferredTestDetailsLink', () => {
  const link = (view) => `/api/component_readiness/test_details?view=${view}`

  test('prefers a current link over a historical main link', () => {
    const regression = {
      ...closedInMainOnly,
      links: {
        self: '/api/component_readiness/regressions/49971',
        'test_details:5.0-main': link('5.0-main'),
        'test_details:5.0-azure': link('5.0-azure'),
      },
    }
    expect(preferredTestDetailsLink(regression)).toBe(link('5.0-azure'))
  })

  test('prefers the main view among current links', () => {
    const regression = {
      ...closedRegression,
      closed: { Valid: false },
      views: [
        { view_name: '5.0-azure', active: true },
        { view_name: '5.0-main', active: true },
      ],
      links: {
        'test_details:5.0-azure': link('5.0-azure'),
        'test_details:5.0-main': link('5.0-main'),
      },
    }
    expect(preferredTestDetailsLink(regression)).toBe(link('5.0-main'))
  })

  test('falls back to a historical link, main first', () => {
    const regression = {
      ...closedRegression,
      views: [
        { view_name: '5.0-azure', active: false },
        { view_name: '5.0-main', active: false },
      ],
      links: {
        'test_details:5.0-azure': link('5.0-azure'),
        'test_details:5.0-main': link('5.0-main'),
      },
    }
    expect(preferredTestDetailsLink(regression)).toBe(link('5.0-main'))
  })

  test('falls back to a plain test_details link', () => {
    expect(
      preferredTestDetailsLink({ links: { test_details: link('x') } })
    ).toBe(link('x'))
    expect(preferredTestDetailsLink({ links: {} })).toBeNull()
    expect(preferredTestDetailsLink(null)).toBeNull()
  })
})

describe('parseUrlDate', () => {
  test.each([
    ['2026-09-30T00:00:00Z', '2026-09-30T00:00:00.000Z'],
    ['2026-09-30 00:00:00', '2026-09-30T00:00:00.000Z'],
    ['2026-09-30T00:00:00', '2026-09-30T00:00:00.000Z'],
    ['2026-09-30T02:00:00+02:00', '2026-09-30T00:00:00.000Z'],
    ['2026-09-30 23:59:59', '2026-09-30T23:59:59.000Z'],
  ])('%s is read as %s', (value, expected) => {
    expect(parseUrlDate(value).toISOString()).toBe(expected)
  })

  test.each([[undefined], [null], [''], ['not a date']])(
    '%s is invalid',
    (value) => {
      expect(parseUrlDate(value)).toBeNull()
    }
  )
})

describe('historical windows and date ranges', () => {
  test('historicalWindowFromLink reads the concrete sample window', () => {
    const window = historicalWindowFromLink(historicalLink)
    expect(window.start.toISOString()).toBe('2026-09-22T00:00:00.000Z')
    expect(window.endExclusive.toISOString()).toBe('2026-09-30T00:00:00.000Z')
    expect(historicalWindowFromLink('/api/x?testId=1')).toBeNull()
    expect(historicalWindowFromLink(undefined)).toBeNull()
  })

  test('historicalWindowFromLink reads zoneless dates as UTC', () => {
    const window = historicalWindowFromLink(
      '/api/x?sampleStartTime=2026-09-22%2000:00:00&sampleEndTime=2026-09-30%2000:00:00'
    )
    expect(window.start.toISOString()).toBe('2026-09-22T00:00:00.000Z')
    expect(window.endExclusive.toISOString()).toBe('2026-09-30T00:00:00.000Z')
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

  test.each([
    ['2026-09-22T00:00:00Z', '2026-09-30T00:00:00Z', 'September 22–29, 2026'],
    ['2026-09-22 00:00:00', '2026-09-28 23:59:59', 'September 22–28, 2026'],
    ['2026-05-10T00:00:00Z', '2026-06-09T23:59:59Z', 'May 10 – June 9, 2026'],
    [undefined, '2026-09-30T00:00:00Z', null],
    ['not a date', '2026-09-30T00:00:00Z', null],
  ])('formatUrlDateRange(%s, %s)', (start, end, expected) => {
    expect(formatUrlDateRange(start, end)).toBe(expected)
  })
})

describe('triagedRegressionViewCell', () => {
  test('closed regression links to the historical report', () => {
    expect(triagedRegressionViewCell(closedRegression, '5.0-main', {})).toEqual(
      {
        kind: CellKind.HISTORICAL,
        scope: ClosureScope.REGRESSION,
        url:
          '/sippy-ng/component_readiness/test_details?view=5.0-main&testId=t1' +
          '&sampleStartTime=2026-09-22T00%3A00%3A00Z&sampleEndTime=2026-09-30T00%3A00%3A00Z&regressionId=49971',
        sampleWindow: {
          start: new Date('2026-09-22T00:00:00Z'),
          endExclusive: new Date('2026-09-30T00:00:00Z'),
        },
        tooltip:
          'Closed regression. View historical test details for September 22–29, 2026.',
      }
    )
  })

  test('closure in only one view is labeled as such', () => {
    const cell = triagedRegressionViewCell(closedInMainOnly, '5.0-main', {})
    expect(cell.kind).toBe(CellKind.HISTORICAL)
    expect(cell.scope).toBe(ClosureScope.VIEW)
    expect(cell.tooltip).toBe(
      'Closed in this view. View historical test details for September 22–29, 2026.'
    )
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
      kind: CellKind.CURRENT,
      status: -400,
      explanations: ['regressed'],
      url: '/sippy-ng/component_readiness/test_details?x=1',
    })
  })

  const unavailable = (reason) =>
    `Closed regression. Historical test details are unavailable because ${reason}.`

  test.each([
    [
      'an API reason for this view',
      {
        links: {},
        historical_link_unavailable: {
          '5.0-main': 'no last failure was recorded',
          '5.0-azure': 'this view is no longer configured',
        },
      },
      'no last failure was recorded',
    ],
    [
      'no API reason',
      { links: {} },
      'the API did not provide a historical link',
    ],
    [
      'a link without a readable window',
      {
        links: {
          'test_details:5.0-main':
            '/api/component_readiness/test_details?testId=t1',
        },
        historical_link_unavailable: { '5.0-main': 'ignored' },
      },
      'its link has no readable sample window',
    ],
  ])('closed regression with %s explains why', (_name, change, reason) => {
    expect(
      triagedRegressionViewCell(
        { ...closedRegression, ...change },
        '5.0-main',
        {}
      )
    ).toEqual({
      kind: CellKind.UNAVAILABLE,
      scope: ClosureScope.REGRESSION,
      tooltip: unavailable(reason),
    })
  })

  test('open regression without report data shows nothing', () => {
    const open = {
      ...closedRegression,
      closed: { Valid: false },
      views: [{ view_name: '5.0-main', active: true }],
    }
    expect(triagedRegressionViewCell(open, '5.0-main', {})).toBeNull()
  })

  test('status cells sort current, historical, unavailable, then empty', () => {
    const cells = [
      null,
      { kind: CellKind.UNAVAILABLE },
      { kind: CellKind.HISTORICAL },
      { kind: CellKind.CURRENT, status: 0 },
      { kind: CellKind.CURRENT, status: -400 },
    ]
    const sorted = [...cells].sort(
      (a, b) => triagedRegressionCellRank(a) - triagedRegressionCellRank(b)
    )
    expect(sorted).toEqual([
      { kind: CellKind.CURRENT, status: -400 },
      { kind: CellKind.CURRENT, status: 0 },
      { kind: CellKind.HISTORICAL },
      { kind: CellKind.UNAVAILABLE },
      null,
    ])
  })
})

describe('end time controls', () => {
  test.each([
    ['2026-09-30T00:00:00Z', '2026-09-29 23:59:59'],
    ['2026-09-30 00:00:00', '2026-09-29 23:59:59'],
    ['2026-09-30T02:00:00+02:00', '2026-09-29 23:59:59'],
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
})

describe('validateHistoricalRegression', () => {
  const request = {
    testId: 't1',
    sampleRelease: '5.0',
    variants: { Platform: 'azure', Network: 'ovn', Topology: 'ha' },
    view: '5.0-main',
  }

  test('accepts the matching record', () => {
    expect(validateHistoricalRegression(closedRegression, request)).toEqual({
      valid: true,
      mismatches: [],
      scope: ClosureScope.REGRESSION,
      association: { view_name: '5.0-main', active: false },
    })
  })

  test.each([
    ['test ID', { test_id: 'other' }],
    ['release', { release: '4.22' }],
    ['variants', { variants: ['Platform:aws', 'Network:ovn'] }],
    ['variants', { variants: ['malformed'] }],
    ['view', { views: [{ view_name: '5.0-azure', active: false }] }],
  ])('rejects a mismatched %s', (field, change) => {
    const result = validateHistoricalRegression(
      { ...closedRegression, ...change },
      request
    )
    expect(result.valid).toBe(false)
    expect(result.mismatches).toEqual([field])
  })

  test('reports closure in only this view', () => {
    expect(validateHistoricalRegression(closedInMainOnly, request)).toEqual({
      valid: true,
      mismatches: [],
      scope: ClosureScope.VIEW,
      association: { view_name: '5.0-main', active: false },
    })
  })

  test('reports an open regression in this view', () => {
    expect(
      validateHistoricalRegression(closedInMainOnly, {
        ...request,
        view: '5.0-azure',
      })
    ).toEqual({
      valid: true,
      mismatches: [],
      scope: ClosureScope.OPEN,
      association: null,
    })
  })

  test.each([
    [
      'closed globally',
      { Valid: true, Time: '2026-10-01T00:00:00Z' },
      ClosureScope.REGRESSION,
    ],
    ['open globally', { Valid: false }, ClosureScope.UNKNOWN],
  ])('without a view, regression %s', (_, closed, scope) => {
    const result = validateHistoricalRegression(
      {
        ...closedRegression,
        closed,
        views: [{ view_name: '5.0-main', active: false }],
      },
      { ...request, view: undefined }
    )
    expect(result).toEqual({
      valid: true,
      mismatches: [],
      scope,
      association: null,
    })
  })

  test('handles a missing record', () => {
    expect(validateHistoricalRegression(null, request)).toEqual({
      valid: false,
      mismatches: ['regression'],
      scope: null,
      association: null,
    })
  })
})
