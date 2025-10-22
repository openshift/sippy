import './Upgrades.css'
import { apiFetch } from '../helpers'
import { Grid, Typography } from '@mui/material'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import Alert from '@mui/material/Alert'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestByVariantTable from '../tests/TestByVariantTable'

/**
 *  Upgrades is the landing page for upgrades.
 */
export default function Upgrades(props) {
  const location = useLocation()
  const basePath = `/upgrade/${props.release}`

  const [fetchError, setFetchError] = React.useState('')
  const [isLoaded, setLoaded] = React.useState(false)
  const [data, setData] = React.useState({})

  const fetchData = () => {
    apiFetch('/api/upgrade?release=' + props.release)
      .then((response) => {
        if (response.status !== 200) {
          throw new Error('server returned ' + response.status)
        }
        return response.json()
      })
      .then((json) => {
        setData(json)
        setLoaded(true)
      })
      .catch((error) => {
        setFetchError(
          'Could not retrieve release ' + props.release + ', ' + error
        )
      })
  }

  useEffect(() => {
    document.title = `Sippy > ${props.release} > Upgrade health`
    fetchData()
  }, [])

  if (fetchError !== '') {
    return <Alert severity="error">Failed to load data, {fetchError}</Alert>
  }

  if (!isLoaded) {
    return <p>Loading...</p>
  }

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Upgrades" />
      <Grid>
        <Typography variant="h4" style={{ margin: 10 }} align="center">
          Upgrade health for {props.release}
        </Typography>
      </Grid>
      <Fragment>
        <Routes>
          <Route
            path="operators"
            element={
              <TestByVariantTable
                release={props.release}
                colorScale={[90, 100]}
                data={data}
              />
            }
          />
          <Route
            path="/"
            element={<Navigate to={basePath + '/operators'} replace />}
          />
        </Routes>
      </Fragment>
    </Fragment>
  )
}

Upgrades.propTypes = {
  release: PropTypes.string.isRequired,
}
