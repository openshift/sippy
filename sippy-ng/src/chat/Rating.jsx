import { Box, Fade, Tooltip, Typography } from '@mui/material'
import { Info as InfoIcon, Star as StarIcon } from '@mui/icons-material'
import { makeStyles } from '@mui/styles'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

const useStyles = makeStyles((theme) => ({
  ratingContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.75),
    padding: 0,
  },
  starsContainer: {
    display: 'flex',
    gap: 2,
  },
  star: {
    cursor: 'pointer',
    transition: 'all 0.2s ease-in-out',
    color: theme.palette.grey[400],
    '&:hover': {
      transform: 'scale(1.2)',
      color: theme.palette.warning.main,
    },
    '&.filled': {
      color: theme.palette.warning.main,
    },
    '&.hovered': {
      color: theme.palette.warning.light,
    },
  },
  ratingLabel: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    marginRight: theme.spacing(0.5),
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
  },
  infoIcon: {
    fontSize: '0.9rem',
    color: theme.palette.text.secondary,
    opacity: 0.6,
    cursor: 'help',
  },
  thankYouMessage: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    fontStyle: 'italic',
  },
}))

const ratingLabels = {
  1: 'Wasted my time',
  2: 'Not helpful',
  3: 'Neutral',
  4: 'Saved me time',
  5: 'Huge time saver!',
}

export default function Rating({ messageId, onRate }) {
  const classes = useStyles()
  const [hoveredStar, setHoveredStar] = useState(null)
  const [selectedRating, setSelectedRating] = useState(null)
  const [showThanks, setShowThanks] = useState(false)
  const [fadeOut, setFadeOut] = useState(false)

  const handleStarClick = (rating) => {
    setSelectedRating(rating)
    setShowThanks(true)
    if (onRate) {
      onRate(messageId, rating)
    }

    // Start fade out after 2 seconds
    setTimeout(() => {
      setFadeOut(true)
    }, 2000)
  }

  // Show thank you message after rating
  if (showThanks) {
    return (
      <Fade in={!fadeOut} timeout={500}>
        <Box className={classes.ratingContainer}>
          <Typography className={classes.thankYouMessage}>
            Thanks for your feedback!
          </Typography>
        </Box>
      </Fade>
    )
  }

  return (
    <Fade in timeout={300}>
      <Box className={classes.ratingContainer}>
        <Typography className={classes.ratingLabel}>
          Have I saved you time today?
          <Tooltip
            title="Ratings collect anonymous usage metrics, and no chat content is shared"
            arrow
            placement="top"
          >
            <InfoIcon className={classes.infoIcon} />
          </Tooltip>
        </Typography>
        <div className={classes.starsContainer}>
          {[1, 2, 3, 4, 5].map((star) => (
            <Tooltip key={star} title={ratingLabels[star]} arrow>
              <StarIcon
                fontSize="small"
                className={`${classes.star} ${
                  star <= (hoveredStar || selectedRating || 0) ? 'filled' : ''
                } ${star <= hoveredStar && !selectedRating ? 'hovered' : ''}`}
                onMouseEnter={() => setHoveredStar(star)}
                onMouseLeave={() => setHoveredStar(null)}
                onClick={() => handleStarClick(star)}
              />
            </Tooltip>
          ))}
        </div>
      </Box>
    </Fade>
  )
}

Rating.propTypes = {
  messageId: PropTypes.string.isRequired,
  onRate: PropTypes.func,
}
