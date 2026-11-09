import * as data from '../fixtures/workflowData.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { test } from '../fixtures/test.js'

const { When, Then } = createBdd(test)

When('I open and filter the selected run intervals', async ({ page }) => {
  await page.goto(`job_runs/${data.RUN}/${data.JOB}/intervals`)
  await expect(
    page.getByText('Loaded 2 intervals from', { exact: false })
  ).toBeVisible()
  await page.getByRole('button', { name: /^Pod \(/ }).click()
  await page.getByRole('textbox', { name: 'Regex Filter' }).fill('selected-pod')
})
Then(
  'interval requests and visible evidence retain the requested run',
  async ({ api, page }) => {
    await expect(
      page.getByText(/Loaded 2 intervals from .*filtered down to 1\./)
    ).toBeVisible()
    const requests = api.requests.filter(
      (request) => request.pathname === '/api/jobs/runs/intervals'
    )
    expect(requests.length).toBeGreaterThan(0)
    for (const request of requests) {
      const query = new URL(request.url).searchParams
      expect(query.get('prow_job_run_id')).toBe(data.RUN)
      expect(query.get('job_name')).toBe(data.JOB)
    }
    await expect(
      page.getByRole('link', { name: 'GCS job run' })
    ).toHaveAttribute('href', data.GCS)
    await expect(
      page.getByRole('link', { name: 'View Cluster Events' })
    ).toHaveAttribute(
      'href',
      `/sippy-ng/job_runs/${data.RUN}/${data.JOB}/events`
    )
  }
)
When(
  'I follow the selected run to cluster events and filter evidence',
  async ({ page }) => {
    await page.getByRole('link', { name: 'View Cluster Events' }).click()
    await expect(
      page.getByRole('cell', { name: 'other-pod', exact: true })
    ).toBeVisible()
    await page
      .getByRole('textbox', { name: 'Name (contains)', exact: true })
      .fill('selected-pod')
  }
)
Then(
  'only selected event evidence is shown for the requested run',
  async ({ api, page }) => {
    await expect(
      page.getByRole('cell', { name: 'selected-pod', exact: true })
    ).toBeVisible()
    await expect(
      page.getByRole('cell', { name: 'other-pod', exact: true })
    ).toHaveCount(0)
    await expect(
      page.getByText(/Loaded 2 events from .*filtered down to 1\./)
    ).toBeVisible()
    const query = new URL(
      api.requests
        .filter((request) => request.pathname === '/api/jobs/runs/events')
        .at(-1).url
    ).searchParams
    expect(query.get('prow_job_run_id')).toBe(data.RUN)
    expect(query.get('job_name')).toBe(data.JOB)
    await expect(
      page.getByRole('link', { name: 'View Intervals Chart' })
    ).toHaveAttribute(
      'href',
      `/sippy-ng/job_runs/${data.RUN}/${data.JOB}/intervals`
    )
  }
)
When('I open missing run {string}', async ({ page }, kind) => {
  await page.goto(`job_runs/99999/${data.JOB}/${kind}`)
})
Then(
  'the missing {string} view contains no selected-run evidence',
  async ({ api, page }, kind) => {
    await expect(
      page.getByText(new RegExp(`Loaded 0 ${kind} from`))
    ).toBeVisible()
    if (kind === 'intervals') {
      await expect(page.getByRole('alert')).toHaveText(
        'No intervals found for this job run.'
      )
      await expect(
        page.getByRole('button', { name: 'Reset', exact: true })
      ).toBeDisabled()
    }
    if (kind === 'events')
      await expect(
        page.getByText('No events.json found for this job run')
      ).toBeVisible()
    await expect(page.getByText('selected-pod', { exact: true })).toHaveCount(0)
    await expect(page.locator(`a[href="${data.GCS}"]`)).toHaveCount(0)
    const query = new URL(
      api.requests
        .filter((request) => request.pathname === `/api/jobs/runs/${kind}`)
        .at(-1).url
    ).searchParams
    expect(query.get('prow_job_run_id')).toBe('99999')
    expect(query.get('job_name')).toBe(data.JOB)
  }
)

Then(
  'the failed {string} request retains its job without selected-run evidence',
  async ({ api, page }, kind) => {
    const requests = api.requests.filter(
      (request) => request.pathname === `/api/jobs/runs/${kind}`
    )
    expect(requests.length).toBeGreaterThan(0)
    for (const request of requests) {
      const query = new URL(request.url).searchParams
      expect(query.get('prow_job_run_id')).toBe('99999')
      expect(query.get('job_name')).toBe(data.JOB)
    }
    await expect(page.getByText('selected-pod', { exact: true })).toHaveCount(0)
    await expect(page.locator(`a[href="${data.GCS}"]`)).toHaveCount(0)
  }
)

When('I select an interval file without evidence', async ({ page }) => {
  await page.locator('#interval-file').click()
  await page
    .getByRole('option', { name: data.EMPTY_INTERVAL_FILE, exact: true })
    .click()
})

When(
  'I request a delayed interval file from the empty view',
  async ({ api, page, scenario }) => {
    await page.goto(
      `job_runs/${data.RUN}/${data.JOB}/intervals?intervalFile=${data.EMPTY_INTERVAL_FILE}`
    )
    await expect(page.getByRole('alert')).toHaveText(
      'No intervals found for this job run.'
    )
    api.intervalResponseHold = new Promise((resolve) => {
      scenario.releaseIntervalResponse = resolve
    })
    await page.locator('#interval-file').click()
    await page
      .getByRole('option', { name: data.INTERVAL_FILE, exact: true })
      .click()
    await expect
      .poll(() =>
        api.requestsForPath('/api/jobs/runs/intervals').some((request) => {
          return (
            new URL(request.url).searchParams.get('file') === data.INTERVAL_FILE
          )
        })
      )
      .toBe(true)
  }
)

When('the delayed interval response arrives', async ({ api, scenario }) => {
  scenario.releaseIntervalResponse()
  api.intervalResponseHold = null
})

Then(
  'the previous intervals are cleared and the selected file is requested',
  async ({ api, page }) => {
    await expect(
      page.getByText(/Loaded 0 intervals from .*filtered down to 0\./)
    ).toBeVisible()
    // Check again after the pending 800ms filter callback can restore old data.
    await page.waitForTimeout(1000)
    await expect(
      page.getByText(/Loaded 0 intervals from .*filtered down to 0\./)
    ).toBeVisible()
    await expect(page.getByRole('alert')).toHaveText(
      'No intervals found for this job run.'
    )
    await expect(
      page.getByRole('button', { name: 'Reset', exact: true })
    ).toBeDisabled()
    const request = api.requests
      .filter((request) => request.pathname === '/api/jobs/runs/intervals')
      .at(-1)
    const query = new URL(request.url).searchParams
    expect(query.get('file')).toBe(data.EMPTY_INTERVAL_FILE)
    expect(query.get('prow_job_run_id')).toBe(data.RUN)
    await expect(page.getByRole('button', { name: /^Pod \(/ })).toHaveCount(0)
  }
)
