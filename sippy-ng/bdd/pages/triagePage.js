import { JIRA_URL } from '../fixtures/data.js'

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

  async selectRegression() {
    const button = this.page.getByTestId('triage-regressions')
    const box = this.page.locator('input[name="triage-test-id"]')
    if ((await button.innerText()) === 'Triage') {
      await button.click()
    }
    if (!(await box.isVisible())) {
      await button.click()
    }
    await box.click({ force: true })
  }

  async startTriage() {
    await this.selectRegression()
    await this.page.getByRole('button', { name: 'Set Triage Info' }).click()
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
}
