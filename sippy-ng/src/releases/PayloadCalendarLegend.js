import { Box, Typography } from '@mui/material'
import { styled } from '@mui/material/styles';
import { makeStyles, useTheme } from '@mui/material/styles'
import React from 'react'

const PREFIX = 'PayloadCalendarLegend';

const classes = {
  legendContainer: `${PREFIX}-legendContainer`,
  legendLabel: `${PREFIX}-legendLabel`,
  legendItem: `${PREFIX}-legendItem`,
  square: `${PREFIX}-square`
};

const StyledBox = styled(Box)((
  {
    theme
  }
) => ({
  [`&.${classes.legendContainer}`]: {
    display: 'flex',
    alignItems: 'center',
    marginBottom: theme.spacing(2),
    border: '1px solid black',
    padding: theme.spacing(2),
  },

  [`& .${classes.legendLabel}`]: {
    marginBottom: theme.spacing(1),
  },

  [`& .${classes.legendItem}`]: {
    marginRight: theme.spacing(2),
    display: 'flex',
    alignItems: 'center',
  },

  [`& .${classes.square}`]: {
    width: theme.spacing(1.5),
    height: theme.spacing(1.5),
    marginRight: theme.spacing(1),
    borderRadius: '2px',
  }
}));

export default function PayloadCalendarLegend() {
  const theme = useTheme()


  return (
    <StyledBox className={classes.legendContainer}>
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
          style={{ backgroundColor: theme.palette.common.black }}
        />
        <Typography variant="body2">Incident</Typography>
      </Box>
    </StyledBox>
  );
}
