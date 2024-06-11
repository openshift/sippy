import './ComponentReadiness.css'
import { CompReadyVarsContext } from './CompReadyVars'
import {
  dateFormat,
  formatLongDate,
  getUpdatedUrlParts,
  groupByList,
} from './CompReadyUtils'
import { Fragment } from 'react'
import { Link } from 'react-router-dom'
import { makeStyles, useTheme } from '@mui/styles'
import AdvancedOptions from './AdvancedOptions'
import Button from '@mui/material/Button'
import CheckBoxList from './CheckboxList'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import ReleaseSelector from './ReleaseSelector'
import Tooltip from '@mui/material/Tooltip'

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

  const varsContext = useContext(CompReadyVarsContext)
  const compReadyEnvOptions = (
    <div>
      <CheckBoxList
        headerName="Group By"
        displayList={groupByList}
        checkedItems={varsContext.groupByCheckedItems}
        setCheckedItems={varsContext.setGroupByCheckedItems}
      ></CheckBoxList>
      <CheckBoxList
        headerName="Exclude Arches"
        displayList={varsContext.excludeArchesList}
        checkedItems={varsContext.excludeArchesCheckedItems}
        setCheckedItems={varsContext.setExcludeArchesCheckedItems}
      ></CheckBoxList>
      <CheckBoxList
        headerName="Exclude Networks"
        displayList={varsContext.excludeNetworksList}
        checkedItems={varsContext.excludeNetworksCheckedItems}
        setCheckedItems={varsContext.setExcludeNetworksCheckedItems}
      ></CheckBoxList>
      <CheckBoxList
        headerName="Exclude Clouds"
        displayList={varsContext.excludeCloudsList}
        checkedItems={varsContext.excludeCloudsCheckedItems}
        setCheckedItems={varsContext.setExcludeCloudsCheckedItems}
      ></CheckBoxList>
      <CheckBoxList
        headerName="Exclude Upgrades"
        displayList={varsContext.excludeUpgradesList}
        checkedItems={varsContext.excludeUpgradesCheckedItems}
        setCheckedItems={varsContext.setExcludeUpgradesCheckedItems}
      ></CheckBoxList>
      <CheckBoxList
        headerName="Exclude Variants"
        displayList={varsContext.excludeVariantsList}
        checkedItems={varsContext.excludeVariantsCheckedItems}
        setCheckedItems={varsContext.setExcludeVariantsCheckedItems}
      ></CheckBoxList>
      <AdvancedOptions
        headerName="Advanced"
        confidence={varsContext.confidence}
        pity={varsContext.pity}
        minFail={varsContext.minFail}
        ignoreMissing={varsContext.ignoreMissing}
        ignoreDisruption={varsContext.ignoreDisruption}
        setConfidence={varsContext.setConfidence}
        setPity={varsContext.setPity}
        setMinFail={varsContext.setMinFail}
        setIgnoreMissing={varsContext.setIgnoreMissing}
        setIgnoreDisruption={varsContext.setIgnoreDisruption}
      ></AdvancedOptions>
    </div>
  )
  return (
    <Fragment>
      <div className="cr-report-button">
        <Button
          size="large"
          variant="contained"
          color="primary"
          to={
            '/component_readiness/main' +
            getUpdatedUrlParts(
              varsContext.baseRelease,
              varsContext.baseStartTime,
              varsContext.baseEndTime,
              varsContext.sampleRelease,
              varsContext.sampleStartTime,
              varsContext.sampleEndTime,
              varsContext.groupByCheckedItems,
              varsContext.excludeCloudsCheckedItems,
              varsContext.excludeArchesCheckedItems,
              varsContext.excludeNetworksCheckedItems,
              varsContext.excludeUpgradesCheckedItems,
              varsContext.excludeVariantsCheckedItems,
              varsContext.confidence,
              varsContext.pity,
              varsContext.minFail,
              varsContext.ignoreDisruption,
              varsContext.ignoreMissing
            )
          }
          onClick={varsContext.handleGenerateReport}
        >
          <Tooltip
            title={
              'Click here to generate a report that compares the release you wish to evaluate\
               against a historical (previous) release'
            }
          >
            <Fragment>Generate Report</Fragment>
          </Tooltip>
        </Button>
      </div>

      <div className={classes.crRelease}>
        <ReleaseSelector
          label="Release to Evaluate"
          version={varsContext.sampleRelease}
          onChange={varsContext.setSampleReleaseWithDates}
          startTime={formatLongDate(varsContext.sampleStartTime, dateFormat)}
          setStartTime={varsContext.setSampleStartTime}
          endTime={formatLongDate(varsContext.sampleEndTime, dateFormat)}
          setEndTime={varsContext.setSampleEndTime}
        ></ReleaseSelector>
      </div>
      <div className={classes.crRelease}>
        <ReleaseSelector
          version={varsContext.baseRelease}
          label="Historical Release"
          onChange={varsContext.setBaseReleaseWithDates}
          startTime={formatLongDate(varsContext.baseStartTime, dateFormat)}
          setStartTime={varsContext.setBaseStartTime}
          endTime={formatLongDate(varsContext.baseEndTime, dateFormat)}
          setEndTime={varsContext.setBaseEndTime}
        ></ReleaseSelector>
      </div>
      {props.isTestDetails ? '' : compReadyEnvOptions}
    </Fragment>
  )
}

// component and environment may be null so they are not required
CompReadyMainInputs.propTypes = {
  isTestDetails: PropTypes.bool,
}
