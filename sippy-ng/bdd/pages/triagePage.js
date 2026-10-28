import { JIRA_URL, REGRESSION_ID } from '../fixtures/data.js'

export class TriagePage {
  constructor(page) {
    this.page = page
  }

  async open(id) {
    await this.page.goto(`component_readiness/triages/${id}`)
    await this.page.getByRole('heading', { name: 'Triage Details' }).waitFor()
  }

  async openRegressedTab(name) {
    await this.page.getByTestId(`regressed-tab-${name}`).click()
  }

  async selectRegression(id = REGRESSION_ID) {
    const button = this.page.getByTestId('triage-regressions')
    const box = this.page.locator(`input[name="triage-test-id"][value="${id}"]`)
    if (!(await box.isVisible())) {
      await button.click()
    }
    await box.waitFor()
    await box.click()
  }

  async startTriage() {
    await this.selectRegression()
    const matchesSettled = this.page.waitForResponse(
      (response) =>
        response.url().includes('/matches') &&
        response.request().method() === 'GET'
    )
    await this.page.getByRole('button', { name: 'Set Triage Info' }).click()
    await matchesSettled
    await this.page
      .getByRole('tab', { name: 'Potential Matches (none found)' })
      .waitFor()
    await this.page.getByRole('tab', { name: 'New Triage' }).click()
  }

  async fillNewTriage({
    url = JIRA_URL,
    description = 'pod network regression',
    type = 'product',
  } = {}) {
    const dialog = this.page.getByRole('dialog', { name: 'Triage' })
    await dialog.getByLabel('Jira URL').fill(url)
    await dialog.getByLabel('Description').fill(description)
    await dialog.getByRole('combobox', { name: 'type' }).click()
    await this.page.getByRole('option', { name: type, exact: true }).click()
  }

  async submitTriage() {
    await this.page.getByTestId('triage-submit').click()
  }

  async addSelectedToExisting() {
    await this.selectRegression()
    const triageDialog = this.page.getByRole('dialog', { name: 'Triage' })
    await this.page.getByRole('button', { name: 'Set Triage Info' }).click()
    await triageDialog.getByRole('tab', { name: 'Existing Triage' }).click()
    await triageDialog.getByRole('button', { name: 'Add to Entry' }).click()
  }

  async triageFromTestDetails() {
    await this.page.getByRole('button', { name: 'Triage', exact: true }).click()
    await this.page
      .getByRole('tab', { name: 'Potential Matches (none found)' })
      .waitFor()
    await this.page.getByRole('tab', { name: 'New Triage' }).click()
    await this.fillNewTriage()
    await this.submitTriage()
  }

  async searchRegressedTests(text) {
    const search = this.page.getByPlaceholder('Search…')
    await search.fill(text)
    await search.press('Enter')
  }

  async openNextRegressedPage() {
    await this.page.getByRole('button', { name: 'Go to next page' }).click()
  }
}
