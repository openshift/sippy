import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { ALT_COMPONENT, COMPONENT, OTHER_COMPONENT } from '../fixtures/data.js'
import { test } from '../fixtures/test.js'
import { ReportPage } from '../pages/reportPage.js'

const { Given, When, Then } = createBdd(test)

Given('a component readiness API', async ({ api }) => {
  api.reportMode = 'ok'
})

Given('a read-only component readiness API', async ({ api }) => {
  api.readOnly = true
})

Given('the API returns an empty report', async ({ api }) => {
  api.reportMode = 'empty'
})

Given('the API returns an error for the report', async ({ api }) => {
  api.reportMode = 'error'
})

Given('the API returns a cancelled report', async ({ api }) => {
  api.reportMode = 'cancelled'
})

Given('the current report is slow', async ({ api }) => {
  api.holdCurrentReport()
})

When('I open the component readiness report', async ({ page }) => {
  const report = new ReportPage(page)
  await report.open()
  await report.expectComponent(COMPONENT)
})

When('I open the component readiness report directly', async ({ page }) => {
  const report = new ReportPage(page)
  await report.open()
})

When('I open the loading component readiness report', async ({ page }) => {
  await page.goto('component_readiness/main')
  await page.getByTestId('generate-report-top').waitFor()
})

When('I search rows for {string}', async ({ page }, text) => {
  await new ReportPage(page).searchRows(text)
})

When('I clear the report search', async ({ page }) => {
  await new ReportPage(page).clearSearch()
})

When('I show only red rows', async ({ page }) => {
  await new ReportPage(page).openAllComponents()
  await new ReportPage(page).setRedOnly(true)
})

When(
  'I change the sample release to {string} without generating',
  async ({ page }, release) => {
    await new ReportPage(page).changeSampleRelease(release)
  }
)

async function generateAndWait(page) {
  const reportResponse = page.waitForResponse((response) => {
    const path = new URL(response.url()).pathname
    return path.endsWith('/api/component_readiness')
  })
  await new ReportPage(page).generate()
  await reportResponse
}

When('I generate the report', async ({ page }) => {
  await generateAndWait(page)
})

When('I reload the report', async ({ page }) => {
  await page.reload()
})

When('I set a sample pull request and generate', async ({ page }) => {
  const report = new ReportPage(page)
  await report.fillPullRequest('https://github.com/openshift/sippy/pull/4130')
  await generateAndWait(page)
})

When('I set sample payload tags and generate', async ({ page }) => {
  const report = new ReportPage(page)
  await report.fillPayloadTags('5.1.0-0.ci-2026-10-01-120000')
  await generateAndWait(page)
})

When('I drop the Platform group-by and generate', async ({ page }) => {
  const report = new ReportPage(page)
  await report.setGroupBy('Platform', false)
  await generateAndWait(page)
})

When('I set advanced options and generate', async ({ page }) => {
  const report = new ReportPage(page)
  await report.setNewTestPassRate(90)
  await report.setFlakeAsFailure()
  await generateAndWait(page)
})

When('I filter by the sdn capability and generate', async ({ page }) => {
  const report = new ReportPage(page)
  await report.selectTestCapability('sdn')
  await generateAndWait(page)
})

When('I release the held report', async ({ api, page }) => {
  const heldResponse = page.waitForResponse((response) => {
    const path = new URL(response.url()).pathname
    return path.endsWith('/api/component_readiness')
  })
  api.releaseHeldReport()
  await heldResponse
})

Then('I see component {string}', async ({ page }, name) => {
  await expect(page.getByTestId(`cr-component-${name}`)).toBeVisible()
})

Then('I do not see component {string}', async ({ page }, name) => {
  await expect(page.getByTestId(`cr-component-${name}`)).toHaveCount(0)
})

Then('the regressed components tab shows {int}', async ({ page }, count) => {
  await expect(
    page.getByRole('tab', {
      name: new RegExp(`Regressed Components \\(${count}\\)`),
    })
  ).toBeVisible()
})

Then('the all components tab shows {string}', async ({ page }, name) => {
  await new ReportPage(page).openAllComponents()
  await expect(page.getByTestId(`cr-component-${name}`)).toBeVisible()
})

Then('the report says there are no regressions', async ({ page }) => {
  await expect(page.getByText('No regressions detected.')).toBeVisible()
})

Then('the report says no data was found', async ({ page }) => {
  await expect(page.getByText('No Data found')).toBeVisible()
})

Then('the report says the operation was cancelled', async ({ page }) => {
  await expect(page.getByText(/Operation Cancelled/)).toBeVisible()
})

Then('the report shows a load failure', async ({ page }) => {
  await expect(
    page.getByRole('heading', {
      name: 'Failed to load component readiness data',
    })
  ).toBeVisible()
})

Then('the latest report request includes {string}', async ({ api }, text) => {
  const reports = api.reportRequests()
  expect(reports.length).toBeGreaterThan(0)
  const url = reports[reports.length - 1].url
  expect(url, url).toContain(text)
})

Then(
  'the latest report request does not include {string}',
  async ({ api }, text) => {
    const reports = api.reportRequests()
    expect(reports[reports.length - 1].url).not.toContain(text)
  }
)

Then('no report request includes {string}', async ({ api }, text) => {
  const matched = api
    .reportRequests()
    .filter((request) => request.url.includes(text))
  expect(matched).toEqual([])
})

Then('the page URL includes {string}', async ({ page }, text) => {
  await expect(page).toHaveURL(
    new RegExp(text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
  )
})

Then('triage write controls are hidden', async ({ page }) => {
  await expect(page.getByTestId('triage-regressions')).toHaveCount(0)
})

Then('no mutation requests were sent', async ({ api }) => {
  expect(api.mutationRequests()).toEqual([])
})

Then('I still see the generated component', async ({ page }) => {
  await expect(page.getByTestId(`cr-component-${ALT_COMPONENT}`)).toBeVisible()
  await expect(page.getByTestId(`cr-component-${OTHER_COMPONENT}`)).toHaveCount(
    0
  )
})
