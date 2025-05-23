import { Box, Card, CardContent, Tooltip, Typography } from '@mui/material'
import { Link } from 'react-router-dom'
import { makeStyles, useTheme } from '@mui/styles'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles({
  cardContent: {
    textAlign: 'center',
  },
  numberCard: (props) => ({
    height: '100%',
  }),
})

export default function NumberCard(props) {
  const classes = useStyles(props)
  const theme = useTheme()

  let card = (
    <Card
      elevation={5}
      className={`${classes.numberCard}`}
      style={{ backgroundColor: props.bgColor }}
    >
      <CardContent className={`${classes.cardContent}`}>
        <Typography variant="h6">{props.title}</Typography>
        <div style={{ fontSize: `${props.size ? props.size : 6}em` }}>
          {props.number}
        </div>
        <div align="center">{props.caption}</div>
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

NumberCard.propTypes = {
  title: PropTypes.string,
  caption: PropTypes.oneOfType([PropTypes.object, PropTypes.string]),
  tooltip: PropTypes.string,
  bgColor: PropTypes.string,
  number: PropTypes.number,
  size: PropTypes.number,
  link: PropTypes.element,
}
