import {
  BOOTSTRAP_THRESHOLDS,
  INFRASTRUCTURE_THRESHOLDS,
  INSTALL_CONFIG_THRESHOLDS,
  INSTALL_OTHER_THRESHOLDS,
  INSTALL_THRESHOLDS,
  TEST_THRESHOLDS,
  UPGRADE_THRESHOLDS,
} from '../constants'
import { Box, Tooltip, Typography } from '@material-ui/core'
import { TOOLTIP } from './ReleaseOverview'
import Grid from '@material-ui/core/Grid'
import InfoIcon from '@material-ui/icons/Info'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SummaryCard from '../components/SummaryCard'

function useNewInstallTests(release) {
  let digits = release.split('.', 2)
  if (digits.length < 2) {
    return false
  }
  const major = parseInt(digits[0])
  const minor = parseInt(digits[1])
  if (isNaN(major) || isNaN(minor)) {
    return false
  }
  if (major < 4) {
    return false
  } else if (major == 4 && minor < 11) {
    return false
  }
  return true
}

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

      {newInstall ? (
        <Grid item md={3} sm={6}>
          <SummaryCard
            key="infrastructure-summary"
            threshold={INFRASTRUCTURE_THRESHOLDS}
            name="Infrastructure"
            link={
              '/tests/' +
              props.release +
              '/details?test=cluster install.install should succeed: infrastructure'
            }
            success={props.indicators.infrastructure.current.percentage}
            fail={100 - props.indicators.infrastructure.current.percentage}
            caption={indicatorCaption(props.indicators.infrastructure)}
            tooltip="How often install fails due to infrastructure failures."
          />
        </Grid>
      ) : (
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
      )}

      {newInstall && (
        <Grid item md={3} sm={6}>
          <SummaryCard
            key="install-config-summary"
            threshold={INSTALL_CONFIG_THRESHOLDS}
            name="Install-Configuration"
            link={
              '/tests/' +
              props.release +
              '/details?test=cluster install.install should succeed: configuration'
            }
            success={props.indicators.installConfig.current.percentage}
            fail={100 - props.indicators.installConfig.current.percentage}
            caption={indicatorCaption(props.indicators.installConfig)}
            tooltip="How often the install configuration check completes successfully."
          />
        </Grid>
      )}

      {newInstall && (
        <Grid item md={3} sm={6}>
          <SummaryCard
            key="bootstrap-summary"
            threshold={BOOTSTRAP_THRESHOLDS}
            name="Bootstrap"
            link={
              '/tests/' +
              props.release +
              '/details?test=cluster install.install should succeed: cluster bootstrap'
            }
            success={props.indicators.bootstrap.current.percentage}
            fail={100 - props.indicators.bootstrap.current.percentage}
            caption={indicatorCaption(props.indicators.bootstrap)}
            tooltip="How often bootstrap completes successfully."
          />
        </Grid>
      )}

      {newInstall && (
        <Grid item md={3} sm={6}>
          <SummaryCard
            key="install-other"
            threshold={INSTALL_OTHER_THRESHOLDS}
            name="Install Other"
            link={
              '/tests/' +
              props.release +
              '/details?test=cluster install.install should succeed: other'
            }
            success={props.indicators.installOther.current.percentage}
            fail={100 - props.indicators.installOther.current.percentage}
            caption={indicatorCaption(props.indicators.installOther)}
            tooltip="How often install fails because other reasons."
          />
        </Grid>
      )}

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
