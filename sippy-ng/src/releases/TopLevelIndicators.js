import { Box, Tooltip, Typography } from '@mui/material'
import {
  INFRASTRUCTURE_THRESHOLDS,
  INSTALL_THRESHOLDS,
  TEST_THRESHOLDS,
  UPGRADE_THRESHOLDS,
} from '../constants'
import { pathForTestByVariant, useNewInstallTests } from '../helpers'
import Grid from '@mui/material/Grid'
import InfoIcon from '@mui/icons-material/Info'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SummaryCard from '../components/SummaryCard'

export default function TopLevelIndicators(props) {
  const TOOLTIP = 'Top level release indicators showing product health'

  const indicatorCaption = (indicator) => {
    return (
      <Box component="h3">
        {indicator.current_working_percentage.toFixed(0)}% (
        {indicator.current_runs} runs)
        <br />
        <PassRateIcon improvement={indicator.net_working_improvement} />
        <br />
        {indicator.previous_working_percentage.toFixed(0)}% (
        {indicator.previous_runs} runs)
      </Box>
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

  return (
    <Fragment>
      <Grid item md={12} sm={12} style={{ display: 'flex' }}>
        <Typography variant="h5">
          Top Level Release Indicators
          <Tooltip title={TOOLTIP}>
            <InfoIcon />
          </Tooltip>
        </Typography>
      </Grid>

      {props.indicators.infrastructure && (
        <Grid item md={3} sm={6}>
          <SummaryCard
            key="infrastructure-summary"
            threshold={INFRASTRUCTURE_THRESHOLDS}
            name="Infrastructure"
            link={pathForTestByVariant(
              props.release,
              newInstall
                ? 'install should succeed: infrastructure'
                : '[sig-sippy] infrastructure should work'
            )}
            success={props.indicators.infrastructure.current_pass_percentage}
            flakes={props.indicators.infrastructure.current_flake_percentage}
            fail={props.indicators.infrastructure.current_failure_percentage}
            caption={indicatorCaption(props.indicators.infrastructure)}
            tooltip={
              newInstall
                ? 'How often install fails due to infrastructure failures.'
                : "How often we get to the point of running the installer. This is judged by whether a kube-apiserver is available, it's not perfect, but it's very close."
            }
          />
        </Grid>
      )}

      {props.indicators.install && (
        <Grid item md={3} sm={6}>
          <SummaryCard
            key="install-summary"
            threshold={INSTALL_THRESHOLDS}
            name="Install"
            link={'/install/' + props.release}
            success={props.indicators.install.current_pass_percentage}
            flakes={props.indicators.install.current_flake_percentage}
            fail={props.indicators.install.current_failure_percentage}
            caption={indicatorCaption(props.indicators.install)}
            tooltip="How often the install completes successfully."
          />
        </Grid>
      )}

      {props.indicators.upgrade && (
        <Grid item md={3} sm={6}>
          <SummaryCard
            key="upgrade-summary"
            threshold={UPGRADE_THRESHOLDS}
            name="Upgrade"
            link={'/upgrade/' + props.release}
            success={props.indicators.upgrade.current_pass_percentage}
            flakes={props.indicators.upgrade.current_flake_percentage}
            fail={props.indicators.upgrade.current_failure_percentage}
            caption={indicatorCaption(props.indicators.upgrade)}
            tooltip="How often an upgrade that is started completes successfully."
          />
        </Grid>
      )}

      {props.indicators.tests && (
        <Grid item md={3} sm={6}>
          <SummaryCard
            key="test-summary"
            threshold={TEST_THRESHOLDS}
            link={pathForTestByVariant(
              props.release,
              '[sig-sippy] openshift-tests should work'
            )}
            name="Tests"
            success={props.indicators.tests.current_pass_percentage}
            flakes={props.indicators.tests.current_flake_percentage}
            fail={props.indicators.tests.current_failure_percentage}
            caption={indicatorCaption(props.indicators.tests)}
            tooltip={
              'How often e2e tests complete successfully. Sippy tries to figure out which runs ran an e2e test ' +
              'suite, and then determine which failed. A low pass rate could be due to any number of temporary ' +
              'problems, most of the utility from this noisy metric is monitoring changes over time.'
            }
          />
        </Grid>
      )}
    </Fragment>
  )
}

TopLevelIndicators.propTypes = {
  release: PropTypes.string,
  indicators: PropTypes.object,
}
