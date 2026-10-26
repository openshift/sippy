import { Box, Link } from '@mui/material'
import PropTypes from 'prop-types'
import React from 'react'

export const jiraIssueURLPrefix = 'https://redhat.atlassian.net/browse/'
export const jiraKeyPattern = /^[A-Z][A-Z0-9_]*-[0-9]+$/

// isValidJiraKey uses the same issue-key format as the labels API.
export function isValidJiraKey(key) {
  return jiraKeyPattern.test(key)
}

// normalizeJiraKeys removes whitespace, blank entries, and duplicate associations.
export function normalizeJiraKeys(keys) {
  return [...new Set((keys || []).map((key) => key.trim()).filter(Boolean))]
}

// JiraBugLinks renders associated issues and omits the section for labels without bugs.
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
