import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { expectSettledEmpty, latestRequest } from '../fixtures/expectations.js'
import {
  JIRA_URL,
  KEPT_TEST_ID,
  KEPT_TEST_NAME,
  REGRESSION_ID,
  SECOND_REGRESSION_ID,
  SEEDED_TRIAGE_ID,
  TEST_NAME,
  triageRecord,
} from '../fixtures/data.js'
import { ReportPage } from '../pages/reportPage.js'
import { test } from '../fixtures/test.js'
import { TriagePage } from '../pages/triagePage.js'

const { Given, When, Then } = createBdd(test)

Given('an unresolved triage exists', async ({ api }) => {
  api.triages.push(triageRecord())
})

Given('a resolved triage exists', async ({ api }) => {
  api.triages.push(
    triageRecord({
      resolved: { Time: '2026-10-23T00:00:00Z', Valid: true },
    })
  )
})

Given('a triage with two regressions exists', async ({ api }) => {
  api.triages.push(
    triageRecord({
      regressions: [
        {
          id: REGRESSION_ID,
          test_name: TEST_NAME,
          opened: '2026-10-21T00:00:00Z',
          variants: ['Platform:aws'],
        },
        {
          id: SECOND_REGRESSION_ID,
          test_name: KEPT_TEST_NAME,
          opened: '2026-10-22T00:00:00Z',
          variants: ['Platform:aws'],
        },
      ],
    })
  )
})

Given(
  'the report lists the fixture regression and another',
  async ({ api }) => {
    api.reportMode = 'pair'
  }
)

Given('the report lists distinct regression tabs', async ({ api }) => {
  api.reportMode = 'membership'
})

Given('the report lists more regressions than one page', async ({ api }) => {
  api.reportMode = 'paged'
})

Given('a triage exists for the other regression', async ({ api }) => {
  api.triages.push(
    triageRecord({
      regressions: [
        {
          id: SECOND_REGRESSION_ID,
          test_name: KEPT_TEST_NAME,
          test_id: KEPT_TEST_ID,
          opened: '2026-10-22T00:00:00Z',
          variants: ['Platform:aws'],
        },
      ],
    })
  )
})

Given('the next mutation is rejected', async ({ api }) => {
  api.failNextMutation = true
})

Given('the force-close preview is empty', async ({ api }) => {
  api.reportMode = 'empty-preview'
})

When('I open the regressed tests dialog', async ({ page }) => {
  await new ReportPage(page).openRegressedTests()
})

When('I open the {string} regressed tab', async ({ page }, name) => {
  await new TriagePage(page).openRegressedTab(name)
})

When('I check the regression for triage', async ({ page }) => {
  await new TriagePage(page).selectRegression()
})

When('I open the new triage form', async ({ page }) => {
  await new TriagePage(page).startTriage()
})

When(
  'I add the selected regression to the existing triage',
  async ({ page }) => {
    await new TriagePage(page).addSelectedToExisting()
  }
)

When('I triage the test from its details', async ({ page }) => {
  await new TriagePage(page).triageFromTestDetails()
})

When('I search the regressed tests for {string}', async ({ page }, text) => {
  await new TriagePage(page).searchRegressedTests(text)
})

When('I open the next regressed page', async ({ page }) => {
  await new TriagePage(page).openNextRegressedPage()
})

When('I create a triage for the selected regression', async ({ page }) => {
  const triage = new TriagePage(page)
  await triage.startTriage()
  await triage.fillNewTriage()
  await triage.submitTriage()
})

When('I submit an empty triage', async ({ page }) => {
  const triage = new TriagePage(page)
  await triage.startTriage()
  await triage.submitTriage()
})

When('I confirm a duplicate triage URL', async ({ page }) => {
  const triage = new TriagePage(page)
  await triage.startTriage()
  await triage.fillNewTriage()
  page.once('dialog', (dialog) => dialog.accept())
  await triage.submitTriage()
})

When('I cancel a duplicate triage URL', async ({ page }) => {
  const triage = new TriagePage(page)
  await triage.startTriage()
  await triage.fillNewTriage()
  page.once('dialog', (dialog) => dialog.dismiss())
  await triage.submitTriage()
})

When('I close the triage dialog without saving', async ({ page }) => {
  await page.keyboard.press('Escape')
})

When('I select a regressed test row', async ({ page }) => {
  await page.getByRole('gridcell', { name: TEST_NAME }).first().click()
})

When('I open triage {int}', async ({ page }, id) => {
  await new TriagePage(page).open(id)
})

When('I update the triage description', async ({ page }) => {
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Triage' })
  await dialog.getByLabel('Description').fill('updated pod network note')
  await page.getByTestId('triage-submit').click()
})

When('I try to remove every regression', async ({ page }) => {
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  await page.getByRole('tab', { name: 'Remove Regressions' }).click()
  const boxes = page.locator('input[name="regression"]')
  await boxes.nth(0).click()
  await boxes.nth(1).click()
})

When('I remove one regression', async ({ page }) => {
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  await page.getByRole('tab', { name: 'Remove Regressions' }).click()
  await page.locator('input[name="regression"]').first().click()
  await page.getByRole('button', { name: 'Remove Selected' }).click()
})

When('I delete the triage', async ({ page }) => {
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByTestId('delete-triage').click()
})

When('I cancel deleting the triage', async ({ page }) => {
  page.once('dialog', (dialog) => dialog.dismiss())
  await page.getByTestId('delete-triage').click()
})

When('I open force close', async ({ page }) => {
  await page.getByTestId('force-close-regressions').click()
})

When('I force close without a reason', async ({ page }) => {
  await page.getByTestId('force-close-confirm').click()
})

When('I force close with a reason', async ({ page }) => {
  await page.getByLabel('Reason').fill('fixed outside the sample window')
  await page.getByTestId('force-close-confirm').click()
})

When('I cancel force close', async ({ page }) => {
  await page.getByRole('button', { name: 'Cancel' }).click()
})

Then('the untriaged list shows the fixture test', async ({ page }) => {
  await expect(
    page.getByRole('gridcell', { name: TEST_NAME }).first()
  ).toBeVisible()
})

Then('the triaged list shows the new triage', async ({ page }) => {
  await expect(page.getByText('pod network regression').first()).toBeVisible()
})

Then('the untriaged list does not show the fixture test', async ({ page }) => {
  await expect(page.getByRole('gridcell', { name: TEST_NAME })).toHaveCount(0)
})

Then(
  'the {string} regressed tab shows {string}',
  async ({ page }, tab, name) => {
    await new TriagePage(page).openRegressedTab(tab)
    await expect(page.getByRole('gridcell', { name }).first()).toBeVisible()
  }
)

Then(
  'the {string} regressed tab does not show {string}',
  async ({ page }, tab, name) => {
    await new TriagePage(page).openRegressedTab(tab)
    await expect(page.getByRole('gridcell', { name })).toHaveCount(0)
  }
)

Then('the regressed tests badge shows {int}', async ({ page }, count) => {
  await expect(
    page.locator('[aria-label="Show Regressed Tests"] .MuiBadge-badge')
  ).toHaveText(String(count))
})

Then('the regressed page is {int}', async ({ page }, pageNumber) => {
  await expect(page).toHaveURL(new RegExp(`regressedModalPage=${pageNumber}`))
})

function decodedPageUrl(page) {
  let url = page.url()
  for (let i = 0; i < 3; i++) {
    try {
      const next = decodeURIComponent(url)
      if (next === url) {
        break
      }
      url = next
    } catch {
      break
    }
  }
  return url
}

Then('the regressed filters stay in the URL', async ({ page }) => {
  await expect.poll(() => decodedPageUrl(page)).toContain('paged regression 01')
  await expect(page).not.toHaveURL(/regressedModalPage=/)
  await expect
    .poll(() => decodedPageUrl(page))
    .toContain('regressedModalFilters=')
})

Then('the triage update keeps both regressions', async ({ api }) => {
  const put = latestRequest(
    api.putRequests().filter((request) => request.url.includes('/triages/7'))
  )
  const ids = put.body.regressions.map((regression) => regression.id).sort()
  expect(ids).toEqual([REGRESSION_ID, SECOND_REGRESSION_ID])
})

Then('the existing triage success message is shown', async ({ page }) => {
  await expect(
    page.getByText(`successfully added test to triage record: ${JIRA_URL}`, {
      exact: true,
    })
  ).toBeVisible()
})

Then('the triage form shows validation errors', async ({ page }) => {
  await expect(page.getByText(/invalid url/)).toBeVisible()
  await expect(page.getByText(/invalid description/)).toBeVisible()
  await expect(page.getByText(/invalid type/)).toBeVisible()
})

Then('the triage was not created', async ({ api }) => {
  await expectSettledEmpty(() => api.postRequests())
})

Then('the triage create request includes the regression', async ({ api }) => {
  const post = latestRequest(api.postRequests())
  expect(post.body.regressions).toEqual([{ id: REGRESSION_ID }])
  expect(post.body.url).toBe(JIRA_URL)
})

Then('the triage success message is shown', async ({ page }) => {
  await expect(
    page.getByText('successfully created triage entry', { exact: true })
  ).toBeVisible()
})

Then(
  'the triage error is shown without a success message',
  async ({ page }) => {
    await expect(page.getByText(/error creating triage entry/)).toBeVisible()
    await expect(
      page.getByText('successfully created triage entry', { exact: true })
    ).toHaveCount(0)
  }
)

Then('the selected regressed row was cleared', async ({ page }) => {
  await expect(page).not.toHaveURL(/regressedModalRow=/)
})

Then('the triage selection was cleared', async ({ page }) => {
  await expect(page.getByTestId('triage-regressions')).toHaveText('Triage')
  await expect(page.locator('input[name="triage-test-id"]')).toHaveCount(0)
})

Then('the updated triage description is shown', async ({ api, page }) => {
  await expect(page.getByText('updated pod network note')).toBeVisible()
  const put = latestRequest(api.putRequests())
  expect(put.body.description).toBe('updated pod network note')
  expect(put.body.url).toBe(JIRA_URL)
  expect(put.body.type).toBe('product')
  expect(put.body.regressions.map((regression) => regression.id)).toEqual([
    REGRESSION_ID,
  ])
})

Then(
  'the update error is shown without a success message',
  async ({ page }) => {
    await expect(page.getByText(/error updating triage entry/)).toBeVisible()
    await expect(
      page.getByText('successfully updated triage entry', { exact: true })
    ).toHaveCount(0)
    await expect(page.getByRole('dialog', { name: 'Triage' })).toBeVisible()
    await expect(page).toHaveURL(/triages\/7/)
  }
)

Then('removing every regression is rejected', async ({ page }) => {
  await expect(
    page.getByText(
      'Cannot remove all regressions, must leave at least one associated with the triage record',
      { exact: true }
    )
  ).toBeVisible()
})

Then('one regression was removed', async ({ api, page }) => {
  await expect(
    page.getByText(/successfully removed 1 regression/)
  ).toBeVisible()
  const put = latestRequest(api.putRequests())
  const ids = put.body.regressions.map((regression) => regression.id)
  expect(ids).toEqual([SECOND_REGRESSION_ID])
})

Then('the triage is deleted', async ({ page, api }) => {
  await expect(
    page.getByText('Triage record has been deleted.', { exact: true })
  ).toBeVisible()
  const deleted = latestRequest(api.deleteRequests())
  expect(deleted.pathname).toBe(
    `/api/component_readiness/triages/${SEEDED_TRIAGE_ID}`
  )
  expect(deleted.method).toBe('DELETE')
})

Then('the triage details stay open', async ({ page }) => {
  await expect(
    page.getByRole('heading', { name: 'Triage Details' })
  ).toBeVisible()
})

Then('delete and force close are hidden', async ({ page }) => {
  await expect(page.getByTestId('delete-triage')).toHaveCount(0)
  await expect(page.getByTestId('force-close-regressions')).toHaveCount(0)
})

Then(
  'force close is disabled until the triage is resolved',
  async ({ page }) => {
    await expect(page.getByTestId('force-close-regressions')).toBeDisabled()
  }
)

Then('the force-close preview shows the fixture test', async ({ page }) => {
  const dialog = page.getByRole('dialog', { name: 'Force Close Regressions' })
  await expect(dialog.getByText(TEST_NAME)).toBeVisible()
})

Then('a force-close reason is required', async ({ page }) => {
  await expect(
    page.getByText('A reason is required to force close regressions', {
      exact: true,
    })
  ).toBeVisible()
})

Then('force close succeeds', async ({ page, api }) => {
  await expect(
    page.getByText('Successfully force closed 1 regression', { exact: true })
  ).toBeVisible()
  const post = latestRequest(api.forceCloseRequests())
  expect(post.body).toEqual({
    reason: 'fixed outside the sample window',
  })
  const closed = page.locator('[data-field="force_closed"][role="gridcell"]')
  await expect(closed.getByTestId('CheckCircleIcon')).toBeVisible()
  await closed.getByTestId('CheckCircleIcon').hover()
  await expect(
    page.getByRole('tooltip', { name: /fixed outside the sample window/ })
  ).toBeVisible()
})

Then(
  'the force-close error is shown without a success message',
  async ({ page }) => {
    await expect(
      page.getByText(/Error force closing regressions/)
    ).toBeVisible()
    await expect(page.getByText(/Successfully force closed/)).toHaveCount(0)
    await expect(
      page.getByRole('heading', { name: 'Force Close Regressions' })
    ).toBeVisible()
    await expect(page).toHaveURL(/triages\/7/)
  }
)

Then('there is nothing to force close', async ({ page }) => {
  await expect(
    page.getByText('There are no regressions to force close.', { exact: true })
  ).toBeVisible()
  await expect(page.getByTestId('force-close-confirm')).toBeDisabled()
})

Then('force close was not submitted', async ({ api }) => {
  await expectSettledEmpty(() => api.forceCloseRequests())
})

Then('the delete request failed', async ({ page }) => {
  await expect(page.getByText(/API server returned 400/)).toBeVisible()
})
