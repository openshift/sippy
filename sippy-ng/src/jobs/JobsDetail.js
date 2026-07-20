import { Backdrop, CircularProgress } from '@mui/material'
import { makeStyles } from '@mui/styles'
import { PropTypes } from 'prop-types'
import { safeEncodeURIComponent } from '../helpers'
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

      fetch(
        process.env.REACT_APP_API_URL +
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
          setStartDate(Temporal.PlainDate.from(response.start))
          setEndDate(Temporal.PlainDate.from(response.end))
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

  const numDays = startDate.until(endDate, { largestUnit: 'days' }).days + 1

  const columns = []
  let d = endDate
  while (Temporal.PlainDate.compare(d, startDate) >= 0) {
    columns.push(d.month + '/' + d.day)
    d = d.subtract({ days: 1 })
  }

  const rows = []
  for (const job of data.jobs) {
    const buckets = Array.from({ length: numDays }, () => [])

    for (let i = 0; i < job.results.length; i++) {
      const resultDate = Temporal.Instant.from(job.results[i].timestamp)
        .toZonedDateTimeISO('UTC')
        .toPlainDate()
      const dayIndex = resultDate.until(endDate, { largestUnit: 'days' }).days
      if (dayIndex < 0 || dayIndex >= numDays) continue

      buckets[dayIndex].push({
        name: job.name,
        id: i,
        failedTestNames: job.results[i].failedTestNames,
        text: job.results[i].result,
        prowLink: job.results[i].url,
        className: 'result result-' + job.results[i].result,
      })
    }

    rows.push({
      name: job.name,
      results: buckets,
    })
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
