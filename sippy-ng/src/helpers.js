// Compute relative times -- Intl.RelativeTimeFormat is new-ish,
// and not supported in all browsers, and it's not in node yet.
import React from 'react'

const minute = 1000 * 60 // Milliseconds in a minute
const hour = 60 * minute // Milliseconds in an hour
const day = 24 * hour // Milliseconds in a day

// relativeTime shows a plain English rendering of a time, e.g. "30 minutes ago".
// This is because the ES6 Intl.RelativeTime isn't available in all environments yet,
// e.g. Safari and NodeJS.
export function relativeTime(date) {
  if (!date instanceof Date) {
    date = new Date(date)
  }

  const millisAgo = date.getTime() - Date.now()
  if (Math.abs(millisAgo) < hour) {
    return Math.round(Math.abs(millisAgo) / minute) + ' minutes ago'
  } else if (Math.abs(millisAgo) < day) {
    return Math.round(Math.abs(millisAgo) / hour) + ' hours ago'
  } else if (Math.abs(millisAgo) < 1.5 * day) {
    return 'about a day ago'
  } else {
    return Math.round(Math.abs(millisAgo) / day) + ' days ago'
  }
}

export function millisToTime(millis) {
  let hours = Math.floor(millis / hour)
  let minutes = ((millis % hour) / minute).toFixed(0)

  return `${hours}h${minutes}m`
}

export function escapeRegex(str) {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function searchCI(query) {
  query = encodeURIComponent(escapeRegex(query))
  return `https://search.ci.openshift.org/?search=${query}&maxAge=336h&context=1&type=bug%2Bjunit&name=&excludeName=&maxMatches=5&maxBytes=20971520&groupBy=job`
}

// A set of functions for getting paths to specific tests and jobs:

export function withSort(queryString, sortField, sort) {
  if (queryString.includes('?')) {
    return `${queryString}&sortField=${sortField}&sort=${sort}`
  } else {
    return `${queryString}?sortField=${sortField}&sort=${sort}`
  }
}

export function pathForVariantAnalysis(release, variant) {
  return `/jobs/${release}/analysis?${single(
    filterFor('variants', 'contains', variant)
  )}`
}

export function queryForBookmark(...bookmarks) {
  return multiple(...bookmarks)
}

export function pathForExactJob(release, job) {
  return `/jobs/${release}?${single(filterFor('name', 'equals', job))}`
}

export function pathForExactJobAnalysis(release, job) {
  return `/jobs/${release}/analysis?${single(filterFor('name', 'equals', job))}`
}

export function pathForExactTest(release, test) {
  return `/tests/${release}?${single(filterFor('name', 'equals', test))}`
}

export function pathForExactJobRuns(release, job) {
  return `/jobs/${release}/runs?${single(filterFor('job', 'equals', job))}`
}

export function pathForVariantsWithTestFailure(release, variant, test) {
  return `/jobs/${release}/runs?${multiple(
    filterFor('failedTestNames', 'contains', test),
    filterFor('variants', 'contains', variant)
  )}`
}

export function pathForJobRunsWithTestFailure(release, test) {
  return `/jobs/${release}/runs?${single(
    filterFor('failedTestNames', 'contains', test)
  )}`
}

export function pathForJobRunsWithFilter(release, filter) {
  if (!filter || filter.items === []) {
    return `/jobs/${release}/runs`
  }

  return `/jobs/${release}/runs?filters=${encodeURIComponent(
    JSON.stringify(filter)
  )}`
}

export function pathForJobsWithFilter(release, filter) {
  if (!filter || filter.items === []) {
    return `/jobs/${release}`
  }

  return `/jobs/${release}?filters=${encodeURIComponent(
    JSON.stringify(filter)
  )}`
}

export function pathForJobVariant(release, variant) {
  return `/jobs/${release}?${single(
    filterFor('variants', 'contains', variant)
  )}`
}

export function pathForJobsInPercentile(release, start, end) {
  return `/jobs/${release}?${multiple(
    filterFor('current_pass_percentage', '>=', `${start}`),
    filterFor('current_pass_percentage', '<', `${end}`),
    not(filterFor('variants', 'contains', 'never-stable'))
  )}`
}

export function filterFor(column, operator, value) {
  return { columnField: column, operatorValue: operator, value: value }
}

export function withoutUnstable() {
  return [
    not(filterFor('variants', 'contains', 'never-stable')),
    not(filterFor('variants', 'contains', 'techpreview')),
  ]
}

export function multiple(...filters) {
  return `filters=${encodeURIComponent(
    JSON.stringify({ items: filters, linkOperator: 'and' })
  )}`
}

export function single(filter) {
  return `filters=${encodeURIComponent(JSON.stringify({ items: [filter] }))}`
}

export function not(filter) {
  filter.not = true
  return filter
}
