import './ComponentReadiness.css'
import {
  dateFormat,
  excludeArchesList,
  excludeCloudsList,
  excludeNetworksList,
  excludeUpgradesList,
  excludeVariantsList,
  formatLongDate,
  getUpdatedUrlParts,
  groupByList,
} from './CompReadyUtils'
import { DateTimePicker, MuiPickersUtilsProvider } from '@material-ui/pickers'
import { Fragment, useEffect } from 'react'
import { GridToolbarFilterDateUtils } from '../datagrid/GridToolbarFilterDateUtils'
import { Link } from 'react-router-dom'
import Button from '@material-ui/core/Button'
import CheckBoxList from './CheckboxList'
import PropTypes from 'prop-types'
import React from 'react'
import ReleaseSelector from './ReleaseSelector'

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
    handleGenerateReportDebug,
  } = props

  return (
    <Fragment>
      <div className="cr-report-button">
        <Button
          size="large"
          variant="contained"
          color="primary"
          component={Link}
          to={
            '/component_readiness/' +
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
              component
            )
          }
          onClick={handleGenerateReport}
        >
          Generate Report
        </Button>
      </div>
      <div className="cr-report-button">
        <Button
          size="large"
          variant="contained"
          color="primary"
          component={Link}
          to={
            '/component_readiness/' +
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
              component
            )
          }
          onClick={handleGenerateReportDebug}
        >
          Generate From File
        </Button>
      </div>
      <div className="cr-release-historical">
        <ReleaseSelector
          version={baseRelease}
          label="Historical"
          onChange={setBaseRelease}
        ></ReleaseSelector>
        <MuiPickersUtilsProvider utils={GridToolbarFilterDateUtils}>
          <DateTimePicker
            showTodayButton
            disableFuture
            label="From"
            format={dateFormat}
            ampm={false}
            value={baseStartTime}
            onChange={(e) => {
              const formattedTime = formatLongDate(e, dateFormat)
              setBaseStartTime(formattedTime)
            }}
          />
        </MuiPickersUtilsProvider>
        <MuiPickersUtilsProvider utils={GridToolbarFilterDateUtils}>
          <DateTimePicker
            showTodayButton
            disableFuture
            label="To"
            format={dateFormat}
            ampm={false}
            value={baseEndTime}
            onChange={(e) => {
              const formattedTime = formatLongDate(e, dateFormat)
              setBaseEndTime(formattedTime)
            }}
          />
        </MuiPickersUtilsProvider>
      </div>
      <div className="cr-release-sample">
        <ReleaseSelector
          label="Sample Release"
          version={sampleRelease}
          onChange={setSampleRelease}
        ></ReleaseSelector>
        <MuiPickersUtilsProvider utils={GridToolbarFilterDateUtils}>
          <DateTimePicker
            showTodayButton
            disableFuture
            label="From"
            format={dateFormat}
            ampm={false}
            value={sampleStartTime}
            onChange={(e) => {
              const formattedTime = formatLongDate(e, dateFormat)
              setSampleStartTime(formattedTime)
            }}
          />
        </MuiPickersUtilsProvider>
        <MuiPickersUtilsProvider utils={GridToolbarFilterDateUtils}>
          <DateTimePicker
            showTodayButton
            disableFuture
            label="To"
            format={dateFormat}
            ampm={false}
            value={sampleEndTime}
            onChange={(e) => {
              const formattedTime = formatLongDate(e, dateFormat)
              setSampleEndTime(formattedTime)
            }}
          />
        </MuiPickersUtilsProvider>
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
  handleGenerateReportDebug: PropTypes.func.isRequired,
}
