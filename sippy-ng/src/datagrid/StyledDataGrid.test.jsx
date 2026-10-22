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

    expect(normal['& .row-percent-0'].backgroundColor).toBe(
      theme.palette.error.light
    )
    expect(normal['& .row-percent-100'].backgroundColor).toBe(
      theme.palette.success.light
    )
    expect(inverted['& .row-percent-0'].backgroundColor).toBe(
      theme.palette.success.light
    )
    expect(inverted['& .row-percent-100'].backgroundColor).toBe(
      theme.palette.error.light
    )
  })

  it('maps descending threshold domains to correct colors', () => {
    const descending = { error: 80, warning: 60, success: 0 }
    const styles = buildColoredStyles(descending, false, theme)

    expect(styles['& .row-percent-0'].backgroundColor).toBe(
      theme.palette.success.light
    )
    expect(styles['& .row-percent-80'].backgroundColor).toBe(
      theme.palette.error.light
    )
    expect(styles['& .row-percent-100']).toBeDefined()
  })

  it('throws on non-finite thresholds', () => {
    expect(() =>
      buildColoredStyles({ error: 0, warning: NaN, success: 80 }, false, theme)
    ).toThrow('thresholds.warning must be a finite number')

    expect(() =>
      buildColoredStyles(
        { error: 0, warning: 60, success: undefined },
        false,
        theme
      )
    ).toThrow('thresholds.success must be a finite number')

    expect(() =>
      buildColoredStyles(
        { error: null, warning: 60, success: 80 },
        false,
        theme
      )
    ).toThrow('thresholds.error must be a finite number')
  })

  it('produces valid hex color strings', () => {
    const styles = buildColoredStyles(thresholds, false, theme)
    const hexPattern = /^#[0-9a-f]{6}$/i
    for (let i = 0; i <= 100; i++) {
      expect(styles[`& .row-percent-${i}`].backgroundColor).toMatch(hexPattern)
    }
  })
})
