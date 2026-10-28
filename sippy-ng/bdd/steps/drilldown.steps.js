import {
  BASE_RELEASE,
  CAPABILITY,
  COLUMN_NAME,
  COMPONENT,
  SAMPLE_RELEASE,
  TEST_ID,
  TEST_NAME,
  VIEW,
  VIEW_BASE_START,
  VIEW_SAMPLE_START,
} from '../fixtures/data.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { ReportPage } from '../pages/reportPage.js'
import { test } from '../fixtures/test.js'
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
  await page
    .getByTestId(`cr-test-details-link-${TEST_ID}-${COLUMN_NAME}`)
    .click()
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

Then('the page keeps the generated view context', async ({ page }) => {
  const url = decodeURIComponent(page.url())
  expect(url).toContain(`sampleRelease=${SAMPLE_RELEASE}`)
  expect(url).toContain(`baseRelease=${BASE_RELEASE}`)
  expect(url).toContain(VIEW_SAMPLE_START.slice(0, 10))
  expect(url).toContain(VIEW_BASE_START.slice(0, 10))
  expect(url).toContain('includeVariant=Platform:aws')
  expect(url).toContain('includeVariant=Architecture:amd64')
})

Then('the page keeps the environment variants', async ({ page }) => {
  const url = decodeURIComponent(page.url())
  expect(url).toContain('Platform=aws')
  expect(url).toContain('Architecture=amd64')
  expect(url).toContain(`environment=${COLUMN_NAME}`)
})

Then(
  'the test details request keeps the view context',
  async ({ api, page }) => {
    const url = decodeURIComponent(page.url())
    expect(url).toContain(`view=${VIEW}`)
    expect(url).toContain(`sampleRelease=${SAMPLE_RELEASE}`)
    expect(url).toContain(`baseRelease=${BASE_RELEASE}`)
    expect(url).toContain(VIEW_SAMPLE_START)
    expect(url).toContain(VIEW_BASE_START)
    expect(url).toContain('includeVariant=Platform:aws')
    const details = api.requests.filter(
      (request) =>
        request.method === 'GET' &&
        request.pathname.endsWith('/api/component_readiness/test_details')
    )
    expect(details.length).toBeGreaterThan(0)
    const requested = decodeURIComponent(details[details.length - 1].url)
    expect(requested).toContain(`view=${VIEW}`)
    expect(requested).toContain(`baseRelease=${BASE_RELEASE}`)
    expect(requested).toContain(VIEW_SAMPLE_START)
    expect(requested).toContain('includeVariant=Platform:aws')
  }
)
