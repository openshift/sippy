import {
  BOOTSTRAP_THRESHOLDS,
  INFRASTRUCTURE_THRESHOLDS,
  INSTALL_CONFIG_THRESHOLDS,
  INSTALL_OTHER_THRESHOLDS,
  INSTALL_THRESHOLDS,
} from '../constants'
import { Box } from '@mui/material'
import {
  pathForTestByVariant,
  productLabelFor,
  useNewInstallTests,
} from '../helpers'
import Grid from '@mui/material/Grid'
import PassRateIcon from '../components/PassRateIcon'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import SummaryCard from '../components/SummaryCard'

export default function TopLevelIndicators(props) {
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
    'productInstall',
  ].forEach((indicator) => {
    let ind = props.indicators[indicator]
    if (ind && (ind.current_runs !== 0 || ind.previous_runs !== 0)) {
      noData = false
    }
  })

  let newInstall = useNewInstallTests(props.release)
  const isProduct = Boolean(props.links)

  if (noData || (!newInstall && !isProduct)) {
    return <></>
  }

  const productLabel = productLabelFor(props.release)

  const infrastructureLink = pathForTestByVariant(
    props.release,
    'install should succeed: infrastructure'
  )
  const installConfigLink = pathForTestByVariant(
    props.release,
    'install should succeed: configuration'
  )
  const bootstrapLink = pathForTestByVariant(
    props.release,
    'install should succeed: cluster bootstrap'
  )
  const installOtherLink = pathForTestByVariant(
    props.release,
    'install should succeed: other'
  )

  return (
    <Fragment>
      {props.indicators.infrastructure && (
        <Grid item md={2} sm={4}>
          <SummaryCard
            key="infrastructure-summary"
            threshold={INFRASTRUCTURE_THRESHOLDS}
            name="Infrastructure"
            link={infrastructureLink}
            success={props.indicators.infrastructure.current_pass_percentage}
            flakes={props.indicators.infrastructure.current_flake_percentage}
            fail={props.indicators.infrastructure.current_failure_percentage}
            caption={indicatorCaption(props.indicators.infrastructure)}
            tooltip="How often install fails due to infrastructure failures."
          />
        </Grid>
      )}

      {props.indicators.installConfig && (
        <Grid item md={2} sm={4}>
          <SummaryCard
            key="install-config-summary"
            threshold={INSTALL_CONFIG_THRESHOLDS}
            name="Install-Config"
            link={installConfigLink}
            success={props.indicators.installConfig.current_pass_percentage}
            flakes={props.indicators.installConfig.current_flake_percentage}
            fail={props.indicators.installConfig.current_failure_percentage}
            caption={indicatorCaption(props.indicators.installConfig)}
            tooltip="How often the install configuration check completes successfully."
          />
        </Grid>
      )}

      {props.indicators.bootstrap && (
        <Grid item md={2} sm={4}>
          <SummaryCard
            key="bootstrap-summary"
            threshold={BOOTSTRAP_THRESHOLDS}
            name="Bootstrap"
            link={bootstrapLink}
            success={props.indicators.bootstrap.current_pass_percentage}
            flakes={props.indicators.bootstrap.current_flake_percentage}
            fail={props.indicators.bootstrap.current_failure_percentage}
            caption={indicatorCaption(props.indicators.bootstrap)}
            tooltip="How often bootstrap completes successfully."
          />
        </Grid>
      )}

      {props.indicators.installOther && (
        <Grid item md={2} sm={4}>
          <SummaryCard
            key="install-other"
            threshold={INSTALL_OTHER_THRESHOLDS}
            name="Install Other"
            link={installOtherLink}
            success={props.indicators.installOther.current_pass_percentage}
            flakes={props.indicators.installOther.current_flake_percentage}
            fail={props.indicators.installOther.current_failure_percentage}
            caption={indicatorCaption(props.indicators.installOther)}
            tooltip="How often install fails because other reasons."
          />
        </Grid>
      )}

      {props.indicators.install && (
        <Grid item md={2} sm={4}>
          <SummaryCard
            key="install-summary"
            threshold={INSTALL_THRESHOLDS}
            name={isProduct ? 'OpenShift Install' : 'Install'}
            link={'/install/' + props.release}
            success={props.indicators.install.current_pass_percentage}
            flakes={props.indicators.install.current_flake_percentage}
            fail={props.indicators.install.current_failure_percentage}
            caption={indicatorCaption(props.indicators.install)}
            tooltip="How often the install completes successfully."
          />
        </Grid>
      )}

      {props.indicators.productInstall && (
        <Grid item md={2} sm={4}>
          <SummaryCard
            key="product-install-summary"
            threshold={INSTALL_THRESHOLDS}
            name={`${productLabel} Install`}
            link={pathForTestByVariant(
              props.release,
              props.indicators.productInstall.name
            )}
            success={props.indicators.productInstall.current_pass_percentage}
            flakes={props.indicators.productInstall.current_flake_percentage}
            fail={props.indicators.productInstall.current_failure_percentage}
            caption={indicatorCaption(props.indicators.productInstall)}
            tooltip={`How often the ${productLabel} install completes successfully.`}
          />
        </Grid>
      )}
    </Fragment>
  )
}

TopLevelIndicators.propTypes = {
  release: PropTypes.string,
  indicators: PropTypes.object,
  links: PropTypes.object,
}
