import {
  FAILED_RUN_ID,
  historicalTestDetailsPath,
  PASSED_RUN_ID,
  TEST_NAME,
} from '../fixtures/data.js'

export class TestDetailsPage {
  constructor(page) {
    this.page = page
  }

  // openHistorical opens the historical test details link the API builds for
  // a regression, as the triage table's history icon does.
  async openHistorical(regressionId, options) {
    const apiPath = historicalTestDetailsPath(regressionId, options)
    await this.page.goto(
      apiPath.replace('/api/component_readiness/', 'component_readiness/')
    )
  }

  historicalNotice() {
    return this.page.getByTestId('historical-notice')
  }

  // triageRow is a row of the test details page's triage list.
  triageRow(id) {
    return this.page.locator(`.MuiDataGrid-row[data-id="${id}"]`)
  }

  async expectTest(name = TEST_NAME) {
    await this.page.getByRole('heading', { name, exact: true }).waitFor()
  }

  failedRunLink() {
    return this.page.locator(`a[href*="${FAILED_RUN_ID}"]`)
  }

  passedRunLink() {
    return this.page.locator(`a[href*="${PASSED_RUN_ID}"]`)
  }

  async showOnlyFailures() {
    await this.page.getByLabel('Only show failures').click()
  }
}
