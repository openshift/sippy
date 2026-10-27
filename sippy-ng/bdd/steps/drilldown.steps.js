import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import {
  BASE_RELEASE,
  CAPABILITY,
  COMPONENT,
  SAMPLE_RELEASE,
  TEST_ID,
  TEST_NAME,
} from '../fixtures/data.js'
import { test } from '../fixtures/test.js'
import { ReportPage } from '../pages/reportPage.js'
import { TestDetailsPage } from '../pages/testDetailsPage.js'

const { When, Then } = createBdd(test)

When('I open the component-wide drill-down', async ({ page }) => {
  await new ReportPage(page).componentLink().click()
  await expect(page.getByRole('heading', { name: COMPONENT })).toBeVisible()
})

When('I open the environment drill-down', async ({ page }) => {
  await new ReportPage(page).cellLink().click()
  await expect(
    page.getByRole('heading', { name: new RegExp(COMPONENT) })
  ).toBeVisible()
})

When('I open capability {string}', async ({ page }, capability) => {
  await page.getByTestId(`cr-capability-${capability}`).click()
})

When('I open test {string}', async ({ page }, testId) => {
  await page.getByTestId(`cr-test-${testId}`).click()
})

When('I open the test details from the test row', async ({ page }) => {
  await page.getByTestId('cr-test-details-link-0').click()
})

When('I open the test details deep link', async ({ page }) => {
  const params = new URLSearchParams({
    testId: TEST_ID,
    component: COMPONENT,
    capability: CAPABILITY,
    baseRelease: BASE_RELEASE,
    sampleRelease: SAMPLE_RELEASE,
  })
  await page.goto(`component_readiness/test_details?${params}`)
})

Then('I see capability {string}', async ({ page }, capability) => {
  await expect(page.getByTestId(`cr-capability-${capability}`)).toBeVisible()
})

Then('I see test {string}', async ({ page }, testId) => {
  await expect(page.getByTestId(`cr-test-${testId}`)).toBeVisible()
})

Then('the environment heading keeps the variant identity', async ({ page }) => {
  await expect(
    page.getByRole('heading', {
      name: /Platform:aws Architecture:amd64 > Networking/,
    })
  ).toBeVisible()
})

Then('I am on the test details page for the fixture test', async ({ page }) => {
  await expect(page).toHaveURL(new RegExp(`testId=${TEST_ID}`))
  await expect(page).toHaveURL(new RegExp(`component=${COMPONENT}`))
  await expect(page).toHaveURL(new RegExp(`capability=${CAPABILITY}`))
  await new TestDetailsPage(page).expectTest(TEST_NAME)
})
