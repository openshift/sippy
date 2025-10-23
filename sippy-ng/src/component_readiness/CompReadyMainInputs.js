import './ComponentReadiness.css'
import { CompReadyVarsContext } from './CompReadyVars'
import { dateFormat, formatLongDate } from './CompReadyUtils'
import { makeStyles, useTheme } from '@mui/styles'
import { useNavigate } from 'react-router-dom'
import AdvancedOptions from './AdvancedOptions'
import Button from '@mui/material/Button'
import GroupByCheckboxList from './GroupByCheckboxList'
import IncludeVariantCheckBoxList from './IncludeVariantCheckboxList'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'
import ReleaseSelector from './ReleaseSelector'
import SidebarTestFilters from './SidebarTestFilters'
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

export default function CompReadyMainInputs({ controlsOpts }) {
  const theme = useTheme()
  const classes = useStyles(theme)
  // checkBoxHiddenIncludeVariants defines what variants are excluded when creating Include Variant CheckBox
  // This could also be deduced from varsContext.dbGroupByVariants
  const checkBoxHiddenIncludeVariants = new Set([
    'Aggregation',
    'FromRelease',
    'FromReleaseMajor',
    'FromReleaseMinor',
    'NetworkStack',
    'Release',
    'ReleaseMajor',
    'ReleaseMinor',
    'Scheduler',
    'SecurityMode',
  ])

  const varsContext = useContext(CompReadyVarsContext)
  const navigate = useNavigate()
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
      <SidebarTestFilters
        headerName="Test Options"
        controlsOpts={controlsOpts}
      />
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
      !controlsOpts?.isTestDetails &&
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
          payloadSupport={true}
          payloadTags={varsContext.samplePayloadTags}
          setPayloadTags={varsContext.setSamplePayloadTags}
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

      <ReportButton handler={varsContext.handleGenerateReport} />

      {controlsOpts?.isTestDetails || (
        <>
          {compReadyEnvOptions}
          <ReportButton handler={varsContext.handleGenerateReport} />
        </>
      )}
    </Fragment>
  )
}

function ReportButton(props) {
  return (
    <div className="cr-report-button">
      <Button
        size="large"
        variant="contained"
        color="primary"
        onClick={(event) => props.handler(event)}
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
  )
}

ReportButton.propTypes = { handler: PropTypes.func }

// component and environment may be null so they are not required
CompReadyMainInputs.propTypes = {
  controlsOpts: PropTypes.object,
}
