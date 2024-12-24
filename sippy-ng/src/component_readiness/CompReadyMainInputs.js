import './ComponentReadiness.css'
import { CompReadyVarsContext } from './CompReadyVars'
import {
  dateFormat,
  formatLongDate,
  getUpdatedUrlParts,
} from './CompReadyUtils'
import { FormControl, Grid, InputLabel, MenuItem, Select } from '@mui/material'
import { Fragment } from 'react'
import { makeStyles, useTheme } from '@mui/styles'
import { useHistory } from 'react-router-dom'
import AdvancedOptions from './AdvancedOptions'
import Button from '@mui/material/Button'
import GroupByCheckboxList from './GroupByCheckboxList'
import IncludeVariantCheckBoxList from './IncludeVariantCheckboxList'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import ReleaseSelector from './ReleaseSelector'
import Tooltip from '@mui/material/Tooltip'
import ViewPicker from './ViewPicker'

export const useStyles = makeStyles((theme) => ({
  crRelease: {
    textAlign: 'center',
    marginBottom: 50,
    fontWeight: 'bold',
    padding: 5,
    backgroundColor:
      theme.palette.mode == 'dark'
        ? theme.palette.grey[800]
        : theme.palette.grey[300],
  },
}))

export default function CompReadyMainInputs(props) {
  const theme = useTheme()
  const classes = useStyles(theme)
  // checkBoxHiddenIncludeVariants defines what variants are excluded when creating Include Variant CheckBox
  // This could also be deduced from varsContext.dbGroupByVariants
  const checkBoxHiddenIncludeVariants = new Set([
    'Aggregation',
    'FromRelease',
    'FromReleaseMajor',
    'FromReleaseMinor',
    'NetworkAccess',
    'NetworkStack',
    'Release',
    'ReleaseMajor',
    'ReleaseMinor',
    'Scheduler',
    'SecurityMode',
  ])

  const varsContext = useContext(CompReadyVarsContext)
  const history = useHistory()
  const compReadyEnvOptions = (
    <div>
      <GroupByCheckboxList
        headerName="Group By"
        displayList={varsContext.dbGroupByVariants}
        checkedItems={varsContext.columnGroupByCheckedItems}
        setCheckedItems={varsContext.setColumnGroupByCheckedItems}
      />
      {Object.keys(varsContext.allJobVariants)
        .filter((key) => !checkBoxHiddenIncludeVariants.has(key))
        .map((variant) => (
          <IncludeVariantCheckBoxList
            key={variant}
            variantGroupName={variant}
          />
        ))}
      <AdvancedOptions
        headerName="Advanced"
        confidence={varsContext.confidence}
        pity={varsContext.pity}
        minFail={varsContext.minFail}
        passRateNewTests={varsContext.passRateNewTests}
        passRateAllTests={varsContext.passRateAllTests}
        ignoreMissing={varsContext.ignoreMissing}
        ignoreDisruption={varsContext.ignoreDisruption}
        flakeAsFailure={varsContext.flakeAsFailure}
        includeMultiReleaseAnalysis={varsContext.includeMultiReleaseAnalysis}
        setConfidence={varsContext.setConfidence}
        setPity={varsContext.setPity}
        setMinFail={varsContext.setMinFail}
        setPassRateNewTests={varsContext.setPassRateNewTests}
        setPassRateAllTests={varsContext.setPassRateAllTests}
        setIgnoreMissing={varsContext.setIgnoreMissing}
        setIgnoreDisruption={varsContext.setIgnoreDisruption}
        setFlakeAsFailure={varsContext.setFlakeAsFailure}
        setIncludeMultiReleaseAnalysis={
          varsContext.setIncludeMultiReleaseAnalysis
        }
      ></AdvancedOptions>
    </div>
  )

  const shouldDisplayViewPicker = () => {
    return (
      varsContext.views.length > 0 &&
      !props.isTestDetails &&
      varsContext.environment === undefined &&
      varsContext.capability === undefined &&
      varsContext.component === undefined
    )
  }

  return (
    <Fragment>
      <ViewPicker
        varsContext={varsContext}
        enabled={shouldDisplayViewPicker()}
      />

      <div className={classes.crRelease}>
        <ReleaseSelector
          label="Sample Release"
          tooltip="Release and dates to compare for regression against the basis (historical) release"
          version={varsContext.sampleRelease}
          onChange={varsContext.setSampleReleaseWithDates}
          startTime={formatLongDate(varsContext.sampleStartTime, dateFormat)}
          setStartTime={varsContext.setSampleStartTime}
          endTime={formatLongDate(varsContext.sampleEndTime, dateFormat)}
          setEndTime={varsContext.setSampleEndTime}
          pullRequestSupport={true}
          pullRequestOrg={varsContext.samplePROrg}
          setPullRequestOrg={varsContext.setSamplePROrg}
          pullRequestRepo={varsContext.samplePRRepo}
          setPullRequestRepo={varsContext.setSamplePRRepo}
          pullRequestNumber={varsContext.samplePRNumber}
          setPullRequestNumber={varsContext.setSamplePRNumber}
        ></ReleaseSelector>
      </div>
      <div className={classes.crRelease}>
        <ReleaseSelector
          version={varsContext.baseRelease}
          label="Basis Release"
          tooltip="Release and dates to specify a historical record of how tests have performed"
          onChange={varsContext.setBaseReleaseWithDates}
          startTime={formatLongDate(varsContext.baseStartTime, dateFormat)}
          setStartTime={varsContext.setBaseStartTime}
          endTime={formatLongDate(varsContext.baseEndTime, dateFormat)}
          setEndTime={varsContext.setBaseEndTime}
        ></ReleaseSelector>
      </div>

      <div className="cr-report-button">
        <Button
          size="large"
          variant="contained"
          color="primary"
          to={'/component_readiness/main' + getUpdatedUrlParts(varsContext)}
          onClick={varsContext.handleGenerateReport}
        >
          <Tooltip
            title={
              'Click here to generate a custom report that compares the release you wish to evaluate\
                                                     against a historical (previous) release using all the specific parameters specified'
            }
          >
            <Fragment>Generate Report</Fragment>
          </Tooltip>
        </Button>
      </div>

      {props.isTestDetails ? '' : compReadyEnvOptions}
    </Fragment>
  )
}

// component and environment may be null so they are not required
CompReadyMainInputs.propTypes = {
  isTestDetails: PropTypes.bool,
}
