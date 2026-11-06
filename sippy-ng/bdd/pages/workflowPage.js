import { expect } from '@playwright/test'

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

  async expectExactFilter(api, path, expected, params = {}) {
    await this.expectFilter(api, path, expected, params, true)
  }

  async expectFilter(api, path, expected, params = {}, exact = false) {
    await expect
      .poll(() => {
        const requests = api.requests.filter(
          (request) => request.pathname === path
        )
        const last = requests.at(-1)
        if (!last) return null
        const query = new URL(last.url).searchParams
        const items = JSON.parse(query.get('filter') || '{"items":[]}').items
        return {
          items,
          itemCount: items.length,
          ...Object.fromEntries(
            Object.keys(params).map((key) => [key, query.get(key)])
          ),
        }
      })
      .toMatchObject({
        items: expect.arrayContaining(
          expected.map((item) => expect.objectContaining(item))
        ),
        ...(exact ? { itemCount: expected.length } : {}),
        ...params,
      })
  }

  async expectLinkFilters(link, path, expected) {
    await expect(link).toBeVisible()
    const url = new URL(await link.getAttribute('href'), this.page.url())
    expect(url.pathname).toBe(`/sippy-ng${path}`)
    expect(JSON.parse(url.searchParams.get('filters')).items).toEqual(
      expect.arrayContaining(
        expected.map((item) => expect.objectContaining(item))
      )
    )
  }
}
