import * as d3scale from 'd3-scale'
import PropTypes from 'prop-types'
import React, { useEffect, useRef } from 'react'
import TimelinesChart from 'timelines-chart'

// TimelineChart is a React component to wrap the plain TimelinesChart we used
// in origin previously.
export default function TimelineChart({
  eventIntervals,
  data,
  segmentClickedFunc,
  segmentTooltipContentFunc,
  intervalColors,
}) {
  const ref = useRef(null)

  // split the intervalColors, a map of interval color keys to color strings. The timeline charts wants
  // these in two positional arrays.
  let domain = []
  let range = []
  for (const key in intervalColors) {
    domain.push(key)
    range.push(intervalColors[key])
  }

  const ordinalScale = d3scale.scaleOrdinal().domain(domain).range(range)

  useEffect(() => {
    let chart = null
    const node = ref.current
    if (node) {
      //const el = document.querySelector('#chart')
      chart = TimelinesChart()(node)
        .data(data)
        .useUtc(true)
        .zQualitative(true)
        .enableAnimations(false)
        .enableOverview(false)
        .leftMargin(150)
        .rightMargin(750)
        .maxLineHeight(30)
        .maxHeight(20000)
        .onSegmentClick(segmentClickedFunc)
        .segmentTooltipContent(segmentTooltipContentFunc)
        .zColorScale(ordinalScale) // seems to enable the use of our own colors
      if (eventIntervals.length > 0) {
        chart.zoomX([
          new Date(eventIntervals[0].from),
          new Date(eventIntervals[eventIntervals.length - 1].to),
        ])
      }
    }
    return () => {
      if (node) {
        while (node.firstChild) {
          node.removeChild(node.firstChild)
        }
      }
    }
  }, [data])
  return React.createElement('div', { ref: ref })
}

TimelineChart.propTypes = {
  data: PropTypes.array,
  eventIntervals: PropTypes.array,
  segmentClickedFunc: PropTypes.func,
  segmentTooltipContentFunc: PropTypes.func,
  intervalColors: PropTypes.object,
}
