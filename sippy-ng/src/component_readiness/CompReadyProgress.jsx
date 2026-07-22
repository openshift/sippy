import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  CircularProgress,
  Link,
  Stack,
  Typography,
} from '@mui/material'
import { makeStyles } from '@mui/styles'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import PropTypes from 'prop-types'
import React from 'react'

const useStyles = makeStyles((theme) => ({
  container: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: '60vh',
    padding: theme.spacing(4),
  },
  contentStack: {
    maxWidth: 800,
    width: '100%',
  },
  spinner: {
    color: theme.palette.primary.main,
  },
  accordion: {
    width: '100%',
    marginTop: theme.spacing(2),
    boxShadow: theme.shadows[1],
    '&:before': {
      display: 'none',
    },
  },
  accordionSummary: {
    backgroundColor: theme.palette.action.hover,
    '&:hover': {
      backgroundColor: theme.palette.action.selected,
    },
  },
  accordionDetails: {
    paddingTop: theme.spacing(2),
  },
  apiLink: {
    display: 'block',
    marginBottom: theme.spacing(2),
    wordBreak: 'break-all',
    fontSize: '0.875rem',
  },
  parameterList: {
    margin: 0,
    paddingLeft: theme.spacing(2),
    listStyle: 'none',
    '& li': {
      paddingTop: theme.spacing(0.5),
      paddingBottom: theme.spacing(0.5),
      fontSize: '0.875rem',
      fontFamily: 'monospace',
      color: theme.palette.text.primary,
    },
  },
}))

export default function CompReadyProgress(props) {
  const { apiLink } = props
  const classes = useStyles()

  document.title = 'Loading...'

  return (
    <Box className={classes.container}>
      <Stack spacing={3} alignItems="center" className={classes.contentStack}>
        <CircularProgress size={60} className={classes.spinner} />

        <Typography variant="h5" color="text.primary" gutterBottom>
          Loading Component Readiness Data
        </Typography>

        <Typography variant="body1" color="text.secondary" align="center">
          Please wait while we fetch your data. Large datasets may take several
          minutes to load.
        </Typography>

        <Accordion className={classes.accordion}>
          <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            className={classes.accordionSummary}
          >
            <Typography variant="body2" color="text.secondary">
              View API Request Details
            </Typography>
          </AccordionSummary>
          <AccordionDetails className={classes.accordionDetails}>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              API Endpoint:
            </Typography>
            <Link
              href={apiLink}
              target="_blank"
              rel="noopener noreferrer"
              className={classes.apiLink}
            >
              {apiLink}
            </Link>
            {apiLink.includes('?') && (
              <>
                <Typography variant="body2" color="text.secondary" gutterBottom>
                  Request Parameters:
                </Typography>
                <Box component="ul" className={classes.parameterList}>
                  {apiLink
                    .split('?')[1]
                    .split('&')
                    .map((param, index) => {
                      if (param && !param.endsWith('=')) {
                        return <li key={index}>{param}</li>
                      }
                      return null
                    })}
                </Box>
              </>
            )}
          </AccordionDetails>
        </Accordion>
      </Stack>
    </Box>
  )
}

CompReadyProgress.propTypes = {
  apiLink: PropTypes.string.isRequired,
}
