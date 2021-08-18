/**
 * bugzillaURL generates a link to a prefilled out bug.
 */
export default function bugzillaURL (item) {
  const title = item.name
  const titleEncoded = encodeURIComponent(title)
  let url = `https://search.ci.openshift.org/?maxAge=168h&context=1&type=bug%2Bjunit&name=&maxMatches=5&maxBytes=20971520&groupBy=job&search=${titleEncoded}`
  if (item.test_grid_url) {
    url = item.test_grid_url
  }

  const bugText = encodeURIComponent(`
${title}

is failing frequently in CI, see:
${url}

FIXME: Replace this paragraph with a particular job URI from the search results to ground discussion.  A given test may fail for several reasons, and this bug should be scoped to one of those reasons.  Ideally you'd pick a job showing the most-common reason, but since that's hard to determine, you may also chose to pick a job at random.  Release-gating jobs (release-openshift-...) should be preferred over presubmits (pull-ci-...) because they are closer to the released product and less likely to have in-flight code changes that complicate analysis.

FIXME: Provide a snippet of the test failure or error from the job log
`)

  return `https://bugzilla.redhat.com/enter_bug.cgi?classification=Red%20Hat&product=OpenShift%20Container%20Platform&cf_internal_whiteboard=buildcop&short_desc=${titleEncoded}&comment=${bugText}&version=4.9&cc=sippy@dptools.openshift.org`
}

export function bugColor (item) {
  if (item.bugs.length > 0) {
    return 'black'
  } else if (item.associated_bugs.length > 0) {
    return 'darkred'
  } else {
    return 'lightgray'
  }
}

export function weightedBugComparator (linkedBug1, associatedBug1, linkedBug2, associatedBug2) {
  return ((100 * linkedBug1.length) + associatedBug1.length) - ((100 * linkedBug2.length) + associatedBug2.length)
}
