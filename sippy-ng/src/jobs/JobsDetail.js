import { Backdrop, CircularProgress } from '@mui/material'
import { makeStyles } from '@mui/styles'
import { PropTypes } from 'prop-types'
import { apiFetch, safeEncodeURIComponent } from '../helpers'
import { StringParam, useQueryParam } from 'use-query-params'
import Alert from '@mui/material/Alert'
import GridToolbarSearchBox from '../datagrid/GridToolbarSearchBox'
import JobDetailTable from './JobDetailTable'
import React, { Fragment, useEffect } from 'react'

const useStyles = makeStyles((theme) => ({
  backdrop: {
    zIndex: theme.zIndex.drawer + 1,
    color: '#fff',
  },
}))

const msPerDay = 86400 * 1000

/**
 * JobsDetail is the landing page for the JobDetailTable.
 */
export default function JobsDetail(props) {
  const classes = useStyles()

  const [query, setQuery] = React.useState('')
  const [data, setData] = React.useState({ jobs: [] })
  const [isLoaded, setLoaded] = React.useState(false)
  const [fetchError, setFetchError] = React.useState('')

  const [filter = props.filter, setFilter] = useQueryParam('job', StringParam)

  const [startDate, setStartDate] = React.useState('')
  const [endDate, setEndDate] = React.useState('')

  useEffect(() => {
    if (filter && filter.length > 0) {
      let urlQuery = ''
      if (filter.length > 0) {
        setQuery(filter)
        urlQuery = '&job=' + safeEncodeURIComponent(filter)
      }

      apiFetch(
        '/api/jobs/details?release=' +
          props.release +
          urlQuery
      )
        .then((response) => {
          if (response.status !== 200) {
            throw new Error('server returned ' + response.status)
          }

          return response.json()
        })
        .then((response) => {
          setData(response)
          setStartDate(
            new Date(Math.floor(response.start / msPerDay) * msPerDay)
          )
          setEndDate(new Date(Math.floor(response.end / msPerDay) * msPerDay))
          setLoaded(true)
        })
        .catch((error) => {
          setFetchError(error.toString())
          setLoaded(true)
        })
    }
  }, [filter])

  if (filter !== '' && !isLoaded) {
    return (
      <Backdrop className={classes.backdrop} open={!isLoaded}>
        Fetching data...
        <CircularProgress color="inherit" />
      </Backdrop>
    )
  }

  if (fetchError !== '') {
    return <Alert severity="error">Failed to load data: {fetchError}</Alert>
  }

  const updateFilter = () => {
    setFilter(query)
    setLoaded(false)
  }

  const filterSearch = (
    <Fragment>
      <Alert severity="warning">Enter a search query below</Alert>
      <br />
      <GridToolbarSearchBox
        value={query}
        setValue={setQuery}
        action={updateFilter}
      />
    </Fragment>
  )

  if (data.jobs.length === 0) {
    return filterSearch
  }

  const timestampBegin = new Date(startDate).getTime()
  const timestampEnd = new Date(endDate).getTime()

  let ts = timestampEnd
  const columns = []
  while (ts >= timestampBegin) {
    const d = new Date(ts)
    const value = d.getUTCMonth() + 1 + '/' + d.getUTCDate()
    columns.push(value)
    ts -= msPerDay
  }

  const rows = []
  for (const job of data.jobs) {
    const row = {
      name: job.name,
      results: [],
    }

    for (
      let today = timestampBegin, tomorrow = timestampBegin + msPerDay;
      today <= timestampEnd;
      today += msPerDay, tomorrow += msPerDay
    ) {
      const day = []

      for (let i = 0; i < job.results.length; i++) {
        if (
          job.results[i].timestamp >= today &&
          job.results[i].timestamp < tomorrow
        ) {
          const result = {}
          result.name = job.name
          result.id = i
          result.failedTestNames = job.results[i].failedTestNames
          result.text = job.results[i].result
          result.prowLink = job.results[i].url
          result.className = 'result result-' + result.text
          day.push(result)
          i++
        }
      }

      row.results.unshift(day)
    }

    rows.push(row)
  }

  return (
    <Fragment>
      {filterSearch}
      <JobDetailTable release={props.release} rows={rows} columns={columns} />
    </Fragment>
  )
}

JobsDetail.defaultProps = {
  filter: '',
}

JobsDetail.propTypes = {
  release: PropTypes.string.isRequired,
  filter: PropTypes.string,
}
