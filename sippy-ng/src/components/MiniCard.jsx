import {
  Box,
  Card,
  CardContent,
  CardHeader,
  Grid,
  Tooltip,
} from '@mui/material'
import { Link } from 'react-router'
import { parseVariantName } from '../helpers'
import { useTheme } from '@mui/material/styles'
import chroma from 'chroma-js'
import PassRateIcon from './PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function MiniCard({ tooltip = '', ...props }) {
  const theme = useTheme()

  const colorScale = chroma
    .scale([
      theme.palette.error.light,
      theme.palette.warning.light,
      theme.palette.success.light,
    ])
    .domain([
      props.threshold.error,
      props.threshold.warning,
      props.threshold.success,
    ])

  let bgColor = colorScale(props.current).hex()
  if (props.currentRuns === 0) {
    bgColor = theme.palette.text.disabled
  }

  let variantInfo = parseVariantName(props.name)

  const summary = (
    <Fragment>
      <div align="center">
        {props.current.toFixed(1)}%{' '}
        <PassRateIcon improvement={props.current - props.previous} />{' '}
        {props.previous.toFixed(1)}%
      </div>
    </Fragment>
  )

  let card = (
    <Card elevation={5} sx={{ height: '100%', backgroundColor: bgColor }}>
      <CardHeader title={variantInfo.name} subheader={variantInfo.variant} />
      <CardContent sx={{ textAlign: 'center' }}>
        <Grid
          container
          direction="row"
          alignItems="center"
          sx={{ textAlign: 'center' }}
        >
          {props.currentRuns > 0 ? summary : 'No data'}
        </Grid>
      </CardContent>
    </Card>
  )

  // Wrap in tooltip if we have one
  if (tooltip !== undefined) {
    card = (
      <Tooltip title={tooltip} placement="top">
        {card}
      </Tooltip>
    )
  }

  // Link if we have one
  if (props.link !== undefined) {
    return (
      <Box component={Link} to={props.link}>
        {card}
      </Box>
    )
  } else {
    return card
  }
}

MiniCard.propTypes = {
  tooltip: PropTypes.string,
  name: PropTypes.string,
  link: PropTypes.string,
  current: PropTypes.number,
  currentRuns: PropTypes.number,
  previous: PropTypes.number,
  previousRuns: PropTypes.number,
  threshold: PropTypes.shape({
    success: PropTypes.number,
    warning: PropTypes.number,
    error: PropTypes.number,
  }),
}
