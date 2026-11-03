import './ComponentReadiness.css'
import { CompReadyVarsContext } from './CompReadyVars'
import { dateFormat, formatLongDate } from './CompReadyUtils'
import { styled } from '@mui/material/styles'
import { useNavigate } from 'react-router'
import AdvancedOptions from './AdvancedOptions'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import GroupByCheckboxList from './GroupByCheckboxList'
import IncludeVariantCheckBoxList from './IncludeVariantCheckboxList'
import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'
import ReleaseSelector from './ReleaseSelector'
import SidebarTestFilters from './SidebarTestFilters'
import Tooltip from '@mui/material/Tooltip'
import ViewPicker from './ViewPicker'

export const CrReleaseBox = styled(Box)(({ theme }) => ({
  textAlign: 'center',
  marginBottom: '50px',
  fontWeight: 'bold',
  padding: '5px',
  backgroundColor:
    theme.palette.mode === 'dark'
      ? theme.palette.grey[800]
      : theme.palette.grey[300],
}))

export default function CompReadyMainInputs({ controlsOpts }) {
  // checkBoxHiddenIncludeVariants defines what variants are excluded when creating Include Variant CheckBox
  // This could also be deduced from varsContext.dbGroupByVariants
  const checkBoxHiddenIncludeVariants = new Set([
    'Aggregation',
    'Capability',
    'Component',
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
  const _navigate = useNavigate()
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

      <CrReleaseBox>
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
      </CrReleaseBox>
      <CrReleaseBox>
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
      </CrReleaseBox>

      <ReportButton
        testId="generate-report-top"
        handler={varsContext.handleGenerateReport}
      />

      {controlsOpts?.isTestDetails || (
        <>
          {compReadyEnvOptions}
          <ReportButton
            testId="generate-report-bottom"
            handler={varsContext.handleGenerateReport}
          />
        </>
      )}
    </Fragment>
  )
}

function ReportButton(props) {
  return (
    <div className="cr-report-button">
      <Button
        data-testid={props.testId}
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
          <span>Generate Report</span>
        </Tooltip>
      </Button>
    </div>
  )
}

ReportButton.propTypes = {
  handler: PropTypes.func,
  testId: PropTypes.string,
}

// component and environment may be null so they are not required
CompReadyMainInputs.propTypes = {
  controlsOpts: PropTypes.object,
}
