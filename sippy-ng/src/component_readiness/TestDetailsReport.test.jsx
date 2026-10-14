import '@testing-library/jest-dom'
import { AccessibilityModeContext } from '../components/AccessibilityModeProvider'
import { BrowserRouter, Link } from 'react-router'
import { CompReadyVarsContext } from './CompReadyVars'
import { ReleasesContext, SippyCapabilitiesContext } from '../App'
import { render, screen } from '@testing-library/react'
import React from 'react'
import TestDetailsReport from './TestDetailsReport'

vi.mock('../App', async () => {
  const ReactModule = await import('react')
  return {
    ReleasesContext: ReactModule.createContext({}),
    SippyCapabilitiesContext: ReactModule.createContext([]),
  }
})
vi.mock('../bugs/BugButton', () => ({ default: () => null }))
vi.mock('../bugs/BugTable', () => ({ default: () => null }))
vi.mock('../bugs/FileBug', () => ({ default: () => null }))
vi.mock('./CompReadyCancelled', () => ({ default: () => null }))
vi.mock('./CompReadyPageTitle', () => ({ default: () => null }))
vi.mock('./CompReadyProgress', () => ({ default: () => null }))
vi.mock('./CompReadyTestPanel', () => ({ default: () => null }))
vi.mock('./CopyPageURL', () => ({ default: () => null }))
vi.mock('./GeneratedAt', () => ({ default: () => null }))
vi.mock('./Sidebar', () => ({ default: () => null }))
vi.mock('./TriagedTestsPanel', () => ({ default: () => null }))
vi.mock('./TriageSymptomLabels', () => ({
  default: () => null,
  aggregateLabelSummaries: vi.fn(() => []),
}))
vi.mock('./UpsertTriageModal', () => ({ default: () => null }))

const latestReportApiUrl =
  'https://sippy.example/api/component_readiness/test_details?' +
  'sampleStartTime=2026-01-01T00%3A00%3A00Z&' +
  'sampleEndTime=2026-01-08T00%3A00%3A00Z&' +
  'includeVariant=Architecture%3Aamd64&' +
  'includeVariant=Platform%3Aaws'
const latestReportUiUrl = latestReportApiUrl.replace(
  'https://sippy.example/api/',
  '/sippy-ng/'
)

const report = {
  code: 200,
  component: 'Example component',
  test_name: 'example test',
  lifecycle: 'blocking',
  generated_at: '2026-01-08T01:00:00Z',
  jira_component_id: 1,
  jira_component: 'Example',
  links: { latest: latestReportApiUrl },
  rows: [
    {
      component: 'Example component',
      columns: [{ status: 0, variants: {} }],
    },
  ],
  analyses: [
    {
      status: 0,
      explanations: [],
      sample_stats: {
        release: '5.0',
        Start: '2026-01-01T00:00:00Z',
        End: '2026-01-08T00:00:00Z',
        success_rate: 1,
        success_count: 1,
        failure_count: 0,
        flake_count: 0,
      },
      base_stats: {
        release: '4.22',
        Start: '2025-12-01T00:00:00Z',
        End: '2025-12-31T00:00:00Z',
        success_rate: 1,
        success_count: 1,
        failure_count: 0,
        flake_count: 0,
      },
    },
  ],
}

const originalFetch = global.fetch

beforeEach(() => {
  window.history.replaceState(
    {},
    '',
    '/sippy-ng/component_readiness/test_details?baseRelease=4.22'
  )
  global.fetch = vi.fn().mockResolvedValue({
    json: () => Promise.resolve(report),
  })
})

afterEach(() => {
  global.fetch = originalFetch
  window.history.replaceState({}, '', '/')
})

test('renders the latest report as a native anchor without duplicating the basename', async () => {
  render(
    <BrowserRouter basename="/sippy-ng">
      <AccessibilityModeContext.Provider value={{ accessibilityModeOn: false }}>
        <ReleasesContext.Provider
          value={{ releases: [], release_attrs: {}, ga_dates: {} }}
        >
          <SippyCapabilitiesContext.Provider value={[]}>
            <CompReadyVarsContext.Provider
              value={{ sampleRelease: '5.0', urlParams: 'test' }}
            >
              <TestDetailsReport
                component="Example component"
                capability="Example capability"
                testId="example:test"
                environment="Architecture:amd64"
              />
              <Link to={latestReportUiUrl}>Router Link control</Link>
            </CompReadyVarsContext.Provider>
          </SippyCapabilitiesContext.Provider>
        </ReleasesContext.Provider>
      </AccessibilityModeContext.Provider>
    </BrowserRouter>
  )

  const latestReportLink = await screen.findByRole('link', {
    name: 'View the latest report with data from the last 7 days',
  })
  expect(latestReportLink).toHaveAttribute('href', latestReportUiUrl)
  expect(latestReportLink).not.toHaveAttribute('target')
  expect(
    screen.getByRole('link', { name: 'Router Link control' })
  ).toHaveAttribute('href', `/sippy-ng${latestReportUiUrl}`)

  const renderedUrl = new URL(latestReportLink.href)
  expect(renderedUrl.pathname).toBe(
    '/sippy-ng/component_readiness/test_details'
  )
  expect(renderedUrl.searchParams.get('sampleStartTime')).toBe(
    '2026-01-01T00:00:00Z'
  )
  expect(renderedUrl.searchParams.get('sampleEndTime')).toBe(
    '2026-01-08T00:00:00Z'
  )
  expect(renderedUrl.searchParams.getAll('includeVariant')).toEqual([
    'Architecture:amd64',
    'Platform:aws',
  ])
})
