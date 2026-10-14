import '@testing-library/jest-dom'
import { BrowserRouter, Route, Routes, useLocation } from 'react-router'
import { render, screen } from '@testing-library/react'
import React from 'react'
import RegressionRedirect from './RegressionRedirect'

const testDetailsApiUrl =
  'https://sippy.example/api/component_readiness/test_details?' +
  'sampleStartTime=2026-01-01T00%3A00%3A00Z&' +
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

test('redirects to a basename-relative test details route', async () => {
  window.history.replaceState(
    {},
    '',
    '/sippy-ng/component_readiness/regressions/42'
  )
  global.fetch = vi.fn().mockResolvedValue({
    status: 200,
    json: () =>
      Promise.resolve({
        links: { 'test_details:4.22-main': testDetailsApiUrl },
      }),
  })

  render(
    <BrowserRouter basename="/sippy-ng">
      <Routes>
        <Route
          path="/component_readiness/regressions/:regressionId"
          element={<RegressionRedirect />}
        />
        <Route
          path="/component_readiness/test_details"
          element={<LocationDisplay />}
        />
      </Routes>
    </BrowserRouter>
  )

  const expectedSearch = new URL(testDetailsApiUrl).search
  expect(await screen.findByTestId('location')).toHaveTextContent(
    `/component_readiness/test_details${expectedSearch}`
  )
  expect(window.location.pathname).toBe(
    '/sippy-ng/component_readiness/test_details'
  )
  expect(
    new URLSearchParams(window.location.search).getAll('includeVariant')
  ).toEqual(['Architecture:amd64', 'Platform:aws'])
})
