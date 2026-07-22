import '@testing-library/jest-dom'
import { act, render, screen, waitFor } from '@testing-library/react'
import React from 'react'
import ReEvaluateButton from './ReEvaluateSymptoms'
import userEvent from '@testing-library/user-event'

beforeEach(() => {
  jest.restoreAllMocks()
  import.meta.env.VITE_API_URL = ''
})

function mockFetchResponses(...responses) {
  const iter = responses[Symbol.iterator]()
  global.fetch = jest.fn(() => {
    const next = iter.next()
    if (next.done) throw new Error('unexpected extra fetch call')
    return Promise.resolve(next.value)
  })
}

function successResponse(buildID) {
  return {
    ok: true,
    status: 200,
    json: () =>
      Promise.resolve({
        results: [{ prow_job_build_id: buildID, status: 'success' }],
      }),
  }
}

function errorResponse(buildID, status) {
  return {
    ok: true,
    status: 200,
    json: () =>
      Promise.resolve({
        results: [{ prow_job_build_id: buildID, status }],
      }),
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

  it('shows progress bar during execution', async () => {
    let resolveSecond
    const deferredSecond = new Promise((r) => {
      resolveSecond = r
    })
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce(successResponse('1'))
      .mockReturnValueOnce(deferredSecond.then(() => successResponse('2')))
    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(screen.getByRole('progressbar')).toBeInTheDocument()
      expect(screen.getByText(/1\/2 completed/)).toBeInTheDocument()
    })

    await act(async () => {
      resolveSecond()
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Successfully re-evaluated 2/)
      ).toBeInTheDocument()
    })
  })

  it('shows success snackbar when all runs succeed', async () => {
    mockFetchResponses(successResponse('1'), successResponse('2'))
    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(
        screen.getByText(
          'Successfully re-evaluated 2 job run(s). Refresh the page to see updated labels.'
        )
      ).toBeInTheDocument()
    })
  })

  it('shows error snackbar on rewrite_error (inconsistent state)', async () => {
    mockFetchResponses(
      errorResponse('1', 'rewrite_error'),
      errorResponse('1', 'rewrite_error')
    )
    render(<ReEvaluateButton prowJobBuildIDs={['1']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(
        screen.getByText(
          /failed during rewrite and may be in an inconsistent state/
        )
      ).toBeInTheDocument()
    })
  })

  it('shows error snackbar when all runs are missing_error', async () => {
    mockFetchResponses(errorResponse('1', 'missing_error'))
    render(<ReEvaluateButton prowJobBuildIDs={['1']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(
        screen.getByText('None of the selected job run(s) were found in Sippy')
      ).toBeInTheDocument()
    })
  })

  it('retries eval_error and rewrite_error once', async () => {
    mockFetchResponses(errorResponse('1', 'eval_error'), successResponse('1'))
    render(<ReEvaluateButton prowJobBuildIDs={['1']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Successfully re-evaluated 1/)
      ).toBeInTheDocument()
    })
    expect(global.fetch).toHaveBeenCalledTimes(2)
  })

  it('does not retry missing_error', async () => {
    mockFetchResponses(errorResponse('1', 'missing_error'))
    render(<ReEvaluateButton prowJobBuildIDs={['1']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(screen.getByText(/were found in Sippy/)).toBeInTheDocument()
    })
    expect(global.fetch).toHaveBeenCalledTimes(1)
  })

  it('shows warning snackbar on partial success', async () => {
    mockFetchResponses(
      successResponse('1'),
      errorResponse('2', 'eval_error'),
      errorResponse('2', 'eval_error')
    )
    render(<ReEvaluateButton prowJobBuildIDs={['1', '2']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Re-evaluated 1 job run\(s\)/)
      ).toBeInTheDocument()
      expect(screen.getByText(/1 failed/)).toBeInTheDocument()
    })
  })

  it('sends one request per build ID', async () => {
    mockFetchResponses(
      successResponse('1'),
      successResponse('2'),
      successResponse('3')
    )
    render(<ReEvaluateButton prowJobBuildIDs={['1', '2', '3']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledTimes(3)
    })
    const sentIDs = global.fetch.mock.calls.map((call) => {
      const body = JSON.parse(call[1].body)
      expect(body.prow_job_build_ids).toHaveLength(1)
      return body.prow_job_build_ids[0]
    })
    expect(sentIDs.sort()).toEqual(['1', '2', '3'])
  })

  it('treats 503 as eval_error and retries', async () => {
    mockFetchResponses(
      httpErrorResponse(503, 'Service Unavailable'),
      httpErrorResponse(503, 'Service Unavailable'),
      httpErrorResponse(503, 'Service Unavailable'),
      httpErrorResponse(503, 'Service Unavailable'),
      httpErrorResponse(503, 'Service Unavailable'),
      httpErrorResponse(503, 'Service Unavailable')
    )
    render(<ReEvaluateButton prowJobBuildIDs={['1', '2', '3']} />)

    await act(async () => {
      userEvent.click(screen.getByRole('button'))
    })

    await waitFor(() => {
      expect(
        screen.getByText(/Re-evaluation failed for all 3/)
      ).toBeInTheDocument()
      expect(screen.getByText(/Service Unavailable/)).toBeInTheDocument()
    })
    expect(global.fetch).toHaveBeenCalledTimes(6)
  })
})
