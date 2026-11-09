import { expect } from '@playwright/test'

export async function expectExactFilter(api, path, expected, params = {}) {
  await expectFilter(api, path, expected, params, true)
}

export async function expectFilter(
  api,
  path,
  expected,
  params = {},
  exact = false
) {
  await expect
    .poll(() => {
      const requests = api.requestsForPath(path)
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

export async function expectLinkFilters(page, link, path, expected) {
  await expect(link).toBeVisible()
  const url = new URL(await link.getAttribute('href'), page.url())
  expect(url.pathname).toBe(`/sippy-ng${path}`)
  expect(JSON.parse(url.searchParams.get('filters')).items).toEqual(
    expect.arrayContaining(
      expected.map((item) => expect.objectContaining(item))
    )
  )
}
