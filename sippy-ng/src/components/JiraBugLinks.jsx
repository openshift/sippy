import { Box, Link } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'

export const jiraIssueURLPrefix = 'https://redhat.atlassian.net/browse/'
export const jiraKeyPattern = /^[A-Z][A-Z0-9_]*-[0-9]+$/

export function isValidJiraKey(key) {
  return jiraKeyPattern.test(key)
}

export function normalizeJiraKeys(keys) {
  return [...new Set((keys || []).map((key) => key.trim()).filter(Boolean))]
}

export default function JiraBugLinks({ bugs, showLabel = false }) {
  const normalizedBugs = normalizeJiraKeys(bugs)
  if (normalizedBugs.length === 0) {
    return null
  }

  return (
    <Box component="span">
      {showLabel && 'Bugs: '}
      {normalizedBugs.map((bug, index) => (
        <React.Fragment key={bug}>
          {index > 0 && ', '}
          <Link
            href={`${jiraIssueURLPrefix}${encodeURIComponent(bug)}`}
            target="_blank"
            rel="noopener noreferrer"
          >
            {bug}
          </Link>
        </React.Fragment>
      ))}
    </Box>
  )
}

JiraBugLinks.propTypes = {
  bugs: PropTypes.arrayOf(PropTypes.string),
  showLabel: PropTypes.bool,
}
