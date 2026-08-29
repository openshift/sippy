import '@testing-library/jest-dom'
import { act, render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import ReEvaluateButton from './ReEvaluateSymptoms'
import userEvent from '@testing-library/user-event'

beforeEach(() => {
  vi.restoreAllMocks()
  vi.useFakeTimers({ shouldAdvanceTime: true })
  import.meta.env.VITE_API_URL = ''
})

afterEach(() => {
  vi.useRealTimers()
})

function submitResponse(batchID, requested) {
  return {
    ok: true,
    status: 202,
    json: () =>
      Promise.resolve({
        batch_id: batchID,
        requested,
        links: { status: `/api/jobs/runs/reevaluate/${batchID}` },
      }),
  }
}

function statusResponse(batchID, overrides = {}) {
  const defaults = {
    batch_id: batchID,
    status: 'pending',
    requested: 1,
    enqueued: 1,
    deduped: 0,
    completed: 0,
    failed: 0,
    running: 0,
    pending: 1,
    items: [],
  }
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve({ ...defaults, ...overrides }),
  }
}

function httpErrorResponse(httpStatus, message) {
  return {
    ok: false,
    status: httpStatus,
    json: () => Promise.resolve({ code: httpStatus, message }),
  }
}

describe('ReEvaluateButton', () => {
  it('renders in default state', () => {
    render(<ReEvaluateButton prowJobBuildIDs={['1']} />)
    expect(screen.getByText('Re-evaluate Symptoms')).toBeInTheDocument()
    expect(screen.getByRole('button')).not.toBeDisabled()
  })

  it('is disabled when prowJobBuildIDs is empty', () => {
    render(<ReEvaluateButton prowJobBuildIDs={[]} />)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('is disabled when disabled prop is true', () => {
    render(<ReEvaluateButton prowJobBuildIDs={['1']} disabled={true} />)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('submits a single batch POST with all IDs', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-abc', 3))
      .mockResolvedValue(
        statusResponse('batch-abc', {
          status: 'complete',
          requested: 3,
          completed: 3,
          pending: 0,
        })
      )

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2', '3']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    // Verify the submit call sent all IDs in one request.
    const submitCall = global.fetch.mock.calls[0]
    const body = JSON.parse(submitCall[1].body)
    expect(body.prow_job_build_ids).toEqual(['1', '2', '3'])
    expect(submitCall[1].method).toBe('POST')
  })

  it('shows progress bar during polling', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-1', 2))
      .mockResolvedValueOnce(
        statusResponse('batch-1', {
          status: 'running',
          requested: 2,
          completed: 1,
          failed: 0,
          running: 1,
          pending: 0,
        })
      )
      .mockResolvedValue(
        statusResponse('batch-1', {
          status: 'complete',
          requested: 2,
          completed: 2,
          failed: 0,
          running: 0,
          pending: 0,
        })
      )

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    // After submit, progress bar should appear.
    await waitFor(() => {
      expect(screen.getByRole('progressbar')).toBeInTheDocument()
      expect(screen.getByText(/0\/2 completed/)).toBeInTheDocument()
    })

    // Advance to first poll.
    await act(async () => {
      vi.advanceTimersByTime(2500)
    })

    await waitFor(() => {
      expect(screen.getByText(/1\/2 completed/)).toBeInTheDocument()
    })

    // Advance to second poll (terminal).
    await act(async () => {
      vi.advanceTimersByTime(2500)
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Successfully re-evaluated 2/)
      ).toBeInTheDocument()
    })
  })

  it('shows success snackbar when batch completes', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-ok', 2))
      .mockResolvedValue(
        statusResponse('batch-ok', {
          status: 'complete',
          requested: 2,
          completed: 2,
          failed: 0,
          pending: 0,
        })
      )

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await act(async () => {
      vi.advanceTimersByTime(2500)
    })

    await waitFor(() => {
      expect(
        screen.getByText(
          'Successfully re-evaluated 2 job run(s). Refresh the page to see updated labels.'
        )
      ).toBeInTheDocument()
    })
  })

  it('shows error snackbar when all items fail', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-fail', 2))
      .mockResolvedValue(
        statusResponse('batch-fail', {
          status: 'failed',
          requested: 2,
          completed: 0,
          failed: 2,
          pending: 0,
        })
      )

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await act(async () => {
      vi.advanceTimersByTime(2500)
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Re-evaluation failed for all 2/)
      ).toBeInTheDocument()
    })
  })

  it('shows warning snackbar on partial failure', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-partial', 3))
      .mockResolvedValue(
        statusResponse('batch-partial', {
          status: 'complete',
          requested: 3,
          completed: 2,
          failed: 1,
          pending: 0,
        })
      )

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2', '3']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await act(async () => {
      vi.advanceTimersByTime(2500)
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Re-evaluated 2 job run\(s\)/)
      ).toBeInTheDocument()
      expect(screen.getByText(/1 failed/)).toBeInTheDocument()
    })
  })

  it('shows error snackbar when submit fails', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(httpErrorResponse(503, 'Service Unavailable'))

    render(<ReEvaluateButton prowJobBuildIDs={['1']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Re-evaluation failed: Service Unavailable/)
      ).toBeInTheDocument()
    })
  })

  it('shows force refresh link when forceRefreshURL is provided', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-link', 1))
      .mockResolvedValue(
        statusResponse('batch-link', {
          status: 'complete',
          requested: 1,
          completed: 1,
          failed: 0,
          pending: 0,
        })
      )

    render(
      <ReEvaluateButton prowJobBuildIDs={['1']} forceRefreshURL="/some/url" />
    )

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await act(async () => {
      vi.advanceTimersByTime(2500)
    })

    await waitFor(() => {
      expect(screen.getByText('Reload with fresh data')).toBeInTheDocument()
    })
  })

  it('continues polling until terminal status', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-poll', 3))
      .mockResolvedValueOnce(
        statusResponse('batch-poll', {
          status: 'running',
          requested: 3,
          completed: 0,
          running: 2,
          pending: 1,
        })
      )
      .mockResolvedValueOnce(
        statusResponse('batch-poll', {
          status: 'running',
          requested: 3,
          completed: 1,
          running: 1,
          pending: 1,
        })
      )
      .mockResolvedValue(
        statusResponse('batch-poll', {
          status: 'complete',
          requested: 3,
          completed: 3,
          pending: 0,
        })
      )

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2', '3']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    // First poll: running
    await act(async () => {
      vi.advanceTimersByTime(2500)
    })
    await waitFor(() => {
      expect(screen.getByText(/0\/3 completed/)).toBeInTheDocument()
    })

    // Second poll: 1 completed
    await act(async () => {
      vi.advanceTimersByTime(2500)
    })
    await waitFor(() => {
      expect(screen.getByText(/1\/3 completed/)).toBeInTheDocument()
    })

    // Third poll: terminal
    await act(async () => {
      vi.advanceTimersByTime(2500)
    })
    await waitFor(() => {
      expect(
        screen.getByText(/Successfully re-evaluated 3/)
      ).toBeInTheDocument()
    })
  })

  it('shows cancel button during polling', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-cancel', 2))
      .mockResolvedValue(
        statusResponse('batch-cancel', {
          status: 'running',
          requested: 2,
          completed: 0,
          running: 2,
          pending: 0,
        })
      )

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button', { name: /Re-evaluate/ }))
    })

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    })
  })

  it('cancels batch and shows info snackbar', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-cancel2', 2))
      .mockResolvedValueOnce(
        statusResponse('batch-cancel2', {
          status: 'running',
          requested: 2,
          completed: 1,
          running: 1,
          pending: 0,
        })
      )
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            batch_id: 'batch-cancel2',
            status: 'cancelled',
            requested: 2,
            completed: 1,
            failed: 0,
            running: 0,
            pending: 1,
            items: [],
          }),
      })

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button', { name: /Re-evaluate/ }))
    })

    // Wait for first poll so cancel button appears.
    await act(async () => {
      vi.advanceTimersByTime(2500)
    })

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument()
    })

    // Click cancel.
    await act(async () => {
      userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Batch cancelled\. 1 job run\(s\) completed/)
      ).toBeInTheDocument()
    })

    // Verify the DELETE call was made.
    const deleteCall = global.fetch.mock.calls.find(
      (call) => call[1]?.method === 'DELETE'
    )
    expect(deleteCall).toBeDefined()
    expect(deleteCall[0]).toContain('batch-cancel2')
  })

  it('stops polling and shows error after 5 consecutive poll failures', async () => {
    global.fetch = vi
      .fn()
      .mockResolvedValueOnce(submitResponse('batch-err', 2))
      // All subsequent polls fail.
      .mockRejectedValue(new Error('network error'))

    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button', { name: /Re-evaluate/ }))
    })

    // Advance through 5 poll intervals to trigger 5 consecutive failures.
    for (let i = 0; i < 5; i++) {
      await act(async () => {
        vi.advanceTimersByTime(2500)
      })
    }

    await waitFor(() => {
      expect(
        screen.getByText(
          /Lost connection to batch status after 5 failed attempts/
        )
      ).toBeInTheDocument()
    })

    // Button should be re-enabled after polling stops.
    expect(
      screen.getByRole('button', { name: /Re-evaluate/ })
    ).not.toBeDisabled()
  })
})
