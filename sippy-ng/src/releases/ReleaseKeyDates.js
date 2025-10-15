import { Box, Chip, Typography } from '@mui/material'
import { makeStyles } from '@mui/styles'
import CalendarTodayIcon from '@mui/icons-material/CalendarToday'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import Grid from '@mui/material/Grid'
import PropTypes from 'prop-types'
import React from 'react'
import ScheduleIcon from '@mui/icons-material/Schedule'

const useStyles = makeStyles((theme) => ({
  card: {
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    marginBottom: theme.spacing(1.5),
  },
  icon: {
    marginRight: theme.spacing(0.75),
    color: theme.palette.text.secondary,
    fontSize: 20,
  },
  dateItem: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: theme.spacing(1),
    borderRadius: theme.spacing(0.75),
    backgroundColor: theme.palette.action.hover,
    '&:not(:last-child)': {
      marginBottom: theme.spacing(1),
    },
  },
  dateLabel: {
    display: 'flex',
    alignItems: 'center',
  },
  dateIcon: {
    marginRight: theme.spacing(0.75),
    fontSize: 18,
  },
  inlineDates: {
    display: 'flex',
    gap: theme.spacing(2),
    alignItems: 'center',
    flexWrap: 'wrap',
  },
  inlineDateItem: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(1),
  },
}))

export default function ReleaseKeyDates({ release, releases, inline }) {
  const classes = useStyles()

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
      <Box className={classes.inlineDates}>
        {devStartDate && (
          <Box className={classes.inlineDateItem}>
            <ScheduleIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
            <Typography variant="body2" color="text.secondary">
              Dev Start: {formatDate(devStartDate)}
            </Typography>
          </Box>
        )}
        {gaDate && (
          <Box className={classes.inlineDateItem}>
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
          </Box>
        )}
      </Box>
    )
  }

  // Card mode for grid display
  return (
    <Grid item xs={12} md={4}>
      <Box className={classes.card}>
        <Box className={classes.header}>
          <CalendarTodayIcon className={classes.icon} />
          <Typography variant="h6">Key Dates</Typography>
        </Box>

        <Box>
          {devStartDate && (
            <Box className={classes.dateItem}>
              <Box className={classes.dateLabel}>
                <ScheduleIcon
                  className={classes.dateIcon}
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
            </Box>
          )}

          {gaDate && (
            <Box className={classes.dateItem}>
              <Box className={classes.dateLabel}>
                <CheckCircleIcon
                  className={classes.dateIcon}
                  sx={{
                    color: isDateInPast(gaDate) ? 'success.main' : 'info.main',
                  }}
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
            </Box>
          )}
        </Box>
      </Box>
    </Grid>
  )
}

ReleaseKeyDates.propTypes = {
  release: PropTypes.string.isRequired,
  releases: PropTypes.object.isRequired,
  inline: PropTypes.bool,
}
