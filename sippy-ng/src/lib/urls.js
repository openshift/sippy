// A set of functions for getting paths to specific tests and jobs.
import JSONCrush from 'jsoncrush'

export function withSort (queryString, sortField, sort) {
  return `${queryString}&sortField=${sortField}&sort=${sort}`
}

export function queryForBookmark (...bookmarks) {
  return multiple(...bookmarks)
}

export function pathForExactJob (release, job) {
  return `/jobs/${release}?${single(filterFor('name', 'equals', job))}`
}

export function pathForExactTest (release, test) {
  return `/tests/${release}?${single(filterFor('name', 'equals', test))}`
}

export function pathForExactJobRuns (release, job) {
  return `/jobs/${release}/runs?${single(filterFor('job', 'equals', job))}`
}

export function pathForJobRunsWithTestFailure (release, test) {
  return `/jobs/${release}/runs?${single(filterFor('failedTestNames', 'contains', test))}`
}

export function pathForJobVariant (release, variant) {
  return `/jobs/${release}?${single(filterFor('variants', 'contains', variant))}`
}

// Helpers used by the above
export function filterFor (column, operator, value) {
  return { id: 99, columnField: column, operatorValue: operator, value: value }
}

function multiple (...filters) {
  return `filters=${JSONCrush.crush(JSON.stringify({ items: filters, linkOperator: 'and' }))}`
}

function single (filter) {
  return `filters=${JSONCrush.crush(JSON.stringify({ items: [filter] }))}`
}
