import '@testing-library/jest-dom'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { SippyCapabilitiesContext } from '../App'
import LabelEditor from './LabelEditor'
import React from 'react'
import userEvent from '@testing-library/user-event'

function response(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () =>
      Promise.resolve(typeof body === 'string' ? body : JSON.stringify(body)),
  }
}

const alpha = {
  id: 'Alpha',
  label_title: 'Alpha label',
  explanation: 'Alpha explanation',
  bugs: ['TRT-100'],
  hide_display_contexts: ['metrics'],
  links: { self: 'http://localhost:8080/api/jobs/labels/Alpha' },
}

const beta = {
  id: 'Beta',
  label_title: 'Beta label',
  explanation: 'Beta explanation',
  bugs: ['OCPBUGS-200'],
  hide_display_contexts: ['spyglass', 'jaq-options'],
  links: { self: 'http://localhost:8080/api/jobs/labels/Beta' },
}

const gamma = {
  id: 'Gamma',
  label_title: 'Gamma label',
  explanation: 'Gamma explanation',
  bugs: [],
  hide_display_contexts: [],
  links: { self: 'http://localhost:8080/api/jobs/labels/Gamma' },
}

function Location() {
  const location = useLocation()
  return <div data-testid="location">{location.pathname}</div>
}

function renderEditor({
  capabilities = ['write_endpoints'],
  initialEntry = '/labels/edit/Beta',
} = {}) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <SippyCapabilitiesContext.Provider value={capabilities}>
        <Location />
        <Routes>
          <Route path="/labels/edit/:labelId?" element={<LabelEditor />} />
        </Routes>
      </SippyCapabilitiesContext.Provider>
    </MemoryRouter>
  )
}

beforeEach(() => {
  vi.restoreAllMocks()
  global.fetch = vi.fn()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('LabelEditor', () => {
  it('blocks direct access without writable endpoints', () => {
    renderEditor({ capabilities: [] })

    expect(
      screen.getByText(
        'Label editing is unavailable because writable endpoints are disabled.'
      )
    ).toBeInTheDocument()
    expect(global.fetch).not.toHaveBeenCalled()
  })

  it('loads the selected label and keeps its ID immutable', async () => {
    global.fetch.mockResolvedValueOnce(response([gamma, alpha, beta]))
    renderEditor()

    expect(await screen.findByDisplayValue('Beta label')).toBeInTheDocument()
    expect(screen.getByLabelText('Label ID')).toBeDisabled()
    expect(screen.getByLabelText('Label ID')).toHaveValue('Beta')
    expect(screen.getByLabelText('Explanation')).toHaveValue('Beta explanation')
    expect(screen.getByText('OCPBUGS-200')).toBeInTheDocument()
    expect(screen.getByText('Spyglass')).toBeInTheDocument()
    expect(screen.getByText('JAQ options')).toBeInTheDocument()
  })

  it('previews the explanation without writing to the API', async () => {
    const markdownLabel = {
      ...beta,
      explanation: '**Expected** and ~~obsolete~~',
    }
    global.fetch.mockResolvedValueOnce(response([markdownLabel]))
    renderEditor()

    const explanation = await screen.findByRole('textbox', {
      name: 'Explanation',
    })
    expect(explanation).toHaveValue(markdownLabel.explanation)

    await userEvent.click(screen.getByRole('button', { name: 'Preview' }))

    expect(screen.getByText('Expected').tagName).toBe('STRONG')
    expect(screen.getByText('obsolete').tagName).toBe('DEL')
    expect(global.fetch).toHaveBeenCalledTimes(1)

    await userEvent.click(screen.getByRole('button', { name: 'Edit' }))

    expect(screen.getByRole('textbox', { name: 'Explanation' })).toHaveValue(
      markdownLabel.explanation
    )
    expect(global.fetch).toHaveBeenCalledTimes(1)
  })

  it('lists every label and loads values when the selection changes', async () => {
    global.fetch.mockResolvedValueOnce(response([gamma, alpha, beta]))
    renderEditor()

    await screen.findByDisplayValue('Beta label')
    await userEvent.click(screen.getByRole('combobox', { name: 'Label' }))

    expect(
      screen.getByRole('option', { name: 'Alpha label (Alpha)' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'Beta label (Beta)' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: 'Gamma label (Gamma)' })
    ).toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('option', { name: 'Alpha label (Alpha)' })
    )

    expect(screen.getByTestId('location')).toHaveTextContent(
      '/labels/edit/Alpha'
    )
    expect(screen.getByLabelText(/Label title/)).toHaveValue('Alpha label')
    expect(screen.getByLabelText('Explanation')).toHaveValue(
      'Alpha explanation'
    )
  })

  it('sends a complete replacement and renders the saved values', async () => {
    const updated = {
      ...beta,
      label_title: 'Updated beta label',
      explanation: 'Updated by the server',
    }
    global.fetch
      .mockResolvedValueOnce(response([alpha, beta]))
      .mockResolvedValueOnce(response(updated))
    renderEditor()

    const title = await screen.findByLabelText(/Label title/)
    await userEvent.clear(title)
    await userEvent.type(title, 'Updated beta label')
    await userEvent.click(screen.getByRole('button', { name: 'Save label' }))

    await waitFor(() => expect(global.fetch).toHaveBeenCalledTimes(2))
    expect(global.fetch).toHaveBeenNthCalledWith(
      2,
      'http://localhost:8080/api/jobs/labels/Beta',
      expect.objectContaining({ method: 'PUT' })
    )
    const request = global.fetch.mock.calls[1][1]
    expect(JSON.parse(request.body)).toEqual({
      id: 'Beta',
      label_title: 'Updated beta label',
      explanation: 'Beta explanation',
      bugs: ['OCPBUGS-200'],
      hide_display_contexts: ['spyglass', 'jaq-options'],
    })
    expect(
      await screen.findByText('Label "Updated beta label" saved successfully.')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Explanation')).toHaveValue(
      'Updated by the server'
    )
  })

  it('shows server errors when a save fails', async () => {
    global.fetch
      .mockResolvedValueOnce(response([beta]))
      .mockResolvedValueOnce(response({ message: 'title already exists' }, 400))
    renderEditor()

    await screen.findByDisplayValue('Beta label')
    await userEvent.click(screen.getByRole('button', { name: 'Save label' }))

    expect(
      await screen.findByText('Failed to update label: title already exists')
    ).toBeInTheDocument()
  })

  it.each([
    {
      name: 'selects the former next label',
      initial: [alpha, beta, gamma],
      refreshed: [alpha, gamma],
      expected: gamma,
    },
    {
      name: 'selects the first label when the deleted label was last',
      initial: [alpha, beta],
      refreshed: [alpha],
      expected: alpha,
    },
  ])('$name after deletion', async ({ initial, refreshed, expected }) => {
    global.fetch
      .mockResolvedValueOnce(response(initial))
      .mockResolvedValueOnce(response('', 204))
      .mockResolvedValueOnce(response(refreshed))
    renderEditor()

    await screen.findByDisplayValue('Beta label')
    await userEvent.click(
      screen.getByRole('button', { name: 'Delete label Beta' })
    )
    expect(screen.getByText('Delete failure label?')).toBeInTheDocument()

    vi.useFakeTimers()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Delete label' }))
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(
      screen.getByText(
        'Label "Beta label" deleted successfully. The next label will load in 10 seconds.'
      )
    ).toBeInTheDocument()
    expect(global.fetch).toHaveBeenCalledTimes(2)

    await act(async () => {
      vi.advanceTimersByTime(9999)
    })
    expect(global.fetch).toHaveBeenCalledTimes(2)

    await act(async () => {
      vi.advanceTimersByTime(1)
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(global.fetch).toHaveBeenCalledTimes(3)
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/labels/edit/${expected.id}`
    )
    expect(screen.getByLabelText(/Label title/)).toHaveValue(
      expected.label_title
    )
    expect(screen.queryByText(/deleted successfully/)).not.toBeInTheDocument()
  })

  it('keeps the selected label and reports delete errors', async () => {
    global.fetch
      .mockResolvedValueOnce(response([alpha, beta]))
      .mockResolvedValueOnce(response({ message: 'label is in use' }, 409))
    renderEditor()

    await screen.findByDisplayValue('Beta label')
    await userEvent.click(
      screen.getByRole('button', { name: 'Delete label Beta' })
    )
    await userEvent.click(screen.getByRole('button', { name: 'Delete label' }))

    expect(
      await screen.findByText('Failed to delete label: label is in use')
    ).toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent(
      '/labels/edit/Beta'
    )
    expect(screen.getByLabelText(/Label title/)).toHaveValue('Beta label')
  })

  it('disables editing when the label list cannot refresh after deletion', async () => {
    global.fetch
      .mockResolvedValueOnce(response([alpha, beta]))
      .mockResolvedValueOnce(response('', 204))
      .mockResolvedValueOnce(response({ message: 'refresh failed' }, 500))
    renderEditor()

    await screen.findByDisplayValue('Beta label')
    await userEvent.click(
      screen.getByRole('button', { name: 'Delete label Beta' })
    )

    vi.useFakeTimers()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Delete label' }))
      await Promise.resolve()
      await Promise.resolve()
    })

    await act(async () => {
      vi.advanceTimersByTime(10000)
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(
      screen.getByText(
        'Label was deleted, but the label list could not be refreshed: Failed to load label: refresh failed'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText('The selected label was not found.')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Save label' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Delete label Beta' })
    ).not.toBeInTheDocument()
  })
})
