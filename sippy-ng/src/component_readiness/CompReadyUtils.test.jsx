//import HelloWorld from './HelloWorld'
import {
  compareViewNames,
  convertApiUrlToUiUrl,
  createLatestRequest,
  dateEndFormat,
  dateFormat,
  formatLongDate,
  generateTestDetailsReportLink,
  getForceClosePreviewUrl,
  getForceCloseRegressionsUrl,
  getTestDetailsLink,
  getTriagesAPIUrl,
  parseEnvironmentVariants,
  parseVariantPairs,
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

test('compareViewNames lists main views first, then alphabetically', () => {
  expect(
    ['5.0-zeta', '4.22-main', '5.0-azure', '5.0-main'].sort(compareViewNames)
  ).toEqual(['4.22-main', '5.0-main', '5.0-azure', '5.0-zeta'])
})
