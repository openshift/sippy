import * as data from '../fixtures/workflowData.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import {
  expectFilter,
  expectLinkFilters,
} from '../fixtures/workflowExpectations.js'
import { filter } from '../fixtures/workflowFilters.js'
import { test } from '../fixtures/test.js'
import { WorkflowPage } from '../pages/workflowPage.js'

const { When, Then } = createBdd(test)

const repoFilters = [
  filter('org', 'equals', data.ORG),
  filter('repo', 'equals', data.REPO),
]

When(
  'I select the synthetic build cluster from the overview',
  async ({ page }) => {
    await page.goto('build_clusters')
    const row = new WorkflowPage(page).row(data.CLUSTER).first()
    await expect(row).toContainText('80.00%')
    await expect(row).toContainText('90.00%')
    await row.getByRole('link', { name: data.CLUSTER, exact: true }).click()
  }
)
Then(
  'the cluster health and periodic runs match the selected cluster',
  async ({ api, page }) => {
    await expect(
      page.getByRole('heading', {
        name: `Periodic Job Runs On ${data.CLUSTER}`,
      })
    ).toBeVisible()
    await expectFilter(
      api,
      '/api/jobs/runs',
      [
        filter('cluster', 'equals', data.CLUSTER),
        { ...filter('name', 'starts with', 'pull-ci'), not: true },
      ],
      { useCurrentRelease: 'true' }
    )
    await expect(page.getByText('1–10 of 26')).toBeVisible()
    await expect(
      page.getByRole('link', { name: data.JOB, exact: true })
    ).toHaveCount(10)
    await expect(
      page.getByRole('link', { name: 'periodic-other-cluster', exact: true })
    ).toHaveCount(0)
  }
)
Then(
  'cluster health is {string} without healthy fixture rows',
  async ({ page }, mode) => {
    if (mode === 'empty') {
      await expect(page.getByText('No rows')).toHaveCount(2)
      await expect(page.getByRole('alert')).toHaveCount(0)
    } else
      await expect(page.getByRole('alert').first()).toContainText(
        'server returned 500'
      )
    await expect(
      page.getByRole('link', { name: data.CLUSTER, exact: true })
    ).toHaveCount(0)
  }
)
When(
  'I search for and select the synthetic repository',
  async ({ api, page }) => {
    await page.goto(`repositories/${data.RELEASE}`)
    await new WorkflowPage(page).search(data.REPO)
    await expectFilter(
      api,
      '/api/repositories',
      [filter('repo', 'contains', data.REPO)],
      { release: data.RELEASE }
    )
    await expect(
      page.getByRole('gridcell', { name: 'another-repo', exact: true })
    ).toHaveCount(0)
    await page.getByRole('gridcell', { name: data.REPO, exact: true }).click()
  }
)
Then(
  'repository detail requests and job links retain the repository filters',
  async ({ api, page }) => {
    await expect(
      page.getByRole('heading', {
        name: `${data.ORG}/${data.REPO}`,
        exact: true,
      })
    ).toBeVisible()
    await expectFilter(api, '/api/jobs', repoFilters, {
      release: data.RELEASE,
    })
    await expectFilter(api, '/api/pull_requests', repoFilters, {
      release: data.RELEASE,
    })
    await expect(
      page.getByText('Synthetic selected change', { exact: true })
    ).toBeVisible()
    await expect(
      page.getByText('Other repository change', { exact: true })
    ).toHaveCount(0)
    await expectLinkFilters(
      page,
      page.getByRole('link', { name: 'Presubmit jobs', exact: true }),
      `/jobs/${data.RELEASE}`,
      repoFilters
    )
  }
)
When('I follow repository jobs', async ({ page }) => {
  await page.getByRole('link', { name: 'Presubmit jobs', exact: true }).click()
})
Then(
  'the jobs page shows only the selected repository evidence',
  async ({ api, page }) => {
    await expectFilter(api, '/api/jobs', repoFilters, {
      release: data.RELEASE,
    })
    await expect(page.getByText('1–25 of 26')).toBeVisible()
    await expect(
      page.getByRole('link', { name: 'periodic-synthetic-26', exact: true })
    ).toHaveCount(0)
  }
)
When(
  'I filter pull requests to the selected number and repository',
  async ({ page }) => {
    const params = new URLSearchParams({
      filters: JSON.stringify({ items: repoFilters }),
    })
    await page.goto(`pull_requests/${data.RELEASE}?${params}`)
    await new WorkflowPage(page).numericFilter(
      'PR number',
      'equals',
      String(data.PR_NUMBER),
      true
    )
  }
)
Then(
  'the selected pull request links to its own GitHub and Prow history',
  async ({ api, page }) => {
    await expectFilter(
      api,
      '/api/pull_requests',
      [...repoFilters, filter('number', 'equals', String(data.PR_NUMBER))],
      { release: data.RELEASE }
    )
    const row = new WorkflowPage(page).row('Synthetic selected change')
    await expect(
      row.locator(
        `a[href="https://github.example.invalid/${data.ORG}/${data.REPO}/pull/${data.PR_NUMBER}"]`
      )
    ).toHaveAttribute('target', '_blank')
    await expect(row.locator('a[href*="pr-history"]')).toHaveAttribute(
      'href',
      `https://prow.ci.openshift.org/pr-history/?org=${data.ORG}&repo=${data.REPO}&pr=${data.PR_NUMBER}`
    )
    await expect(
      page.getByText('Other repository change', { exact: true })
    ).toHaveCount(0)
  }
)
When('I follow the chat navigation', async ({ page }) => {
  await page.goto(`jobs/${data.RELEASE}`)
  await page.goto('chat')
})
Then(
  'the Slack transition destination is configured without contacting Slack',
  async ({ api, page }) => {
    await expect(
      page.getByRole('heading', { name: 'Sippy Chat has moved' })
    ).toBeVisible()
    await expect(
      page.getByRole('link', { name: 'Open Slack', exact: true })
    ).toHaveAttribute('href', 'https://redhat.enterprise.slack.com/')
    await expect(
      page.getByRole('link', { name: 'Open Slack', exact: true })
    ).toHaveAttribute('target', '_blank')
    expect(api.unexpectedRequests).toEqual([])
  }
)
Then('local database navigation is hidden', async ({ page }) => {
  await expect(
    page.getByRole('heading', { name: 'Sippy Chat has moved' })
  ).toBeVisible()
  await expect(page.getByText('Releases', { exact: true })).toHaveCount(0)
  await expect(page.getByText('Last updated', { exact: false })).toHaveCount(0)
})

Then(
  'the selected cluster run evidence is {string}',
  async ({ api, page }, mode) => {
    await expectFilter(
      api,
      '/api/jobs/runs',
      [filter('cluster', 'equals', data.CLUSTER)],
      { useCurrentRelease: 'true' }
    )
    if (mode === 'empty') {
      await expect(page.getByText('No rows')).toBeVisible()
      await expect(page.getByRole('alert')).toHaveCount(0)
    } else
      await expect(page.getByRole('alert')).toContainText('server returned 500')
    await expect(
      page.getByRole('link', { name: data.JOB, exact: true })
    ).toHaveCount(0)
  }
)

When('I follow repository pull requests', async ({ page }) => {
  const link = page.getByRole('link', {
    name: 'Recently merged PR',
    exact: true,
  })
  await expectLinkFilters(page, link, `/pull_requests/${data.RELEASE}`, [
    ...repoFilters,
    { field: 'merged_at', operator: 'is not empty' },
  ])
  await link.click()
})
Then(
  'merged pull request evidence retains the repository and release',
  async ({ api, page }) => {
    await expectFilter(
      api,
      '/api/pull_requests',
      [...repoFilters, { field: 'merged_at', operator: 'is not empty' }],
      { release: data.RELEASE, sortField: 'merged_at', sort: 'desc' }
    )
    await expect(
      page.getByText('Synthetic selected change', { exact: true })
    ).toBeVisible()
    await expect(
      page.getByText('Other repository change', { exact: true })
    ).toHaveCount(0)
  }
)
