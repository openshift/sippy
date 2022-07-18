import FullCalendar from '@fullcalendar/react'

import { filterFor } from '../helpers'
import { useHistory } from 'react-router-dom'
import { useTheme } from '@material-ui/core/styles'
import dayGridPlugin from '@fullcalendar/daygrid'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'

export default function ReleasePayloadCalendar(props) {
  const theme = useTheme()
  const history = useHistory()

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
    },
  ]

  const eventClick = (info) =>
    history.push(`/release/${props.release}/tags/${info.event.title}`)

  return (
    <Fragment>
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
      />
    </Fragment>
  )
}

ReleasePayloadCalendar.propTypes = {
  release: PropTypes.string,
  arch: PropTypes.string,
  stream: PropTypes.string,
  view: PropTypes.string,
}
