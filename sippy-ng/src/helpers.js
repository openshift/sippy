// Compute relative times -- Intl.RelativeTimeFormat is new-ish,
// and not supported in all browsers, and it's not in node yet.

export const SafeJSONParam = {
  encode: (j) => {
    return safeEncodeURIComponent(JSON.stringify(j))
  },
  decode: (j) => {
    try {
      return JSON.parse(decodeURIComponent(j))
    } catch (e) {
      // return undefined
    }
  },
}

export const SafeStringParam = {
  encode: (s) => {
    return safeEncodeURIComponent(s)
  },
  decode: (s) => {
    if (s === undefined || s === null) return s
    try {
      return decodeURIComponent(s)
    } catch (e) {
      // return undefined
    }
  },
}

// safeEncodeURIComponent wraps the library function, and additionally encodes
// square brackets.  Square brackets are NOT unsafe per RFC1738, but Google and
// others mishandle them.
export function safeEncodeURIComponent(value) {
  if (value === undefined || value === null) return value
  return encodeURIComponent(value)
    .replaceAll('[', '%5B')
    .replaceAll(']', '%5D')
    .replaceAll('{', '%7B')
    .replaceAll('}', '%7D')
}

// Helper function to format dates to second precision
export function formatDateToSeconds(dateString) {
  if (!dateString) return ''
  const date = new Date(dateString)
  return date.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

// relativeTime shows a plain English rendering of a time, e.g. "30 minutes ago".
// This is because the ES6 Intl.RelativeTime isn't available in all environments yet,
// e.g. Safari and NodeJS.
export function relativeTime(date, startDate) {
  if (!date instanceof Date) {
    date = new Date(date)
  }

  const minute = 1000 * 60 // Milliseconds in a minute
  const hour = 60 * minute // Milliseconds in an hour
  const day = 24 * hour // Milliseconds in a day

  const millisAgo = date.getTime() - startDate
  if (Math.abs(millisAgo) < hour) {
    return Math.round(Math.abs(millisAgo) / minute) + ' minutes ago'
  } else if (Math.abs(millisAgo) < day) {
    let hours = Math.round(Math.abs(millisAgo) / hour)
    return `${hours} ${hours === 1 ? 'hour' : 'hours'} ago`
  } else if (Math.abs(millisAgo) < 1.5 * day) {
    return 'about a day ago'
  } else {
    return Math.round(Math.abs(millisAgo) / day) + ' days ago'
  }
}

// relativeDuration shows a plain English rendering of a duration, e.g. "30 minutes".
export function relativeDuration(secondsAgo) {
  if (secondsAgo === undefined) {
    return { value: 'N/A', units: 'N/A' }
  }

  const minute = 60
  const hour = 60 * minute
  const day = 24 * hour

  if (Math.abs(secondsAgo) < hour) {
    return { value: Math.abs(secondsAgo) / minute, units: 'minutes' }
  } else if (Math.abs(secondsAgo) < day) {
    let hours = Math.abs(secondsAgo) / hour
    return { value: hours, units: hours === 1 ? 'hour' : 'hours' }
  } else if (Math.abs(secondsAgo) < 1.5 * day) {
    return { value: 1, units: 'day' }
  } else {
    let days = Math.abs(secondsAgo) / day
    return { value: days, units: days === 1 ? 'day' : 'days' }
  }
}

export function escapeRegex(str) {
  if (str === undefined) {
    return ''
  }
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

export function searchCI(query) {
  query = safeEncodeURIComponent(escapeRegex(query))
  return `https://search.dptools.openshift.org/?search=${query}&maxAge=336h&context=1&type=bug%2Bjunit&name=&excludeName=&maxMatches=5&maxBytes=20971520&groupBy=job`
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

export function pathForExactTestAnalysis(release, test, excludedVariants) {
  console.log(excludedVariants)

  let filters = [filterFor('name', 'equals', test)]
  if (Array.isArray(excludedVariants)) {
    excludedVariants.forEach((variant) => {
      filters.push(not(filterFor('variants', 'contains', variant)))
    })
  }

  return `/tests/${release}/analysis?test=${safeEncodeURIComponent(
    test
  )}&${multiple(...filters)}`
}

export function pathForExactTestAnalysisWithFilter(release, test, filter) {
  let filters = [filterFor('name', 'equals', test)]
  if (filter && filter.items) {
    filter.items.forEach((item) => {
      if (item.columnField === 'variants') {
        filters.push(item)
      }
    })
  }
  return `/tests/${release}/analysis?test=${safeEncodeURIComponent(
    test
  )}&${multiple(...filters)}`
}

export function pathForExactTest(release, test) {
  return `/tests/${release}?${single(filterFor('name', 'equals', test))}`
}

export function pathForExactJobRuns(release, job) {
  return `/jobs/${release}/runs?${single(filterFor('job', 'equals', job))}`
}

export function pathForVariantsWithTestFailure(release, variant, test) {
  return `/jobs/${release}/runs?${multiple(
    filterFor('failed_test_names', 'contains', test),
    filterFor('variants', 'contains', variant)
  )}`
}

export function pathForJobRunsWithTestFailure(release, test, filter) {
  let filters = []
  filters.push(filterFor('failed_test_names', 'contains', test))
  if (filter && filter.items) {
    filter.items.forEach((item) => {
      if (item.columnField === 'variants') {
        filters.push(item)
      }
    })
  }

  return `/jobs/${release}/runs?${multiple(...filters)}`
}

export function pathForJobRunsWithTestFlake(release, test, filter) {
  let filters = []
  filters.push(filterFor('flaked_test_names', 'contains', test))
  if (filter && filter.items) {
    filter.items.forEach((item) => {
      if (item.columnField === 'variants') {
        filters.push(item)
      }
    })
  }

  return `/jobs/${release}/runs?${multiple(...filters)}`
}

export function pathForJobRunsWithFilter(release, filter) {
  if (!filter || filter.items === []) {
    return `/jobs/${release}/runs`
  }

  return `/jobs/${release}/runs?filters=${safeEncodeURIComponent(
    JSON.stringify(filter)
  )}`
}

export function pathForTestByVariant(release, test) {
  return (
    `/tests/${release}/details?` + single(filterFor('name', 'equals', test))
  )
}

export function pathForTestSubstringByVariant(release, test) {
  return (
    `/tests/${release}/details?` + single(filterFor('name', 'contains', test))
  )
}

export function pathForTestsWithFilter(release, filter) {
  if (!filter || filter.items === []) {
    return `/tests/${release}`
  }

  return `/tests/${release}?filters=${safeEncodeURIComponent(
    JSON.stringify(filter)
  )}`
}

// apiPath should be something like '/tests/4.12' or '/tests/4.12/details'
export function pathForAPIWithFilter(apiPath, filter) {
  if (!filter || filter.items === []) {
    return apiPath
  }

  return `${apiPath}?filters=${safeEncodeURIComponent(JSON.stringify(filter))}`
}

export function pathForJobsWithFilter(release, filter) {
  if (!filter || filter.items === []) {
    return `/jobs/${release}`
  }

  return `/jobs/${release}?filters=${safeEncodeURIComponent(
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
    filterFor('current_runs', '>=', '7'),
    ...withoutUnstable()
  )}`
}

export function pathForRepository(release, org, repo) {
  return `/repositories/${release}/${org}/${repo}`
}

export function filterFor(column, operator, value) {
  return { columnField: column, operatorValue: operator, value: value }
}

export function withoutUnstable() {
  return [not(filterFor('variants', 'contains', 'never-stable'))]
}

export function multiple(...filters) {
  return `filters=${safeEncodeURIComponent(
    JSON.stringify({ items: filters, linkOperator: 'and' })
  )}`
}

export function multiple_or(...filters) {
  return `filters=${safeEncodeURIComponent(
    JSON.stringify({ items: filters, linkOperator: 'or' })
  )}`
}

export function single(filter) {
  return `filters=${safeEncodeURIComponent(
    JSON.stringify({ items: [filter] })
  )}`
}

export function not(filter) {
  filter.not = true
  return filter
}

export function useNewInstallTests(release) {
  let digits = release.split('.', 2)
  if (digits.length < 2) {
    return false
  }
  const major = parseInt(digits[0])
  const minor = parseInt(digits[1])
  if (isNaN(major) || isNaN(minor)) {
    return false
  }
  if (major < 4) {
    return false
  } else if (major == 4 && minor < 11) {
    return false
  }
  return true
}

export function getReportStartDate(reportDate) {
  let startDate = new Date()
  if (reportDate.length > 0) {
    startDate = new Date(reportDate)
  }
  return startDate
}

export function getUrlWithoutParams(params) {
  const url = new URL(window.location.href)
  params.forEach((param) => url.searchParams.delete(param))
  return url.href
}

export function parseVariantName(variantName) {
  let name = variantName
  let variant = ''
  if (variantName.split(':').length > 1) {
    name = variantName.split(':')[1]
    variant = variantName.split(':')[0]
  }
  return {
    name,
    variant,
  }
}

export function findFirstNonGARelease(releases) {
  if (
    releases === undefined ||
    releases.releases === undefined ||
    releases.releases.length <= 0 ||
    releases.ga_dates === undefined
  ) {
    return ''
  }
  let firstNonGA = releases.releases[0]
  releases.releases.forEach((r) => {
    let attrs = releases.release_attrs[r]
    if (
      attrs &&
      !attrs.ga &&
      attrs.previous_release &&
      attrs.previous_release in releases.ga_dates
    ) {
      firstNonGA = r
    }
  })
  return firstNonGA
}

// getTestStatus returns the status of a test represented by a test_stats object.
export function getTestStatus(stats, flake, fail, success) {
  return stats.flake_count > 0
    ? flake
    : stats.failure_count > 0
    ? fail
    : success
}
