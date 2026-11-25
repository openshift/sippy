import {
  CellKind,
  ClosureScope,
  endTimeControlValue,
  endTimeParam,
  formatHistoricalDateRange,
  formatUrlDateRange,
  historicalClosure,
  historicalWindowFromLink,
  parseEnvironmentVariants,
  parseUrlDate,
  parseVariantPairs,
  preferredTestDetailsLink,
  triagedRegressionCellRank,
  triagedRegressionViewCell,
  triagedRegressionViewNames,
  validateHistoricalRegression,
  viewControlDefaults,
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
      'no last failure',
      { last_failure: { Valid: false }, links: {} },
      'no last failure was recorded',
    ],
    [
      'a removed view or unresolvable basis',
      { links: {} },
      'this view is no longer configured or its basis could not be resolved',
    ],
    [
      'a spot-check test',
      { test_id: 'spotcheck-30d:networking:ovn', links: {} },
      'spot-check reports always cover recent runs',
    ],
    [
      'a link without a readable window',
      {
        links: {
          'test_details:5.0-main':
            '/api/component_readiness/test_details?testId=t1',
        },
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

describe('viewControlDefaults', () => {
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
  const crossCompareView = {
    ...view,
    variant_options: {
      ...view.variant_options,
      variant_cross_compare: ['Architecture'],
      compare_variants: { Architecture: ['arm64'] },
    },
  }

  test('uses the view when the URL has no overrides', () => {
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

  test('lets explicit URL parameters win', () => {
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

  test('includes cross-compare defaults when the URL has neither parameter', () => {
    const defaults = viewControlDefaults(crossCompareView, { view: '5.0-main' })
    expect(defaults.variantCrossCompare).toEqual(['Architecture'])
    expect(defaults.compareVariants).toEqual({ Architecture: ['arm64'] })
  })

  test.each([
    ['compareVariant', { compareVariant: ['Architecture:amd64'] }],
    ['variantCrossCompare', { variantCrossCompare: ['Platform'] }],
  ])(
    'an explicit %s suppresses both cross-compare defaults',
    (_name, urlParams) => {
      const defaults = viewControlDefaults(crossCompareView, {
        view: '5.0-main',
        ...urlParams,
      })
      expect(defaults).not.toHaveProperty('variantCrossCompare')
      expect(defaults).not.toHaveProperty('compareVariants')
      expect(defaults.includeVariants).toEqual({ Platform: ['aws'] })
    }
  )
})

describe('variant parsing', () => {
  test('parseEnvironmentVariants splits key:value pairs', () => {
    expect(parseEnvironmentVariants('Platform:azure Network:ovn')).toEqual({
      Platform: 'azure',
      Network: 'ovn',
    })
    expect(parseEnvironmentVariants(undefined)).toEqual({})
  })

  test('parseVariantPairs skips entries without a key', () => {
    expect(
      parseVariantPairs(['Platform:azure', 'garbage', ':ovn', 'Arch:a:b'])
    ).toEqual({ Platform: 'azure', Arch: 'a:b' })
    expect(parseVariantPairs(undefined)).toEqual({})
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
