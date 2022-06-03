import {
  BOOTSTRAP_THRESHOLDS,
  INFRASTRUCTURE_THRESHOLDS,
  INSTALL_CONFIG_THRESHOLDS,
  INSTALL_OTHER_THRESHOLDS,
  INSTALL_THRESHOLDS,
} from '../constants'
import { Box, Tooltip, Typography } from '@material-ui/core'
import { pathForTestByVariant, useNewInstallTests } from '../helpers'
import Grid from '@material-ui/core/Grid'
import InfoIcon from '@material-ui/icons/Info'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SummaryCard from '../components/SummaryCard'

export default function TopLevelIndicators(props) {
  const TOOLTIP = 'Top level install indicators showing install health'

  const indicatorCaption = (indicator) => {
    return (
      <Box component="h3">
        {indicator.current.percentage.toFixed(0)}% ({indicator.current.runs}{' '}
        runs)
        <br />
        <PassRateIcon
          improvement={
            indicator.current.percentage - indicator.previous.percentage
          }
        />
        <br />
        {indicator.previous.percentage.toFixed(0)}% ({indicator.previous.runs}{' '}
        runs)
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
      props.indicators[indicator].current.runs !== 0 ||
      props.indicators[indicator].previous.runs !== 0
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
      <Grid item md={12} sm={12} style={{ display: 'flex' }}>
        <Typography variant="h5">
          Top Level Install Indicators
          <Tooltip title={TOOLTIP}>
            <InfoIcon />
          </Tooltip>
        </Typography>
      </Grid>

      <Grid item md={2} sm={4}>
        <SummaryCard
          key="infrastructure-summary"
          threshold={INFRASTRUCTURE_THRESHOLDS}
          name="Infrastructure"
          link={pathForTestByVariant(
            props.release,
            'cluster install.install should succeed: infrastructure'
          )}
          success={props.indicators.infrastructure.current.percentage}
          fail={100 - props.indicators.infrastructure.current.percentage}
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
            'cluster install.install should succeed: configuration'
          )}
          success={props.indicators.installConfig.current.percentage}
          fail={100 - props.indicators.installConfig.current.percentage}
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
            'cluster install.install should succeed: cluster bootstrap'
          )}
          success={props.indicators.bootstrap.current.percentage}
          fail={100 - props.indicators.bootstrap.current.percentage}
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
            'cluster install.install should succeed: other'
          )}
          success={props.indicators.installOther.current.percentage}
          fail={100 - props.indicators.installOther.current.percentage}
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
          success={props.indicators.install.current.percentage}
          fail={100 - props.indicators.install.current.percentage}
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
