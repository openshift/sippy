import { Alert } from '@mui/lab'
import { styled } from '@mui/material/styles';
import { ArrayParam, useQueryParam } from 'use-query-params'
import { Backdrop, CircularProgress, makeStyles } from '@mui/material'
import { safeEncodeURIComponent } from '../helpers'
import GridToolbarSearchBox from '../datagrid/GridToolbarSearchBox'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import TestByVariantTable from './TestByVariantTable'

const PREFIX = 'TestsDetail';

const classes = {
  backdrop: `${PREFIX}-backdrop`
};

// TODO jss-to-styled codemod: The Fragment root was replaced by div. Change the tag if needed.
const Root = styled('div')((
  {
    theme
  }
) => ({
  [`& .${classes.backdrop}`]: {
    zIndex: theme.zIndex.drawer + 1,
    color: '#fff',
  }
}));

export default function TestsDetails(props) {


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
    fetch(
      process.env.REACT_APP_API_URL +
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
    <Root>
      <GridToolbarSearchBox
        value={query}
        setValue={setQuery}
        action={updateFilter}
        required={true}
      />
    </Root>
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
