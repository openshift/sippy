import '@testing-library/jest-dom'
import { filterFor, pathForJobRunsWithFilter } from '../helpers'
import { MemoryRouter } from 'react-router-dom'
import { render, screen, waitFor, within } from '@testing-library/react'
import React from 'react'
import TriageSymptomLabels, {
  aggregateLabelSummaries,
  filterRegressionsByLabel,
} from './TriageSymptomLabels'
import userEvent from '@testing-library/user-event'

describe('aggregateLabelSummaries', () => {
  const labels = [
    {
      id: 'ManualLabel',
      label_title: 'Manual label',
      explanation: 'Applied manually after investigation',
    },
    { id: 'KnownFailure', label_title: 'Known failure' },
  ]

  it('aggregates failed runs by label and ignores unlabeled runs', () => {
    const summaries = aggregateLabelSummaries(
      [
        { job_labels: ['ManualLabel', 'KnownFailure'] },
        { job_labels: ['KnownFailure', 'KnownFailure'] },
        { job_labels: [] },
      ],
      labels
    )

    expect(summaries).toHaveLength(2)
    expect(summaries[0]).toEqual(
      expect.objectContaining({
        label: expect.objectContaining({ id: 'KnownFailure' }),
        job_run_count: 2,
      })
    )
    expect(summaries[0].percentage).toBeCloseTo(200 / 3)
    expect(summaries[1]).toEqual(
      expect.objectContaining({
        label: expect.objectContaining({ id: 'ManualLabel' }),
        job_run_count: 1,
      })
    )
    expect(summaries[1].percentage).toBeCloseTo(100 / 3)
  })

  it('counts each triage regression once per label', () => {
    const summaries = aggregateLabelSummaries(
      [
        { regression_id: 10, job_labels: ['KnownFailure'] },
        { regression_id: 10, job_labels: ['KnownFailure'] },
        { regression_id: 11, job_labels: ['ManualLabel'] },
      ],
      labels,
      2
    )

    expect(summaries[0]).toEqual(
      expect.objectContaining({
        regression_count: 1,
        job_run_count: 2,
        percentage: 50,
      })
    )
    expect(summaries[1]).toEqual(
      expect.objectContaining({
        regression_count: 1,
        job_run_count: 1,
        percentage: 50,
      })
    )
  })

  it('filters triage regressions by the selected label', () => {
    const regressions = [{ id: 10 }, { id: 11 }, { id: 12 }]
    const summaries = aggregateLabelSummaries(
      [
        { regression_id: 10, job_labels: ['KnownFailure'] },
        { regression_id: 12, job_labels: ['KnownFailure'] },
      ],
      labels,
      regressions.length
    )

    expect(
      filterRegressionsByLabel(regressions, 'KnownFailure', summaries)
    ).toEqual([{ id: 10 }, { id: 12 }])
    expect(filterRegressionsByLabel(regressions, null, summaries)).toBe(
      regressions
    )
    expect(
      filterRegressionsByLabel(regressions, 'UnknownLabel', summaries)
    ).toBe(regressions)
  })

  it('opens label details with a filtered job-runs link', async () => {
    const summaries = aggregateLabelSummaries(
      [{ job_labels: ['ManualLabel'] }],
      labels
    )
    render(
      <MemoryRouter>
        <TriageSymptomLabels labelSummaries={summaries} release="4.22" />
      </MemoryRouter>
    )

    expect(screen.getByText('Failure Labels (1)')).toBeInTheDocument()
    const expectedJobRunsPath = pathForJobRunsWithFilter('4.22', {
      items: [filterFor('labels', 'has entry', 'ManualLabel')],
    })
    const jobRunsLink = screen.getByRole('link', {
      name: 'View job runs with this label',
    })
    expect(jobRunsLink).toHaveAttribute('href', expectedJobRunsPath)
    expect(jobRunsLink).toHaveAttribute('target', '_blank')
    expect(jobRunsLink).toHaveAttribute('rel', 'noopener noreferrer')
    userEvent.hover(jobRunsLink)
    expect(
      await screen.findByText('View job runs with this label')
    ).toBeInTheDocument()

    userEvent.click(screen.getByRole('button', { name: 'Manual label' }))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(
      screen.getByText('Applied manually after investigation')
    ).toBeInTheDocument()
    const link = within(screen.getByRole('dialog')).getByRole('link', {
      name: 'View job runs with this label',
    })
    expect(link).toHaveAttribute('href', expectedJobRunsPath)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')

    userEvent.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })

  it('renders and toggles the regression filter next to the count', async () => {
    const setLabelFilter = vi.fn()
    const summaries = aggregateLabelSummaries(
      [{ regression_id: 10, job_labels: ['ManualLabel'] }],
      labels,
      1
    )
    const { rerender } = render(
      <MemoryRouter>
        <TriageSymptomLabels
          labelSummaries={summaries}
          setLabelFilter={setLabelFilter}
        />
      </MemoryRouter>
    )

    const labelRow = screen.getByRole('row', { name: /Manual label/ })
    const regressionCell = within(labelRow).getAllByRole('cell')[1]
    const filterButton = within(regressionCell).getByRole('button', {
      name: 'Filter regressions to Manual label',
    })
    expect(filterButton).toHaveAttribute('aria-pressed', 'false')
    userEvent.hover(filterButton)
    expect(
      await screen.findByText('Filter regressions to this label')
    ).toBeInTheDocument()
    userEvent.click(filterButton)
    expect(setLabelFilter).toHaveBeenCalledWith('ManualLabel')

    rerender(
      <MemoryRouter>
        <TriageSymptomLabels
          labelFilter="ManualLabel"
          labelSummaries={summaries}
          setLabelFilter={setLabelFilter}
        />
      </MemoryRouter>
    )
    expect(
      screen.getByRole('button', {
        name: 'Filter regressions to Manual label',
      })
    ).toHaveAttribute('aria-pressed', 'true')
    userEvent.click(
      screen.getByRole('button', {
        name: 'Filter regressions to Manual label',
      })
    )
    expect(setLabelFilter).toHaveBeenLastCalledWith(null)
  })

  it('handles empty labels and unavailable label details', () => {
    const { rerender } = render(
      <MemoryRouter>
        <TriageSymptomLabels labelSummaries={[]} release="4.22" />
      </MemoryRouter>
    )
    expect(screen.getByText('No labels applied')).toBeInTheDocument()

    const summaries = aggregateLabelSummaries(
      [{ job_labels: ['KnownFailure'] }],
      labels
    )
    rerender(
      <MemoryRouter>
        <TriageSymptomLabels labelSummaries={summaries} />
      </MemoryRouter>
    )
    userEvent.click(screen.getByRole('button', { name: 'Known failure' }))

    expect(screen.getByText('No description available.')).toBeInTheDocument()
    expect(screen.getByText('Job run link unavailable.')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'View job runs with this label' })
    ).not.toBeInTheDocument()
  })
})
