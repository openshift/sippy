import { Box, Card, CardContent, Tooltip, Typography } from '@mui/material'
import { Doughnut } from 'react-chartjs-2'
import { Link } from 'react-router-dom'
import { makeStyles, useTheme } from '@mui/styles'
import { scale } from 'chroma-js'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles({
  cardContent: {
    color: 'black',
    textAlign: 'center',
  },
  summaryCard: (props) => ({
    height: '100%',
  }),
})

/**
 * SummaryCard with a simple pie chart showing some data,
 * along with a caption. Used mainly by the ReleaseOverview
 * page.
 */
export default function SummaryCard(props) {
  const classes = useStyles(props)
  const theme = useTheme()

  const percent =
    ((props.success + props.flakes) /
      ((props.flakes || 0) + props.fail + props.success)) *
    100

  const colors = scale([
    theme.palette.error.light,
    theme.palette.warning.light,
    theme.palette.success.light,
  ]).domain([
    props.threshold.error,
    props.threshold.warning,
    props.threshold.success,
  ])

  const bgColor = colors(percent).hex()

  const labels = ['Pass', 'Flake', 'Fail']
  const data = [props.success, props.flakes, props.fail]
  const color = [
    theme.palette.success.dark,
    theme.palette.warning.dark,
    theme.palette.error.dark,
  ]

  let card = (
    <Card
      elevation={5}
      className={`${classes.summaryCard}`}
      style={{ backgroundColor: bgColor }}
    >
      <CardContent className={`${classes.cardContent}`}>
        <Typography variant="h6">
          {props.name}
          {props.tooltip ? <InfoIcon /> : ''}
        </Typography>
        <div align="center">
          <div style={{ width: '70%' }}>
            <Doughnut
              data={{
                labels: labels,
                datasets: [
                  {
                    label: props.name,
                    data: data,
                    borderColor: bgColor,
                    backgroundColor: color,
                  },
                ],
              }}
              options={{
                plugins: {
                  legend: {
                    display: false,
                  },
                },
                cutout: '60%',
              }}
            />
          </div>
          {props.caption}
        </div>
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

SummaryCard.defaultProps = {
  success: 0,
  fail: 0,
  flakes: 0,
  caption: '',
  tooltip: '',
  units: 'percent',
}

SummaryCard.propTypes = {
  units: PropTypes.string,
  flakes: PropTypes.number,
  success: PropTypes.number,
  fail: PropTypes.number,
  caption: PropTypes.oneOfType([PropTypes.object, PropTypes.string]),
  tooltip: PropTypes.string,
  name: PropTypes.string,
  link: PropTypes.string,
  threshold: PropTypes.shape({
    success: PropTypes.number,
    warning: PropTypes.number,
    error: PropTypes.number,
  }),
}
