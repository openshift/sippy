import { defineBddConfig } from 'playwright-bdd'
import { defineConfig } from '@playwright/test'

const testDir = defineBddConfig({
  features: 'features/**/*.feature',
  steps: ['steps/**/*.js', 'fixtures/test.js'],
})

export default defineConfig({
  testDir,
  timeout: 60_000,
  // One browser. Several Chromium processes against one Vite server crash.
  workers: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    // A leading slash replaces the base path, so paths stay relative to this trailing slash.
    baseURL: 'http://localhost:3000/sippy-ng/',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    viewport: { width: 1600, height: 1200 },
    // Full Chromium. The headless shell often dies on its first page here.
    launchOptions: {
      channel: 'chrome-for-testing',
      args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu'],
    },
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
  webServer: {
    command: 'npm run start',
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
