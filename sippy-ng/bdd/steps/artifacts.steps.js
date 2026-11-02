import {
  ARTIFACT_MATCH_LINE,
  ARTIFACT_PATH,
  CREATED_LABEL_ID,
} from '../fixtures/data.js'
import { ArtifactQueryPage } from '../pages/artifactQueryPage.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { latestRequest } from '../fixtures/expectations.js'
import { test } from '../fixtures/test.js'

const { Given, When, Then } = createBdd(test)

const artifactsPath = /\/api\/jobs\/artifacts$/

function latestArtifactQuery(api) {
  const request = latestRequest(api.requestsTo(artifactsPath))
  return new URLSearchParams(request.search)
}

Given('test details include a failed basis run', async ({ api }) => {
  api.testDetailsMode = 'basis-failure'
})

Given('artifact searches find nothing', async ({ api }) => {
  api.artifactMode = 'empty'
})

When('artifact searches start failing', async ({ api }) => {
  api.artifactMode = 'error'
})

When('artifact searches succeed again', async ({ api }) => {
  api.artifactMode = 'matches'
})

When('I compare the sample failures', async ({ page }) => {
  await page.getByRole('button', { name: 'Compare Sample Failures' }).click()
  await new ArtifactQueryPage(page).dialog().waitFor()
})

When('I choose runs to compare', async ({ page }) => {
  await page.getByLabel('Select runs to compare').check()
})

When('I toggle the failed runs of job {string}', async ({ page }, job) => {
  await page.getByText(job, { exact: true }).getByRole('checkbox').click()
})

When('I compare the selected runs', async ({ page }) => {
  await page
    .getByRole('button', { name: 'Compare Selected', exact: true })
    .click()
  await new ArtifactQueryPage(page).dialog().waitFor()
})

When(
  'I search artifacts with the path glob {string}',
  async ({ page }, glob) => {
    const searched = page.waitForResponse((response) =>
      artifactsPath.test(new URL(response.url()).pathname)
    )
    await new ArtifactQueryPage(page).searchPathGlob(glob)
    await searched
  }
)

When(
  'I match artifact content with the string {string}',
  async ({ page }, text) => {
    await new ArtifactQueryPage(page).matchContent('String', text)
  }
)

When(
  'I match artifact content with the regex {string}',
  async ({ page }, text) => {
    await new ArtifactQueryPage(page).matchContent('Regex', text)
  }
)

When('I show only runs with artifacts', async ({ page }) => {
  await new ArtifactQueryPage(page)
    .dialog()
    .getByLabel('Only runs with artifacts')
    .check()
})

When('I open the save as symptom section', async ({ page }) => {
  const summary = page.getByLabel('Symptom Summary')
  // Saving remounts the section collapsed, so open it only when it is closed.
  await expect(
    page.getByRole('button', { name: 'Save as Symptom' })
  ).toBeVisible()
  if (!(await summary.isVisible())) {
    await page.getByRole('button', { name: 'Save as Symptom' }).click()
  }
  await expect(summary).toBeVisible()
})

When('I create the label {string}', async ({ page }, title) => {
  await page.getByRole('button', { name: 'Add New Label' }).click()
  await page.getByLabel('New Label Title').fill(title)
  await page.getByRole('button', { name: 'Create Label' }).click()
})

When('I save the symptom {string}', async ({ page }, summary) => {
  await page.getByLabel('Symptom Summary').fill(summary)
  await page.getByRole('button', { name: 'Save as New Symptom' }).click()
})

When(
  'I select job run {string} in the artifact results',
  async ({ page }, id) => {
    await new ArtifactQueryPage(page)
      .jobRunRow(id)
      .getByRole('checkbox')
      .check()
  }
)

When('I re-evaluate symptoms', async ({ page }) => {
  await page.getByRole('button', { name: 'Re-evaluate Symptoms' }).click()
})

Then('the artifact query lists only job run {string}', async ({ page }, id) => {
  const query = new ArtifactQueryPage(page)
  await expect(query.jobRunRow(id)).toBeVisible()
  await expect(query.jobRunRows()).toHaveCount(1)
})

Then('the artifact query lists job run {string}', async ({ page }, id) => {
  await expect(new ArtifactQueryPage(page).jobRunRow(id)).toBeVisible()
})

Then(
  'the latest artifact request searches runs {string} with the path glob {string}',
  async ({ api }, runs, glob) => {
    const query = latestArtifactQuery(api)
    expect(query.get('prowJobRuns')).toBe(runs)
    expect(query.get('pathGlob')).toBe(glob)
  }
)

Then('the latest artifact request has no content match', async ({ api }) => {
  const query = latestArtifactQuery(api)
  expect(query.has('textContains')).toBe(false)
  expect(query.has('textRegex')).toBe(false)
})

Then('the latest artifact request includes {string}', async ({ api }, text) => {
  const [key, value] = text.split(/=(.*)/s)
  await expect.poll(() => latestArtifactQuery(api).get(key)).toBe(value)
})

Then(
  'the latest artifact request does not include {string}',
  async ({ api }, text) => {
    const key = text.replace(/=$/, '')
    expect(latestArtifactQuery(api).has(key)).toBe(false)
  }
)

Then(
  'job run {string} lists the artifact {string}',
  async ({ page }, id, path) => {
    const files = new ArtifactQueryPage(page)
      .jobRunRow(id)
      .locator('xpath=following-sibling::tr[1]')
    const link = files.getByRole('link', { name: path, exact: true })
    await expect(link).toHaveAttribute(
      'href',
      `https://artifacts.example/${id}/${path}`
    )
  }
)

Then('the matched artifact line is shown', async ({ page }) => {
  const dialog = new ArtifactQueryPage(page).dialog()
  await expect(
    dialog.locator('samp b', { hasText: ARTIFACT_MATCH_LINE })
  ).toBeVisible()
  await expect(
    dialog.locator('samp i', { hasText: 'starting etcd watch' })
  ).toBeVisible()
})

Then('the artifact query says no results were found', async ({ page }) => {
  await expect(
    new ArtifactQueryPage(page).dialog().getByText('No results found', {
      exact: true,
    })
  ).toBeVisible()
})

Then('no artifact is listed', async ({ page }) => {
  await expect(
    new ArtifactQueryPage(page)
      .dialog()
      .getByRole('link', { name: ARTIFACT_PATH, exact: true })
  ).toHaveCount(0)
})

Then('the artifact query shows a search error', async ({ page }) => {
  await expect(
    new ArtifactQueryPage(page)
      .dialog()
      .getByRole('alert')
      .filter({ hasText: 'Artifact query failed: Return code = 500' })
  ).toBeVisible()
})

Then('the artifact query shows no search error', async ({ page }) => {
  await expect(
    new ArtifactQueryPage(page)
      .dialog()
      .getByRole('alert')
      .filter({ hasText: 'Artifact query failed' })
  ).toHaveCount(0)
})

Then('the label create request is for {string}', async ({ api }, title) => {
  await expect
    .poll(
      () =>
        api
          .requestsTo(/\/api\/jobs\/labels$/)
          .filter((r) => r.method === 'POST').length
    )
    .toBe(1)
  const post = latestRequest(
    api.postRequests().filter((request) => request.pathname.endsWith('/labels'))
  )
  expect(post.body).toEqual({
    label_title: title,
    explanation: '',
    bugs: [],
  })
})

Then('the message {string} is shown', async ({ page }, text) => {
  await expect(page.getByText(text).first()).toBeVisible()
})

Then('the message {string} is not shown', async ({ page }, text) => {
  await expect(page.getByText(text)).toHaveCount(0)
})

Then(
  'the symptom create request records {string} for this search and label',
  async ({ api }, summary) => {
    await expect
      .poll(
        () =>
          api
            .postRequests()
            .filter((request) => request.pathname.endsWith('/symptoms')).length
      )
      .toBe(1)
    const post = latestRequest(
      api
        .postRequests()
        .filter((request) => request.pathname.endsWith('/symptoms'))
    )
    expect(post.body).toEqual({
      summary,
      matcher_type: 'string',
      file_pattern: ARTIFACT_PATH,
      match_string: 'level=error',
      label_ids: [CREATED_LABEL_ID],
      filter_releases: null,
      filter_release_statuses: null,
      filter_products: null,
    })
  }
)

Then(
  'the re-evaluation request targets only run {string}',
  async ({ api }, id) => {
    await expect
      .poll(() => api.requestsTo(/\/api\/jobs\/runs\/reevaluate$/).length)
      .toBe(1)
    const post = latestRequest(api.requestsTo(/\/api\/jobs\/runs\/reevaluate$/))
    expect(post.method).toBe('POST')
    expect(post.body).toEqual({ prow_job_build_ids: [id] })
  }
)

Then(
  'the save as symptom section is editing {string}',
  async ({ page }, summary) => {
    await expect(
      page.getByRole('alert').filter({ hasText: `Editing symptom: ${summary}` })
    ).toBeVisible()
    await expect(page.getByLabel('Symptom Summary')).toHaveValue(summary)
  }
)

Then('symptom and re-evaluation controls are hidden', async ({ page }) => {
  const dialog = new ArtifactQueryPage(page).dialog()
  await expect(
    dialog.getByRole('button', { name: 'Return to details report' }).first()
  ).toBeVisible()
  await expect(dialog.getByText('Save as Symptom')).toHaveCount(0)
  await expect(
    dialog.getByRole('button', { name: 'Re-evaluate Symptoms' })
  ).toHaveCount(0)
})
