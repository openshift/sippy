import { test as base } from 'playwright-bdd'
import { createApi, installApi } from './api.js'

// A fresh Chromium often dies on its first page. Later pages in that same
// process stay up, so the worker keeps one browser that has already opened a page.
const launchAttempts = 8

async function launchReadyBrowser(browserType, launchOptions) {
  let lastError
  for (let attempt = 1; attempt <= launchAttempts; attempt++) {
    let browser
    try {
      browser = await browserType.launch(launchOptions)
      const context = await browser.newContext()
      await context.newPage()
      await context.close()
      return browser
    } catch (error) {
      lastError = error
      console.warn(
        `browser launch attempt ${attempt} of ${launchAttempts} failed: ${error}`
      )
      await browser?.close().catch(() => {})
    }
  }
  throw lastError
}

export const test = base.extend({
  browser: [
    async ({ playwright, browserName, launchOptions }, use) => {
      const browser = await launchReadyBrowser(
        playwright[browserName],
        launchOptions
      )
      await use(browser)
      await browser.close().catch(() => {})
    },
    { scope: 'worker' },
  ],

  api: async ({ page }, use) => {
    const api = createApi()
    await installApi(page, api)
    await use(api)
  },
})
