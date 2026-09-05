import '@testing-library/jest-dom'
import { CompReadyVarsContext } from './CompReadyVars'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { render, screen, waitFor } from '@testing-library/react'
import { SippyCapabilitiesContext } from '../App'
import React from 'react'
import Triage from './Triage'
import userEvent from '@testing-library/user-event'

const mocks = vi.hoisted(() => ({
  forceClose: vi.fn(),
  setPageContextForChat: vi.fn(),
  unsetPageContextForChat: vi.fn(),
}))

vi.mock('../App', async () => {
  const ReactModule = await import('react')
  return {
    ReleasesContext: ReactModule.createContext({}),
    SippyCapabilitiesContext: ReactModule.createContext([]),
  }
})
vi.mock('../chat/store/useChatStore', () => ({
  usePageContextForChat: () => ({
    setPageContextForChat: mocks.setPageContextForChat,
    unsetPageContextForChat: mocks.unsetPageContextForChat,
  }),
}))
vi.mock('../chat/AskSippyButton', () => ({ default: () => null }))
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
        <CompReadyVarsContext.Provider
          value={{ sampleRelease: '4.20', view: '4.20-main' }}
        >
          <Triage id="42" />
        </CompReadyVarsContext.Provider>
      </SippyCapabilitiesContext.Provider>
    </ThemeProvider>
  )
}

beforeEach(() => {
  vi.restoreAllMocks()
  mocks.forceClose.mockClear()
  mocks.setPageContextForChat.mockClear()
  mocks.unsetPageContextForChat.mockClear()
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
