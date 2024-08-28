import { debugMode, getSummaryDate } from './CompReadyUtils'
import { Fragment, useContext, useMemo } from 'react'
import { Grid } from '@mui/material'
import { ReleaseGADates } from '../App'
import { VarsAtPageLoad } from './CompReadyVars'
import PropTypes from 'prop-types'
import React from 'react'

// CompReadyPageTitle is used to print the title of the pages, important report parameters,
// and (when debugging) the accompanying api call string. This allows us to have consistent page headers
// and on page1, we can see the api call string change dynamically.
export default function CompReadyPageTitle(props) {
  const { pageTitle, pageNumber, apiCallStr } = props
  const callStr = `${apiCallStr}`

  const pageHeader = (pageNumber, vars) => {
    // console.log('inside pageHeader', vars.baseRelease, ReleaseGADates)
    if (pageNumber === 5) {
      // Page 5 is test details which already shows this info
      return ''
    }
    let rows = []
    rows.push(
      <Fragment key="release">
        <Grid item xs={6}>
          {getSummaryDate(
            vars.baseStartTime.toString(),
            vars.baseEndTime.toString(),
            vars.baseRelease,
            ReleaseGADates
          )}
        </Grid>
        <Grid item xs={6}>
          {getSummaryDate(
            vars.sampleStartTime.toString(),
            vars.sampleEndTime.toString(),
            vars.sampleRelease,
            ReleaseGADates
          )}
        </Grid>
      </Fragment>
    )
    console.log('page title', vars)
    if (vars.variantCrossCompare) {
      let group
      for (group of vars.variantCrossCompare) {
        let basisVariants = vars.includeVariantsCheckedItems[group]
        let sampleVariants = vars.compareVariantsCheckedItems[group]
        rows.push(
          <Fragment key={'variantCross_' + group}>
            <Grid item xs={6}>
              {group}:&nbsp;
              <strong>
                {basisVariants ? basisVariants.join(', ') : '(any)'}
              </strong>
            </Grid>
            <Grid item xs={6}>
              {group}:&nbsp;
              <strong>
                {sampleVariants ? sampleVariants.join(', ') : '(any)'}
              </strong>
            </Grid>
          </Fragment>
        )
      }
    }

    return (
      <Grid container spacing={2} style={{ marginBottom: '10px' }}>
        <Grid item xs={6}>
          <h3>Basis:</h3>
        </Grid>
        <Grid item xs={6}>
          <h3>Sample:</h3>
        </Grid>
        {rows}
      </Grid>
    )
  }

  return (
    <div>
      {pageTitle}
      {pageHeader(pageNumber, VarsAtPageLoad)}
      {debugMode ? <p>curl -sk &apos;{callStr}&apos;</p> : null}
    </div>
  )
}

CompReadyPageTitle.propTypes = {
  pageTitle: PropTypes.object.isRequired,
  pageNumber: PropTypes.number, // .isRequired,
  apiCallStr: PropTypes.string.isRequired,
}
