import {
  Box,
  Button,
  Card,
  Chip,
  Divider,
  List,
  ListItemButton,
  ListItemText,
  Typography,
  useTheme,
} from '@mui/material'
import { COMPONENT_READINESS_THRESHOLDS } from '../constants'
import {
  convertApiUrlToUiUrl,
  getTestDetailsLink,
  getTriagesAPIUrl,
} from './CompReadyUtils'
import { Link } from 'react-router'
import { relativeTime } from '../helpers'
import { styled } from '@mui/material/styles'
import ArrowForwardIcon from '@mui/icons-material/ArrowForward'
import Grid from '@mui/material/Grid'
import HealingIcon from '@mui/icons-material/Healing'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { useEffect, useState } from 'react'
import Tooltip from '@mui/material/Tooltip'
import WarningIcon from '@mui/icons-material/Warning'

const CenteredMessageBox = styled(Box)(({ theme }) => ({
  padding: theme.spacing(4),
  textAlign: 'center',
  color: theme.palette.text.secondary,
}))

const StatsHeaderBox = styled(Box)(({ theme }) => ({
  marginBottom: theme.spacing(2),
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'space-between',
}))

const StatsColumnBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  flexDirection: 'column',
  gap: theme.spacing(2),
}))

const StatCardBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  padding: theme.spacing(2),
  borderRadius: theme.spacing(1),
  backgroundColor: theme.palette.action.hover,
}))

const SectionHeaderBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  alignItems: 'center',
  marginBottom: theme.spacing(2),
}))

const RegressionList = styled(List)(({ theme }) => ({
  maxHeight: 240,
  overflow: 'auto',
  backgroundColor: theme.palette.action.hover,
  borderRadius: theme.spacing(1),
}))

const TruncatedTestName = styled(Typography)({
  fontWeight: 500,
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  display: '-webkit-box',
  WebkitLineClamp: 2,
  WebkitBoxOrient: 'vertical',
})

const VariantChipsBox = styled(Box)({
  display: 'flex',
  flexWrap: 'wrap',
  gap: 4,
})

export default function ComponentReadinessIndicator({ release }) {
  const theme = useTheme()
  const [regressions, setRegressions] = useState(null)
  const [isLoaded, setIsLoaded] = useState(false)

  useEffect(() => {
    const viewName = `${release}-main`
    const componentReportUrl =
      import.meta.env.VITE_API_URL +
      '/api/component_readiness?view=' +
      encodeURIComponent(viewName)

    // Fetch both the component report and triages
    const componentReportPromise = fetch(componentReportUrl)
      .then((response) => {
        if (response.status !== 200) {
          return null
        }
        return response.json()
      })
      .catch(() => null)

    const triagesPromise = fetch(getTriagesAPIUrl(null, viewName))
      .then((response) => {
        if (response.status !== 200) {
          return []
        }
        return response.json()
      })
      .catch(() => [])

    Promise.all([componentReportPromise, triagesPromise])
      .then(([componentReport, triages]) => {
        if (!componentReport || !componentReport.rows) {
          setIsLoaded(true)
          return
        }

        // Extract regressed tests from component report
        const regressionIds = new Set()
        triages.forEach((tr) => {
          tr.regressions?.forEach((regression) => {
            regressionIds.add(regression.id)
          })
        })

        let untriagedCount = 0
        let totalCount = 0
        let unresolvedCount = 0
        const allRegressedTests = []
        const twentyFourHoursAgo = new Date(Date.now() - 24 * 60 * 60 * 1000)

        componentReport.rows.forEach((row) => {
          row.columns.forEach((column) => {
            const regressed = column.regressed_tests
            if (regressed && regressed.length > 0) {
              regressed.forEach((r) => {
                totalCount++
                const regressedTest = {
                  ...r,
                  component: row.component,
                  capability: row.capability,
                }
                allRegressedTests.push(regressedTest)

                if (!regressionIds.has(r.regression?.id)) {
                  untriagedCount++
                }
                if (r.status <= -200) {
                  unresolvedCount++
                }
              })
            }
          })
        })

        // Find regressions opened in last 24 hours
        const recentRegressions = allRegressedTests
          .filter((r) => {
            if (r.regression?.opened) {
              const openedDate = new Date(r.regression.opened)
              return openedDate >= twentyFourHoursAgo
            }
            return false
          })
          .sort((a, b) => {
            const dateA = new Date(a.regression.opened)
            const dateB = new Date(b.regression.opened)
            return dateB - dateA // Most recent first
          })
          .slice(0, 10) // Limit to 10 most recent

        setRegressions({
          total: totalCount,
          untriaged: untriagedCount,
          unresolved: unresolvedCount,
          recent: recentRegressions,
        })
        setIsLoaded(true)
      })
      .catch(() => {
        setIsLoaded(true)
      })
  }, [release])

  const getSeverityColor = (count, thresholds) => {
    if (count <= thresholds.success) return theme.palette.success.main
    if (count <= thresholds.warning) return theme.palette.warning.main
    return theme.palette.error.main
  }

  const unresolvedColor =
    regressions && isLoaded
      ? getSeverityColor(regressions.unresolved, COMPONENT_READINESS_THRESHOLDS)
      : theme.palette.text.secondary

  return (
    <Grid item xs={12}>
      <Card elevation={5} sx={{ p: 2.5 }}>
        {!isLoaded ? (
          <CenteredMessageBox>
            <Typography>Loading component readiness data...</Typography>
          </CenteredMessageBox>
        ) : !regressions ? (
          <CenteredMessageBox>
            <Typography>No component readiness data available</Typography>
          </CenteredMessageBox>
        ) : (
          <Grid container spacing={3}>
            {/* Left Column - Stats */}
            <Grid item xs={12} md={6}>
              <StatsHeaderBox>
                <Typography variant="h6">Component Readiness</Typography>
                <Button
                  component={Link}
                  to={`/component_readiness/main?view=${release}-main`}
                  variant="contained"
                  color="primary"
                  size="small"
                  endIcon={<ArrowForwardIcon />}
                >
                  View Full Report
                </Button>
              </StatsHeaderBox>
              <StatsColumnBox>
                <StatCardBox>
                  <WarningIcon sx={{ mr: 2, color: unresolvedColor }} />
                  <Box>
                    <Typography variant="h3" sx={{ color: unresolvedColor }}>
                      {regressions.unresolved}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      Unresolved Regressions
                    </Typography>
                  </Box>
                </StatCardBox>

                <StatCardBox>
                  <HealingIcon sx={{ mr: 2, color: 'warning.main' }} />
                  <Box>
                    <Typography variant="h3" color="warning.main">
                      {regressions.untriaged}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      Untriaged Regressions
                    </Typography>
                  </Box>
                </StatCardBox>
              </StatsColumnBox>
            </Grid>

            {/* Right Column - Recent Regressions */}
            <Grid item xs={12} md={6}>
              <SectionHeaderBox>
                <Typography variant="h6">
                  New Regressions ({regressions.recent.length})
                </Typography>
                <Tooltip title="Regressions opened in the last 24 hours">
                  <InfoIcon sx={{ ml: 1 }} color="action" />
                </Tooltip>
              </SectionHeaderBox>
              {regressions.recent.length === 0 ? (
                <CenteredMessageBox sx={{ p: 3 }}>
                  <Typography>
                    No new regressions in the last 24 hours
                  </Typography>
                </CenteredMessageBox>
              ) : (
                <RegressionList>
                  {regressions.recent.map((regression, index) => {
                    // Get the test details URL from the regression links
                    const viewName = `${release}-main`
                    const rawUrl = getTestDetailsLink(
                      regression.links,
                      viewName
                    )
                    const testDetailsUrl = rawUrl
                      ? convertApiUrlToUiUrl(rawUrl)
                      : null

                    return (
                      <React.Fragment key={index}>
                        <ListItemButton
                          alignItems="flex-start"
                          component={testDetailsUrl ? 'a' : 'div'}
                          href={testDetailsUrl || undefined}
                          disabled={!testDetailsUrl}
                        >
                          <ListItemText
                            primary={
                              <TruncatedTestName variant="body2">
                                {regression.test_name}
                              </TruncatedTestName>
                            }
                            secondaryTypographyProps={{ component: 'div' }}
                            secondary={
                              <Box>
                                <Box>
                                  <Typography
                                    component="span"
                                    variant="caption"
                                    color="text.secondary"
                                  >
                                    {regression.component} ›{' '}
                                    {regression.capability}
                                  </Typography>
                                  <Typography
                                    component="span"
                                    variant="caption"
                                    sx={{ fontStyle: 'italic' }}
                                  >
                                    {' '}
                                    {relativeTime(
                                      new Date(regression.regression.opened),
                                      new Date()
                                    )}
                                  </Typography>
                                </Box>
                                {regression.regression?.variants &&
                                  regression.regression.variants.length > 0 && (
                                    <VariantChipsBox>
                                      {[...regression.regression.variants]
                                        .sort()
                                        .map((variant, vIndex) => (
                                          <Chip
                                            key={vIndex}
                                            label={variant}
                                            size="small"
                                            sx={{
                                              height: 16,
                                              fontSize: '0.65rem',
                                            }}
                                          />
                                        ))}
                                    </VariantChipsBox>
                                  )}
                              </Box>
                            }
                          />
                        </ListItemButton>
                        {index < regressions.recent.length - 1 && (
                          <Divider component="li" />
                        )}
                      </React.Fragment>
                    )
                  })}
                </RegressionList>
              )}
            </Grid>
          </Grid>
        )}
      </Card>
    </Grid>
  )
}

ComponentReadinessIndicator.propTypes = {
  release: PropTypes.string.isRequired,
}
