import { COLUMN_NAME, COMPONENT } from '../fixtures/data.js'

export class ReportPage {
  constructor(page) {
    this.page = page
  }

  async open(path = 'component_readiness/main') {
    await this.page.goto(path)
    // Variant loading and the report fetch use the same status text, and that
    // text is missing for a moment between them. The generate control is on
    // screen only for the report fetch, so wait for it before deciding the
    // fetch has finished.
    await this.page.getByTestId('generate-report-top').waitFor()
    await this.page
      .getByText('Please wait while we fetch your data')
      .waitFor({ state: 'hidden', timeout: 30_000 })
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

  async searchColumns(text) {
    await this.page.getByTestId('search-column').fill(text)
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

  async changeBasisRelease(release) {
    await this.page.getByLabel('Basis Release').click()
    await this.page.getByRole('option', { name: release, exact: true }).click()
  }

  releaseSection(label) {
    const labelId = `${label.toLowerCase().replaceAll(' ', '-')}-label`
    return this.page
      .locator(`#${labelId}`)
      .locator('xpath=ancestor::*[contains(@class,"MuiBox-root")][1]')
  }

  async setReleaseDate(releaseLabel, which, value) {
    const field = this.releaseSection(releaseLabel).getByRole('textbox', {
      name: which,
    })
    await field.click()
    await field.fill(value)
    await field.press('Tab')
  }

  async setIncludeVariant(group, value) {
    await this.expandSection(`Include ${group}`)
    const box = this.page.locator(`input[name="${value}"]`)
    if (!(await box.isChecked())) {
      await box.click()
    }
  }

  async selectTestLifecycle(name) {
    await this.expandSection('Test Options')
    await this.page.getByLabel('Lifecycles').fill(name)
    await this.page.getByRole('option', { name, exact: true }).click()
  }

  async copySearchLink() {
    await this.page
      .context()
      .grantPermissions(['clipboard-read', 'clipboard-write'], {
        origin: 'http://127.0.0.1:3000',
      })
    await this.page.getByRole('button', { name: 'Copy Link' }).click()
    await this.page.getByText('Link copied!').waitFor()
    return this.page.evaluate(() => navigator.clipboard.readText())
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
