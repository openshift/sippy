import '@testing-library/jest-dom'
import { render, screen } from '@testing-library/react'
import MarkdownEditor from './MarkdownEditor'
import PropTypes from 'prop-types'
import React from 'react'
import userEvent from '@testing-library/user-event'

function ControlledMarkdownEditor({ initialValue = '', disabled = false }) {
  const [value, setValue] = React.useState(initialValue)

  return (
    <MarkdownEditor
      label="Explanation"
      value={value}
      onChange={(event) => setValue(event.target.value)}
      helperText="Markdown is supported."
      disabled={disabled}
      minRows={5}
      size="medium"
    />
  )
}

ControlledMarkdownEditor.propTypes = {
  initialValue: PropTypes.string,
  disabled: PropTypes.bool,
}

describe('MarkdownEditor', () => {
  it('renders Markdown and GFM, then returns to the raw value', async () => {
    const raw = '**Expected** and ~~obsolete~~'
    render(<ControlledMarkdownEditor initialValue={raw} />)

    expect(screen.getByRole('textbox', { name: 'Explanation' })).toHaveValue(
      raw
    )
    expect(screen.getByRole('button', { name: 'Edit' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )

    await userEvent.click(screen.getByRole('button', { name: 'Preview' }))

    expect(
      screen.getByRole('region', { name: 'Explanation preview' })
    ).toBeInTheDocument()
    expect(screen.getByText('Expected').tagName).toBe('STRONG')
    expect(screen.getByText('obsolete').tagName).toBe('DEL')
    expect(screen.getByRole('button', { name: 'Preview' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )

    await userEvent.click(screen.getByRole('button', { name: 'Edit' }))

    expect(screen.getByRole('textbox', { name: 'Explanation' })).toHaveValue(
      raw
    )
  })

  it('shows the empty preview fallback', async () => {
    render(<ControlledMarkdownEditor />)

    await userEvent.click(screen.getByRole('button', { name: 'Preview' }))

    expect(screen.getByText('No content to preview')).toBeInTheDocument()
  })

  it('disables editing and mode changes', () => {
    render(<ControlledMarkdownEditor disabled initialValue="raw value" />)

    expect(screen.getByRole('textbox', { name: 'Explanation' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Edit' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Preview' })).toBeDisabled()
  })
})
