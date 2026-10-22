import { Box, Typography } from '@mui/material'
import React from 'react'

export default function PayloadCalendarLegend() {
  return (
    <Box
      sx={(theme) => ({
        display: 'flex',
        alignItems: 'center',
        marginBottom: theme.spacing(2),
        border: '1px solid black',
        padding: theme.spacing(2),
      })}
    >
      <Box
        sx={(theme) => ({
          marginRight: theme.spacing(2),
          display: 'flex',
          alignItems: 'center',
        })}
      >
        <Box
          sx={(theme) => ({
            width: theme.spacing(1.5),
            height: theme.spacing(1.5),
            marginRight: theme.spacing(1),
            borderRadius: '2px',
            backgroundColor: theme.palette.success.light,
          })}
        />
        <Typography variant="body2">Accepted</Typography>
      </Box>
      <Box
        sx={(theme) => ({
          marginRight: theme.spacing(2),
          display: 'flex',
          alignItems: 'center',
        })}
      >
        <Box
          sx={(theme) => ({
            width: theme.spacing(1.5),
            height: theme.spacing(1.5),
            marginRight: theme.spacing(1),
            borderRadius: '2px',
            backgroundColor: theme.palette.error.light,
          })}
        />
        <Typography variant="body2">Rejected</Typography>
      </Box>
      <Box
        sx={(theme) => ({
          marginRight: theme.spacing(2),
          display: 'flex',
          alignItems: 'center',
        })}
      >
        <Box
          sx={(theme) => ({
            width: theme.spacing(1.5),
            height: theme.spacing(1.5),
            marginRight: theme.spacing(1),
            borderRadius: '2px',
            backgroundColor: theme.palette.common.black,
          })}
        />
        <Typography variant="body2">Incident</Typography>
      </Box>
    </Box>
  )
}
