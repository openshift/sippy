import { ageColor, jiraProjectForRelease } from './helpers'

const mockTheme = {
  palette: {
    warning: { main: '#ff9800' },
    error: { main: '#f44336' },
  },
}

describe('ageColor', () => {
  test('returns undefined for null date', () => {
    expect(ageColor(null, mockTheme)).toBeUndefined()
  })

  test('returns undefined for date less than 7 days ago', () => {
    const recent = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000)
    expect(ageColor(recent, mockTheme)).toBeUndefined()
  })

  test('returns warning color for date 7-13 days ago', () => {
    const sevenDays = new Date(Date.now() - 10 * 24 * 60 * 60 * 1000)
    expect(ageColor(sevenDays, mockTheme)).toBe('#ff9800')
  })

  test('returns error color for date 14+ days ago', () => {
    const twoWeeks = new Date(Date.now() - 20 * 24 * 60 * 60 * 1000)
    expect(ageColor(twoWeeks, mockTheme)).toBe('#f44336')
  })

  test('returns warning at exactly 7 days', () => {
    const exactlySeven = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)
    expect(ageColor(exactlySeven, mockTheme)).toBe('#ff9800')
  })

  test('returns error at exactly 14 days', () => {
    const exactlyFourteen = new Date(Date.now() - 14 * 24 * 60 * 60 * 1000)
    expect(ageColor(exactlyFourteen, mockTheme)).toBe('#f44336')
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
