//import HelloWorld from './HelloWorld'
import {
  convertApiUrlToUiUrl,
  dateEndFormat,
  dateFormat,
  formatLongDate,
  generateTestDetailsReportLink,
  getTestDetailsLink,
} from './CompReadyUtils'

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
