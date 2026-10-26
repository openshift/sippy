import '@testing-library/jest-dom'
import { act, render, screen, waitFor } from '@testing-library/react'
import { CompReadyVarsContext } from './CompReadyVars'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { SippyCapabilitiesContext } from '../App'
import React from 'react'
import Triage from './Triage'
import userEvent from '@testing-library/user-event'

const mocks = vi.hoisted(() => ({
  forceClose: vi.fn(),
}))

vi.mock('../App', async () => {
  const ReactModule = await import('react')
  return {
    ReleasesContext: ReactModule.createContext({}),
    SippyCapabilitiesContext: ReactModule.createContext([]),
  }
})
vi.mock('../components/Laundry', () => ({
  default: ({ children }) => <span>{children}</span>,
}))
vi.mock('./CompSeverityIcon', () => ({ default: () => null }))
vi.mock('./TriageAuditLogsModal', () => ({ default: () => null }))
vi.mock('./TriagePotentialMatches', () => ({ default: () => null }))
vi.mock('./TriageSymptomLabels', () => ({
  default: () => null,
  aggregateLabelSummaries: vi.fn(() => []),
}))
vi.mock('./TriagedRegressionTestList', () => ({ default: () => null }))
vi.mock('./UpsertTriageModal', () => ({ default: () => null }))
vi.mock('./ForceCloseRegressionsModal', () => ({
  default: (props) => {
    mocks.forceClose(props)
    return (
      <button
        data-testid="force-close-modal"
        onClick={() => props.setIsUpdated(true)}
      >
        refresh
      </button>
    )
  },
}))

const triage = {
  id: 42,
  description: 'test triage',
  resolved: { Valid: true, Time: '2026-08-20T12:00:00Z' },
  regressions: [],
  regressed_tests: {},
  links: { self: 'http://localhost:8080/api/component_readiness/triages/42' },
  bug: {},
}

function renderTriage(capabilities = ['local_db', 'write_endpoints']) {
  return render(
    <ThemeProvider theme={createTheme()}>
      <SippyCapabilitiesContext.Provider value={capabilities}>
        <CompReadyVarsContext.Provider value={{ sampleRelease: '4.20' }}>
          <Triage id="42" />
        </CompReadyVarsContext.Provider>
      </SippyCapabilitiesContext.Provider>
    </ThemeProvider>
  )
}

beforeEach(() => {
  vi.restoreAllMocks()
  mocks.forceClose.mockClear()
  global.fetch = vi.fn(() =>
    Promise.resolve({
      status: 200,
      json: () => Promise.resolve(triage),
    })
  )
})

describe('Triage force-close integration', () => {
  test('passes the loaded triage and refresh callback to the modal', async () => {
    renderTriage()

    await screen.findByTestId('force-close-modal')
    const props = mocks.forceClose.mock.calls.at(-1)[0]
    expect(props.triageId).toBe(42)
    expect(props.resolved).toBe(true)
    expect(props.setIsUpdated).toEqual(expect.any(Function))
  })

  test('refreshes the triage when the modal reports an update', async () => {
    renderTriage()

    await userEvent.click(await screen.findByTestId('force-close-modal'))

    await waitFor(() =>
      expect(global.fetch.mock.calls.length).toBeGreaterThan(1)
    )
  })

  test('does not integrate the modal without the write capability', async () => {
    renderTriage(['local_db'])

    await screen.findByText('Triage Details')
    expect(screen.queryByTestId('force-close-modal')).not.toBeInTheDocument()
  })
})

describe('Triage delete', () => {
  test('ignores a delete error after the triage id changes', async () => {
    let rejectDelete
    global.fetch = vi.fn((url, options = {}) => {
      if (options.method === 'DELETE') {
        return new Promise((_, reject) => {
          rejectDelete = reject
        })
      }
      const recordId = String(url).includes('/triages/43') ? 43 : 42
      return Promise.resolve({
        status: 200,
        json: () =>
          Promise.resolve({
            ...triage,
            id: recordId,
            description: `triage ${recordId}`,
          }),
      })
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const page = (id) => (
      <ThemeProvider theme={createTheme()}>
        <SippyCapabilitiesContext.Provider
          value={['local_db', 'write_endpoints']}
        >
          <CompReadyVarsContext.Provider value={{ sampleRelease: '4.20' }}>
            <Triage id={id} />
          </CompReadyVarsContext.Provider>
        </SippyCapabilitiesContext.Provider>
      </ThemeProvider>
    )

    const view = render(page('42'))
    await userEvent.click(await screen.findByTestId('delete-triage'))

    view.rerender(page('43'))
    await screen.findByText('triage 43')

    await act(async () => {
      rejectDelete(new Error('API server returned 400'))
    })

    expect(
      screen.queryByText(/API server returned 400/)
    ).not.toBeInTheDocument()
  })

  test('ignores a completed delete after the triage id changes', async () => {
    let resolveDelete
    global.fetch = vi.fn((url, options = {}) => {
      if (options.method === 'DELETE') {
        return new Promise((resolve) => {
          resolveDelete = () => resolve({ status: 200 })
        })
      }
      const recordId = String(url).includes('/triages/43') ? 43 : 42
      return Promise.resolve({
        status: 200,
        json: () =>
          Promise.resolve({
            ...triage,
            id: recordId,
            description: `triage ${recordId}`,
          }),
      })
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const page = (id) => (
      <ThemeProvider theme={createTheme()}>
        <SippyCapabilitiesContext.Provider
          value={['local_db', 'write_endpoints']}
        >
          <CompReadyVarsContext.Provider value={{ sampleRelease: '4.20' }}>
            <Triage id={id} />
          </CompReadyVarsContext.Provider>
        </SippyCapabilitiesContext.Provider>
      </ThemeProvider>
    )

    const view = render(page('42'))
    await userEvent.click(await screen.findByTestId('delete-triage'))

    view.rerender(page('43'))
    await screen.findByText('triage 43')

    await act(async () => {
      resolveDelete()
    })

    expect(
      screen.queryByText('Triage record has been deleted.')
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'Triage Details' })
    ).toBeVisible()
  })
})

describe('Triage load', () => {
  function page(id) {
    return (
      <ThemeProvider theme={createTheme()}>
        <SippyCapabilitiesContext.Provider
          value={['local_db', 'write_endpoints']}
        >
          <CompReadyVarsContext.Provider value={{ sampleRelease: '4.20' }}>
            <Triage id={id} />
          </CompReadyVarsContext.Provider>
        </SippyCapabilitiesContext.Provider>
      </ThemeProvider>
    )
  }

  test('drops the previous record while the next id is loading', async () => {
    const pending = []
    global.fetch = vi.fn((url) => {
      const recordId = String(url).includes('/triages/43') ? 43 : 42
      return new Promise((resolve) => {
        pending.push({ recordId, resolve })
      })
    })

    const fulfill = async (recordId, description) => {
      const entry = pending.find(
        (item) => item.recordId === recordId && !item.fulfilled
      )
      entry.fulfilled = true
      await act(async () => {
        entry.resolve({
          status: 200,
          json: () =>
            Promise.resolve({
              ...triage,
              id: recordId,
              description,
            }),
        })
        await Promise.resolve()
        await Promise.resolve()
      })
    }

    const view = render(page('42'))
    await fulfill(42, 'triage 42')
    expect(await screen.findByText('triage 42')).toBeInTheDocument()

    view.rerender(page('43'))
    expect(screen.queryByText('triage 42')).not.toBeInTheDocument()
    expect(screen.getByText('Loading...')).toBeInTheDocument()

    await fulfill(43, 'triage 43')
    expect(await screen.findByText('triage 43')).toBeInTheDocument()

    view.rerender(page('42'))
    view.rerender(page('43'))
    await fulfill(43, 'triage 43')
    expect(await screen.findByText('triage 43')).toBeInTheDocument()

    await fulfill(42, 'late triage 42')
    expect(screen.queryByText('late triage 42')).not.toBeInTheDocument()
    expect(screen.getByText('triage 43')).toBeInTheDocument()
  })
})
