export class WorkflowPage {
  constructor(page) {
    this.page = page
  }

  async search(value) {
    await this.page.getByPlaceholder('Search…').fill(value)
    await this.page.getByRole('button', { name: 'Search', exact: true }).click()
  }

  async numericFilter(field, operator, value, append = false) {
    await this.page.getByRole('button').filter({ hasText: 'Filters' }).click()
    const popover = this.page.locator('#filter-popover')
    if (append) await this.page.getByTestId('add-button').click()
    await popover
      .getByRole('combobox', { name: 'Field', exact: true })
      .last()
      .click()
    await this.page.getByRole('option', { name: field, exact: true }).click()
    await popover
      .getByRole('combobox', { name: 'Operator', exact: true })
      .last()
      .click()
    await this.page.getByRole('option', { name: operator, exact: true }).click()
    await popover
      .getByRole('textbox', { name: 'Value', exact: true })
      .last()
      .fill(value)
    await popover.getByRole('button', { name: 'Filter', exact: true }).click()
  }

  row(text) {
    return this.page.getByRole('row').filter({ hasText: text })
  }
}
