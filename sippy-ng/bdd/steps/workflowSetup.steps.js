import * as data from '../fixtures/workflowData.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { test } from '../fixtures/test.js'

const { Given, When, Then } = createBdd(test)

Given('deterministic Sippy workflow evidence', async ({ api, page }) => {
  api.workflow = true
  api.labels = data.labels()
  await page.clock.setFixedTime(new Date(data.NOW))
})
Given(
  'the {string} workflow response is {string}',
  async ({ api }, path, mode) => {
    api.workflowModes[path] = mode
  }
)
Given('workflow writes are disabled', async ({ api }) => {
  api.readOnly = true
})
Given('the next workflow mutation will fail', async ({ api }) => {
  api.failNextMutation = true
})
When('I open workflow page {string}', async ({ page }, path) => {
  await page.goto(path)
})
Then(
  'I see a workflow error containing {string}',
  async ({ page }, message) => {
    await expect(
      page.getByRole('alert').filter({ hasText: message }).first()
    ).toBeVisible()
    await expect(
      page.getByText('server returned 500', { exact: false }).first()
    ).toBeVisible()
  }
)
Then('I see the workflow empty state {string}', async ({ page }, message) => {
  if (message === 'No data found') {
    await expect(
      page.getByRole('heading', { name: '5.0 Overview' })
    ).toBeVisible()
    await expect(
      page.getByRole('link').filter({
        has: page.getByRole('heading', { name: 'Install', exact: true }),
      })
    ).toHaveCount(0)
    await expect(page.getByText('No new test failures detected')).toBeVisible()
  } else await expect(page.getByText(message, { exact: true })).toBeVisible()
  await expect(page.getByRole('alert')).toHaveCount(0)
})
