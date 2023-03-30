import { Typography } from '@material-ui/core'
import PropTypes from 'prop-types'
import PullRequestsTable from './PullRequestsTable'
import React, { Fragment, useEffect } from 'react'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

/**
 * PullRequests is the landing page for pull requests
 */
export default function PullRequests(props) {
  useEffect(() => {
    document.title = `Sippy > ${props.release} > Pull Requests`
  }, [])

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Pull Requests" />
      <Typography align="center" variant="h4">
        Pull Requests
      </Typography>
      <PullRequestsTable release={props.release} />
    </Fragment>
  )
}

PullRequests.propTypes = {
  release: PropTypes.string,
}
