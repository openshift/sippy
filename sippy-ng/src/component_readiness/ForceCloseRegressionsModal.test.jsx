import '@testing-library/jest-dom'
import { act, render, screen, waitFor } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { SippyCapabilitiesContext } from '../App'
import ForceCloseRegressionsModal from './ForceCloseRegressionsModal'
import React from 'react'
import userEvent from '@testing-library/user-event'

vi.mock('../App', async () => {
  const ReactModule = await import('react')
  return { SippyCapabilitiesContext: ReactModule.createContext([]) }
})

const preview = {
  triage_id: 42,
  resolved: '2026-08-20T12:00:00Z',
  would_close: [
    {
      regression_id: 101,
      test_name: 'eligible test',
      last_failure_before_resolution: '2026-08-20T11:00:00Z',
    },
  ],
  would_not_close: [
    {
      regression_id: 202,
      test_name: 'later test',
      first_failure_after_resolution: '2026-08-20T13:00:00Z',
    },
  ],
}

function jsonResponse(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  }
}

function renderModal({
  capabilities = ['write_endpoints'],
  resolved = true,
  setIsUpdated = vi.fn(),
} = {}) {
  return {
    setIsUpdated,
    ...render(
      <ThemeProvider theme={createTheme()}>
        <SippyCapabilitiesContext.Provider value={capabilities}>
          <ForceCloseRegressionsModal
            triageId={42}
            resolved={resolved}
            setIsUpdated={setIsUpdated}
          />
        </SippyCapabilitiesContext.Provider>
      </ThemeProvider>
    ),
  }
}

async function openModal() {
  await userEvent.click(
    screen.getByRole('button', { name: 'Force Close Regressions' })
  )
}

beforeEach(() => {
  vi.restoreAllMocks()
  global.fetch = vi.fn()
})

describe('ForceCloseRegressionsModal', () => {
  test('is hidden without the write capability', () => {
    renderModal({ capabilities: [] })

    expect(
      screen.queryByRole('button', { name: 'Force Close Regressions' })
    ).not.toBeInTheDocument()
    expect(global.fetch).not.toHaveBeenCalled()
  })

  test('is disabled for an unresolved triage', () => {
    renderModal({ resolved: false })

    expect(
      screen.getByRole('button', { name: 'Force Close Regressions' })
    ).toBeDisabled()
    expect(global.fetch).not.toHaveBeenCalled()
  })

  test('shows loading then renders preview rows using the regression_id shape', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    let resolvePreview
    global.fetch.mockReturnValueOnce(
      new Promise((resolve) => {
        resolvePreview = resolve
      })
    )
    renderModal()

    await openModal()
    expect(screen.getByRole('progressbar')).toBeInTheDocument()

    await act(async () => {
      resolvePreview(jsonResponse(preview))
    })

    expect(await screen.findByText('eligible test')).toBeInTheDocument()
    expect(screen.getByText('later test')).toBeInTheDocument()
    expect(screen.getByText('Would Close (1)')).toBeInTheDocument()
    expect(screen.getByText('Would Not Close (1)')).toBeInTheDocument()
    expect(global.fetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/component_readiness/triages/42/force_close_preview',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect(consoleError.mock.calls.flat().join(' ')).not.toContain(
      'unique "key" prop'
    )
  })

  test('renders the empty preview state and disables submission', async () => {
    global.fetch.mockResolvedValueOnce(
      jsonResponse({ ...preview, would_close: [], would_not_close: [] })
    )
    renderModal()

    await openModal()

    expect(
      await screen.findByText('There are no regressions to force close.')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Force Close' })).toBeDisabled()
    expect(screen.queryByRole('textbox', { name: /Reason/ })).toBeNull()
  })

  test('renders a preview API error', async () => {
    global.fetch.mockResolvedValueOnce(
      jsonResponse({ code: 500, message: 'preview failed' }, 500)
    )
    renderModal()

    await openModal()

    expect(
      await screen.findByText('Error: 500: preview failed')
    ).toBeInTheDocument()
  })

  test('requires a non-blank reason before submission', async () => {
    global.fetch.mockResolvedValueOnce(jsonResponse(preview))
    renderModal()

    await openModal()
    await screen.findByRole('textbox', { name: /Reason/ })
    await userEvent.click(screen.getByRole('button', { name: 'Force Close' }))

    expect(
      screen.getByText('A reason is required to force close regressions')
    ).toBeInTheDocument()
    expect(global.fetch).toHaveBeenCalledTimes(1)
  })

  test('submits a trimmed reason and refreshes after success', async () => {
    const setIsUpdated = vi.fn()
    global.fetch
      .mockResolvedValueOnce(jsonResponse(preview))
      .mockResolvedValueOnce(
        jsonResponse({ closed_regression_ids: [101], timestamp: '2026-08-20' })
      )
    renderModal({ setIsUpdated })

    await openModal()
    const reason = await screen.findByRole('textbox', { name: /Reason/ })
    await userEvent.type(reason, '  stale regression  ')
    await userEvent.click(screen.getByRole('button', { name: 'Force Close' }))

    await waitFor(() => expect(setIsUpdated).toHaveBeenCalledWith(true))
    expect(global.fetch).toHaveBeenNthCalledWith(
      2,
      'http://localhost:8080/api/component_readiness/triages/42/force_close_regressions',
      {
        method: 'POST',
        body: JSON.stringify({ reason: 'stale regression' }),
      }
    )
    expect(
      screen.getByText('Successfully force closed 1 regression')
    ).toBeInTheDocument()
  })

  test('reports a failed submission without refreshing', async () => {
    const setIsUpdated = vi.fn()
    global.fetch
      .mockResolvedValueOnce(jsonResponse(preview))
      .mockResolvedValueOnce(
        jsonResponse({ code: 409, message: 'close failed' }, 409)
      )
    renderModal({ setIsUpdated })

    await openModal()
    await userEvent.type(
      await screen.findByRole('textbox', { name: /Reason/ }),
      'because'
    )
    await userEvent.click(screen.getByRole('button', { name: 'Force Close' }))

    expect(
      await screen.findByText(
        'Error force closing regressions: Error: 409: close failed'
      )
    ).toBeInTheDocument()
    expect(setIsUpdated).not.toHaveBeenCalled()
  })
})
