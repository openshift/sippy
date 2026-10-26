import '@testing-library/jest-dom'
import { render, screen } from '@testing-library/react'
import JiraBugLinks, { isValidJiraKey, normalizeJiraKeys } from './JiraBugLinks'
import React from 'react'

describe('JiraBugLinks', () => {
  it('trims and de-duplicates Jira keys', () => {
    expect(
      normalizeJiraKeys([' OCPBUGS-12345 ', 'TRT-2896', 'OCPBUGS-12345', ''])
    ).toEqual(['OCPBUGS-12345', 'TRT-2896'])
  })

  it('validates Jira key syntax', () => {
    expect(isValidJiraKey('OCPBUGS-12345')).toBe(true)
    expect(isValidJiraKey('TRT-2896')).toBe(true)
    expect(isValidJiraKey('ocpbugs-12345')).toBe(false)
    expect(isValidJiraKey('https://redhat.atlassian.net/browse/TRT-2896')).toBe(
      false
    )
  })

  it('renders each Jira key as an external link', () => {
    render(<JiraBugLinks bugs={['OCPBUGS-12345', 'TRT-2896']} showLabel />)

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

  it('renders nothing for an empty bug list', () => {
    const { container } = render(<JiraBugLinks bugs={[]} />)
    expect(container).toBeEmptyDOMElement()
  })
})
