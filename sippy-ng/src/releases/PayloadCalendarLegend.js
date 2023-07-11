import { Box, Typography } from '@material-ui/core'
import { makeStyles, useTheme } from '@material-ui/core/styles'
import React from 'react'

const useStyles = makeStyles((theme) => ({
  legendContainer: {
    display: 'flex',
    alignItems: 'center',
    marginBottom: theme.spacing(2),
    border: '1px solid black',
    padding: theme.spacing(2),
  },
  legendLabel: {
    marginBottom: theme.spacing(1),
  },
  legendItem: {
    marginRight: theme.spacing(2),
    display: 'flex',
    alignItems: 'center',
  },
  square: {
    width: theme.spacing(1.5),
    height: theme.spacing(1.5),
    marginRight: theme.spacing(1),
    borderRadius: '2px',
  },
}))

export default function PayloadCalendarLegend() {
  const theme = useTheme()
  const classes = useStyles()

  return (
    <Box className={classes.legendContainer}>
      <Box className={classes.legendItem}>
        <Box
          className={classes.square}
          style={{ backgroundColor: theme.palette.success.light }}
        />
        <Typography variant="body2">Accepted</Typography>
      </Box>
      <Box className={classes.legendItem}>
        <Box
          className={classes.square}
          style={{ backgroundColor: theme.palette.error.light }}
        />
        <Typography variant="body2">Rejected</Typography>
      </Box>
      <Box className={classes.legendItem}>
        <Box
          className={classes.square}
          style={{ backgroundColor: theme.palette.error.dark }}
        />
        <Typography variant="body2">Incident</Typography>
      </Box>
    </Box>
  )
}
