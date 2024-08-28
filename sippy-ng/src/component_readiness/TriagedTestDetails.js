import PropTypes from 'prop-types'
import React, { Fragment, useContext } from 'react'
import TriagedTestJobRuns from './TriagedTestJobRuns'

export default function TriagedTestDetails(props) {
  const [jobRuns, setJobRuns] = React.useState([])
  const [showView, setShowView] = React.useState(false)

  const handleTriagedRegressionVariantSelectionChanged = (data) => {
    let groupedJobRuns = new Map()
    let displayView = false

    if (data && data.job_runs && data.job_runs.length > 0) {
      displayView = true
      data.job_runs.forEach((run) => {
        let parts = run.url.split('/')
        if (!(parts == null) && parts.length > 1) {
          let id = parts[parts.length - 1]
          let jobName = parts[parts.length - 2]

          if (!groupedJobRuns.has(jobName)) {
            groupedJobRuns.set(jobName, [])
          }
          let jobRuns = groupedJobRuns.get(jobName)
          groupedJobRuns.set(
            jobName,
            jobRuns.concat({ jobRunId: id, url: run.url })
          )
        }
      })
    }
    let arrayOfJobs = Array.from(groupedJobRuns, ([job_name, job_runs]) => ({
      job_name,
      job_runs,
    }))
    setJobRuns(arrayOfJobs)
    setShowView(displayView)
  }
  props.eventEmitter.on(
    'triagedRegressionVariantSelectionChanged',
    handleTriagedRegressionVariantSelectionChanged
  )

  return (
    <Fragment>
      <div hidden={!showView} className="cr-triage-panel-element">
        <TriagedTestJobRuns jobRuns={jobRuns} />
      </div>
    </Fragment>
  )
}

TriagedTestDetails.propTypes = {
  eventEmitter: PropTypes.object,
}
