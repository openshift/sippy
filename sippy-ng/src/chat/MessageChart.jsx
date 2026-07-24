import {
  Close as CloseIcon,
  Download as DownloadIcon,
  Fullscreen as FullscreenIcon,
} from '@mui/icons-material'
import {
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  Tooltip,
} from '@mui/material'
import { makeStyles, useTheme } from '@mui/styles'
import Plot from 'react-plotly.js'
import PropTypes from 'prop-types'
import React, { useRef, useState } from 'react'

const useStyles = makeStyles((theme) => ({
  visualizationContainer: {
    marginTop: theme.spacing(2),
    marginBottom: theme.spacing(1),
    position: 'relative',
    '& .js-plotly-plot': {
      width: '100% !important',
    },
  },
  chartControls: {
    position: 'absolute',
    top: theme.spacing(1),
    right: theme.spacing(1),
    display: 'flex',
    gap: theme.spacing(0.5),
    zIndex: 10,
  },
  chartControlButton: {
    backgroundColor: theme.palette.background.paper,
    backdropFilter: 'blur(4px)',
    padding: theme.spacing(0.5),
    border: `1px solid ${theme.palette.divider}`,
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  chartModal: {
    '& .MuiDialog-paper': {
      maxWidth: '95vw',
      maxHeight: '95vh',
      width: '95vw',
      height: '95vh',
    },
  },
  chartModalContent: {
    padding: theme.spacing(2),
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    overflow: 'hidden',
  },
  chartModalPlotContainer: {
    flex: 1,
    minHeight: 0,
    display: 'flex',
    '& .js-plotly-plot': {
      width: '100% !important',
      height: '100% !important',
    },
  },
}))

export default function MessageChart({ visualizations }) {
  const classes = useStyles()
  const theme = useTheme()
  const [expandedChart, setExpandedChart] = useState(null)
  const plotRefs = useRef([])

  if (!visualizations || visualizations.length === 0) {
    return null
  }

  const handleDownloadPNG = (index) => {
    const plotElement = plotRefs.current[index]
    if (plotElement && window.Plotly) {
      const gd = plotElement.el
      window.Plotly.downloadImage(gd, {
        format: 'png',
        width: 1200,
        height: 800,
        filename: `sippy_chart_${index + 1}`,
      })
    }
  }

  const handleExpandChart = (viz, index) => {
    const expandedLayout = {
      ...viz.layout,
      paper_bgcolor: theme.palette.background.default,
      plot_bgcolor: theme.palette.background.default,
      font: {
        color: theme.palette.text.primary,
        ...viz.layout?.font,
      },
      xaxis: {
        gridcolor: theme.palette.divider,
        ...viz.layout?.xaxis,
      },
      yaxis: {
        gridcolor: theme.palette.divider,
        ...viz.layout?.yaxis,
      },
      autosize: true,
    }

    setExpandedChart({
      data: viz.data,
      layout: expandedLayout,
      config: viz.config,
    })
  }

  const applyThemeToLayout = (layout) => {
    return {
      ...layout,
      paper_bgcolor: theme.palette.background.default,
      plot_bgcolor: theme.palette.background.default,
      font: {
        color: theme.palette.text.primary,
        ...layout?.font,
      },
      xaxis: {
        gridcolor: theme.palette.divider,
        ...layout?.xaxis,
      },
      yaxis: {
        gridcolor: theme.palette.divider,
        ...layout?.yaxis,
      },
    }
  }

  return (
    <>
      {visualizations.map((viz, index) => {
        const themedLayout = applyThemeToLayout(viz.layout)
        const config = {
          displayModeBar: false,
          displaylogo: false,
          responsive: true,
          ...viz.config,
        }

        return (
          <div key={index} className={classes.visualizationContainer}>
            <div className={classes.chartControls}>
              <Tooltip title="Expand chart" arrow>
                <IconButton
                  size="small"
                  className={classes.chartControlButton}
                  onClick={() => handleExpandChart(viz, index)}
                >
                  <FullscreenIcon fontSize="small" />
                </IconButton>
              </Tooltip>
              <Tooltip title="Download PNG" arrow>
                <IconButton
                  size="small"
                  className={classes.chartControlButton}
                  onClick={() => handleDownloadPNG(index)}
                >
                  <DownloadIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </div>

            <Plot
              ref={(el) => (plotRefs.current[index] = el)}
              data={viz.data}
              layout={themedLayout}
              config={config}
              useResizeHandler={true}
              style={{ width: '100%', height: '100%' }}
            />
          </div>
        )
      })}

      {expandedChart && (
        <Dialog
          open={true}
          onClose={() => setExpandedChart(null)}
          maxWidth={false}
          className={classes.chartModal}
        >
          <DialogTitle
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}
          >
            {expandedChart.layout?.title?.text || 'Chart'}
            <IconButton
              aria-label="close"
              onClick={() => setExpandedChart(null)}
              size="small"
            >
              <CloseIcon />
            </IconButton>
          </DialogTitle>
          <DialogContent className={classes.chartModalContent}>
            <div className={classes.chartModalPlotContainer}>
              <Plot
                data={expandedChart.data}
                layout={expandedChart.layout}
                config={{
                  displayModeBar: true,
                  displaylogo: false,
                  responsive: true,
                }}
                useResizeHandler={true}
                style={{ width: '100%', height: '100%' }}
              />
            </div>
          </DialogContent>
        </Dialog>
      )}
    </>
  )
}

MessageChart.propTypes = {
  visualizations: PropTypes.arrayOf(
    PropTypes.shape({
      data: PropTypes.array.isRequired,
      layout: PropTypes.object.isRequired,
      config: PropTypes.object,
    })
  ),
}
