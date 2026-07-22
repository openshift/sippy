import { makeStyles, useTheme } from '@mui/styles'
import { Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import ReleasePayloadTable from './ReleasePayloadTable'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

const useStyles = makeStyles((theme) => ({
  title: {
    textAlign: 'center',
  },
}))

function ReleasePayloads(props) {
  const theme = useTheme()
  const classes = useStyles(theme)

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Tags" />
      <Typography variant="h4" gutterBottom className={classes.title}>
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
