import { Link, ListItem } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'

// this is a simple link component that takes a URL and renders it via Link, but only if the URL is valid.
// using this soothes snyk's paranoia about insecure links from state vars.
// only the `address` prop is used, all others are passed through to Link.
export default function SecureLink({ address, ...props }) {
  let match = address.match('^(https?:/)?/[^"]+')
  if (!match) {
    throw new Error('Invalid URL format: ' + address)
  }
  return <Link style={{ cursor: 'pointer' }} {...props} href={match[0]} />
}

SecureLink.propTypes = {
  address: PropTypes.string.isRequired,
}

// same approach but for a ListItem with a link
export function LaunderedListItem({ address, ...props }) {
  let match = address.match('^(https?:/)?/[^"]+')
  if (!match) {
    throw new Error('Invalid URL format: ' + address)
  }
  return <ListItem {...props} href={match[0]} />
}

LaunderedListItem.propTypes = {
  address: PropTypes.string.isRequired,
}
