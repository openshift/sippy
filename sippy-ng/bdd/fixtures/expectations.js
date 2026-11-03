import { expect } from '@playwright/test'

export function latestRequest(requests) {
  expect(requests.length).toBeGreaterThan(0)
  return requests[requests.length - 1]
}

// Two consecutive empty reads, one poll apart, so a request that arrives
// after the click still fails the assertion.
export async function expectSettledEmpty(read) {
  let seenEmpty = false
  await expect
    .poll(() => {
      const empty = read().length === 0
      const settled = seenEmpty && empty
      seenEmpty = empty
      return settled
    })
    .toBe(true)
}
