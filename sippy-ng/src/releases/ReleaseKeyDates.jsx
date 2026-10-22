import { Box, Chip, Typography } from '@mui/material'
import { styled } from '@mui/material/styles'
import CalendarTodayIcon from '@mui/icons-material/CalendarToday'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import Grid from '@mui/material/Grid'
import PropTypes from 'prop-types'
import React from 'react'
import ScheduleIcon from '@mui/icons-material/Schedule'

const CardBox = styled(Box)({
  height: '100%',
  display: 'flex',
  flexDirection: 'column',
})

const HeaderBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  marginBottom: theme.spacing(1.5),
}))

const DateItemBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
  padding: theme.spacing(1),
  borderRadius: theme.spacing(0.75),
  backgroundColor: theme.palette.action.hover,
  '&:not(:last-child)': {
    marginBottom: theme.spacing(1),
  },
}))

const InlineDatesBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  gap: theme.spacing(2),
  alignItems: 'center',
  flexWrap: 'wrap',
}))

const InlineDateItemBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  gap: theme.spacing(1),
}))

export default function ReleaseKeyDates({ release, releases, inline }) {
  if (
    !releases ||
    !releases.release_attrs ||
    !releases.release_attrs[release]
  ) {
    return null
  }

  const releaseAttrs = releases.release_attrs[release]
  const gaDate = releaseAttrs.ga ? new Date(releaseAttrs.ga) : null
  const devStartDate = releaseAttrs.development_start
    ? new Date(releaseAttrs.development_start)
    : null

  // Don't show the card if no dates are available
  if (!gaDate && !devStartDate) {
    return null
  }

  const formatDate = (date) => {
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    })
  }

  const getDaysUntil = (date) => {
    const today = new Date()
    const diffTime = date - today
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24))
    return diffDays
  }

  const isDateInPast = (date) => {
    return date < new Date()
  }

  // Inline mode for display next to the title
  if (inline) {
    return (
      <InlineDatesBox>
        {devStartDate && (
          <InlineDateItemBox>
            <ScheduleIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
            <Typography variant="body2" color="text.secondary">
              Dev Start: {formatDate(devStartDate)}
            </Typography>
          </InlineDateItemBox>
        )}
        {gaDate && (
          <InlineDateItemBox>
            <CheckCircleIcon
              sx={{
                fontSize: 18,
                color: isDateInPast(gaDate) ? 'success.main' : 'info.main',
              }}
            />
            <Typography variant="body2" color="text.secondary">
              GA: {formatDate(gaDate)}
            </Typography>
            {!isDateInPast(gaDate) && (
              <Chip
                label={`${getDaysUntil(gaDate)} days`}
                size="small"
                color="info"
                sx={{ fontWeight: 500 }}
              />
            )}
            {isDateInPast(gaDate) && (
              <Chip
                label="Released"
                size="small"
                color="success"
                sx={{ fontWeight: 500 }}
              />
            )}
          </InlineDateItemBox>
        )}
      </InlineDatesBox>
    )
  }

  // Card mode for grid display
  return (
    <Grid item xs={12} md={4}>
      <CardBox>
        <HeaderBox>
          <CalendarTodayIcon
            sx={(theme) => ({
              marginRight: theme.spacing(0.75),
              color: theme.palette.text.secondary,
              fontSize: 20,
            })}
          />
          <Typography variant="h6">Key Dates</Typography>
        </HeaderBox>

        <Box>
          {devStartDate && (
            <DateItemBox>
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                <ScheduleIcon
                  sx={(theme) => ({
                    marginRight: theme.spacing(0.75),
                    fontSize: 18,
                  })}
                  color="text.secondary"
                />
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    Development Start
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {formatDate(devStartDate)}
                  </Typography>
                </Box>
              </Box>
            </DateItemBox>
          )}

          {gaDate && (
            <DateItemBox>
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                <CheckCircleIcon
                  sx={(theme) => ({
                    marginRight: theme.spacing(0.75),
                    fontSize: 18,
                    color: isDateInPast(gaDate) ? 'success.main' : 'info.main',
                  })}
                />
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    General Availability (GA)
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {formatDate(gaDate)}
                  </Typography>
                </Box>
              </Box>
              {!isDateInPast(gaDate) && (
                <Chip
                  label={`${getDaysUntil(gaDate)} days`}
                  size="small"
                  color="info"
                  sx={{ fontWeight: 500 }}
                />
              )}
              {isDateInPast(gaDate) && (
                <Chip
                  label="Released"
                  size="small"
                  color="success"
                  sx={{ fontWeight: 500 }}
                />
              )}
            </DateItemBox>
          )}
        </Box>
      </CardBox>
    </Grid>
  )
}

ReleaseKeyDates.propTypes = {
  release: PropTypes.string.isRequired,
  releases: PropTypes.object.isRequired,
  inline: PropTypes.bool,
}
