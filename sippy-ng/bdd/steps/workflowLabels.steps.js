import * as data from '../fixtures/workflowData.js'
import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { expectSettledEmpty } from '../fixtures/expectations.js'
import { test } from '../fixtures/test.js'

const { When, Then } = createBdd(test)

When('I select the second label in the editor', async ({ page }) => {
  await page.goto('labels/edit')
  await expect(
    page.getByRole('textbox', { name: 'Label ID', exact: true })
  ).toHaveValue('synthetic-a')
  await page.getByRole('combobox', { name: 'Label', exact: true }).click()
  await page
    .getByRole('option', { name: 'Beta label (synthetic-b)', exact: true })
    .click()
  await expect(
    page.getByRole('textbox', { name: 'Label ID', exact: true })
  ).toHaveValue('synthetic-b')
  await expect(page).toHaveURL(/labels\/edit\/synthetic-b$/)
})
Then(
  'invalid label titles and Jira keys prevent saves',
  async ({ api, page }) => {
    const title = page.getByRole('textbox', {
      name: 'Label title',
      exact: false,
    })
    const save = page.getByRole('button', { name: 'Save label', exact: true })
    await title.fill('')
    await expect(save).toBeDisabled()
    await title.fill('Alpha label')
    await expect(page.getByText('Label title must be unique.')).toBeVisible()
    await expect(save).toBeDisabled()
    await title.fill('Changed Beta')
    await page
      .getByRole('combobox', { name: 'Associated Jira issues' })
      .fill('invalid-key')
    await page
      .getByRole('combobox', { name: 'Associated Jira issues' })
      .press('Enter')
    await expect(page.getByText(/Invalid Jira key:/)).toBeVisible()
    await expect(save).toBeDisabled()
    await expectSettledEmpty(() => api.mutationRequests())
    await page.getByRole('button', { name: 'Clear', exact: true }).click()
  }
)
When('I save valid changes to the selected label', async ({ page }) => {
  await page
    .getByRole('textbox', { name: 'Label title', exact: false })
    .fill('  Changed Beta  ')
  await page
    .getByRole('textbox', { name: 'Explanation', exact: true })
    .fill('Updated synthetic explanation')
  await page.getByRole('button', { name: 'Save label', exact: true }).click()
})
Then(
  'only the selected label is updated and success is visible',
  async ({ api, page }) => {
    await expect(page.getByRole('alert')).toHaveText(
      'Label "Changed Beta" saved successfully.'
    )
    expect(api.putRequests()).toHaveLength(1)
    expect(api.putRequests()[0]).toMatchObject({
      pathname: '/api/jobs/labels/synthetic-b',
      body: {
        id: 'synthetic-b',
        label_title: 'Changed Beta',
        explanation: 'Updated synthetic explanation',
        bugs: [],
        hide_display_contexts: ['metrics'],
      },
    })
    expect(api.labels.find((label) => label.id === 'synthetic-a')).toEqual(
      data.labels()[0]
    )
    await expect(
      page.getByRole('textbox', { name: 'Label title', exact: false })
    ).toHaveValue('Changed Beta')
    await page.reload()
    await expect(
      page.getByRole('textbox', { name: 'Explanation', exact: true })
    ).toHaveValue('Updated synthetic explanation')
  }
)
Then(
  'the failed save leaves the original definition and editable draft',
  async ({ api, page }) => {
    await expect(page.getByRole('alert')).toHaveText(
      'Failed to update label: Synthetic mutation rejected: PUT /api/jobs/labels/synthetic-b'
    )
    expect(api.putRequests()).toHaveLength(1)
    expect(api.putRequests()[0]).toMatchObject({
      pathname: '/api/jobs/labels/synthetic-b',
      body: { id: 'synthetic-b', label_title: 'Changed Beta' },
    })
    expect(api.labels).toEqual(data.labels())
    await expect(
      page.getByRole('textbox', { name: 'Label title', exact: false })
    ).toHaveValue('  Changed Beta  ')
    await expect(
      page.getByRole('button', { name: 'Save label', exact: true })
    ).toBeEnabled()
  }
)
Then(
  'label editing is unavailable and no mutation is sent',
  async ({ api, page }) => {
    await expect(page.getByRole('alert')).toHaveText(
      'Label editing is unavailable because writable endpoints are disabled.'
    )
    await expect(page.getByRole('button', { name: 'Save label' })).toHaveCount(
      0
    )
    await expectSettledEmpty(() => api.mutationRequests())
    expect(
      api.requests.filter((request) => request.pathname === '/api/jobs/labels')
    ).toHaveLength(0)
  }
)
When('I cancel deleting the selected label', async ({ page }) => {
  await page
    .getByRole('button', { name: 'Delete label synthetic-b', exact: true })
    .click()
  await expect(page.getByRole('dialog')).toContainText('Beta label')
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Cancel', exact: true })
    .click()
})
Then('no label definition has been deleted', async ({ api, page }) => {
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expectSettledEmpty(() => api.deleteRequests())
  expect(api.labels).toEqual(data.labels())
  await expect(
    page.getByRole('textbox', { name: 'Label ID', exact: true })
  ).toHaveValue('synthetic-b')
})
When('I confirm deleting the selected label', async ({ page }) => {
  await page
    .getByRole('button', { name: 'Delete label synthetic-b', exact: true })
    .click()
  await expect(page.getByRole('dialog')).toContainText('synthetic-b')
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Delete label', exact: true })
    .click()
})
Then(
  'only that label is deleted and success is visible',
  async ({ api, page }) => {
    await expect(page.getByRole('alert')).toContainText(
      'Label "Beta label" deleted successfully.'
    )
    expect(api.deleteRequests()).toHaveLength(1)
    expect(api.deleteRequests()[0].pathname).toBe(
      '/api/jobs/labels/synthetic-b'
    )
    expect(api.labels).toEqual([data.labels()[0]])
    await expect(
      page.getByRole('button', { name: 'Save label', exact: true })
    ).toBeDisabled()
    await page.reload()
    await expect(page.getByRole('alert')).toHaveText(
      'The selected label was not found.'
    )
  }
)
Then(
  'deletion failure is visible and the selected label remains',
  async ({ api, page }) => {
    await expect(page.getByRole('alert')).toHaveText(
      'Failed to delete label: Synthetic mutation rejected: DELETE /api/jobs/labels/synthetic-b'
    )
    expect(api.deleteRequests()).toHaveLength(1)
    expect(api.deleteRequests()[0].pathname).toBe(
      '/api/jobs/labels/synthetic-b'
    )
    expect(api.labels).toEqual(data.labels())
    await expect(
      page.getByRole('button', {
        name: 'Delete label synthetic-b',
        exact: true,
      })
    ).toBeEnabled()
    await page.reload()
    await expect(
      page.getByRole('textbox', { name: 'Label ID', exact: true })
    ).toHaveValue('synthetic-b')
  }
)
