import {
  Box,
  Card,
  CardContent,
  Grid,
  Tooltip,
  Typography,
} from '@mui/material'
import { Link } from 'react-router-dom'
import { makeStyles, useTheme } from '@mui/styles'
import { scale } from 'chroma-js'
import PassRateIcon from './PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

const useStyles = makeStyles({
  cardContent: {
    textAlign: 'center',
  },
  miniCard: (props) => ({
    height: '100%',
  }),
})

export default function MiniCard(props) {
  const classes = useStyles(props)
  const theme = useTheme()

  const colorScale = scale([
    theme.palette.error.light,
    theme.palette.warning.light,
    theme.palette.success.light,
  ]).domain([
    props.threshold.error,
    props.threshold.warning,
    props.threshold.success,
  ])

  let bgColor = colorScale(props.current).hex()
  if (props.currentRuns === 0) {
    bgColor = theme.palette.text.disabled
  }

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
    <Card
      elevation={5}
      className={`${classes.miniCard}`}
      sx={{ backgroundColor: bgColor }}
    >
      <CardContent
        className={`${classes.cardContent}`}
        sx={{ textAlign: 'center' }}
      >
        <Typography variant="h6">{props.name}</Typography>
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
  if (props.tooltip !== undefined) {
    card = (
      <Tooltip title={props.tooltip} placement="top">
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

MiniCard.defaultProps = {
  flakes: 0,
  success: 0,
  fail: 0,
  tooltip: '',
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
