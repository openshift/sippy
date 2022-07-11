import { createTheme, makeStyles } from '@material-ui/core/styles'
import { Typography } from '@material-ui/core'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import ReleaseStreamTable from './ReleaseStreamTable'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

const defaultTheme = createTheme()
const useStyles = makeStyles(
  (theme) => ({
    title: {
      textAlign: 'center',
    },
  }),
  { defaultTheme }
)

function ReleaseStreams(props) {
  const classes = useStyles()

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        currentPage="Payload Streams"
      />
      <Typography variant="h4" gutterBottom className={classes.title}>
        Release Streams
      </Typography>
      <ReleaseStreamTable release={props.release} />
    </Fragment>
  )
}

ReleaseStreams.propTypes = {
  release: PropTypes.string,
}

export default ReleaseStreams
