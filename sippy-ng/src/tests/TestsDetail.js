import { ArrayParam, useQueryParam } from 'use-query-params'
import { Backdrop, CircularProgress } from '@mui/material'
import { makeStyles } from '@mui/styles'
import { apiFetch, safeEncodeURIComponent } from '../helpers'
import Alert from '@mui/material/Alert'
import GridToolbarSearchBox from '../datagrid/GridToolbarSearchBox'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import TestByVariantTable from './TestByVariantTable'

const useStyles = makeStyles((theme) => ({
  backdrop: {
    zIndex: theme.zIndex.drawer + 1,
    color: '#fff',
  },
}))

export default function TestsDetails(props) {
  const classes = useStyles()

  const [names = props.test, setNames] = useQueryParam('test', ArrayParam)
  const [query, setQuery] = React.useState('')

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  const nameParams = () => {
    return names
      .map((param) => '&test=' + safeEncodeURIComponent(param))
      .join('')
  }

  const fetchData = () => {
    apiFetch(
      '/api/tests/details?release=' +
        props.release +
        nameParams()
    )
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setData(json)
        setQuery(names.join('|'))
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve release ' + props.release + ', ' + error
        )
      })
  }

  useEffect(() => {
    fetchData()
  }, [names])

  if (fetchError !== '') {
    return <Alert severity="error">Failed to load data, {fetchError}</Alert>
  }

  const updateFilter = () => {
    const names = query.match(/([^\\|]|\\.)+/g)
    setLoaded(false)
    setNames(names)
  }

  const filterBox = (
    <Fragment>
      <GridToolbarSearchBox
        value={query}
        setValue={setQuery}
        action={updateFilter}
        required={true}
      />
    </Fragment>
  )

  if (!isLoaded) {
    return (
      <Fragment>
        <Backdrop className={classes.backdrop} open={!isLoaded}>
          Fetching data...
          <CircularProgress data-icon="CircularProgress" color="inherit" />
        </Backdrop>
        {filterBox}
      </Fragment>
    )
  }

  if (Object.keys(data.tests).length === 0) {
    return filterBox
  }

  return (
    <Fragment>
      {filterBox}
      <TestByVariantTable release={props.release} data={data} />
    </Fragment>
  )
}

TestsDetails.defaultProps = {
  test: [],
}

TestsDetails.propTypes = {
  release: PropTypes.string.isRequired,
  test: PropTypes.array,
}
