import { Link } from 'react-router-dom'
import Breadcrumbs from '@mui/material/Breadcrumbs'
import PropTypes from 'prop-types'
import React from 'react'
import Typography from '@mui/material/Typography'

export default function SimpleBreadcrumbs(props) {
  return (
    <Breadcrumbs aria-label="breadcrumb">
      <Link to={'/release/' + props.release}>Overview</Link>

      {props.previousPage ? props.previousPage : ''}

      {props.currentPage ? <Typography>{props.currentPage}</Typography> : ''}
    </Breadcrumbs>
  )
}

SimpleBreadcrumbs.propTypes = {
  release: PropTypes.string.isRequired,
  previousPage: PropTypes.element,
  currentPage: PropTypes.string,
}
