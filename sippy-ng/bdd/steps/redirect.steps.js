import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { test } from '../fixtures/test.js'

const { Given, When, Then } = createBdd(test)

Given('the regression does not exist', async ({ api }) => {
  api.regressionMode = 'missing'
})

Given('the regression has no test details link', async ({ api }) => {
  api.regressionMode = 'no-links'
})

Given('the regression links outside Component Readiness', async ({ api }) => {
  api.regressionMode = 'bad-path'
})

When('I open the stable link for regression {int}', async ({ page }, id) => {
  await page.goto(`component_readiness/regressions/${id}`)
})

// React development mode runs the lookup effect twice, so count distinct paths.
Then('the regression lookup was for regression {int}', async ({ api }, id) => {
  await expect
    .poll(() => [
      ...new Set(
        api
          .requestsTo(/\/api\/component_readiness\/regressions\//)
          .map((request) => request.pathname)
      ),
    ])
    .toEqual([`/api/component_readiness/regressions/${id}`])
})

Then(
  'the page stays on the link for regression {int}',
  async ({ api, page }, id) => {
    await expect(page).toHaveURL(
      new RegExp(`/sippy-ng/component_readiness/regressions/${id}(\\?|$)`)
    )
    expect(
      api.requests.filter((request) =>
        request.pathname.endsWith('/test_details')
      )
    ).toEqual([])
  }
)
