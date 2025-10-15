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
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { relativeTime } from '../helpers'
import ArrowForwardIcon from '@mui/icons-material/ArrowForward'
import Grid from '@mui/material/Grid'
import HealingIcon from '@mui/icons-material/Healing'
import InfoIcon from '@mui/icons-material/Info'
import PropTypes from 'prop-types'
import React, { useEffect, useState } from 'react'
import Tooltip from '@mui/material/Tooltip'
import WarningIcon from '@mui/icons-material/Warning'

const useStyles = makeStyles((theme) => ({
  testName: {
    fontWeight: 500,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    display: '-webkit-box',
    WebkitLineClamp: 2,
    WebkitBoxOrient: 'vertical',
  },
  variantChip: {
    height: 16,
    fontSize: '0.65rem',
  },
  statBox: {
    display: 'flex',
    alignItems: 'center',
    padding: theme.spacing(2),
    borderRadius: theme.spacing(1),
    backgroundColor: theme.palette.action.hover,
  },
  regressionsList: {
    maxHeight: 240,
    overflow: 'auto',
    backgroundColor: theme.palette.action.hover,
    borderRadius: theme.spacing(1),
  },
}))

export default function ComponentReadinessIndicator({ release }) {
  const theme = useTheme()
  const classes = useStyles()
  const [regressions, setRegressions] = useState(null)
  const [isLoaded, setIsLoaded] = useState(false)

  useEffect(() => {
    const viewName = `${release}-main`
    const componentReportUrl =
      process.env.REACT_APP_API_URL +
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

    const triagesPromise = fetch(
      process.env.REACT_APP_API_URL + '/api/component_readiness/triages'
    )
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
          <Box sx={{ p: 4, textAlign: 'center', color: 'text.secondary' }}>
            <Typography>Loading component readiness data...</Typography>
          </Box>
        ) : !regressions ? (
          <Box sx={{ p: 4, textAlign: 'center', color: 'text.secondary' }}>
            <Typography>No component readiness data available</Typography>
          </Box>
        ) : (
          <Grid container spacing={3}>
            {/* Left Column - Stats */}
            <Grid item xs={12} md={6}>
              <Box
                sx={{
                  mb: 2,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                }}
              >
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
              </Box>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Box className={classes.statBox}>
                  <WarningIcon sx={{ mr: 2, color: unresolvedColor }} />
                  <Box>
                    <Typography variant="h3" sx={{ color: unresolvedColor }}>
                      {regressions.unresolved}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      Unresolved Regressions
                    </Typography>
                  </Box>
                </Box>

                <Box className={classes.statBox}>
                  <HealingIcon sx={{ mr: 2, color: 'warning.main' }} />
                  <Box>
                    <Typography variant="h3" color="warning.main">
                      {regressions.untriaged}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      Untriaged Regressions
                    </Typography>
                  </Box>
                </Box>
              </Box>
            </Grid>

            {/* Right Column - Recent Regressions */}
            <Grid item xs={12} md={6}>
              <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
                <Typography variant="h6">
                  New Regressions ({regressions.recent.length})
                </Typography>
                <Tooltip title="Regressions opened in the last 24 hours">
                  <InfoIcon sx={{ ml: 1 }} color="action" />
                </Tooltip>
              </Box>
              {regressions.recent.length === 0 ? (
                <Box
                  sx={{ p: 3, textAlign: 'center', color: 'text.secondary' }}
                >
                  <Typography>
                    No new regressions in the last 24 hours
                  </Typography>
                </Box>
              ) : (
                <List className={classes.regressionsList}>
                  {regressions.recent.map((regression, index) => {
                    // Get the test details URL from the regression links
                    let testDetailsUrl = null
                    if (regression.links?.test_details) {
                      // Convert API URL to UI URL
                      const apiIndex =
                        regression.links.test_details.indexOf('/api/')
                      if (apiIndex !== -1) {
                        const pathAfterApi =
                          regression.links.test_details.substring(apiIndex + 5)
                        testDetailsUrl = '/' + pathAfterApi
                      }
                    }

                    return (
                      <React.Fragment key={index}>
                        <ListItemButton
                          alignItems="flex-start"
                          component={testDetailsUrl ? Link : 'div'}
                          to={testDetailsUrl || undefined}
                          disabled={!testDetailsUrl}
                        >
                          <ListItemText
                            primary={
                              <Typography
                                variant="body2"
                                className={classes.testName}
                              >
                                {regression.test_name}
                              </Typography>
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
                                    <Box
                                      sx={{
                                        display: 'flex',
                                        flexWrap: 'wrap',
                                        gap: 0.5,
                                      }}
                                    >
                                      {[...regression.regression.variants]
                                        .sort()
                                        .map((variant, vIndex) => (
                                          <Chip
                                            key={vIndex}
                                            label={variant}
                                            size="small"
                                            className={classes.variantChip}
                                          />
                                        ))}
                                    </Box>
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
                </List>
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
