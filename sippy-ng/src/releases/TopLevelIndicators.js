import { Box, Card, Tooltip, Typography, useTheme } from '@mui/material'
import {
  INFRASTRUCTURE_THRESHOLDS,
  INSTALL_THRESHOLDS,
  TEST_THRESHOLDS,
  UPGRADE_THRESHOLDS,
} from '../constants'
import { Link } from 'react-router-dom'
import { makeStyles } from '@mui/styles'
import { pathForTestByVariant, useNewInstallTests } from '../helpers'
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown'
import ArrowDropUpIcon from '@mui/icons-material/ArrowDropUp'
import CloudIcon from '@mui/icons-material/Cloud'
import ComponentReadinessIndicator from '../component_readiness/ComponentReadinessIndicator'
import Grid from '@mui/material/Grid'
import InstallDesktopIcon from '@mui/icons-material/InstallDesktop'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import UpgradeIcon from '@mui/icons-material/Upgrade'
import VerifiedIcon from '@mui/icons-material/Verified'

const useStyles = makeStyles((theme) => ({
  indicatorCard: {
    padding: theme.spacing(2.5),
    height: '100%',
    transition: 'transform 0.2s, box-shadow 0.2s',
    '&:hover': {
      transform: 'translateY(-4px)',
      boxShadow: theme.shadows[8],
    },
  },
  cardLink: {
    textDecoration: 'none',
    display: 'block',
  },
  stackedBar: {
    display: 'flex',
    height: 8,
    borderRadius: theme.spacing(1),
    overflow: 'hidden',
    backgroundColor: theme.palette.divider,
  },
}))

export default function TopLevelIndicators(props) {
  const theme = useTheme()
  const classes = useStyles()

  const getIndicatorColor = (passPercentage, threshold) => {
    if (passPercentage >= threshold.success) return theme.palette.success.main
    if (passPercentage >= threshold.warning) return theme.palette.warning.main
    return theme.palette.error.main
  }

  const renderIndicatorCard = (
    name,
    icon,
    indicator,
    threshold,
    link,
    tooltip
  ) => {
    const color = getIndicatorColor(
      indicator.current_working_percentage,
      threshold
    )
    const improvement = indicator.net_working_improvement

    const fullTooltip = (
      <Box>
        <Typography variant="body2" sx={{ mb: 1 }}>
          {tooltip}
        </Typography>
        <Typography variant="body2">
          <strong>Pass:</strong> {indicator.current_pass_percentage.toFixed(0)}%
        </Typography>
        <Typography variant="body2">
          <strong>Flake:</strong>{' '}
          {indicator.current_flake_percentage.toFixed(0)}%
        </Typography>
        <Typography variant="body2">
          <strong>Fail:</strong>{' '}
          {indicator.current_failure_percentage.toFixed(0)}%
        </Typography>
      </Box>
    )

    return (
      <Grid item md={3} sm={6}>
        <Tooltip title={fullTooltip}>
          <Box component={Link} to={link} className={classes.cardLink}>
            <Card elevation={5} className={classes.indicatorCard}>
              <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
                {icon}
                <Typography variant="h6" sx={{ ml: 1 }}>
                  {name}
                </Typography>
              </Box>

              <Box sx={{ textAlign: 'center', mb: 2 }}>
                <Typography variant="h2" sx={{ color, fontWeight: 'bold' }}>
                  {indicator.current_working_percentage.toFixed(0)}%
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {indicator.current_runs} runs
                </Typography>
              </Box>

              {/* Stacked bar showing pass/flake/fail breakdown */}
              <Box sx={{ mb: 2, px: 1 }}>
                <Box className={classes.stackedBar}>
                  <Box
                    sx={{
                      width: `${indicator.current_pass_percentage}%`,
                      backgroundColor: theme.palette.success.main,
                    }}
                  />
                  <Box
                    sx={{
                      width: `${indicator.current_flake_percentage}%`,
                      backgroundColor: theme.palette.warning.main,
                    }}
                  />
                  <Box
                    sx={{
                      width: `${indicator.current_failure_percentage}%`,
                      backgroundColor: theme.palette.error.main,
                    }}
                  />
                </Box>
              </Box>

              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: 1,
                }}
              >
                {improvement > 0 ? (
                  <ArrowDropUpIcon sx={{ color: theme.palette.success.main }} />
                ) : improvement < 0 ? (
                  <ArrowDropDownIcon sx={{ color: theme.palette.error.main }} />
                ) : null}
                <Typography
                  variant="body2"
                  sx={{
                    color:
                      improvement > 0
                        ? theme.palette.success.main
                        : improvement < 0
                        ? theme.palette.error.main
                        : theme.palette.text.secondary,
                    fontWeight: 500,
                  }}
                >
                  {improvement > 0 ? '+' : ''}
                  {improvement.toFixed(1)}%
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  vs prev ({indicator.previous_working_percentage.toFixed(0)}%)
                </Typography>
              </Box>
            </Card>
          </Box>
        </Tooltip>
      </Grid>
    )
  }

  // Hide this if there's no data
  let noData = true
  ;['infrastructure', 'install', 'tests', 'upgrade'].forEach((indicator) => {
    let ind = props.indicators[indicator]
    if (ind && (ind.current_runs !== 0 || ind.previous_runs !== 0)) {
      noData = false
    }
  })
  if (noData) {
    return <></>
  }
  let newInstall = useNewInstallTests(props.release)

  const hasComponentReadiness =
    props.releases?.release_attrs?.[props.release]?.capabilities
      ?.componentReadiness

  return (
    <Fragment>
      {props.indicators.infrastructure &&
        renderIndicatorCard(
          'Infrastructure',
          <CloudIcon sx={{ fontSize: 28, color: 'text.secondary' }} />,
          props.indicators.infrastructure,
          INFRASTRUCTURE_THRESHOLDS,
          pathForTestByVariant(
            props.release,
            newInstall
              ? 'install should succeed: infrastructure'
              : '[sig-sippy] infrastructure should work'
          ),
          newInstall
            ? 'How often install fails due to infrastructure failures.'
            : "How often we get to the point of running the installer. This is judged by whether a kube-apiserver is available, it's not perfect, but it's very close."
        )}

      {props.indicators.install &&
        renderIndicatorCard(
          'Install',
          <InstallDesktopIcon sx={{ fontSize: 28, color: 'text.secondary' }} />,
          props.indicators.install,
          INSTALL_THRESHOLDS,
          '/install/' + props.release,
          'How often the install completes successfully.'
        )}

      {props.indicators.upgrade &&
        renderIndicatorCard(
          'Upgrade',
          <UpgradeIcon sx={{ fontSize: 28, color: 'text.secondary' }} />,
          props.indicators.upgrade,
          UPGRADE_THRESHOLDS,
          '/upgrade/' + props.release,
          'How often an upgrade that is started completes successfully.'
        )}

      {props.indicators.tests &&
        renderIndicatorCard(
          'Tests',
          <VerifiedIcon sx={{ fontSize: 28, color: 'text.secondary' }} />,
          props.indicators.tests,
          TEST_THRESHOLDS,
          pathForTestByVariant(
            props.release,
            '[sig-sippy] openshift-tests should work'
          ),
          'How often e2e tests complete successfully. Sippy tries to figure out which runs ran an e2e test ' +
            'suite, and then determine which failed. A low pass rate could be due to any number of temporary ' +
            'problems, most of the utility from this noisy metric is monitoring changes over time.'
        )}

      {hasComponentReadiness && (
        <ComponentReadinessIndicator release={props.release} />
      )}
    </Fragment>
  )
}

TopLevelIndicators.propTypes = {
  release: PropTypes.string,
  indicators: PropTypes.object,
  releases: PropTypes.object,
}
