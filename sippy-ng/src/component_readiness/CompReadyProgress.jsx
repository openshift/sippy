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
import { styled } from '@mui/material/styles'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import PropTypes from 'prop-types'
import React from 'react'

const CenteredContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  minHeight: '60vh',
  padding: theme.spacing(4),
}))

const DetailsAccordion = styled(Accordion)(({ theme }) => ({
  width: '100%',
  marginTop: theme.spacing(2),
  boxShadow: theme.shadows[1],
  '&:before': { display: 'none' },
}))

const ApiLink = styled(Link)(({ theme }) => ({
  display: 'block',
  marginBottom: theme.spacing(2),
  wordBreak: 'break-all',
  fontSize: '0.875rem',
}))

const ParamList = styled(Box)(({ theme }) => ({
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
}))

export default function CompReadyProgress(props) {
  const { apiLink } = props

  document.title = 'Loading...'

  return (
    <CenteredContainer>
      <Stack
        spacing={3}
        alignItems="center"
        sx={{ maxWidth: 800, width: '100%' }}
      >
        <CircularProgress size={60} sx={{ color: 'primary.main' }} />

        <Typography variant="h5" color="text.primary" gutterBottom>
          Loading Component Readiness Data
        </Typography>

        <Typography variant="body1" color="text.secondary" align="center">
          Please wait while we fetch your data. Large datasets may take several
          minutes to load.
        </Typography>

        <DetailsAccordion>
          <AccordionSummary
            expandIcon={<ExpandMoreIcon />}
            sx={(theme) => ({
              backgroundColor: theme.palette.action.hover,
              '&:hover': { backgroundColor: theme.palette.action.selected },
            })}
          >
            <Typography variant="body2" color="text.secondary">
              View API Request Details
            </Typography>
          </AccordionSummary>
          <AccordionDetails sx={(theme) => ({ pt: theme.spacing(2) })}>
            <Typography variant="body2" color="text.secondary" gutterBottom>
              API Endpoint:
            </Typography>
            <ApiLink href={apiLink} target="_blank" rel="noopener noreferrer">
              {apiLink}
            </ApiLink>
            {apiLink.includes('?') && (
              <>
                <Typography variant="body2" color="text.secondary" gutterBottom>
                  Request Parameters:
                </Typography>
                <ParamList component="ul">
                  {apiLink
                    .split('?')[1]
                    .split('&')
                    .map((param, index) => {
                      if (param && !param.endsWith('=')) {
                        return <li key={index}>{param}</li>
                      }
                      return null
                    })}
                </ParamList>
              </>
            )}
          </AccordionDetails>
        </DetailsAccordion>
      </Stack>
    </CenteredContainer>
  )
}

CompReadyProgress.propTypes = {
  apiLink: PropTypes.string.isRequired,
}
