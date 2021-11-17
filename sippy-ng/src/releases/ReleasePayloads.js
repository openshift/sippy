import { Container, Typography } from '@material-ui/core'
import { createTheme, makeStyles } from '@material-ui/core/styles'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import ReleasePayloadTable from './ReleasePayloadTable'
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

function ReleasePayloads(props) {
  const classes = useStyles()

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Tags" />
      <Container size="xl">
        <Typography variant="h4" gutterBottom className={classes.title}>
          Release Payloads
        </Typography>
        <ReleasePayloadTable release={props.release} />
      </Container>
    </Fragment>
  )
}

ReleasePayloads.propTypes = {
  release: PropTypes.string,
}

export default ReleasePayloads
