import { buildColoredStyles } from './StyledDataGrid'
import { createTheme } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'

const theme = createTheme()

const thresholds = { error: 0, warning: 60, success: 80 }

describe('buildColoredStyles', () => {
  it('generates row-percent-0 through row-percent-100', () => {
    const styles = buildColoredStyles(thresholds, false, theme)
    for (let i = 0; i <= 100; i++) {
      expect(styles[`& .row-percent-${i}`]).toBeDefined()
      expect(styles[`& .row-percent-${i}`].backgroundColor).toBeTruthy()
    }
  })

  it('does not generate row-percent-101', () => {
    const styles = buildColoredStyles(thresholds, false, theme)
    expect(styles['& .row-percent-101']).toBeUndefined()
  })

  it('includes the overall rule with correct filter and border', () => {
    const styles = buildColoredStyles(thresholds, false, theme)
    expect(styles['& .overall']).toEqual({
      filter: 'brightness(80%) contrast(150%)',
      borderBottom: '5px solid black',
    })
  })

  it('swaps endpoint colors when invert is true', () => {
    const normal = buildColoredStyles(thresholds, false, theme)
    const inverted = buildColoredStyles(thresholds, true, theme)

    const normalFirst = normal['& .row-percent-0'].backgroundColor
    const invertedFirst = inverted['& .row-percent-0'].backgroundColor
    const normalLast = normal['& .row-percent-100'].backgroundColor
    const invertedLast = inverted['& .row-percent-100'].backgroundColor

    expect(normalFirst).not.toBe(invertedFirst)
    expect(normalLast).not.toBe(invertedLast)
  })

  it('does not throw with descending threshold domains', () => {
    const descending = { error: 80, warning: 60, success: 0 }
    expect(() => buildColoredStyles(descending, false, theme)).not.toThrow()
    const styles = buildColoredStyles(descending, false, theme)
    expect(styles['& .row-percent-0']).toBeDefined()
    expect(styles['& .row-percent-100']).toBeDefined()
  })

  it('produces valid hex color strings', () => {
    const styles = buildColoredStyles(thresholds, false, theme)
    const hexPattern = /^#[0-9a-f]{6}$/i
    for (let i = 0; i <= 100; i++) {
      expect(styles[`& .row-percent-${i}`].backgroundColor).toMatch(hexPattern)
    }
  })
})
