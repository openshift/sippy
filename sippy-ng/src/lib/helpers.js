export function escapeRegex (str) {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function searchCI (query) {
  query = encodeURIComponent(escapeRegex(query))
  return `https://search.ci.openshift.org/?search=${query}&maxAge=336h&context=1&type=bug%2Bjunit&name=&excludeName=&maxMatches=5&maxBytes=20971520&groupBy=job`
}
