// A set of functions for getting paths to specific tests and jobs.

export function pathForExactJob (release, job) {
  return `/jobs/${release}?${single(filterFor('name', 'equals', job))}`
}

export function pathForJobVariant (release, variant) {
  return `/jobs/${release}?${single(filterFor('variants', 'contains', variant))}`
}

// Helpers used by the above
function filterFor (column, operator, value) {
  return { id: 99, columnField: column, operatorValue: operator, value: value }
}

function single (filter) {
  return `filters=${encodeURIComponent(JSON.stringify({ items: [filter] }))}`
}
