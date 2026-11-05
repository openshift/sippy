export class TriageIndexPage {
  constructor(page) {
    this.page = page
  }

  async open() {
    await this.page.goto('component_readiness/triages')
    await this.page
      .getByRole('heading', { name: 'All Triage Records' })
      .waitFor()
  }

  row(description) {
    return this.page
      .getByRole('row')
      .filter({ has: this.page.getByText(description, { exact: true }) })
  }

  async search(text) {
    const search = this.page.getByPlaceholder('Search…').first()
    await search.fill(text)
    await search.press('Enter')
  }

  async applyBookmark(name) {
    await this.page.getByRole('button', { name: 'Bookmarks' }).click()
    await this.page.getByRole('menuitem', { name, exact: true }).click()
  }
}
