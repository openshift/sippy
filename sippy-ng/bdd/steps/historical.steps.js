import {
  CLOSED_IN_VIEW_REGRESSION_ID,
  CLOSED_REGRESSION_ID,
  closedRegression,
  closedRegressionTriage,
  HISTORICAL_BASE_END,
  HISTORICAL_LAST_FAILURE,
  HISTORICAL_RANGE,
  HISTORICAL_SAMPLE_END,
  HISTORICAL_SAMPLE_START,
  historicalTestDetailsPath,
  mixedRegressionTriage,
  NO_FAILURE_REGRESSION_ID,
  REGRESSION_ID,
  VIEW,
} from '../fixtures/data.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { latestRequest } from '../fixtures/expectations.js'
import { test } from '../fixtures/test.js'

const { Given, When, Then } = createBdd(test)

const closedTooltip = `Closed regression. View historical test details for ${HISTORICAL_RANGE}.`
const closedInViewTooltip = `Closed in this view. View historical test details for ${HISTORICAL_RANGE}.`

function gridRow(page, id) {
  return page.locator(`.MuiDataGrid-row[data-id="${id}"]`)
}

Given('a triage whose only regression closed exists', async ({ api }) => {
  api.triages.push(closedRegressionTriage())
})

Given('a triage with open and closed regressions exists', async ({ api }) => {
  api.triages.push(mixedRegressionTriage())
})

Given('the regression API returns the closed regression', async ({ api }) => {
  api.regressionBody = closedRegression()
})

Given(
  'the regression API returns a regression for another test',
  async ({ api }) => {
    api.regressionBody = closedRegression({ test_id: 'some-other-test' })
  }
)

Given('the regression API cannot find the regression', async ({ api }) => {
  api.regressionMode = 'missing'
})

Given('test details no longer track the regression', async ({ api }) => {
  api.testDetailsMode = 'untracked'
})

When('I open the historical test details link', async ({ page }) => {
  const apiPath = historicalTestDetailsPath()
  await page.goto(
    apiPath.replace('/api/component_readiness/', 'component_readiness/')
  )
})

Then('the {string} status column is shown', async ({ page }, view) => {
  // The header's accessible name is its tooltip, which starts with the view.
  const escaped = view.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  await expect(
    page.getByRole('columnheader', { name: new RegExp(`^${escaped}:`) })
  ).toBeVisible()
})

Then(
  'the closed regression links to its historical test details',
  async ({ page }) => {
    const link = gridRow(page, CLOSED_REGRESSION_ID).getByRole('link', {
      name: closedTooltip,
      exact: true,
    })
    await expect(link).toBeVisible()
    await expect(link).toHaveAttribute('target', '_blank')
    const href = new URL(await link.getAttribute('href'), 'http://x')
    expect(href.pathname).toBe('/sippy-ng/component_readiness/test_details')
    expect(href.searchParams.get('view')).toBe(VIEW)
    expect(href.searchParams.get('regressionId')).toBe(
      String(CLOSED_REGRESSION_ID)
    )
    expect(href.searchParams.get('sampleStartTime')).toBe(
      HISTORICAL_SAMPLE_START
    )
    expect(href.searchParams.get('sampleEndTime')).toBe(HISTORICAL_SAMPLE_END)
    await link.hover()
    await expect(page.getByRole('tooltip')).toHaveText(closedTooltip)
  }
)

Then(
  'the historical link can be reached from the keyboard',
  async ({ page }) => {
    const link = gridRow(page, CLOSED_REGRESSION_ID).getByRole('link', {
      name: closedTooltip,
      exact: true,
    })
    await link.focus()
    await expect(link).toBeFocused()
  }
)

Then('the open regression keeps its severity icon', async ({ page }) => {
  const status = gridRow(page, REGRESSION_ID).locator('.status')
  await expect(status.getByAltText('SignificantRegression')).toBeVisible()
  await expect(status.locator('.historical-test-details')).toHaveCount(0)
})

Then('the regression closed only in this view says so', async ({ page }) => {
  await expect(
    gridRow(page, CLOSED_IN_VIEW_REGRESSION_ID).getByRole('link', {
      name: closedInViewTooltip,
      exact: true,
    })
  ).toBeVisible()
})

Then(
  'the regression without a last failure explains that history is unavailable',
  async ({ page }) => {
    const row = gridRow(page, NO_FAILURE_REGRESSION_ID)
    await expect(
      row.getByRole('img', {
        name: 'Closed regression. Historical test details are unavailable because no last failure was recorded.',
        exact: true,
      })
    ).toBeVisible()
    await expect(row.locator('.status').getByRole('link')).toHaveCount(0)
  }
)

Then(
  'the closed regression notice shows its closure details',
  async ({ page }) => {
    const notice = page.getByTestId('historical-notice')
    await expect(notice).toContainText('Closed regression, historical report')
    await expect(notice).toContainText(`Regression ID: ${CLOSED_REGRESSION_ID}`)
    await expect(notice).toContainText('Closed: 2026-10-02T08:00:00Z')
    await expect(notice).toContainText(
      `Last failure: ${HISTORICAL_LAST_FAILURE}`
    )
    await expect(notice).toContainText(
      `Sample window: ${HISTORICAL_RANGE} (UTC)`
    )
    await expect(notice).toContainText(
      'Basis window: May 10 – June 9, 2026 (UTC)'
    )
    await expect(notice).toContainText('may still show a regression')
  }
)

Then(
  'the test details request keeps the historical window',
  async ({ api }) => {
    const request = latestRequest(
      api.requestsTo(/\/api\/component_readiness\/test_details$/)
    )
    const params = new URLSearchParams(request.search)
    expect(params.get('sampleStartTime')).toBe(HISTORICAL_SAMPLE_START)
    expect(params.get('sampleEndTime')).toBe(HISTORICAL_SAMPLE_END)
    expect(params.get('baseEndTime')).toBe(HISTORICAL_BASE_END)
    expect(
      api.requestsForPath(
        `/api/component_readiness/regressions/${CLOSED_REGRESSION_ID}`
      )
    ).not.toHaveLength(0)
  }
)

Then('the regression ID row shows the closed regression', async ({ page }) => {
  const row = page.getByRole('row', { name: /Regression ID:/ })
  await expect(row).toContainText(String(CLOSED_REGRESSION_ID))
})

Then("the closed regression's triage is listed", async ({ page }) => {
  await expect(page.getByRole('heading', { name: 'Triages' })).toBeVisible()
})

Then(
  'the sample date controls show the historical window',
  async ({ page }) => {
    const sample = page
      .locator('#sample-release-label')
      .locator('xpath=ancestor::*[contains(@class,"MuiBox-root")][1]')
    await expect(sample.getByRole('textbox', { name: 'From' })).toHaveValue(
      '2026-09-22 00:00:00'
    )
    await expect(sample.getByRole('textbox', { name: 'To' })).toHaveValue(
      '2026-09-29 23:59:59'
    )
  }
)

When('I generate the report from test details', async ({ page }) => {
  await page.getByTestId('generate-report-top').click()
})

Then(
  'the page keeps the historical window and regression',
  async ({ page }) => {
    await expect(page).toHaveURL((url) => !url.searchParams.has('view'))
    const url = new URL(page.url())
    expect(url.searchParams.get('sampleEndTime')).toBe(HISTORICAL_SAMPLE_END)
    expect(url.searchParams.get('sampleStartTime')).toBe('2026-09-22 00:00:00')
    expect(url.searchParams.get('regressionId')).toBe(
      String(CLOSED_REGRESSION_ID)
    )
    await expect(page.getByTestId('historical-notice')).toContainText(
      `Sample window: ${HISTORICAL_RANGE} (UTC)`
    )
  }
)

Then(
  'the notice says the regression does not match this report',
  async ({ page }) => {
    await expect(page.getByTestId('historical-notice')).toContainText(
      `Regression ${CLOSED_REGRESSION_ID} does not match this report (test ID)`
    )
    await expect(
      page.getByText('Closed regression, historical report')
    ).toHaveCount(0)
  }
)

Then('the notice says the regression was not found', async ({ page }) => {
  await expect(page.getByTestId('historical-notice')).toContainText(
    `Regression ${CLOSED_REGRESSION_ID} was not found`
  )
})

Then(
  'the closed regression notice explains there is no historical data',
  async ({ page }) => {
    const notice = page.getByTestId('historical-notice')
    await expect(notice).toContainText('Closed regression, historical report')
    await expect(notice).toContainText(
      'No test results are available for this historical window.'
    )
  }
)
