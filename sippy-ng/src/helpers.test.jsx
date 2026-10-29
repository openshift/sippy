import { ageColor, jiraProjectForRelease } from './helpers'

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
