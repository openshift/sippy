import { Box, Card, CardContent, Tooltip, Typography } from '@mui/material'
import { Link } from 'react-router'
import PropTypes from 'prop-types'
import React from 'react'

export default function NumberCard(props) {
  let card = (
    <Card elevation={5} sx={{ height: '100%', backgroundColor: props.bgColor }}>
      <CardContent sx={{ textAlign: 'center' }}>
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
