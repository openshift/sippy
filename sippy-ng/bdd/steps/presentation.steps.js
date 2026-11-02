import { createBdd } from 'playwright-bdd'
import { expect } from '@playwright/test'
import { ReportPage } from '../pages/reportPage.js'
import { test } from '../fixtures/test.js'

const { Given, When, Then } = createBdd(test)

async function bodyBrightness(page) {
  const color = await page.evaluate(
    () => getComputedStyle(document.body).backgroundColor
  )
  const [r, g, b] = color.match(/\d+/g).map(Number)
  return (r + g + b) / 3
}

async function cookie(page, name) {
  const cookies = await page.context().cookies()
  return cookies.find((entry) => entry.name === name)?.value
}

function cellIcon(page, component) {
  return new ReportPage(page).cellLink(component).getByRole('img')
}

Given('the browser prefers a light color scheme', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
})

When('I open the component readiness help', async ({ page }) => {
  await page.getByRole('link', { name: 'Help' }).click()
})

When('I return to the report from the help breadcrumb', async ({ page }) => {
  await page
    .getByRole('navigation', { name: 'help-breadcrumbs' })
    .getByRole('link', { name: 'Component Readiness' })
    .click()
  await page.getByTestId('generate-report-top').waitFor()
  await page
    .getByText('Please wait while we fetch your data')
    .waitFor({ state: 'hidden', timeout: 30_000 })
})

When('I switch to dark mode', async ({ page }) => {
  await page.getByRole('button', { name: 'Toggle dark mode' }).click()
})

When('I turn on accessibility mode', async ({ page }) => {
  // The report sidebar has its own toggle and covers the app drawer's copy.
  await page
    .getByRole('main')
    .getByRole('button', { name: 'Toggle accessibility mode on' })
    .click()
})

Then('the component readiness FAQ is shown', async ({ page }) => {
  await expect(page).toHaveURL(/\/sippy-ng\/component_readiness\/help(\?|$)/)
  await expect(
    page.getByRole('heading', { name: 'Frequently Asked Questions' })
  ).toBeVisible()
  await expect(
    page.getByText(
      "How does Component Readiness determine there's a regression?"
    )
  ).toBeVisible()
})

Then('the page uses the light theme', async ({ page }) => {
  await expect.poll(() => bodyBrightness(page)).toBeGreaterThan(200)
  await expect(
    page.getByRole('button', { name: 'Toggle dark mode' })
  ).toBeVisible()
})

Then('the page uses the dark theme', async ({ page }) => {
  await expect.poll(() => bodyBrightness(page)).toBeLessThan(60)
  await expect(
    page.getByRole('button', { name: 'Toggle light mode' })
  ).toBeVisible()
  expect(await cookie(page, 'sippyColorMode')).toBe('dark')
})

Then(
  'the {string} cell tooltip says {string}',
  async ({ page }, component, text) => {
    await cellIcon(page, component).hover()
    await expect(
      page.getByRole('tooltip', { name: text, exact: true })
    ).toBeVisible()
  }
)

Then(
  'the {string} cell uses the standard regression icon',
  async ({ page }, component) => {
    // Vite inlines these icons, so tell them apart by fill color.
    await expect(cellIcon(page, component)).toHaveAttribute('src', /e31017/i)
  }
)

Then(
  'the {string} cell uses the accessible regression icon',
  async ({ page }, component) => {
    await expect(cellIcon(page, component)).toHaveAttribute('src', /FFBC40/i)
  }
)

Then('accessibility mode can be turned off', async ({ page }) => {
  await expect(
    page
      .getByRole('main')
      .getByRole('button', { name: 'Toggle accessibility mode off' })
  ).toBeVisible()
  expect(await cookie(page, 'sippyAccessibilityMode')).toBe('true')
})
