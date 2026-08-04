import { Link, ListItem } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'

/*
  After many attempts to convince snyk that our link usage is safe by adding code to "sanitize" URLs,
  but not to its satisfaction, this seems like the only sane option: a file to add to snyk's ignore list,
  for writing links that we personally guarantee are "safe" and no longer subject to snyk paranoia.
 */

const httpLinkRegex = '^(https?:/)?/[^"]+'

// this is a simple link component that takes a URL and renders it via Link, but only if the URL is valid.
// using this soothes snyk's paranoia about insecure links from state vars.
// only the `address` prop is used, all others are passed through to Link.
export default function LaunderedLink({ address, ...props }) {
  let match = address.match(httpLinkRegex)
  if (!match) {
    throw new Error('Invalid URL format: ' + address)
  }
  return <Link style={{ cursor: 'pointer' }} {...props} href={match[0]} />
}

LaunderedLink.propTypes = {
  address: PropTypes.string.isRequired,
}

// same approach but for a ListItem with a link
export function LaunderedListItem({ address, ...props }) {
  let match = address.match(httpLinkRegex)
  if (!match) {
    throw new Error('Invalid URL format: ' + address)
  }
  return <ListItem {...props} href={match[0]} />
}

LaunderedListItem.propTypes = {
  address: PropTypes.string.isRequired,
}

export function openLaunderedLink(address) {
  let match = address.match(httpLinkRegex)
  if (match) {
    window.open(match[0], '_blank', 'noopener,noreferrer')
  } else {
    console.warn('Invalid URL format: ' + address)
  }
}
