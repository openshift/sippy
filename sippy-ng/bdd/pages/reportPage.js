import {
  ALT_SAMPLE_RELEASE,
  COLUMN_NAME,
  COMPONENT,
  OTHER_COMPONENT,
} from '../fixtures/data.js'

export class ReportPage {
  constructor(page) {
    this.page = page
  }

  async open(path = 'component_readiness/main') {
    await this.page.goto(path)
    await this.page
      .getByText('Please wait while we fetch your data')
      .waitFor({ state: 'hidden', timeout: 30_000 })
      .catch(() => {})
  }

  componentLink(name = COMPONENT) {
    return this.page.getByTestId(`cr-component-${name}`)
  }

  cellLink(name = COMPONENT) {
    return this.page.getByTestId(`cr-cell-${name}-${COLUMN_NAME}`)
  }

  async expectComponent(name) {
    await this.componentLink(name).waitFor()
  }

  async openAllComponents() {
    await this.page.getByRole('tab', { name: /All Components/ }).click()
  }

  async searchRows(text) {
    await this.page.getByTestId('search-row').fill(text)
  }

  async clearSearch() {
    await this.page.getByRole('button', { name: 'Clear Search' }).click()
  }

  async setRedOnly(checked) {
    const toggle = this.page.getByTestId('red-only')
    const input = toggle.locator('input')
    if ((await input.isChecked()) !== checked) {
      await toggle.click()
    }
  }

  async expandSection(name) {
    const summary = this.page.getByRole('button', { name, exact: true })
    await summary.scrollIntoViewIfNeeded()
    if ((await summary.getAttribute('aria-expanded')) !== 'true') {
      await summary.click()
    }
  }

  async changeSampleRelease(release) {
    await this.page.getByLabel('Sample Release').click()
    await this.page.getByRole('option', { name: release, exact: true }).click()
  }

  async generate() {
    await this.page.getByTestId('generate-report-top').click()
  }

  async openRegressedTests() {
    await this.page
      .getByRole('button', { name: 'Show Regressed Tests' })
      .click()
  }

  async fillPullRequest(url) {
    await this.page.locator('#pullRequestURL').fill(url)
  }

  async fillPayloadTags(tags) {
    await this.page.locator('#payloadTags').fill(tags)
  }

  async setGroupBy(name, checked) {
    await this.expandSection('Group By')
    const box = this.page.locator(`input[name="${name}"]`)
    if ((await box.isChecked()) !== checked) {
      await box.click()
    }
  }

  async setNewTestPassRate(value) {
    await this.expandSection('Advanced')
    await this.page.locator('input[type="number"]').first().fill(String(value))
  }

  async setFlakeAsFailure() {
    await this.expandSection('Advanced')
    await this.page.locator('input[name="flakeAsFailure"]').click()
  }

  async selectTestCapability(name) {
    await this.expandSection('Test Options')
    await this.page.getByLabel('Capabilities').fill(name)
    await this.page.getByRole('option', { name, exact: true }).click()
  }
}

export { ALT_SAMPLE_RELEASE, OTHER_COMPONENT }
