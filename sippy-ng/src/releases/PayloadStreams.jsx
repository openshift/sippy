import { Typography } from '@mui/material'
import PayloadStreamsTable from './PayloadStreamsTable'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

function PayloadStreams(props) {
  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        currentPage="Payload Streams"
      />
      <Typography variant="h4" gutterBottom sx={{ textAlign: 'center' }}>
        Payload Streams
      </Typography>
      <PayloadStreamsTable release={props.release} />
    </Fragment>
  )
}

PayloadStreams.propTypes = {
  release: PropTypes.string,
}

export default PayloadStreams
