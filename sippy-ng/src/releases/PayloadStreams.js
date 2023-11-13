import { createTheme, makeStyles } from '@mui/material/styles'
import { Typography } from '@mui/material'
import PayloadStreamsTable from './PayloadStreamsTable'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
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

function PayloadStreams(props) {
  const classes = useStyles()

  return (
    <Fragment>
      <SimpleBreadcrumbs
        release={props.release}
        currentPage="Payload Streams"
      />
      <Typography variant="h4" gutterBottom className={classes.title}>
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
