import { defineBddConfig } from 'playwright-bdd'
import { defineConfig } from '@playwright/test'
import fs from 'node:fs'

function chromiumArgs() {
  const args = ['--disable-dev-shm-usage', '--disable-gpu']
  const container =
    fs.existsSync('/.dockerenv') || fs.existsSync('/run/.containerenv')
  if (container || process.env.PLAYWRIGHT_NO_SANDBOX === '1') {
    args.unshift('--no-sandbox')
  }
  return args
}

const testDir = defineBddConfig({
  features: 'features/**/*.feature',
  steps: ['steps/**/*.js', 'fixtures/test.js'],
})

export default defineConfig({
  testDir,
  timeout: 60_000,
  // Scenarios do not share state, so they run together. Each worker keeps
  // one Chromium that has already opened a page; a fresh process often dies
  // on its first page. Four is the cap: more browsers share one Vite server
  // and the suite gets slower, and an unset count becomes one browser per CPU.
  fullyParallel: true,
  workers: 4,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    // A leading slash replaces the base path, so paths stay relative to this trailing slash.
    baseURL: 'http://127.0.0.1:3000/sippy-ng/',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    viewport: { width: 1600, height: 1200 },
    // Full Chromium. The headless shell often dies on its first page here.
    launchOptions: {
      channel: 'chrome-for-testing',
      args: chromiumArgs(),
    },
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
  webServer: {
    command: 'npm run start -- --host 127.0.0.1',
    port: 3000,
    cwd: '..',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    env: {
      ...process.env,
      VITE_API_URL: '',
    },
  },
})
