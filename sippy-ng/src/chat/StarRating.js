import { Box, Fade, Typography } from '@mui/material'
import { makeStyles } from '@mui/styles'
import { Star as StarIcon } from '@mui/icons-material'
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
  },
  thankYouMessage: {
    fontSize: '0.75rem',
    color: theme.palette.text.secondary,
    fontStyle: 'italic',
  },
  privacyNotice: {
    fontSize: '0.6rem',
    color: theme.palette.text.secondary,
    fontStyle: 'italic',
    marginTop: theme.spacing(0.25),
    textAlign: 'center',
    opacity: 0.8,
  },
}))

export default function StarRating({ messageId, onRate }) {
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
      <Box>
        <Box className={classes.ratingContainer}>
          <Typography className={classes.ratingLabel}>
            How helpful was this conversation?
          </Typography>
          <div className={classes.starsContainer}>
            {[1, 2, 3, 4, 5].map((star) => (
              <StarIcon
                key={star}
                fontSize="small"
                className={`${classes.star} ${
                  star <= (hoveredStar || selectedRating || 0) ? 'filled' : ''
                } ${star <= hoveredStar && !selectedRating ? 'hovered' : ''}`}
                onMouseEnter={() => setHoveredStar(star)}
                onMouseLeave={() => setHoveredStar(null)}
                onClick={() => handleStarClick(star)}
              />
            ))}
          </div>
        </Box>
        <Typography
          className={classes.privacyNotice}
          sx={{
            fontSize: '0.6rem !important',
            opacity: 0.8,
          }}
        >
          Ratings collect anonymous usage metrics, and no chat content is shared
        </Typography>
      </Box>
    </Fade>
  )
}

StarRating.propTypes = {
  messageId: PropTypes.string.isRequired,
  onRate: PropTypes.func,
}
