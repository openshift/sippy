import { Box, Card, CardContent, Tooltip, Typography } from '@material-ui/core'
import PropTypes from 'prop-types'
import { makeStyles, useTheme } from '@material-ui/core/styles'
import React, { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { scale } from 'chroma-js'
import { PieChart } from 'react-minimal-pie-chart'

const useStyles = makeStyles({
  cardContent: {
    color: 'black',
    textAlign: 'center'
  },
  summaryCard: props => ({
    height: '100%'
  })
})

/**
 * SummaryCard with a simple pie chart showing some data,
 * along with a caption. Used mainly by the ReleaseOverview
 * page.
 */
export default function SummaryCard (props) {
  const classes = useStyles(props)
  const theme = useTheme()

  const [currentData, setCurrentData] = React.useState([])

  const percent = (props.success / (props.flakes + props.fail + props.success)) * 100

  const colors = scale(
    [
      theme.palette.error.light,
      theme.palette.warning.light,
      theme.palette.success.light
    ]).domain([props.threshold.error, props.threshold.warning, props.threshold.success])

  const bgColor = colors(percent).hex()

  useEffect(() => {
    const data = []

    if (props.flakes !== 0) {
      data.push({
        title: 'Flakes',
        value: props.flakes,
        color: '#FF8800'
      })
    }

    data.push({
      title: 'Success',
      value: props.success,
      color: '#4A934A'
    })

    data.push({
      title: 'Fail',
      value: props.fail,
      color: 'darkred'
    })

    setCurrentData(data)
  }, [props])

  let card = (
    <Card elevation={5} className={`${classes.summaryCard}`} style={{ backgroundColor: bgColor }}>
      <CardContent className={`${classes.cardContent}`}>
        <Typography variant="h6">{props.name}</Typography>
        <PieChart
          animate
          animationDuration={500}
          animationEasing="ease-out"
          center={[40, 25]}
          data={currentData}
          labelPosition={50}
          lengthAngle={360}
          lineWidth={30}
          paddingAngle={2}
          radius={20}
          segmentsShift={0.5}
          startAngle={0}
          viewBoxSize={[80, 50]}
        />
        {props.caption}
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
  flakes: 0,
  success: 0,
  fail: 0,
  caption: '',
  tooltip: ''
}

SummaryCard.propTypes = {
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
    error: PropTypes.number
  })
}
