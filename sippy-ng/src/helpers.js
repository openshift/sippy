// Compute relative times -- Intl.RelativeTimeFormat is new-ish,
// and not supported in all browsers, and it's not in node yet.
export function relativeTime (date) {
  const minute = 1000 * 60 // Milliseconds in a minute
  const hour = 60 * minute // Milliseconds in an hour
  const day = 24 * hour // Milliseconds in a day
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

// A set of functions for getting paths to specific tests and jobs.

export function pathForExactJob (release, job) {
  return `/jobs/${release}?${single(filterFor('name', 'equals', job))}`
}

export function pathForExactTest (release, test) {
  return `/tests/${release}?${single(filterFor('name', 'equals', test))}`
}

export function pathForExactJobRuns (release, job) {
  return `/jobs/${release}/runs?${single(filterFor('job', 'equals', job))}`
}

export function pathForVariantsWithTestFailure (release, variant, test) {
  return `/jobs/${release}/runs?${multiple(
    filterFor('failedTestNames', 'contains', test),
    filterFor('variants', 'contains', variant)
  )}`
}

export function pathForJobRunsWithTestFailure (release, test) {
  return `/jobs/${release}/runs?${single(filterFor('failedTestNames', 'contains', test))}`
}

export function pathForJobVariant (release, variant) {
  return `/jobs/${release}?${single(filterFor('variants', 'contains', variant))}`
}

// Helpers used by the above
function filterFor (column, operator, value) {
  return { id: 99, columnField: column, operatorValue: operator, value: value }
}

function multiple (...filters) {
  return `filters=${encodeURIComponent(JSON.stringify({ items: filters, linkOperator: 'and' }))}`
}

function single (filter) {
  return `filters=${encodeURIComponent(JSON.stringify({ items: [filter] }))}`
}
