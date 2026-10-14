import '@testing-library/jest-dom'
import { BrowserRouter, Route, Routes, useLocation } from 'react-router'
import { render, screen, waitFor } from '@testing-library/react'
import LatestReportLink from './LatestReportLink'
import React from 'react'
import userEvent from '@testing-library/user-event'

const latestReportApiUrl =
  'https://sippy.example/api/component_readiness/test_details?' +
  'sampleStartTime=2026-01-01T00%3A00%3A00Z&' +
  'sampleEndTime=2026-01-08T00%3A00%3A00Z&' +
  'includeVariant=Architecture%3Aamd64&' +
  'includeVariant=Platform%3Aaws'

function LocationDisplay() {
  const location = useLocation()
  return (
    <div data-testid="location">
      {location.pathname}
      {location.search}
    </div>
  )
}

test('navigates to the latest report within the router basename', async () => {
  window.history.replaceState({}, '', '/sippy-ng/current-report')

  render(
    <BrowserRouter basename="/sippy-ng">
      <Routes>
        <Route
          path="/current-report"
          element={<LatestReportLink apiUrl={latestReportApiUrl} />}
        />
        <Route
          path="/component_readiness/test_details"
          element={<LocationDisplay />}
        />
      </Routes>
    </BrowserRouter>
  )

  const link = screen.getByRole('link', {
    name: 'View the latest report with data from the last 7 days',
  })
  const expectedSearch = new URL(latestReportApiUrl).search
  expect(link).toHaveAttribute(
    'href',
    `/sippy-ng/component_readiness/test_details${expectedSearch}`
  )

  await userEvent.click(link)

  await waitFor(() =>
    expect(screen.getByTestId('location')).toHaveTextContent(
      `/component_readiness/test_details${expectedSearch}`
    )
  )
  expect(window.location.pathname).toBe(
    '/sippy-ng/component_readiness/test_details'
  )

  const searchParams = new URLSearchParams(window.location.search)
  expect(searchParams.get('sampleStartTime')).toBe('2026-01-01T00:00:00Z')
  expect(searchParams.get('sampleEndTime')).toBe('2026-01-08T00:00:00Z')
  expect(searchParams.getAll('includeVariant')).toEqual([
    'Architecture:amd64',
    'Platform:aws',
  ])
})
