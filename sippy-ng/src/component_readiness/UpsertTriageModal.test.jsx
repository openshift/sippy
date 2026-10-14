import '@testing-library/jest-dom'
import { act, render, screen, waitFor } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import React from 'react'
import UpsertTriageModal from './UpsertTriageModal'
import userEvent from '@testing-library/user-event'

vi.mock('./AddRegressionPanel', () => ({
  default: ({ triages }) => (
    <div>
      Triage form loaded: {triages.map(({ description }) => description)}
    </div>
  ),
}))

vi.mock('./UpdateTriagePanel', () => ({
  default: () => <div>Update triage form loaded</div>,
}))

function jsonResponse(body, status = 200) {
  return {
    status,
    json: () => Promise.resolve(body),
  }
}

function deferredResponse() {
  let resolve
  const promise = new Promise((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function renderModal() {
  return render(
    <ThemeProvider theme={createTheme()}>
      <UpsertTriageModal
        regressionIds={[123]}
        fields={['id', 'url', 'description', 'updated_at']}
        setComplete={vi.fn()}
        buttonText="Triage"
      />
    </ThemeProvider>
  )
}

function renderUpdateModal() {
  return render(
    <ThemeProvider theme={createTheme()}>
      <UpsertTriageModal
        triage={{ id: 1, description: 'Existing triage' }}
        setComplete={vi.fn()}
        buttonText="Update"
      />
    </ThemeProvider>
  )
}

beforeEach(() => {
  vi.restoreAllMocks()
  global.fetch = vi.fn()
})

describe('UpsertTriageModal default immediate loading', () => {
  test('opens immediately with the requested accessible loading copy', async () => {
    const request = deferredResponse()
    global.fetch.mockReturnValueOnce(request.promise)
    renderModal()

    await userEvent.click(screen.getByRole('button', { name: 'Triage' }))

    expect(screen.getByRole('dialog', { name: 'Triage' })).toBeInTheDocument()
    expect(
      screen.getByRole('progressbar', {
        name: 'Loading triage information...',
      })
    ).toBeInTheDocument()
    expect(
      screen.getByText('Loading triage information...')
    ).toBeInTheDocument()
    expect(screen.queryByText('Triage form loaded')).not.toBeInTheDocument()
    expect(global.fetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/component_readiness/triages?fields=id%2Curl%2Cdescription%2Cupdated_at'
    )
  })

  test('replaces the loading indicator with the existing form on success', async () => {
    const request = deferredResponse()
    global.fetch.mockReturnValueOnce(request.promise)
    renderModal()

    await userEvent.click(screen.getByRole('button', { name: 'Triage' }))
    expect(screen.queryByText('Triage form loaded')).not.toBeInTheDocument()

    await act(async () => {
      request.resolve(
        jsonResponse([
          {
            id: 1,
            url: 'https://issues.example.test/1',
            description: 'Existing triage',
            updated_at: '2026-10-14T00:00:00Z',
          },
        ])
      )
    })

    expect(
      screen.getByText('Triage form loaded: Existing triage')
    ).toBeInTheDocument()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  })

  test('does not reopen after being closed while the request is pending', async () => {
    const request = deferredResponse()
    global.fetch.mockReturnValueOnce(request.promise)
    renderModal()

    await userEvent.click(screen.getByRole('button', { name: 'Triage' }))
    await userEvent.click(screen.getByRole('button', { name: 'CLOSE' }))

    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await act(async () => {
      request.resolve(jsonResponse([]))
    })

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByText('Triage form loaded')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Triage' })).toBeInTheDocument()
  })

  test('ignores an earlier success after closing and reopening', async () => {
    const firstRequest = deferredResponse()
    const secondRequest = deferredResponse()
    global.fetch
      .mockReturnValueOnce(firstRequest.promise)
      .mockReturnValueOnce(secondRequest.promise)
    renderModal()

    await userEvent.click(screen.getByRole('button', { name: 'Triage' }))
    await userEvent.click(screen.getByRole('button', { name: 'CLOSE' }))
    await userEvent.click(screen.getByRole('button', { name: 'Triage' }))

    await act(async () => {
      firstRequest.resolve(
        jsonResponse([{ id: 1, description: 'Stale triage' }])
      )
    })

    expect(screen.getByRole('dialog', { name: 'Triage' })).toBeInTheDocument()
    expect(
      screen.getByRole('progressbar', {
        name: 'Loading triage information...',
      })
    ).toBeInTheDocument()
    expect(screen.queryByText(/Stale triage/)).not.toBeInTheDocument()

    await act(async () => {
      secondRequest.resolve(
        jsonResponse([{ id: 2, description: 'Current triage' }])
      )
    })

    expect(
      screen.getByText('Triage form loaded: Current triage')
    ).toBeInTheDocument()
    expect(screen.queryByText(/Stale triage/)).not.toBeInTheDocument()
  })

  test('ignores an earlier failure after closing and reopening', async () => {
    const firstRequest = deferredResponse()
    const secondRequest = deferredResponse()
    global.fetch
      .mockReturnValueOnce(firstRequest.promise)
      .mockReturnValueOnce(secondRequest.promise)
    renderModal()

    await userEvent.click(screen.getByRole('button', { name: 'Triage' }))
    await userEvent.click(screen.getByRole('button', { name: 'CLOSE' }))
    await userEvent.click(screen.getByRole('button', { name: 'Triage' }))

    await act(async () => {
      firstRequest.resolve(jsonResponse({}, 500))
    })

    expect(screen.getByRole('dialog', { name: 'Triage' })).toBeInTheDocument()
    expect(
      screen.getByRole('progressbar', {
        name: 'Loading triage information...',
      })
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Error retrieving existing triage records')
    ).not.toBeInTheDocument()

    await act(async () => {
      secondRequest.resolve(
        jsonResponse([{ id: 2, description: 'Current triage' }])
      )
    })

    expect(
      screen.getByText('Triage form loaded: Current triage')
    ).toBeInTheDocument()
  })

  test('closes the loading modal and preserves the error notification', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    global.fetch.mockResolvedValueOnce(jsonResponse({}, 500))
    renderModal()

    await userEvent.click(screen.getByRole('button', { name: 'Triage' }))

    expect(
      await screen.findByText('Error retrieving existing triage records')
    ).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByText('Triage form loaded')).not.toBeInTheDocument()
  })

  test('opens an update form directly without fetching the triage list', async () => {
    renderUpdateModal()

    await userEvent.click(screen.getByRole('button', { name: 'Update' }))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Update triage form loaded')).toBeInTheDocument()
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    expect(global.fetch).not.toHaveBeenCalled()
  })
})
