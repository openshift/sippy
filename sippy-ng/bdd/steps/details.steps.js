import {
  BASE_RELEASE,
  COMPONENT,
  FAILED_RUN_ID,
  PREVIOUS_RELEASE,
  TEST_ID,
} from '../fixtures/data.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { test } from '../fixtures/test.js'
import { TestDetailsPage } from '../pages/testDetailsPage.js'

const { Given, When, Then } = createBdd(test)

Given('test details are empty', async ({ api }) => {
  api.testDetailsMode = 'empty'
})

Given('test details fail', async ({ api }) => {
  api.testDetailsMode = 'error'
})

When('I show only failed runs', async ({ page }) => {
  await new TestDetailsPage(page).showOnlyFailures()
})

Then(
  'the test details show the sample and basis statistics',
  async ({ page }) => {
    const basis = page.getByText(/Basis \(historical\) Release:/)
    await expect(basis.getByText('Successes: 120')).toBeVisible()
    await expect(basis.getByText('Failures: 0')).toBeVisible()
    const sample = page.getByText(/Sample \(being evaluated\) Release:/)
    await expect(sample.getByText('Successes: 80')).toBeVisible()
    await expect(sample.getByText('Failures: 4')).toBeVisible()
  }
)

Then(
  'the test details assessment is a significant regression',
  async ({ page }) => {
    await expect(page.getByText('Assessment:')).toBeVisible()
    await expect(page.getByAltText('SignificantRegression')).toBeVisible()
    await expect(
      page.getByText('pass rate dropped from 99% to 80%').first()
    ).toBeVisible()
  }
)

Then('the informing lifecycle chip is shown', async ({ page }) => {
  await expect(page.getByLabel('Informing lifecycle')).toBeVisible()
})

Then('the historical release tabs are shown', async ({ page }) => {
  await expect(
    page.getByRole('tab', { name: `${PREVIOUS_RELEASE} Basis` })
  ).toBeVisible()
  await expect(
    page.getByRole('tab', { name: `${BASE_RELEASE} Basis` })
  ).toBeVisible()
})

When('I open the {string} historical tab', async ({ page }, name) => {
  await page.getByRole('tab', { name }).click()
})

Then(
  'the historical comparison shows basis release {string}',
  async ({ page }, release) => {
    const panel = page.getByRole('tabpanel')
    await expect(panel).toContainText(`Basis (historical) Release:`)
    await expect(panel.getByText(release, { exact: true })).toBeVisible()
    await expect(panel).toContainText('historical basis did not regress')
  }
)

Then('the latest report link opens that report', async ({ page }) => {
  const link = page.getByRole('link', { name: /View the latest report/ })
  await expect(link).toHaveAttribute(
    'href',
    new RegExp(`/sippy-ng/component_readiness/test_details\\?`)
  )
  await expect(link).toHaveAttribute('href', new RegExp(`testId=${TEST_ID}`))
  await expect(link).toHaveAttribute(
    'href',
    new RegExp(`component=${COMPONENT}`)
  )
  await link.click()
  await expect(page).toHaveURL(new RegExp(`testId=${TEST_ID}`))
  await expect(page).toHaveURL(new RegExp(`component=${COMPONENT}`))
  await expect(page).not.toHaveURL(/capability=/)
})

Then(
  'the stale report warning links to the latest report',
  async ({ page }) => {
    await expect(page.getByText(/more than 48 hours ago/)).toBeVisible()
    await expect(
      page.getByRole('link', { name: /View the latest report/ })
    ).toBeVisible()
  }
)

Then('the failed run link refers to the failed run', async ({ page }) => {
  const details = new TestDetailsPage(page)
  await expect(details.failedRunLink()).toHaveAttribute(
    'href',
    new RegExp(FAILED_RUN_ID)
  )
})

Then('the passing run link is hidden', async ({ page }) => {
  await expect(new TestDetailsPage(page).passedRunLink()).toHaveCount(0)
})

Then('test details say no data was found', async ({ page }) => {
  await expect(page.getByText('No data found for this test.')).toBeVisible()
})

Then('test details show a load failure', async ({ page }) => {
  await expect(
    page.getByRole('heading', {
      name: 'Failed to load component readiness data',
    })
  ).toBeVisible()
})
