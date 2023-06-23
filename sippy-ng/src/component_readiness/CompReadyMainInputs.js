import './ComponentReadiness.css'
import { CompReadyVarsContext } from './CompReadyVars'
import { Fragment } from 'react'
import { getUpdatedUrlParts, groupByList } from './CompReadyUtils'
import { Link } from 'react-router-dom'
import AdvancedOptions from './AdvancedOptions'
import Button from '@material-ui/core/Button'
import CheckBoxList from './CheckboxList'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'
import ReleaseSelector from './ReleaseSelector'
import Tooltip from '@material-ui/core/Tooltip'

export default function CompReadyMainInputs(props) {
  const {
    baseRelease,
    baseStartTime,
    baseEndTime,
    sampleRelease,
    sampleStartTime,
    sampleEndTime,
    groupByCheckedItems,
    excludeCloudsCheckedItems,
    excludeArchesCheckedItems,
    excludeNetworksCheckedItems,
    excludeUpgradesCheckedItems,
    excludeVariantsCheckedItems,
    confidence,
    pity,
    minFail,
    ignoreMissing,
    ignoreDisruption,
    component,
    environment,
    setBaseRelease,
    setSampleRelease,
    setBaseStartTime,
    setBaseEndTime,
    setSampleStartTime,
    setSampleEndTime,
    setGroupByCheckedItems,
    setExcludeArchesCheckedItems,
    setExcludeNetworksCheckedItems,
    setExcludeCloudsCheckedItems,
    setExcludeUpgradesCheckedItems,
    setExcludeVariantsCheckedItems,
    handleGenerateReport,
    setConfidence,
    setPity,
    setMinFail,
    setIgnoreMissing,
    setIgnoreDisruption,
  } = props

  const {
    excludeNetworksList,
    excludeCloudsList,
    excludeArchesList,
    excludeUpgradesList,
    excludeVariantsList,
  } = useContext(CompReadyVarsContext)

  return (
    <Fragment>
      <div className="cr-report-button">
        <Button
          size="large"
          variant="contained"
          color="primary"
          component={Link}
          to={
            '/component_readiness/main' +
            getUpdatedUrlParts(
              baseRelease,
              baseStartTime,
              baseEndTime,
              sampleRelease,
              sampleStartTime,
              sampleEndTime,
              groupByCheckedItems,
              excludeCloudsCheckedItems,
              excludeArchesCheckedItems,
              excludeNetworksCheckedItems,
              excludeUpgradesCheckedItems,
              excludeVariantsCheckedItems,
              confidence,
              pity,
              minFail,
              ignoreDisruption,
              ignoreMissing
            )
          }
          onClick={handleGenerateReport}
        >
          <Tooltip
            title={
              'Click here to generate a report that compares the release you wish to evaluate\
               against a historical (previous) release'
            }
          >
            <span>Generate Report</span>
          </Tooltip>
        </Button>
      </div>

      <div className="cr-release-sample">
        <ReleaseSelector
          label="Release to Evaluate"
          version={sampleRelease}
          onChange={setSampleRelease}
          startTime={sampleStartTime}
          setStartTime={setSampleStartTime}
          endTime={sampleEndTime}
          setEndTime={setSampleEndTime}
        ></ReleaseSelector>
      </div>
      <div className="cr-release-historical">
        <ReleaseSelector
          version={baseRelease}
          label="Historical Release"
          onChange={setBaseRelease}
          startTime={baseStartTime}
          setStartTime={setBaseStartTime}
          endTime={baseEndTime}
          setEndTime={setBaseEndTime}
        ></ReleaseSelector>
      </div>
      <div>
        <CheckBoxList
          headerName="Group By"
          displayList={groupByList}
          checkedItems={groupByCheckedItems}
          setCheckedItems={setGroupByCheckedItems}
        ></CheckBoxList>
        <CheckBoxList
          headerName="Exclude Arches"
          displayList={excludeArchesList}
          checkedItems={excludeArchesCheckedItems}
          setCheckedItems={setExcludeArchesCheckedItems}
        ></CheckBoxList>
        <CheckBoxList
          headerName="Exclude Networks"
          displayList={excludeNetworksList}
          checkedItems={excludeNetworksCheckedItems}
          setCheckedItems={setExcludeNetworksCheckedItems}
        ></CheckBoxList>
        <CheckBoxList
          headerName="Exclude Clouds"
          displayList={excludeCloudsList}
          checkedItems={excludeCloudsCheckedItems}
          setCheckedItems={setExcludeCloudsCheckedItems}
        ></CheckBoxList>
        <CheckBoxList
          headerName="Exclude Upgrades"
          displayList={excludeUpgradesList}
          checkedItems={excludeUpgradesCheckedItems}
          setCheckedItems={setExcludeUpgradesCheckedItems}
        ></CheckBoxList>
        <CheckBoxList
          headerName="Exclude Variants"
          displayList={excludeVariantsList}
          checkedItems={excludeVariantsCheckedItems}
          setCheckedItems={setExcludeVariantsCheckedItems}
        ></CheckBoxList>
        <AdvancedOptions
          headerName="Advanced"
          confidence={confidence}
          pity={pity}
          minFail={minFail}
          ignoreMissing={ignoreMissing}
          ignoreDisruption={ignoreDisruption}
          setConfidence={setConfidence}
          setPity={setPity}
          setMinFail={setMinFail}
          setIgnoreMissing={setIgnoreMissing}
          setIgnoreDisruption={setIgnoreDisruption}
        ></AdvancedOptions>
      </div>
    </Fragment>
  )
}

// component and environment may be null so they are not required
CompReadyMainInputs.propTypes = {
  baseRelease: PropTypes.string.isRequired,
  baseStartTime: PropTypes.string.isRequired,
  baseEndTime: PropTypes.string.isRequired,
  sampleRelease: PropTypes.string.isRequired,
  sampleStartTime: PropTypes.string.isRequired,
  sampleEndTime: PropTypes.string.isRequired,
  groupByCheckedItems: PropTypes.array.isRequired,
  excludeCloudsCheckedItems: PropTypes.array.isRequired,
  excludeArchesCheckedItems: PropTypes.array.isRequired,
  excludeNetworksCheckedItems: PropTypes.array.isRequired,
  excludeUpgradesCheckedItems: PropTypes.array.isRequired,
  excludeVariantsCheckedItems: PropTypes.array.isRequired,
  confidence: PropTypes.number.isRequired,
  pity: PropTypes.number.isRequired,
  minFail: PropTypes.number.isRequired,
  ignoreMissing: PropTypes.bool.isRequired,
  ignoreDisruption: PropTypes.bool.isRequired,
  component: PropTypes.string,
  environment: PropTypes.string,
  setBaseRelease: PropTypes.func.isRequired,
  setSampleRelease: PropTypes.func.isRequired,
  setBaseStartTime: PropTypes.func.isRequired,
  setBaseEndTime: PropTypes.func.isRequired,
  setSampleStartTime: PropTypes.func.isRequired,
  setSampleEndTime: PropTypes.func.isRequired,
  setGroupByCheckedItems: PropTypes.func.isRequired,
  setExcludeArchesCheckedItems: PropTypes.func.isRequired,
  setExcludeNetworksCheckedItems: PropTypes.func.isRequired,
  setExcludeCloudsCheckedItems: PropTypes.func.isRequired,
  setExcludeUpgradesCheckedItems: PropTypes.func.isRequired,
  setExcludeVariantsCheckedItems: PropTypes.func.isRequired,
  handleGenerateReport: PropTypes.func.isRequired,
  setConfidence: PropTypes.func.isRequired,
  setPity: PropTypes.func.isRequired,
  setMinFail: PropTypes.func.isRequired,
  setIgnoreMissing: PropTypes.func.isRequired,
  setIgnoreDisruption: PropTypes.func.isRequired,
}
