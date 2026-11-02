import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { expectSettledEmpty, latestRequest } from '../fixtures/expectations.js'
import {
  JIRA_URL,
  matchableTriage,
  rolledOffTriage,
  SEEDED_TRIAGE_ID,
} from '../fixtures/data.js'
import { test } from '../fixtures/test.js'
import { TriagePage } from '../pages/triagePage.js'

const { Given, When, Then } = createBdd(test)

const triageMatchesPath = /\/triages\/\d+\/matches$/

function matchesDialog(page) {
  return page.getByRole('dialog', { name: /Potential Matching Regressions/ })
}

function matchRow(page, name) {
  return matchesDialog(page)
    .getByRole('row')
    .filter({ has: page.getByText(name, { exact: true }) })
}

Given('the regression has a matching triage', async ({ api }) => {
  api.regressionMatchesMode = 'ok'
})

Given('the regression match lookup fails', async ({ api }) => {
  api.regressionMatchesMode = 'error'
})

Given('a triage with regressions in two views exists', async ({ api }) => {
  api.triages.push(matchableTriage())
})

Given('a triage whose regressions rolled off exists', async ({ api }) => {
  api.triages.push(rolledOffTriage())
})

Given('the triage has potential matching regressions', async ({ api }) => {
  api.triageMatchesMode = 'ok'
})

When('I open the triage dialog with potential matches', async ({ page }) => {
  await new TriagePage(page).selectRegression()
  const matchesSettled = page.waitForResponse((response) =>
    /\/regressions\/\d+\/matches$/.test(new URL(response.url()).pathname)
  )
  await page.getByRole('button', { name: 'Set Triage Info' }).click()
  await matchesSettled
})

When('I add the regression to the matching triage', async ({ page }) => {
  await page
    .getByRole('dialog', { name: 'Triage' })
    .getByRole('button', { name: 'Add to Triage' })
    .click()
})

When('I open the potential matching regressions', async ({ page }) => {
  await page
    .getByRole('button', { name: 'Link Additional Matching Regressions' })
    .click()
  await expect(matchesDialog(page).getByRole('grid')).toBeVisible()
})

When('I turn off the {string} match filter', async ({ page }, name) => {
  await matchesDialog(page)
    .getByRole('checkbox', { name: new RegExp(name) })
    .uncheck()
})

When('I turn on the {string} match filter', async ({ page }, name) => {
  await matchesDialog(page)
    .getByRole('checkbox', { name: new RegExp(name) })
    .check()
})

When(
  'I switch the potential matches view to {string}',
  async ({ page }, view) => {
    const reloaded = page.waitForResponse((response) =>
      response.url().includes(`view=${view}`)
    )
    // The other combobox in the dialog is the grid's rows-per-page picker.
    await matchesDialog(page)
      .getByRole('combobox')
      .filter({ hasText: /^5\.0-/ })
      .click()
    await page.getByRole('option', { name: view, exact: true }).click()
    await reloaded
    await expect(matchesDialog(page).getByRole('grid')).toBeVisible()
  }
)

When('I select the potential match {string}', async ({ page }, name) => {
  await matchRow(page, name).getByRole('checkbox').check()
})

When('I add the selected matches to the triage', async ({ page }) => {
  await matchesDialog(page)
    .getByRole('button', { name: 'Add (1) to Triage' })
    .click()
})

When(
  'I choose to view the matches in a new tab',
  async ({ context, page, scenario }) => {
    // The report reloads soon after and drops the success message, so the
    // tab is awaited by a later step instead of here.
    scenario.newTab = context.waitForEvent('page')
    await page.getByRole('button', { name: 'Yes, View Matches' }).click()
  }
)

Then(
  'the matching triage {string} has confidence {string}',
  async ({ page }, description, confidence) => {
    const dialog = page.getByRole('dialog', { name: 'Triage' })
    await expect(
      dialog.getByRole('tab', { name: 'Potential Matches', exact: true })
    ).toHaveAttribute('aria-selected', 'true')
    const row = dialog.getByRole('row').filter({ hasText: description })
    await expect(row).toContainText(confidence)
    await expect(row.getByRole('link', { name: 'TRT-2631' })).toHaveAttribute(
      'href',
      JIRA_URL
    )
  }
)

Then('the matching triage links to triage {int}', async ({ page }, id) => {
  const row = page
    .getByRole('dialog', { name: 'Triage' })
    .getByRole('row')
    .filter({ hasText: 'pod network regression' })
  await expect(row.locator('[data-field="details"] a')).toHaveAttribute(
    'href',
    `/sippy-ng/component_readiness/triages/${id}`
  )
})

Then('the matching triage success message is shown', async ({ page }) => {
  await expect(
    page.getByText(`Successfully added regression to triage: ${JIRA_URL}`, {
      exact: true,
    })
  ).toBeVisible()
})

Then(
  'the matching triage error is shown without a success message',
  async ({ page }) => {
    await expect(
      page.getByText('error adding test to triage entry: 400: rejected', {
        exact: true,
      })
    ).toBeVisible()
    await expect(page.getByText(/Successfully added regression/)).toHaveCount(0)
  }
)

Then('the match lookup error is shown', async ({ page }) => {
  await expect(
    page.getByText(/Error: API call failed: .*\/regressions\/101\/matches/)
  ).toBeVisible()
  await expect(
    page.getByText(/Return code = 500 \(matching failed\)/)
  ).toBeVisible()
})

Then(
  'the potential matches were requested for view {string}',
  async ({ api }, view) => {
    const request = latestRequest(api.requestsTo(triageMatchesPath))
    expect(request.pathname).toBe(
      `/api/component_readiness/triages/${SEEDED_TRIAGE_ID}/matches`
    )
    expect(new URLSearchParams(request.search).get('view')).toBe(view)
  }
)

Then('the potential matches show {string}', async ({ page }, name) => {
  await expect(matchRow(page, name)).toBeVisible()
})

Then('the potential matches do not show {string}', async ({ page }, name) => {
  await expect(matchesDialog(page).getByRole('grid')).toBeVisible()
  await expect(matchRow(page, name)).toHaveCount(0)
})

Then(
  'the triage link request keeps regression {int} and adds {int}',
  async ({ api }, kept, added) => {
    const put = latestRequest(api.putRequests())
    expect(put.pathname).toBe(
      `/api/component_readiness/triages/${SEEDED_TRIAGE_ID}`
    )
    expect(put.body.regressions.map((regression) => regression.id)).toEqual([
      kept,
      added,
    ])
    expect(put.body.url).toBe(JIRA_URL)
  }
)

Then('the potential matches dialog is closed', async ({ page }) => {
  await expect(matchesDialog(page)).toHaveCount(0)
})

Then(
  'triage {int} still has only regression {int}',
  async ({ api }, id, regressionId) => {
    expect(latestRequest(api.putRequests()).pathname).toBe(
      `/api/component_readiness/triages/${id}`
    )
    const triage = api.triages.find((entry) => entry.id === id)
    expect(triage.regressions.map((regression) => regression.id)).toEqual([
      regressionId,
    ])
  }
)

Then(
  'looking for matching regressions is disabled because they rolled off',
  async ({ page }) => {
    await expect(
      page.getByRole('button', { name: 'Link Additional Matching Regressions' })
    ).toBeDisabled()
    await expect(
      page.getByLabel(
        'All regressions have rolled off; no active views available for matching'
      )
    ).toBeVisible()
  }
)

Then('no potential matches were requested', async ({ api }) => {
  await expectSettledEmpty(() => api.requestsTo(triageMatchesPath))
})

Then(
  'the new triage offers {int} potential matches',
  async ({ page }, count) => {
    const dialog = page.getByRole('dialog', {
      name: 'Potential Matching Regressions Found',
    })
    await expect(dialog).toContainText(
      `We found ${count} potentially matching regressions`
    )
  }
)

Then(
  'the potential matches were checked for triage {int}',
  async ({ api }, id) => {
    const request = latestRequest(api.requestsTo(triageMatchesPath))
    expect(request.pathname).toBe(
      `/api/component_readiness/triages/${id}/matches`
    )
    expect(new URLSearchParams(request.search).has('release')).toBe(true)
  }
)

Then(
  'the new tab shows the potential matching regressions for triage {int}',
  async ({ scenario }, id) => {
    const tab = await scenario.newTab
    await tab.waitForLoadState()
    expect(new URL(tab.url()).pathname).toBe(
      `/sippy-ng/component_readiness/triages/${id}`
    )
    expect(new URL(tab.url()).searchParams.get('openMatches')).toBe('1')
    await expect(
      matchesDialog(tab).getByText('should keep service endpoints', {
        exact: true,
      })
    ).toBeVisible()
  }
)
