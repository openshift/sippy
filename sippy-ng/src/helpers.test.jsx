import { jiraProjectForRelease } from './helpers'

describe('jiraProjectForRelease', () => {
  test.each([
    ['quay-3.18', { key: 'PROJQUAY', pid: '10217' }],
    ['4.22', { key: 'OCPBUGS', pid: '10325' }],
    ['rosa-4.22', { key: 'OCPBUGS', pid: '10325' }],
    [undefined, { key: 'OCPBUGS', pid: '10325' }],
  ])('%s -> %o', (release, expected) => {
    expect(jiraProjectForRelease(release)).toEqual(expected)
  })
})
