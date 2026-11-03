import { FAILED_RUN_ID, PASSED_RUN_ID, TEST_NAME } from '../fixtures/data.js'

export class TestDetailsPage {
  constructor(page) {
    this.page = page
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
