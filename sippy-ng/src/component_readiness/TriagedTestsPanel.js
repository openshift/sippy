import { Grid } from '@mui/material'
import EventEmitter from 'eventemitter3'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import TriagedRegressions from './TriagedRegressions'
import TriagedRegressionTestList from './TriagedRegressionTestList'

export default function TriagedTestsPanel(props) {
  const eventEmitter = new EventEmitter()

  return (
    <Fragment>
      <Grid>
        <TriagedRegressions
          eventEmitter={eventEmitter}
          triageEntries={props.triageEntries}
        />
        <TriagedRegressionTestList eventEmitter={eventEmitter} />
      </Grid>
    </Fragment>
  )
}

TriagedTestsPanel.propTypes = {
  triageEntries: PropTypes.array,
}
