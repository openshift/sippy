import * as data from '../fixtures/workflowData.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { expectExactFilter } from '../fixtures/workflowExpectations.js'
import { filter } from '../fixtures/workflowFilters.js'
import { test } from '../fixtures/test.js'
import { WorkflowPage } from '../pages/workflowPage.js'

const { When, Then } = createBdd(test)

const jobFilter = filter('name', 'equals', data.JOB)
const variantFilter = filter('variants', 'has entry', 'Platform:aws')

When(
  'I browse all jobs and change the result ordering and page',
  async ({ page }) => {
    await page.goto(`jobs/${data.RELEASE}`)
    await expect(
      page.getByRole('link', { name: data.JOB, exact: true })
    ).toBeVisible()
    await page.getByRole('columnheader', { name: /^Name(?: Sort)?$/ }).click()
    await expect(
      page.getByRole('columnheader', { name: /^Name(?: Sort)?$/ })
    ).toHaveAttribute('aria-sort', 'descending')
    await page.getByRole('button', { name: 'Go to next page' }).click()
  }
)
Then(
  'the jobs request and visible rows reflect my choices',
  async ({ api, page }) => {
    await expectExactFilter(api, '/api/jobs', [], {
      release: data.RELEASE,
      sortField: 'name',
      sort: 'desc',
    })
    await expect(page.getByText('26–27 of 27')).toBeVisible()
    await expect(
      page.getByRole('link', { name: data.JOB, exact: true })
    ).toHaveCount(0)
    await expect(
      page.getByRole('link', { name: 'periodic-synthetic-01', exact: true })
    ).toBeVisible()
  }
)
When(
  'I search for the synthetic job and open its analysis',
  async ({ api, page }) => {
    await page.getByRole('button', { name: 'Go to previous page' }).click()
    await new WorkflowPage(page).search(data.JOB)
    await expectExactFilter(api, '/api/jobs', [
      filter('name', 'contains', data.JOB),
    ])
    await expect(
      page.getByRole('link', { name: 'periodic-synthetic-01', exact: true })
    ).toHaveCount(0)
    await page.getByRole('link', { name: data.JOB, exact: true }).click()
  }
)
Then(
  'job analysis requests exactly the selected job',
  async ({ api, page }) => {
    await expect(
      page.getByRole('heading', { name: 'Job Analysis', exact: true })
    ).toBeVisible()
    await expectExactFilter(api, '/api/jobs/analysis', [jobFilter], {
      release: data.RELEASE,
    })
    await expect(page.getByText('20 runs, 16 successful')).toBeVisible()
  }
)
When('I open matching job runs', async ({ page }) => {
  await page
    .getByRole('link', { name: 'View matching job runs', exact: true })
    .click()
})
Then(
  'the selected run links to its own Prow evidence',
  async ({ api, page }) => {
    await expectExactFilter(api, '/api/jobs/runs', [jobFilter], {
      release: data.RELEASE,
    })
    // The oldest selected run is on the second page with descending timestamps.
    await page.getByRole('button', { name: 'Go to next page' }).click()
    await expect(page.locator(`a[href="${data.PROW}"]`)).toHaveAttribute(
      'target',
      '_blank'
    )
  }
)
When('I open the Permafailing jobs tab', async ({ page }) => {
  await page.goto(`jobs/${data.RELEASE}`)
  await page.getByRole('tab', { name: 'Permafailing' }).click()
})
Then(
  'both reporting periods require zero passing results',
  async ({ api, page }) => {
    await expectExactFilter(
      api,
      '/api/jobs',
      [
        filter('current_pass_percentage', '=', '0'),
        filter('previous_pass_percentage', '=', '0'),
      ],
      { release: data.RELEASE, sortField: 'last_pass', sort: 'desc' }
    )
    await expect(
      page.getByRole('link', { name: 'periodic-synthetic-01', exact: true })
    ).toBeVisible()
    await expect(
      page.getByRole('link', { name: data.JOB, exact: true })
    ).toHaveCount(0)
    await expect(page.getByText('1–1 of 1')).toBeVisible()
  }
)
When(
  'I browse all job runs and change page and sort',
  async ({ api, page }) => {
    await page.goto(`jobs/${data.RELEASE}`)
    await page.getByRole('tab', { name: 'All job runs' }).click()
    await page.getByRole('button', { name: 'Go to next page' }).click()
    await expectExactFilter(api, '/api/jobs/runs', [], {
      page: '1',
      perPage: '25',
    })
    await page
      .getByRole('columnheader', { name: /^Date \/ Time(?: Sort)?$/ })
      .click()
  }
)
Then(
  'run requests contain the requested pagination and sort',
  async ({ api, page }) => {
    await expectExactFilter(api, '/api/jobs/runs', [], {
      release: data.RELEASE,
      sortField: 'timestamp',
      sort: 'asc',
      page: '1',
      perPage: '25',
    })
    await expect(page.getByText('26–27 of 27')).toBeVisible()
  }
)
When('I search and filter job runs to failures', async ({ api, page }) => {
  await page.getByRole('button', { name: 'Go to previous page' }).click()
  await new WorkflowPage(page).search(data.JOB)
  await expectExactFilter(api, '/api/jobs/runs', [
    filter('job', 'contains', data.JOB),
  ])
  await new WorkflowPage(page).numericFilter('Failures', '>', '0', true)
})
Then(
  'only the failed selected run and its Prow link remain',
  async ({ api, page }) => {
    await expectExactFilter(api, '/api/jobs/runs', [
      filter('job', 'contains', data.JOB),
      filter('test_failures', '>', '0'),
    ])
    await expect(page.getByText('1–1 of 1')).toBeVisible()
    await expect(page.locator(`a[href="${data.PROW}"]`)).toBeVisible()
    await expect(
      page.getByRole('gridcell', { name: 'Failure (e2e)', exact: true })
    ).toBeVisible()
  }
)

When(
  'I select the URL-sensitive test from {string}',
  async ({ api, page }, tab) => {
    const params = new URLSearchParams({
      filters: JSON.stringify({ items: [variantFilter] }),
      period: 'twoDay',
    })
    await page.goto(`tests/${data.RELEASE}?${params}`)
    await page.getByRole('tab', { name: tab, exact: true }).click()
    await expect(
      page.getByRole('tab', { name: tab, exact: true })
    ).toHaveAttribute('aria-selected', 'true')
    await expect(page).toHaveURL(
      tab === 'All tests' ? /tests\/5\.0\?/ : /tests\/5\.0\/details\?/
    )
    await new WorkflowPage(page).search(data.TEST)
    await expectExactFilter(
      api,
      '/api/tests',
      [variantFilter, filter('name', 'contains', data.TEST)],
      { collapse: tab === 'All tests' ? 'true' : 'false', period: 'twoDay' }
    )
    await expect(
      page.getByRole('link', { name: `${data.TEST} near match`, exact: true })
    ).toBeVisible()
    await page.getByRole('link', { name: data.TEST, exact: true }).click()
  }
)
Then(
  'every analysis request identifies the exact test and variants',
  async ({ api, page }) => {
    await expect(
      page.getByRole('heading', { name: data.TEST, exact: true })
    ).toBeVisible()
    await expectExactFilter(
      api,
      '/api/tests',
      [filter('name', 'equals', data.TEST), variantFilter],
      { release: data.RELEASE, period: 'twoDay' }
    )
    for (const path of [
      '/api/tests/analysis/overall',
      '/api/tests/analysis/jobs',
      '/api/tests/analysis/variants',
      '/api/tests/durations',
      '/api/tests/outputs',
    ]) {
      await expect
        .poll(() => api.requestsForPath(path).length)
        .toBeGreaterThan(0)
      const query = new URL(api.requestsForPath(path).at(-1).url).searchParams
      expect(query.get('test'), path).toBe(data.TEST)
      expect(query.get('release'), path).toBe(data.RELEASE)
      expect(JSON.parse(query.get('filter')).items, path).toEqual([
        expect.objectContaining(filter('name', 'equals', data.TEST)),
        expect.objectContaining(variantFilter),
      ])
    }
  }
)
When('I open failed runs for the selected test', async ({ page }) => {
  await page.getByRole('link', { name: 'Failed Runs (3)', exact: true }).click()
})
Then(
  'run evidence retains test, variant, period and failure filters',
  async ({ api, page }) => {
    await expectExactFilter(
      api,
      '/api/jobs/runs',
      [
        variantFilter,
        filter('failed_test_names', 'has entry', data.TEST),
        filter('timestamp', '>', '2026-10-24T12:00:00.000Z'),
      ],
      { release: data.RELEASE, sortField: 'timestamp', sort: 'desc' }
    )
    await expect(page.getByText('1–1 of 1')).toBeVisible()
    await expect(page.locator(`a[href="${data.PROW}"]`)).toBeVisible()
  }
)
