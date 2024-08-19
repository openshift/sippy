import { Grid } from '@mui/material'
import EventEmitter from 'eventemitter3'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import TriagedIncidentGroups from './TriagedIncidentGroups'
import TriagedTestDetails from './TriagedTestDetails'
import TriagedVariants from './TriagedVariants'

export default function TriagedIncidentsPanel(props) {
  const eventEmitter = new EventEmitter()

  return (
    <Fragment>
      <Grid>
        <TriagedIncidentGroups
          eventEmitter={eventEmitter}
          regressedTests={props.regressedTests}
          triagedIncidents={props.triagedIncidents}
        />
        <TriagedVariants eventEmitter={eventEmitter} />
        <TriagedTestDetails eventEmitter={eventEmitter} />
      </Grid>
    </Fragment>
  )
}

TriagedIncidentsPanel.propTypes = {
  /* regressedTests is currently a placeholder
  for future work adding / updating incidents */
  regressedTests: PropTypes.array,
  triagedIncidents: PropTypes.array,
}
