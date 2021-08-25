// Compute relative times -- Intl.RelativeTimeFormat is new-ish,
// and not supported in all browsers, and it's not in node yet.
export function relativeTime (date) {
  const minute = 1000 * 60 // Milliseconds in a minute
  const hour = 60 * minute // Milliseconds in an hour
  const day = 24 * hour // Milliseconds in a day
  const millisAgo = date.getTime() - Date.now()

  if (Math.abs(millisAgo) < hour) {
    return Math.round(Math.abs(millisAgo) / minute) + ' minutes ago'
  } else if (Math.abs(millisAgo) < day) {
    return Math.round(Math.abs(millisAgo) / hour) + ' hours ago'
  } else if (Math.abs(millisAgo) < 1.5 * day) {
    return 'about a day ago'
  } else {
    return Math.round(Math.abs(millisAgo) / day) + ' days ago'
  }
}
