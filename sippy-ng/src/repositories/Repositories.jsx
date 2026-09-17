import { Container, Typography } from '@mui/material'
import PropTypes from 'prop-types'
import React, { Fragment, useEffect } from 'react'
import RepositoriesTable from './RepositoriesTable'
import SimpleBreadcrumbs from '../components/SimpleBreadcrumbs'

export default function Repositories(props) {
  useEffect(() => {
    document.title = `Sippy > ${props.release} > Repositories`
  }, [])

  return (
    <Fragment>
      <SimpleBreadcrumbs release={props.release} currentPage="Repositories" />
      <Container size="xl">
        <Typography align="center" variant="h4">
          {props.release} Repositories
        </Typography>
        <RepositoriesTable release={props.release} />
      </Container>
    </Fragment>
  )
}

Repositories.propTypes = {
  release: PropTypes.string.isRequired,
}
