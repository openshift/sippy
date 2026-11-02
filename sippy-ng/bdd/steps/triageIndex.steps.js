import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { expectSettledEmpty } from '../fixtures/expectations.js'
import {
  failedFixTriage,
  JIRA_URL,
  LABEL_ID,
  labelledTriage,
  MISSING_TRIAGE_ID,
  resolvedTriage,
  SAMPLE_RELEASE,
  TEST_NAME,
  VIEW,
} from '../fixtures/data.js'
import { test } from '../fixtures/test.js'
import { TriageIndexPage } from '../pages/triageIndexPage.js'

const { Given, When, Then } = createBdd(test)

function detailRow(page, label) {
  return page.getByRole('row').filter({
    has: page.getByRole('cell', { name: label, exact: true }),
  })
}

function resolvedCell(page) {
  return detailRow(page, 'Resolved')
}

function includedTests(page) {
  return page.locator('.cr-triage-panel-element').filter({
    has: page.getByText('Test Failures', { exact: true }),
  })
}

function decodedHref(href) {
  return decodeURIComponent(decodeURIComponent(href))
}

Given('a resolved triage exists for another regression', async ({ api }) => {
  api.triages.push(resolvedTriage())
})

Given('a resolved triage with a failed fix exists', async ({ api }) => {
  api.triages.push(failedFixTriage())
})

Given('a triage with a labelled failed run exists', async ({ api }) => {
  api.triages.push(labelledTriage())
})

Given('the triage list fails', async ({ api }) => {
  api.triageListMode = 'error'
})

Given('the audit history is empty', async ({ api }) => {
  api.auditMode = 'empty'
})

Given('the audit history fails', async ({ api }) => {
  api.auditMode = 'error'
})

When('I open the triage index', async ({ page }) => {
  await new TriageIndexPage(page).open()
})

When('I open the triage index without waiting for rows', async ({ page }) => {
  await page.goto('component_readiness/triages')
})

When('I search the triage index for {string}', async ({ page }, text) => {
  await new TriageIndexPage(page).search(text)
})

When('I apply the {string} triage bookmark', async ({ page }, name) => {
  await new TriageIndexPage(page).applyBookmark(name)
})

When('I select the triage {string}', async ({ page }, description) => {
  await page.getByText(description, { exact: true }).click()
})

When('I open the missing triage', async ({ page }) => {
  await page.goto(`component_readiness/triages/${MISSING_TRIAGE_ID}`)
})

When('I open the triage audit logs', async ({ page }) => {
  await page.getByRole('button', { name: 'View Audit Logs' }).click()
})

When('I filter regressions to the {string} label', async ({ page }, title) => {
  await page
    .getByRole('button', { name: `Filter regressions to ${title}` })
    .click()
})

When('I clear the label filter', async ({ page }) => {
  await page
    .getByRole('button', { name: /Filtered by:/ })
    .getByTestId('CancelIcon')
    .click()
})

Then('the triage index shows {string}', async ({ page }, description) => {
  await expect(new TriageIndexPage(page).row(description)).toBeVisible()
})

Then(
  'the triage index does not show {string}',
  async ({ page }, description) => {
    await expect(new TriageIndexPage(page).row(description)).toHaveCount(0)
  }
)

Then(
  'the Jira link for {string} opens {string}',
  async ({ page }, description, url) => {
    const link = new TriageIndexPage(page)
      .row(description)
      .locator('[data-field="jira_key"] a')
    await expect(link).toHaveAttribute('href', url)
    await expect(link).toHaveText(url.split('/').pop())
  }
)

Then(
  'the details link for {string} opens triage {int}',
  async ({ page }, description, id) => {
    const link = new TriageIndexPage(page)
      .row(description)
      .locator('[data-field="details"] a')
    await expect(link).toHaveAttribute(
      'href',
      `/sippy-ng/component_readiness/triages/${id}`
    )
  }
)

Then('the triage index has no rows', async ({ page }) => {
  await expect(page.getByText('No rows', { exact: true })).toBeVisible()
  await expect(
    page.locator('[data-field="description"][role="gridcell"]')
  ).toHaveCount(0)
})

Then('the page shows {string}', async ({ page }, text) => {
  await expect(page.getByText(text, { exact: true })).toBeVisible()
})

Then('the included tests show {string}', async ({ page }, name) => {
  await expect(
    includedTests(page).getByRole('link', { name, exact: true })
  ).toBeVisible()
})

Then('the included tests do not show {string}', async ({ page }, name) => {
  await expect(includedTests(page)).toBeVisible()
  await expect(
    includedTests(page).getByRole('link', { name, exact: true })
  ).toHaveCount(0)
})

Then('the triage is shown as not resolved', async ({ page }) => {
  const cell = resolvedCell(page).first()
  await expect(cell.getByTestId('ErrorIcon')).toBeVisible()
  await expect(cell.getByTestId('CheckCircleIcon')).toHaveCount(0)
  await cell.getByTestId('ErrorIcon').hover()
  await expect(
    page.getByRole('tooltip', { name: 'Not resolved' })
  ).toBeVisible()
})

Then('the triage is shown as resolved on {string}', async ({ page }, date) => {
  const cell = resolvedCell(page).first()
  await expect(cell.getByTestId('CheckCircleIcon')).toBeVisible()
  await expect(cell.getByAltText('Fixed regression')).toHaveCount(0)
  await cell.getByTestId('CheckCircleIcon').hover()
  await expect(
    page.getByRole('tooltip', { name: new RegExp(date) })
  ).toBeVisible()
  await expect(detailRow(page, 'Resolution Date')).toContainText(date)
})

Then('the triage is shown as a failed fix', async ({ page }) => {
  const cell = resolvedCell(page).first()
  await expect(cell.getByAltText('Fixed regression')).toBeVisible()
  await expect(cell.getByTestId('CheckCircleIcon')).toHaveCount(0)
  await cell.getByAltText('Fixed regression').hover()
  await expect(
    page.getByRole('tooltip', { name: /Failed fix detected/ })
  ).toBeVisible()
})

Then('the triage Jira link opens {string}', async ({ page }, url) => {
  const link = detailRow(page, 'Jira').getByRole('link')
  await expect(link).toHaveAttribute('href', url)
  await expect(link).toHaveText(url.split('/').pop())
})

Then(
  'the included test {string} links to its test analysis',
  async ({ page }, name) => {
    const link = includedTests(page).getByRole('link', { name, exact: true })
    const href = decodedHref(await link.getAttribute('href'))
    expect(href).toContain(`/sippy-ng/tests/${SAMPLE_RELEASE}/analysis?`)
    expect(href).toContain(`test=${name}`)
    expect(href).toContain('"value":"Platform:aws"')
  }
)

Then(
  'the included test status for view {string} links to its test details',
  async ({ page }, view) => {
    await expect(
      includedTests(page).getByRole('columnheader', { name: view })
    ).toBeVisible()
    const link = includedTests(page).locator('.status a')
    await expect(link.getByAltText('Fixed regression')).toBeVisible()
    const href = decodedHref(await link.getAttribute('href'))
    expect(href).toContain('/sippy-ng/component_readiness/test_details?')
    expect(href).toContain(`view=${VIEW}`)
    expect(href).toContain('testId=test-sdn-pod-network')
  }
)

Then('the audit logs were requested for triage {int}', async ({ api }, id) => {
  const requests = api.requestsTo(/\/triages\/\d+\/audit$/)
  expect(requests.map((request) => request.pathname)).toEqual([
    `/api/component_readiness/triages/${id}/audit`,
  ])
})

Then(
  'the audit log shows the description change by {string}',
  async ({ page }, user) => {
    const dialog = page.getByRole('dialog')
    const row = dialog.getByRole('row').filter({ hasText: 'Updated' })
    await expect(row).toContainText(user)
    await expect(row).toContainText('description')
    await expect(row.getByText('- pod network flake')).toBeVisible()
    await expect(row.getByText('+ pod network regression')).toBeVisible()
  }
)

Then('the audit log shows the creation by {string}', async ({ page }, user) => {
  const row = page.getByRole('dialog').getByRole('row').filter({
    hasText: 'Created',
  })
  await expect(row).toContainText(user)
  await expect(row).toContainText('No material fields modified')
})

Then('the audit log says there is no history', async ({ page }) => {
  await expect(
    page.getByText('No audit logs found for this triage.', { exact: true })
  ).toBeVisible()
})

Then('the audit log shows a load error', async ({ page }) => {
  await expect(
    page.getByText(/Error loading audit logs: Error: API server returned 500/)
  ).toBeVisible()
  await expect(page.getByText('No audit logs found')).toHaveCount(0)
})

Then(
  'the failure labels list {string} for {int} regression',
  async ({ page }, title, count) => {
    // The heading's accessible name is its tooltip, so match the visible text.
    await expect(
      page.getByText('Failure Labels (1)', { exact: true })
    ).toBeVisible()
    const row = page.getByRole('row').filter({ hasText: title })
    await expect(row.getByRole('cell').nth(2)).toHaveText(String(count))
    await expect(row).toContainText('50.0%')
  }
)

Then(
  'the {string} label links to its Jira bug {string}',
  async ({ page }, title, bug) => {
    const row = page.getByRole('row').filter({ hasText: title })
    await expect(row.getByRole('link', { name: bug })).toHaveAttribute(
      'href',
      `https://redhat.atlassian.net/browse/${bug}`
    )
  }
)

Then(
  'the {string} job runs link filters runs by that label',
  async ({ page }, title) => {
    const row = page.getByRole('row').filter({ hasText: title })
    const link = row.getByRole('link', {
      name: 'View job runs with this label',
    })
    const href = decodedHref(await link.getAttribute('href'))
    expect(href).toMatch(/^\/sippy-ng\/jobs\/[^/]+\/runs\?filters=/)
    expect(JSON.parse(href.split('filters=')[1])).toEqual({
      items: [{ field: 'labels', operator: 'has entry', value: LABEL_ID }],
    })
  }
)

Then('the included tests are filtered by {string}', async ({ page }, title) => {
  await expect(page.getByText(`Filtered by: ${title}`)).toBeVisible()
  await expect(
    page.getByRole('button', { name: `Filter regressions to ${title}` })
  ).toHaveAttribute('aria-pressed', 'true')
})

Then('the failure labels say none are applied', async ({ page }) => {
  await expect(
    page.getByText('No labels applied', { exact: true })
  ).toBeVisible()
})

Then('no job labels were requested', async ({ api }) => {
  await expectSettledEmpty(() => api.requestsTo(/\/api\/jobs\/labels$/))
})

Then('the triage details are not shown', async ({ page }) => {
  await expect(
    page.getByRole('heading', { name: 'Triage Details' })
  ).toHaveCount(0)
  await expect(page.getByText(TEST_NAME)).toHaveCount(0)
  await expect(page.getByText(JIRA_URL)).toHaveCount(0)
})
