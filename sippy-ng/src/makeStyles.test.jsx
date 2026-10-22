/* eslint-disable react/prop-types */
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { makeStyles, withStyles } from './makeStyles'
import { render, renderHook } from '@testing-library/react'
import React from 'react'

const theme = createTheme()

function wrapper({ children }) {
  return <ThemeProvider theme={theme}>{children}</ThemeProvider>
}

describe('makeStyles', () => {
  describe('static object styles', () => {
    test('returns an object with the declared class keys', () => {
      const useStyles = makeStyles({
        root: { color: 'red' },
        title: { fontSize: 14 },
      })
      const { result } = renderHook(() => useStyles(), { wrapper })
      expect(Object.keys(result.current)).toEqual(['root', 'title'])
    })

    test('each value is a non-empty string class name', () => {
      const useStyles = makeStyles({ root: { color: 'red' } })
      const { result } = renderHook(() => useStyles(), { wrapper })
      expect(typeof result.current.root).toBe('string')
      expect(result.current.root.length).toBeGreaterThan(0)
    })

    test('returns the same object across re-renders (caching)', () => {
      const useStyles = makeStyles({ root: { color: 'red' } })
      const { result, rerender } = renderHook(() => useStyles(), { wrapper })
      const first = result.current
      rerender()
      expect(result.current).toBe(first)
    })
  })

  describe('theme-creator resolution', () => {
    test('receives the MUI theme and resolves styles from it', () => {
      const useStyles = makeStyles((t) => ({
        root: { zIndex: t.zIndex.drawer + 1 },
      }))
      const { result } = renderHook(() => useStyles(), { wrapper })
      expect(typeof result.current.root).toBe('string')
      expect(result.current.root.length).toBeGreaterThan(0)
    })

    test('caches when all resolved values are static', () => {
      const creator = vi.fn((t) => ({
        root: { zIndex: t.zIndex.drawer + 1 },
      }))
      const useStyles = makeStyles(creator)
      const { result, rerender } = renderHook(() => useStyles(), { wrapper })
      const first = result.current
      rerender()
      expect(result.current).toBe(first)
    })
  })

  describe('function-valued styles (dynamic, props-dependent)', () => {
    test('calls function-valued rules with props', () => {
      const styleFn = vi.fn(() => ({ backgroundColor: 'blue' }))
      const useStyles = makeStyles({ root: styleFn })
      const props = { variant: 'filled' }
      renderHook(() => useStyles(props), { wrapper })
      expect(styleFn).toHaveBeenCalledWith(props)
    })

    test('returns a valid class name for function-valued rules', () => {
      const useStyles = makeStyles({
        root: (props) => ({ backgroundColor: props.bg }),
      })
      const { result } = renderHook(() => useStyles({ bg: 'green' }), {
        wrapper,
      })
      expect(typeof result.current.root).toBe('string')
      expect(result.current.root.length).toBeGreaterThan(0)
    })

    test('does not cache when styles contain function-valued rules', () => {
      let callCount = 0
      const useStyles = makeStyles({
        root: () => {
          callCount++
          return { color: 'red' }
        },
      })
      const { rerender } = renderHook(() => useStyles(), { wrapper })
      const countAfterFirst = callCount
      rerender()
      expect(callCount).toBeGreaterThan(countAfterFirst)
    })
  })
})

describe('withStyles', () => {
  test('injects a classes prop with the declared keys', () => {
    let received = null
    function Inner({ classes }) {
      received = classes
      return <div />
    }
    const Styled = withStyles({ root: { color: 'red' }, cell: { padding: 4 } })(
      Inner
    )
    render(
      <ThemeProvider theme={theme}>
        <Styled />
      </ThemeProvider>
    )
    expect(received).toBeDefined()
    expect(Object.keys(received)).toEqual(['root', 'cell'])
    expect(typeof received.root).toBe('string')
    expect(received.root.length).toBeGreaterThan(0)
  })

  test('resolves theme-creator functions', () => {
    let received = null
    function Inner({ classes }) {
      received = classes
      return <div />
    }
    const Styled = withStyles((t) => ({
      root: { zIndex: t.zIndex.drawer + 1 },
    }))(Inner)
    render(
      <ThemeProvider theme={theme}>
        <Styled />
      </ThemeProvider>
    )
    expect(received).toBeDefined()
    expect(typeof received.root).toBe('string')
  })

  test('forwards other props and ref to the wrapped component', () => {
    const ref = React.createRef()
    let received = null
    const Inner = React.forwardRef(function Inner(props, fwdRef) {
      received = props
      return <div ref={fwdRef} data-testid="inner" />
    })
    const Styled = withStyles({ root: { color: 'red' } })(Inner)
    render(
      <ThemeProvider theme={theme}>
        <Styled data-custom="hello" ref={ref} />
      </ThemeProvider>
    )
    expect(received['data-custom']).toBe('hello')
    expect(ref.current).toBeInstanceOf(HTMLDivElement)
  })

  test('sets a descriptive displayName', () => {
    function MyTable() {
      return <div />
    }
    const Styled = withStyles({ root: { color: 'red' } })(MyTable)
    expect(Styled.displayName).toBe('WithStyles(MyTable)')
  })

  test('function-valued rules are stringified, not evaluated with props', () => {
    let received = null
    function Inner({ classes }) {
      received = classes
      return <div />
    }
    const dynamicRule = (props) => ({ backgroundColor: props.bg })
    const Styled = withStyles({ root: dynamicRule })(Inner)
    render(
      <ThemeProvider theme={theme}>
        <Styled bg="green" />
      </ThemeProvider>
    )
    // withStyles passes the function itself to css(), which stringifies it
    // rather than calling it with props. This documents that limitation.
    expect(received).toBeDefined()
    expect(typeof received.root).toBe('string')
  })
})
