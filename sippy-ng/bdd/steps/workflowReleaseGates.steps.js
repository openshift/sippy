import * as data from '../fixtures/workflowData.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { filter } from '../fixtures/workflowFilters.js'
import { test } from '../fixtures/test.js'
import { WorkflowPage } from '../pages/workflowPage.js'

const { When, Then } = createBdd(test)

const jobFilter = filter('name', 'equals', data.JOB)

When('I open the synthetic release overview', async ({ page }) => {
  await page.goto(`release/${data.RELEASE}`)
})
Then(
  'the release indicators show fixture-backed health values',
  async ({ api, page }) => {
    const summary = page.getByRole('link').filter({
      has: page.getByRole('heading', { name: 'Install', exact: true }),
    })
    await expect(summary).toContainText('85%')
    await expect(summary).toContainText('20 runs')
    await expect(summary).toHaveAttribute(
      'href',
      `/sippy-ng/install/${data.RELEASE}`
    )
    expect(
      new URL(
        api.requests.find((request) => request.pathname === '/api/health').url
      ).searchParams.get('release')
    ).toBe(data.RELEASE)
  }
)
When('I follow the install summary', async ({ page }) => {
  await page
    .getByRole('link')
    .filter({
      has: page.getByRole('heading', { name: 'Install', exact: true }),
    })
    .click()
})
Then(
  'install values and infrastructure links match the selected release',
  async ({ api, page }) => {
    await expect(
      page.getByRole('heading', { name: `Install health for ${data.RELEASE}` })
    ).toBeVisible()
    await expect(page.getByText('85% (20 runs)').first()).toBeVisible()
    const link = page.getByRole('link', { name: 'Infrastructure', exact: true })
    await new WorkflowPage(page).expectLinkFilters(
      link,
      `/tests/${data.RELEASE}/details`,
      [filter('name', 'equals', 'install should succeed: infrastructure')]
    )
    await link.click()
    await new WorkflowPage(page).expectFilter(
      api,
      '/api/tests',
      [filter('name', 'equals', 'install should succeed: infrastructure')],
      { release: data.RELEASE }
    )
    await expect(
      page.getByRole('link', {
        name: 'install should succeed: infrastructure',
        exact: true,
      })
    ).toBeVisible()
    await expect(
      page.getByRole('link', { name: data.TEST, exact: true })
    ).toHaveCount(0)
  }
)
When('I open the synthetic upgrade summary', async ({ page }) => {
  await page.goto(`release/${data.RELEASE}`)
  await page
    .getByRole('link')
    .filter({
      has: page.getByRole('heading', { name: 'Upgrade', exact: true }),
    })
    .click()
})
Then(
  'the upgrade test links to the exact test analysis',
  async ({ api, page }) => {
    await expect(
      page.getByRole('heading', { name: `Upgrade health for ${data.RELEASE}` })
    ).toBeVisible()
    await page.getByRole('checkbox', { name: 'Show Full' }).check()
    await expect(
      page.getByRole('cell').filter({ hasText: '80.00%' }).first()
    ).toBeVisible()
    await page.getByRole('link', { name: data.TEST, exact: true }).click()
    await expect(
      page.getByRole('heading', { name: data.TEST, exact: true })
    ).toBeVisible()
    await new WorkflowPage(page).expectFilter(
      api,
      '/api/tests',
      [filter('name', 'equals', data.TEST)],
      { release: data.RELEASE }
    )
  }
)
When(
  'I filter and select the synthetic feature gate',
  async ({ api, page }) => {
    await page.goto(`feature_gates/${data.RELEASE}`)
    await new WorkflowPage(page).search(data.GATE)
    await new WorkflowPage(page).expectFilter(
      api,
      '/api/feature_gates',
      [filter('feature_gate', 'contains', data.GATE)],
      { release: data.RELEASE }
    )
    await expect(
      page.getByRole('gridcell', { name: 'OtherGate', exact: true })
    ).toHaveCount(0)
    await new WorkflowPage(page)
      .row(data.GATE)
      .getByRole('link', { name: '1', exact: true })
      .click()
  }
)
Then(
  'its details and promotion view retain the selected gate and release',
  async ({ api, page }) => {
    await expect(
      page.getByRole('heading', { name: data.GATE, exact: true })
    ).toBeVisible()
    await expect(
      page.getByText(`Release: ${data.RELEASE}`, { exact: true })
    ).toBeVisible()
    await page.getByRole('tab', { name: 'Promotion Readiness' }).click()
    await expect(
      page
        .getByRole('alert')
        .filter({ hasText: `CI testing for "${data.GATE}"` })
    ).toBeVisible()
    const request = api.requests.find(
      (request) => request.pathname === `/api/feature_gates/${data.GATE}`
    )
    expect(new URL(request.url).searchParams.get('release')).toBe(data.RELEASE)
    await new WorkflowPage(page).expectLinkFilters(
      page.getByRole('link', { name: 'Analyze All', exact: true }).first(),
      `/jobs/${data.RELEASE}/analysis`,
      [jobFilter]
    )
    await expect(
      page.getByRole('cell', { name: data.TEST, exact: true })
    ).toBeVisible()
  }
)
