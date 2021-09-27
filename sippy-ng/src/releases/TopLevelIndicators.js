import { Box, Tooltip, Typography } from '@material-ui/core'
import {
  INFRASTRUCTURE_THRESHOLDS,
  INSTALL_THRESHOLDS,
  TEST_THRESHOLDS,
  UPGRADE_THRESHOLDS,
} from '../constants'
import { TOOLTIP } from './ReleaseOverview'
import Grid from '@material-ui/core/Grid'
import InfoIcon from '@material-ui/icons/Info'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SummaryCard from '../components/SummaryCard'

export default function TopLevelIndicators(props) {
  const TOOLTIP = 'Top level release indicators showing product health'

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
  ;['infrastructure', 'install', 'tests', 'upgrade'].forEach((indicator) => {
    if (
      props.indicators[indicator].current.runs !== 0 ||
      props.indicators[indicator].previous.runs !== 0
    ) {
      noData = false
    }
  })
  if (noData) {
    return <></>
  }

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

      <Grid item md={3} sm={6}>
        <SummaryCard
          key="infrastructure-summary"
          threshold={INFRASTRUCTURE_THRESHOLDS}
          name="Infrastructure"
          link={
            '/tests/' +
            props.release +
            '/details?test=[sig-sippy] infrastructure should work'
          }
          success={props.indicators.infrastructure.current.percentage}
          fail={100 - props.indicators.infrastructure.current.percentage}
          caption={indicatorCaption(props.indicators.infrastructure)}
          tooltip="How often we get to the point of running the installer. This is judged by whether a kube-apiserver is available, it's not perfect, but it's very close."
        />
      </Grid>

      <Grid item md={3} sm={6}>
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
      <Grid item md={3} sm={6}>
        <SummaryCard
          key="upgrade-summary"
          threshold={UPGRADE_THRESHOLDS}
          name="Upgrade"
          link={'/upgrade/' + props.release}
          success={props.indicators.upgrade.current.percentage}
          fail={100 - props.indicators.upgrade.current.percentage}
          caption={indicatorCaption(props.indicators.upgrade)}
          tooltip="How often an upgrade that is started completes successfully."
        />
      </Grid>

      <Grid item md={3} sm={6}>
        <SummaryCard
          key="test-summary"
          threshold={TEST_THRESHOLDS}
          link={
            '/tests/' +
            props.release +
            '/details?test=[sig-sippy] openshift-tests should work'
          }
          name="Tests"
          success={props.indicators.tests.current.percentage}
          fail={100 - props.indicators.tests.current.percentage}
          caption={indicatorCaption(props.indicators.tests)}
          tooltip={
            'How often e2e tests complete successfully. Sippy tries to figure out which runs ran an e2e test ' +
            'suite, and then determine which failed. A low pass rate could be due to any number of temporary ' +
            'problems, most of the utility from this noisy metric is monitoring changes over time.'
          }
        />
      </Grid>
    </Fragment>
  )
}

TopLevelIndicators.propTypes = {
  release: PropTypes.string,
  indicators: PropTypes.object,
}
