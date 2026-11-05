export class ArtifactQueryPage {
  constructor(page) {
    this.page = page
  }

  dialog() {
    return this.page.getByRole('dialog').filter({
      has: this.page.getByRole('heading', {
        name: 'Test Details Job Artifact Query',
      }),
    })
  }

  jobRunRow(id) {
    return this.dialog()
      .locator('tr.cr-artifacts-table-row-jobrun')
      .filter({ hasText: `/${id}` })
  }

  jobRunRows() {
    return this.dialog().locator('tr.cr-artifacts-table-row-jobrun')
  }

  // The glob only takes effect on Enter or blur, which is when the search runs.
  async searchPathGlob(glob) {
    const field = this.dialog().getByLabel('Artifact path glob')
    await field.fill(glob)
    await field.press('Enter')
  }

  async matchContent(type, text) {
    await this.dialog().getByRole('combobox', { name: 'Type' }).click()
    await this.page.getByRole('option', { name: type, exact: true }).click()
    const label = type === 'String' ? 'Match string' : 'Match regex'
    const field = this.dialog().getByLabel(label)
    await field.fill(text)
    await field.press('Enter')
  }
}
