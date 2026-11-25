import {
  CLOSED_IN_VIEW_REGRESSION_ID,
  CLOSED_REGRESSION_ID,
  closedInViewRegression,
  closedInViewWithoutClosedAtRegression,
  closedRegression,
  closedRegressionTriage,
  HISTORICAL_BASE_END,
  HISTORICAL_CLOSED_AT,
  HISTORICAL_LAST_FAILURE,
  HISTORICAL_RANGE,
  HISTORICAL_SAMPLE_END,
  HISTORICAL_SAMPLE_START,
  historicalTriageBug,
  JIRA_URL,
  mixedRegressionTriage,
  NO_FAILURE_REGRESSION_ID,
  REGRESSION_ID,
  SEEDED_TRIAGE_ID,
  VIEW,
} from '../fixtures/data.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { latestRequest } from '../fixtures/expectations.js'
import { test } from '../fixtures/test.js'
import { TestDetailsPage } from '../pages/testDetailsPage.js'
import { TriagePage } from '../pages/triagePage.js'

const { Given, When, Then } = createBdd(test)

const closedTooltip = `Closed regression. View historical test details for ${HISTORICAL_RANGE}.`
const closedInViewTooltip = `Closed in this view. View historical test details for ${HISTORICAL_RANGE}.`

function regressionRow(page, id) {
  return new TriagePage(page).regressionRow(id)
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

Given(
  'the regression API returns a regression closed only in this view',
  async ({ api }) => {
    api.regressionBody = closedInViewRegression()
  }
)

Given(
  'the regression API returns a regression closed only in this view without a closure time',
  async ({ api }) => {
    api.regressionBody = closedInViewWithoutClosedAtRegression()
  }
)

Given('the regression API cannot find the regression', async ({ api }) => {
  api.regressionMode = 'missing'
})

Given('the regression API fails', async ({ api }) => {
  api.regressionMode = 'error'
})

Given('test details no longer track the regression', async ({ api }) => {
  api.testDetailsMode = 'untracked'
})

When('I open the historical test details link', async ({ page }) => {
  await new TestDetailsPage(page).openHistorical(CLOSED_REGRESSION_ID)
})

When(
  'I open the historical test details link for the regression closed only in this view',
  async ({ page }) => {
    await new TestDetailsPage(page).openHistorical(CLOSED_IN_VIEW_REGRESSION_ID)
  }
)

When(
  'I open the historical test details link for the regression closed only in this view without a view',
  async ({ page }) => {
    await new TestDetailsPage(page).openHistorical(
      CLOSED_IN_VIEW_REGRESSION_ID,
      { withView: false }
    )
  }
)

When('I sort by the {string} status column', async ({ page }, view) => {
  await new TriagePage(page).sortByStatus(view)
})

Then('the {string} status column is shown', async ({ page }, view) => {
  await expect(new TriagePage(page).statusColumnHeader(view)).toBeVisible()
})

Then(
  'the {string} status column is sorted {word}',
  async ({ page }, view, direction) => {
    await expect(new TriagePage(page).statusColumnHeader(view)).toHaveAttribute(
      'aria-sort',
      direction
    )
  }
)

Then(
  'the {string} rows start with unavailable history and end with the open row',
  async ({ page }, view) => {
    await expect
      .poll(async () => {
        const ids = await new TriagePage(page).regressionRowIds(view)
        return [ids[0], ids[ids.length - 1]]
      })
      .toEqual([NO_FAILURE_REGRESSION_ID, REGRESSION_ID])
  }
)

Then(
  'the {string} rows are ordered open, historical, then unavailable',
  async ({ page }, view) => {
    // The status columns are keyed by view name, so the sort applies to the
    // view's column even though historical columns were added to the grid.
    await expect
      .poll(() => new TriagePage(page).regressionRowIds(view))
      .toEqual([
        REGRESSION_ID,
        CLOSED_REGRESSION_ID,
        CLOSED_IN_VIEW_REGRESSION_ID,
        NO_FAILURE_REGRESSION_ID,
      ])
  }
)

Then(
  'the closed regression links to its historical test details',
  async ({ page }) => {
    const link = regressionRow(page, CLOSED_REGRESSION_ID).getByRole('link', {
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
    // Focus the row's first cell, then arrow to the status cell. The grid's
    // managed tab index must hand focus to the link for Enter to open it.
    const row = regressionRow(page, CLOSED_REGRESSION_ID)
    const link = row.getByRole('link', { name: closedTooltip, exact: true })
    const idCell = row.locator('[data-field="id"][role="gridcell"]')
    const statusCell = row.locator(
      `[data-field="status:${VIEW}"][role="gridcell"]`
    )
    await idCell.click()
    await expect(idCell).toBeFocused()
    const columnCount = await row.locator('[role="gridcell"]').count()
    for (let i = 0; i < columnCount; i++) {
      const inStatusCell = await statusCell.evaluate((cell) =>
        cell.contains(document.activeElement)
      )
      if (inStatusCell) break
      await page.keyboard.press('ArrowRight')
    }
    await expect(link).toBeFocused()
    const popup = page.waitForEvent('popup')
    await page.keyboard.press('Enter')
    const report = await popup
    await report.waitForURL(/\/component_readiness\/test_details\?/, {
      waitUntil: 'commit',
    })
    const opened = new URL(report.url())
    expect(opened.searchParams.get('regressionId')).toBe(
      String(CLOSED_REGRESSION_ID)
    )
    await report.close()
  }
)

Then('the open regression keeps its severity icon', async ({ page }) => {
  const status = regressionRow(page, REGRESSION_ID).locator('.status')
  await expect(status.getByAltText('SignificantRegression')).toBeVisible()
  await expect(status.locator('.historical-test-details')).toHaveCount(0)
})

Then('the regression closed only in this view says so', async ({ page }) => {
  await expect(
    regressionRow(page, CLOSED_IN_VIEW_REGRESSION_ID).getByRole('link', {
      name: closedInViewTooltip,
      exact: true,
    })
  ).toBeVisible()
})

Then(
  'the regression without a last failure explains that history is unavailable',
  async ({ page }) => {
    const row = regressionRow(page, NO_FAILURE_REGRESSION_ID)
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
    const notice = new TestDetailsPage(page).historicalNotice()
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
  const triageRow = new TestDetailsPage(page).triageRow(SEEDED_TRIAGE_ID)
  await expect(triageRow).toBeVisible()
  await expect(
    triageRow.locator('[data-field="jira_key"]').getByRole('link')
  ).toHaveAttribute('href', JIRA_URL)
})

Then(
  "the closed regression's triage shows its Jira details",
  async ({ page }) => {
    const triageRow = new TestDetailsPage(page).triageRow(SEEDED_TRIAGE_ID)
    await expect(triageRow.locator('[data-field="bug_state"]')).toHaveText(
      historicalTriageBug.status
    )
    await expect(
      triageRow.locator('[data-field="release_blocker"]')
    ).toHaveText(historicalTriageBug.release_blocker)
  }
)

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
    await expect(new TestDetailsPage(page).historicalNotice()).toContainText(
      `Sample window: ${HISTORICAL_RANGE} (UTC)`
    )
  }
)

Then(
  'the notice says the regression does not match this report',
  async ({ page }) => {
    await expect(new TestDetailsPage(page).historicalNotice()).toContainText(
      `Regression ${CLOSED_REGRESSION_ID} does not match this report (test ID)`
    )
    await expect(
      page.getByText('Closed regression, historical report')
    ).toHaveCount(0)
  }
)

Then('the notice says the regression was not found', async ({ page }) => {
  await expect(new TestDetailsPage(page).historicalNotice()).toContainText(
    `Regression ${CLOSED_REGRESSION_ID} was not found`
  )
})

Then(
  'the closed regression notice explains there is no historical data',
  async ({ page }) => {
    const notice = new TestDetailsPage(page).historicalNotice()
    await expect(notice).toContainText('Closed regression, historical report')
    await expect(notice).toContainText(
      'No test results are available for this historical window.'
    )
  }
)

Then(
  'the notice says the regression closed only in this view',
  async ({ page }) => {
    const notice = new TestDetailsPage(page).historicalNotice()
    await expect(notice).toContainText('Closed in this view, historical report')
    await expect(notice).toContainText(
      `Regression ID: ${CLOSED_IN_VIEW_REGRESSION_ID}`
    )
    await expect(notice).toContainText(
      `Closed: Closed in ${VIEW} on ${HISTORICAL_CLOSED_AT}; still open in another view`
    )
    await expect(notice).toContainText(
      `Sample window: ${HISTORICAL_RANGE} (UTC)`
    )
  }
)

Then(
  'the notice says the regression closed only in this view without a date',
  async ({ page }) => {
    const notice = new TestDetailsPage(page).historicalNotice()
    await expect(notice).toContainText('Closed in this view, historical report')
    await expect(notice).toContainText(
      `Closed: Closed in ${VIEW}; still open in another view`
    )
  }
)

Then(
  'the notice says closure in a specific view is unknown',
  async ({ page }) => {
    const notice = new TestDetailsPage(page).historicalNotice()
    await expect(notice).toContainText(
      'Historical report for a tracked regression'
    )
    await expect(notice).toContainText(
      'Not closed overall; closure in a specific view is unknown because this report has no view'
    )
  }
)

Then('the notice says the regression could not be loaded', async ({ page }) => {
  const notice = new TestDetailsPage(page).historicalNotice()
  await expect(notice).toContainText(
    `Unable to load regression ${CLOSED_REGRESSION_ID}: API server returned 500`
  )
  await expect(
    page.getByText('Closed regression, historical report')
  ).toHaveCount(0)
})
