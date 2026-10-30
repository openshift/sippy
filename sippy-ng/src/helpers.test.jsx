import {
  ageColor,
  jiraProjectForRelease,
  pathForExactTestAnalysisWithVariants,
  pathForJobRunsWithTestFailureHistory,
  pathForJobRunsWithTestHistory,
  safeEncodeURIComponent,
} from './helpers'

const lightTheme = {
  palette: {
    mode: 'light',
    warning: { dark: '#f57c00' },
    error: { dark: '#d32f2f' },
  },
}

const darkTheme = {
  palette: {
    mode: 'dark',
    warning: { dark: '#f57c00' },
    error: { dark: '#d32f2f', light: '#e57373' },
  },
}

describe('ageColor', () => {
  test('returns undefined for null date', () => {
    expect(ageColor(null, lightTheme)).toBeUndefined()
  })

  test('returns undefined for date less than 7 days ago', () => {
    const recent = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000)
    expect(ageColor(recent, lightTheme)).toBeUndefined()
  })

  test('returns WCAG-compliant warning color in light mode for 7-13 days', () => {
    const sevenDays = new Date(Date.now() - 10 * 24 * 60 * 60 * 1000)
    expect(ageColor(sevenDays, lightTheme)).toBe('#9a6700')
  })

  test('returns theme warning color in dark mode for 7-13 days', () => {
    const sevenDays = new Date(Date.now() - 10 * 24 * 60 * 60 * 1000)
    expect(ageColor(sevenDays, darkTheme)).toBe('#f57c00')
  })

  test('returns dark error color in light mode for 14+ days', () => {
    const twoWeeks = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000)
    expect(ageColor(twoWeeks, lightTheme)).toBe('#d32f2f')
  })

  test('returns light error color in dark mode for 14+ days', () => {
    const twoWeeks = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000)
    expect(ageColor(twoWeeks, darkTheme)).toBe('#e57373')
  })

  test('returns warning at exactly 7 days', () => {
    const exactlySeven = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)
    expect(ageColor(exactlySeven, lightTheme)).toBe('#9a6700')
  })

  test('returns error at exactly 14 days', () => {
    const exactlyFourteen = new Date(Date.now() - 14 * 24 * 60 * 60 * 1000)
    expect(ageColor(exactlyFourteen, lightTheme)).toBe('#d32f2f')
  })
})

describe('jiraProjectForRelease', () => {
  test.each([
    ['quay-3.18', { key: 'PROJQUAY', pid: '10217' }],
    ['4.22', { key: 'OCPBUGS', pid: '10325' }],
    ['rosa-4.22', { key: 'OCPBUGS', pid: '10325' }],
    [undefined, { key: 'OCPBUGS', pid: '10325' }],
  ])('%s -> %o', (release, expected) => {
    expect(jiraProjectForRelease(release)).toEqual(expected)
  })
})

describe('pathForExactTestAnalysisWithVariants', () => {
  const testName =
    '[sig-network][OCPFeatureGate:NetworkSegmentation] pod connected [Suite:openshift/conformance/parallel]'

  test('generates path with all variant filters and exclusions', () => {
    const variants = [
      'Architecture:amd64',
      'Platform:gcp',
      'Network:ovn',
      'Suite:unknown',
      'Upgrade:none',
    ]
    const path = pathForExactTestAnalysisWithVariants('5.0', testName, variants)

    expect(path).toContain('/tests/5.0/analysis?')
    expect(path).toContain(`test=${safeEncodeURIComponent(testName)}`)

    const filtersParam = new URLSearchParams(path.split('?')[1]).get('filters')
    const filters = JSON.parse(filtersParam)

    expect(filters.logicOperator).toBe('and')

    const nameFilter = filters.items.find(
      (f) => f.field === 'name' && f.operator === 'equals'
    )
    expect(nameFilter.value).toBe(testName)

    const qualifiedAggregatedExclusion = filters.items.find(
      (f) => f.value === 'Aggregation:aggregated' && f.not === true
    )
    expect(qualifiedAggregatedExclusion).toBeDefined()

    const bareAggregatedExclusion = filters.items.find(
      (f) => f.value === 'aggregated' && f.not === true
    )
    expect(bareAggregatedExclusion).toBeDefined()

    const neverStableExclusion = filters.items.find(
      (f) => f.value === 'never-stable' && f.not === true
    )
    expect(neverStableExclusion).toBeDefined()

    variants.forEach((variant) => {
      const found = filters.items.find(
        (f) =>
          f.field === 'variants' &&
          f.operator === 'has entry' &&
          f.value === variant &&
          !f.not
      )
      expect(found).toBeDefined()
    })
  })

  test('falls back to analysis without variant filters when variants are empty', () => {
    const path = pathForExactTestAnalysisWithVariants('5.0', testName, [])

    const filtersParam = new URLSearchParams(path.split('?')[1]).get('filters')
    const filters = JSON.parse(filtersParam)

    const variantPositive = filters.items.filter(
      (f) => f.field === 'variants' && !f.not
    )
    expect(variantPositive).toHaveLength(0)

    expect(
      filters.items.find((f) => f.value === 'Aggregation:aggregated' && f.not)
    ).toBeDefined()
    expect(
      filters.items.find((f) => f.value === 'aggregated' && f.not)
    ).toBeDefined()
    expect(
      filters.items.find((f) => f.value === 'never-stable' && f.not)
    ).toBeDefined()
  })

  test('falls back when variants is undefined', () => {
    const path = pathForExactTestAnalysisWithVariants(
      '5.0',
      testName,
      undefined
    )

    const filtersParam = new URLSearchParams(path.split('?')[1]).get('filters')
    const filters = JSON.parse(filtersParam)

    const variantPositive = filters.items.filter(
      (f) => f.field === 'variants' && !f.not
    )
    expect(variantPositive).toHaveLength(0)
  })

  test('preserves special characters in test names', () => {
    const special = 'test [brackets] & ampersand + plus % percent / slash'
    const path = pathForExactTestAnalysisWithVariants('4.16', special, [])

    expect(path).toContain(`test=${safeEncodeURIComponent(special)}`)

    const filtersParam = new URLSearchParams(path.split('?')[1]).get('filters')
    const filters = JSON.parse(filtersParam)
    expect(filters.items[0].value).toBe(special)
  })

  test('uses the row release, not a hardcoded one', () => {
    const path = pathForExactTestAnalysisWithVariants('4.16', testName, [])
    expect(path).toMatch(/^\/tests\/4\.16\/analysis\?/)
  })
})

describe('pathForJobRunsWithTestHistory', () => {
  test('generates path without timestamp filter', () => {
    const path = pathForJobRunsWithTestHistory('5.0', 'my-test', {
      items: [
        { field: 'variants', operator: 'has entry', value: 'Platform:gcp' },
      ],
    })

    expect(path).toContain('/jobs/5.0/runs?')

    const filtersParam = new URLSearchParams(path.split('?')[1]).get('filters')
    const filters = JSON.parse(filtersParam)

    expect(
      filters.items.find((f) => f.field === 'ran_test_names')
    ).toBeDefined()
    expect(filters.items.find((f) => f.field === 'variants')).toBeDefined()
    expect(filters.items.find((f) => f.field === 'timestamp')).toBeUndefined()
  })
})

describe('pathForJobRunsWithTestFailureHistory', () => {
  test('generates path without timestamp filter', () => {
    const path = pathForJobRunsWithTestFailureHistory('5.0', 'my-test', {
      items: [
        { field: 'variants', operator: 'has entry', value: 'Network:ovn' },
      ],
    })

    const filtersParam = new URLSearchParams(path.split('?')[1]).get('filters')
    const filters = JSON.parse(filtersParam)

    expect(
      filters.items.find((f) => f.field === 'failed_test_names')
    ).toBeDefined()
    expect(filters.items.find((f) => f.field === 'variants')).toBeDefined()
    expect(filters.items.find((f) => f.field === 'timestamp')).toBeUndefined()
  })
})
