import FullCalendar from '@fullcalendar/react'

import { Alert, Grid } from '@mui/material'
import { filterFor } from '../helpers'
import { useHistory } from 'react-router-dom'
import { useTheme } from '@mui/material/styles'
import dayGridPlugin from '@fullcalendar/daygrid'
import PayloadCalendarLegend from './PayloadCalendarLegend'
import PropTypes from 'prop-types'
import React, { Fragment, useState } from 'react'

export default function PayloadCalendar(props) {
  const theme = useTheme()
  const history = useHistory()

  const [acceptedFound, setAcceptedFound] = useState(true)

  const acceptedSourceSuccess = (info) => {
    setAcceptedFound(info.length > 0)
    displayAlertIfNecessary()
  }

  const [rejectedFound, setRejectedFound] = useState(true)

  const rejectedSourceSuccess = (info) => {
    setRejectedFound(info.length > 0)
    displayAlertIfNecessary()
  }

  const [alertMessage, setAlertMessage] = useState('')

  const displayAlertIfNecessary = () => {
    if (!acceptedFound && !rejectedFound) {
      setAlertMessage('Warning: no results found for payload')
    } else {
      setAlertMessage('')
    }
  }

  const failedRetrieval = (error) => {
    console.error(error)
    setAlertMessage('Warning: error retrieving results for payload')
  }

  const eventSources = [
    {
      url:
        process.env.REACT_APP_API_URL +
        '/api/releases/tags/events?release=' +
        props.release,
      method: 'GET',
      extraParams: {
        filter: JSON.stringify({
          items: [
            filterFor('phase', 'equals', 'Accepted'),
            filterFor('architecture', 'equals', props.arch),
            filterFor('stream', 'equals', props.stream),
          ],
        }),
      },
      color: theme.palette.success.light,
      textColor: theme.palette.success.contrastText,
      success: acceptedSourceSuccess,
    },
    {
      url:
        process.env.REACT_APP_API_URL +
        '/api/releases/tags/events?release=' +
        props.release,
      method: 'GET',
      extraParams: {
        filter: JSON.stringify({
          items: [
            filterFor('phase', 'equals', 'Rejected'),
            filterFor('architecture', 'equals', props.arch),
            filterFor('stream', 'equals', props.stream),
          ],
        }),
      },
      color: theme.palette.error.light,
      textColor: theme.palette.error.contrastText,
      success: rejectedSourceSuccess,
    },
    {
      url: process.env.REACT_APP_API_URL + '/api/incidents',
      method: 'GET',
      color: theme.palette.common.black,
      textColor: theme.palette.error.contrastText,
    },
  ]

  const eventClick = (info) => {
    if (info.event?.extendedProps?.phase === 'incident') {
      window.open(
        'https://issues.redhat.com/browse/' + info.event.extendedProps.jira,
        '_blank'
      )
    } else {
      history.push(`/release/${props.release}/tags/${info.event.title}`)
    }
  }

  return (
    <Fragment>
      {alertMessage && (
        <Grid container justifyContent="center" width="100%">
          <Alert severity="error">{alertMessage}</Alert>
        </Grid>
      )}
      <PayloadCalendarLegend />
      <FullCalendar
        timeZone="UTC"
        headerToolbar={{
          start: 'title',
          center: '',
          end: 'today prev,next',
        }}
        plugins={[dayGridPlugin]}
        initialView="dayGridMonth"
        eventClick={eventClick}
        eventSources={eventSources}
        eventSourceFailure={failedRetrieval}
      />
    </Fragment>
  )
}

PayloadCalendar.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string,
  stream: PropTypes.string,
  view: PropTypes.string,
}
