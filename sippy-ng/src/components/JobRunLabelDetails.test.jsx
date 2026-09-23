import '@testing-library/jest-dom'
import { render, screen } from '@testing-library/react'
import JobRunLabelDetails from './JobRunLabelDetails'
import React from 'react'

describe('JobRunLabelDetails', () => {
  it('renders label details and associated Jira links', () => {
    render(
      <JobRunLabelDetails
        label={{
          label_title: 'Known failure',
          explanation: '**Expected** failure mode.',
          bugs: ['OCPBUGS-12345', 'TRT-2896'],
        }}
        labelId="KnownFailure"
      />
    )

    expect(screen.getByText('Known failure')).toBeInTheDocument()
    expect(screen.getByText('Expected')).toBeInTheDocument()
    expect(screen.getByText(/Bugs:/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'OCPBUGS-12345' })).toHaveAttribute(
      'href',
      'https://redhat.atlassian.net/browse/OCPBUGS-12345'
    )
    expect(screen.getByRole('link', { name: 'TRT-2896' })).toHaveAttribute(
      'href',
      'https://redhat.atlassian.net/browse/TRT-2896'
    )
  })

  it('omits Jira links when the label has no bugs', () => {
    render(
      <JobRunLabelDetails
        label={{ label_title: 'No bugs', explanation: '', bugs: [] }}
        labelId="NoBugs"
      />
    )

    expect(screen.getByText('No bugs')).toBeInTheDocument()
    expect(screen.queryByText(/Bugs:/)).not.toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('preserves the fallback for an unknown label', () => {
    render(<JobRunLabelDetails labelId="MissingLabel" />)

    expect(screen.getByText('MissingLabel')).toBeInTheDocument()
    expect(screen.getByText('Label not found')).toBeInTheDocument()
  })
})
