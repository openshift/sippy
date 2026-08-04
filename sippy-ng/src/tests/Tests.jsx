import { Box, Paper, Tab, Tabs, Typography } from '@mui/material'
import { TabContext } from '@mui/lab'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'

import { Link, Route, Routes, useLocation } from 'react-router-dom'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'
import TestTable from './TestTable'

/**
 * Tests is the landing page for tests, with tabs for all tests,
 * and test results by variant.
 */
export default function Tests(props) {
  const location = useLocation()
  const search = location.search
  const basePath = `/tests/${props.release}`

  useEffect(() => {
    document.title = `Sippy > ${props.release} > Tests`
  }, [])

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Tests" />

      <TabContext value={location.pathname}>
        <Typography align="center" variant="h4">
          Tests for {props.release}
        </Typography>
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
          }}
        >
          <Paper
            sx={{
              margin: 2,
              border: 1,
              borderColor: 'divider',
              display: 'inline-block',
            }}
          >
            <Tabs
              value={location.pathname.substring(
                location.pathname.lastIndexOf('/') + 1
              )}
              indicatorColor="primary"
              textColor="primary"
            >
              <Tab
                label="All tests"
                value={props.release}
                component={Link}
                sx={{ padding: '6px 12px !important' }}
                to={basePath + search}
              />
              <Tab
                label="Tests by variant"
                value="details"
                sx={{ padding: '6px 12px !important' }}
                component={Link}
                to={basePath + '/details' + search}
              />
            </Tabs>
          </Paper>
        </Box>
        <Routes>
          <Route
            path="details"
            element={<TestTable release={props.release} collapse={false} />}
          />
          <Route path="/" element={<TestTable release={props.release} />} />
        </Routes>
      </TabContext>
    </Fragment>
  )
}

Tests.propTypes = {
  release: PropTypes.string,
}
