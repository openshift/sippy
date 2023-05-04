import { Alert } from '@material-ui/lab'
import { format } from 'date-fns'
import { safeEncodeURIComponent } from '../helpers'
import React from 'react'

// Make the HH:mm:ss as zeros to be more conducive to caching query caching.
export const dateFormat = 'yyyy-MM-dd 00:00:00'

// This is the table we use when the first page is initially rendered.
export const initialPageTable = {
  rows: [
    {
      component: 'None',
      columns: [
        {
          empty: 'None',
          status: 3, // Let's start with success
        },
      ],
    },
  ],
}
export const noDataTable = {
  rows: [
    {
      component: 'No Data found',
      columns: [
        {
          empty: 'None',
          status: 3, // Let's start with success
        },
      ],
    },
  ],
}
export const cancelledDataTable = {
  rows: [
    {
      component: 'Cancelled',
      columns: [
        {
          empty: 'None',
          status: 3, // Let's start with success
        },
      ],
    },
  ],
}
// Make one place to create the Component Readiness api call
export function getAPIUrl() {
  const mainUrl = window.location.host.split(':')[0]

  //console.log('mainUrl: ', mainUrl)
  return 'http://' + mainUrl + ':8080/api/component_readiness'
}

// This is used when the user clicks on one of the columns at the top of the table
// Not quite used just yet.
export function unusedSingleRowReport(columnName) {
  return '/component_readiness/' + safeEncodeURIComponent(columnName) + '/tests'
}

// When we get a fetch error, this will print a standard message.
export function gotFetchError(fetchError) {
  return (
    <Alert severity="error">
      <h2>Failed to load component readiness data</h2>
      <h3>
        {fetchError.split('\n').map((item) => (
          <>
            <hr />
            {item}
          </>
        ))}
      </h3>
      <hr />
      <h3>Check, and possibly fix api server, then reload page to retry</h3>
    </Alert>
  )
}

// Given the data pulled from the API server, calculate an array
// of columns using the first row.  Assumption: the number of columns
// is the same across all rows.
// A column looks like this and we concatenate all fields except status:
//   "columns": [
//       {
//         "network": "ovn",
//         "arch": "amd64",
//         "platform": "alibaba",
//         "status": 0
//       },
// Do our best to handle empty data or the a "cancelled" condition.
export function getColumns(data) {
  let isBad = false
  if (!data) {
    isBad = true
    console.log('data is undefined')
  } else if (!data.rows) {
    isBad = true
    console.log('data has no rows')
  } else if (!data.rows[0]) {
    isBad = true
    console.log('data has no rows[0]')
  }
  if (isBad) {
    console.log('No way to generate columns')
    return ['No column']
  }
  if (data.rows[0].component === 'Cancelled') {
    console.log('got cancelled')
    return ['Cancelled']
  } else if (data.rows[0].component == 'None') {
    //console.log('got no data')
    return ['No data']
  } else if (!data.rows[0].columns) {
    return ['No data']
  }
  const row0Columns = data.rows[0].columns
  let columnNames = []
  row0Columns.forEach((column) => {
    let columnName = ''
    for (const key in column) {
      if (key !== 'status') {
        columnName = columnName + ' ' + column[key]
      }
    }
    columnNames.push(columnName.trimStart())
  })
  return columnNames
}

// The API likes RFC3339 times and the date pickers don't.  So we use this
// function to convert for when we call the API.
// 4 digits, followed by a -, followed by 2 digits, and so on all wrapped in
// a group so we can refer to them as $1 and $2 respectively.
// We add a 'T' in the middle and a 'Z' on the end.
export function makeRFC3339Time(aUrlStr) {
  // Translate all the %20 and %3a into spaces and colons so that the regex can work.
  //console.log('rfc anUrlStr: ', aUrlStr)
  const decodedStr = decodeURIComponent(aUrlStr)
  const regex = /(\d{4}-\d{2}-\d{2})\s(\d{2}:\d{2}:\d{2})/g
  const replaceStr = '$1T$2Z'
  let retVal = decodedStr.replace(regex, replaceStr)

  // The api thinks that the null component is real and will filter accordingly
  // so omit it.
  retVal = retVal.replace(/&component=null/g, '')
  //console.log('rfc retVal: ', retVal)
  return retVal
}

// Return a formatted date given a long form date from the date picker.
export function formatLongDate(aLongDateStr) {
  const dateObj = new Date(aLongDateStr)
  const ret = format(dateObj, dateFormat)
  //console.log('formatLongDate: ', ret)
  return ret
}

// These next set of variables are used for CompReadyMainInputs

export const groupByList = ['cloud', 'arch', 'network', 'upgrade', 'variants']

// TODO: Get these from single place.
export const excludeCloudsList = [
  'alibaba',
  'aws',
  'azure',
  'gcp',
  'ibmcloud',
  'libvirt',
  'metal-assisted',
  'metal-ipi',
  'openstack',
  'ovirt',
  'unknown',
  'vsphere',
  'vsphere-upi',
]

// TODO: Get these from single place.
export const excludeArchesList = [
  'amd64',
  'arm64',
  'ppc64le',
  's390x',
  'heterogeneous',
]

export const excludeNetworksList = ['ovn', 'sdn']

export const excludeUpgradesList = [
  'no-upgrade',
  'none',
  'upgrade-micro',
  'upgrade-minor',
]

export const excludeVariantsList = [
  'assisted',
  'compact',
  'fips',
  'hypershift',
  'microshift',
  'osd',
  'proxy',
  'rt',
  'serial',
  'single-node',
  'standard',
  'techpreview',
]

// Take a string that is an "environment" (environment is a list of strings that describe
// an items in one or more of the lists above) and split it up so that it can be used in
// an api call.
export function expandEnvironment(str) {
  if (str == null || str == '') {
    return ''
  }
  const items = str.split(' ')
  const params = {}
  items.forEach((item) => {
    if (excludeCloudsList.includes(item)) {
      params.platform = item
    } else if (excludeArchesList.includes(item)) {
      params.arch = item
    } else if (excludeNetworksList.includes(item)) {
      params.network = item
    } else if (excludeUpgradesList.includes(item)) {
      params.upgrade = item
    } else if (excludeVariantsList.includes(item)) {
      params.variant = item
    } else {
      //console.log(`Item '${item}' not found in lists`)
    }
  })
  const paramStrings = Object.entries(params).map(
    ([key, value]) => `${key}=${value}`
  )
  const retVal = '&' + paramStrings.join('&')
  return retVal
}

// Take the values needed to make an api call and return a string that can be used to
// make that call.
export function getUpdatedUrlParts(
  baseRelease,
  baseStartTime,
  baseEndTime,
  sampleRelease,
  sampleStartTime,
  sampleEndTime,
  groupByCheckedItems,
  excludeCloudsCheckedItems,
  excludeArchesCheckedItems,
  excludeNetworksCheckedItems,
  excludeUpgradesCheckedItems,
  excludeVariantsCheckedItems,
  component
) {
  //console.log('getUpdatedUrlParts()')
  const valuesMap = {
    baseRelease: baseRelease,
    baseStartTime: formatLongDate(baseStartTime),
    baseEndTime: formatLongDate(baseEndTime),
    sampleRelease: sampleRelease,
    sampleStartTime: formatLongDate(sampleStartTime),
    sampleEndTime: formatLongDate(sampleEndTime),
    //component: component,
  }

  const arraysMap = {
    exclude_clouds: excludeCloudsCheckedItems,
    exclude_arches: excludeArchesCheckedItems,
    exclude_networks: excludeNetworksCheckedItems,
    exclude_upgrades: excludeUpgradesCheckedItems,
    exclude_variants: excludeVariantsCheckedItems,
    group_by: groupByCheckedItems,
  }

  // Render the plain values first.
  let retVal = '?'
  let fieldList1 = Object.entries(valuesMap)
  fieldList1.map(([key, value]) => {
    let amper = '&'
    if (key === 'baseRelease') {
      amper = ''
    }
    retVal = retVal + amper + key + '=' + safeEncodeURIComponent(value)
  })

  const fieldList = Object.entries(arraysMap)
  fieldList.map(([key, value]) => {
    retVal = retVal + '&' + key + '='
    let first = true

    // Account for the case where value is undefined
    // because the url said something like exclude_clouds=, ...
    if (value) {
      value.map((item) => {
        let comma = ','
        if (first) {
          comma = ''
          first = false
        }
        retVal = retVal + comma + item
      })
    }
  })
  return retVal
}
