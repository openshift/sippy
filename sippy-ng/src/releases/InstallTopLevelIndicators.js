import {
  BOOTSTRAP_THRESHOLDS,
  INFRASTRUCTURE_THRESHOLDS,
  INSTALL_CONFIG_THRESHOLDS,
  INSTALL_OTHER_THRESHOLDS,
  INSTALL_THRESHOLDS,
} from '../constants'
import { Box } from '@mui/material'
import { pathForTestByVariant, useNewInstallTests } from '../helpers'
import Grid from '@mui/material/Grid'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SummaryCard from '../components/SummaryCard'

export default function TopLevelIndicators(props) {
  const TOOLTIP = 'Top level install indicators showing install health'

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
  ;[
    'infrastructure',
    'installConfig',
    'installOther',
    'bootstrap',
    'install',
  ].forEach((indicator) => {
    if (
      props.indicators[indicator].current_runs !== 0 ||
      props.indicators[indicator].previous_runs !== 0
    ) {
      noData = false
    }
  })

  let newInstall = useNewInstallTests(props.release)
  if (noData || !newInstall) {
    return <></>
  }

  return (
    <Fragment>
      <Grid item md={2} sm={4}>
        <SummaryCard
          key="infrastructure-summary"
          threshold={INFRASTRUCTURE_THRESHOLDS}
          name="Infrastructure"
          link={pathForTestByVariant(
            props.release,
            'install should succeed: infrastructure'
          )}
          success={props.indicators.infrastructure.current_pass_percentage}
          flakes={props.indicators.infrastructure.current_flake_percentage}
          fail={props.indicators.infrastructure.current_failure_percentage}
          caption={indicatorCaption(props.indicators.infrastructure)}
          tooltip="How often install fails due to infrastructure failures."
        />
      </Grid>

      <Grid item md={2} sm={4}>
        <SummaryCard
          key="install-config-summary"
          threshold={INSTALL_CONFIG_THRESHOLDS}
          name="Install-Config"
          link={pathForTestByVariant(
            props.release,
            'install should succeed: configuration'
          )}
          success={props.indicators.installConfig.current_pass_percentage}
          flakes={props.indicators.installConfig.current_flake_percentage}
          fail={props.indicators.installConfig.current_failure_percentage}
          caption={indicatorCaption(props.indicators.installConfig)}
          tooltip="How often the install configuration check completes successfully."
        />
      </Grid>

      <Grid item md={2} sm={4}>
        <SummaryCard
          key="bootstrap-summary"
          threshold={BOOTSTRAP_THRESHOLDS}
          name="Bootstrap"
          link={pathForTestByVariant(
            props.release,
            'install should succeed: cluster bootstrap'
          )}
          success={props.indicators.bootstrap.current_pass_percentage}
          flakes={props.indicators.bootstrap.current_flake_percentage}
          fail={props.indicators.bootstrap.current_failure_percentage}
          caption={indicatorCaption(props.indicators.bootstrap)}
          tooltip="How often bootstrap completes successfully."
        />
      </Grid>

      <Grid item md={2} sm={4}>
        <SummaryCard
          key="install-other"
          threshold={INSTALL_OTHER_THRESHOLDS}
          name="Install Other"
          link={pathForTestByVariant(
            props.release,
            'install should succeed: other'
          )}
          success={props.indicators.installOther.current_pass_percentage}
          flakes={props.indicators.installOther.current_flake_percentage}
          fail={props.indicators.installOther.current_failure_percentage}
          caption={indicatorCaption(props.indicators.installOther)}
          tooltip="How often install fails because other reasons."
        />
      </Grid>

      <Grid item md={2} sm={4}>
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
    </Fragment>
  )
}

TopLevelIndicators.propTypes = {
  release: PropTypes.string,
  indicators: PropTypes.object,
}
