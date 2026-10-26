import { Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import ReleasePayloadTable from './ReleasePayloadTable'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

function ReleasePayloads(props) {
  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Tags" />
      <Typography variant="h4" gutterBottom sx={{ textAlign: 'center' }}>
        Release Payloads
      </Typography>
      <ReleasePayloadTable release={props.release} />
    </Fragment>
  )
}

ReleasePayloads.propTypes = {
  release: PropTypes.string,
}

export default ReleasePayloads
