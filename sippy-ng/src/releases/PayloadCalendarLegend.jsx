import { Box, Typography } from '@mui/material'
import { styled } from '@mui/material/styles'
import React from 'react'

const LegendContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  marginBottom: theme.spacing(2),
  border: '1px solid black',
  padding: theme.spacing(2),
}))

const LegendItem = styled(Box)(({ theme }) => ({
  marginRight: theme.spacing(2),
  display: 'flex',
  alignItems: 'center',
}))

const LegendSwatch = styled(Box)(({ theme }) => ({
  width: theme.spacing(1.5),
  height: theme.spacing(1.5),
  marginRight: theme.spacing(1),
  borderRadius: '2px',
}))

export default function PayloadCalendarLegend() {
  return (
    <LegendContainer>
      <LegendItem>
        <LegendSwatch sx={{ backgroundColor: 'success.light' }} />
        <Typography variant="body2">Accepted</Typography>
      </LegendItem>
      <LegendItem>
        <LegendSwatch sx={{ backgroundColor: 'error.light' }} />
        <Typography variant="body2">Rejected</Typography>
      </LegendItem>
      <LegendItem>
        <LegendSwatch sx={{ backgroundColor: 'common.black' }} />
        <Typography variant="body2">Incident</Typography>
      </LegendItem>
    </LegendContainer>
  )
}
