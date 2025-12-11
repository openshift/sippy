import { AutoAwesome as AutoAwesomeIcon } from '@mui/icons-material'
import {
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  TextField,
  Typography,
} from '@mui/material'
import { makeStyles } from '@mui/styles'
import { useSessionState } from './store/useChatStore'
import OneShotChatModal from './OneShotChatModal'
import PropTypes from 'prop-types'
import React, { useState } from 'react'

const useStyles = makeStyles((theme) => ({
  dialogPaper: {
    minWidth: 500,
  },
  content: {
    display: 'flex',
    flexDirection: 'column',
    gap: theme.spacing(2),
  },
  description: {
    color: theme.palette.text.secondary,
    marginBottom: theme.spacing(1),
  },
}))

/**
 * CreatePromptDialog - Dialog for AI-assisted prompt creation
 * Allows user to describe desired prompt and optionally include chat history as example
 */
export default function CreatePromptDialog({ open, onClose, onYAMLGenerated }) {
  const classes = useStyles()
  const { activeSession } = useSessionState()

  const [promptDescription, setPromptDescription] = useState('')
  const [includeHistory, setIncludeHistory] = useState(false)
  const [aiModalOpen, setAiModalOpen] = useState(false)

  // Pre-fill when there's conversation history
  React.useEffect(() => {
    if (open && activeSession && activeSession.messages?.length > 0) {
      setIncludeHistory(true)
      if (!promptDescription) {
        setPromptDescription(
          'Create a reusable prompt template based on the conversation above. Identify any specific values (like release versions, job names, test names, etc.) and make them parameterized arguments.'
        )
      }
    }
  }, [open, activeSession])

  const handleCreate = () => {
    setAiModalOpen(true)
  }

  const handleClose = () => {
    setPromptDescription('')
    setIncludeHistory(false)
    onClose()
  }

  const handleAIResult = (result) => {
    console.log('=== AI Generated Prompt Response ===')
    console.log('Full response length:', result.length)

    // Pass the generated YAML to parent (should be raw YAML)
    onYAMLGenerated(result)
    handleClose()
  }

  const buildAIPrompt = () => {
    let prompt = `Create a Sippy prompt in YAML format based on this request:

${promptDescription}

`

    // Include chat history if checkbox is checked
    if (includeHistory && activeSession && activeSession.messages) {
      const conversationHistory = activeSession.messages
        .map((msg) => {
          const role = msg.role === 'user' ? 'User' : 'Assistant'
          return `${role}: ${msg.content}`
        })
        .join('\n\n')

      prompt += `Example Conversation:
---
${conversationHistory}
---

Based on this conversation, create a reusable prompt template with appropriate arguments.
Use Jinja2/Nunjucks templating for variable substitution (e.g., {{ variable_name }}).

`
    }

    prompt += `IMPORTANT GUIDELINES FOR CREATING SIPPY PROMPTS:

1. **Arguments are for VARIABLES only** - Use arguments to parameterize inputs like job names, releases, test names, etc.
2. **All instructions go in the prompt field** - The entire workflow, formatting requirements, and detailed instructions should be in the prompt template itself
3. **Be VERY detailed in the prompt** - Include exact steps, SQL queries, tool usage patterns, formatting requirements, and output structure
4. **Specify exact output format** - Tell the LLM exactly how to structure the response (markdown headings, tables, lists, etc.)
5. **Include examples** - Show the LLM what good output looks like

**CRITICAL OUTPUT REQUIREMENT:**
Return ONLY the YAML content. Do NOT wrap it in markdown code blocks or add any other text.
Start your response directly with "name:" and nothing else before it.

Here's an example of a WELL-WRITTEN prompt:

name: test-failure-analysis
description: Analyze a test's performance, identify failure patterns, and provide recommendations
arguments:
  - name: release
    description: Release version (e.g., 4.18, 4.17)
    required: true
    type: string
    autocomplete: releases
  - name: test_name
    description: Fully qualified test name
    required: true
    type: string
    autocomplete: tests
prompt: |
  Analyze test: **{{ test_name }}** on release **{{ release }}**

  ## 1. Overview
  - Display test name as a markdown link to the Sippy analysis page
  - Use format: \`[Test Name]({base_url}/sippy-ng/tests/{release}/analysis?test={url_encoded_test_name})\`
  
  ## 2. 7-Day Performance
  Use \`prow_test_report_7d_matview\` materialized view:
  - Overall pass rate: sum(current_successes) / sum(current_runs) × 100
  - Total runs, failures, flakes
  - Trend: consistent passing/failing or intermittent
  
  ## 3. Variant Analysis
  Query grouped by variants:
  - Calculate failure rate per variant combination
  - Report: variant combo, pass rate, failure count vs runs
  - Order by worst performing first
  
  ## 4. Failure Modes
  Query \`prow_job_run_test_outputs\` for recent failures:
  - Examine up to 10 failure outputs
  - Categorize: Consistent error vs diverse issues
  - Include exact error messages
  
  ## 5. Assessment & Recommendations
  **Root Cause (confidence: High/Medium/Low):**
  - Product bug / Test issue / Infrastructure / Variant-specific
  - Key evidence
  - Next steps
  
  **Guidelines:** Use exact data, include links, use markdown tables, state explicitly if data unavailable

Now create a prompt following this pattern. Make your prompt DETAILED with:
- Exact step-by-step workflow
- Specific tool calls and database queries  
- Clear output format with markdown structure
- Explicit guidelines for the LLM

**CRITICAL: GENERALIZE THE CONVERSATION**
When basing this on a conversation:
- Identify any SPECIFIC VALUES mentioned (release "4.21", job name "periodic-ci-...", test name, payload URL, etc.)
- Make those into ARGUMENTS - do NOT hardcode them in the prompt
- Example: If user asked about "release 4.21", create an argument \`release\` and use \`{{ release }}\` in the prompt
- Example: If user asked about a specific job name, create a \`job_name\` argument with autocomplete: jobs
- The prompt should work for ANY similar scenario, not just the specific one from the conversation

**MAKE THE PROMPT COMPREHENSIVE**
- Include ALL the steps the LLM took in the conversation
- Specify exact database tables, materialized views, and query patterns
- **CRITICAL**: Tell the LLM that the database tables DO EXIST and should be used as specified
- If the LLM needs table schema information, instruct it to query the PostgreSQL database directly to retrieve column names and types
- The Sippy database has many more tables than what's documented - the LLM should trust the table names in the prompt
- Describe the output format in detail (headings, sections, tables, charts)
- Include any guidelines about when to stop, how to handle errors, parallel tool calls
- Tell the LLM exactly what to do, don't leave anything ambiguous

**ABOUT PLOTLY CHARTS**
- If the conversation included creating a chart, describe the chart requirements in detail
- Do NOT include sample Plotly JSON in the prompt itself
- Instead, describe what the chart should look like (chart type, axes, colors, hover mode, etc.)
- The LLM executing the prompt will generate the actual Plotly JSON at runtime
`

    return prompt
  }

  return (
    <>
      <Dialog
        open={open}
        onClose={handleClose}
        classes={{ paper: classes.dialogPaper }}
      >
        <DialogTitle>
          <Typography variant="h6">Create Custom Prompt</Typography>
        </DialogTitle>

        <DialogContent className={classes.content}>
          <Typography variant="body2" className={classes.description}>
            Describe the prompt you want to create, and AI will generate a YAML
            template for you to customize.
          </Typography>

          <TextField
            label="Prompt Description"
            placeholder="e.g., Analyze test failures across multiple releases..."
            value={promptDescription}
            onChange={(e) => setPromptDescription(e.target.value)}
            multiline
            rows={4}
            fullWidth
            autoFocus
          />

          {activeSession && activeSession.messages?.length > 0 && (
            <FormControlLabel
              control={
                <Checkbox
                  checked={includeHistory}
                  onChange={(e) => setIncludeHistory(e.target.checked)}
                />
              }
              label={
                <Typography variant="body2">
                  Include current chat history as an example for the LLM
                  <Typography
                    variant="caption"
                    display="block"
                    color="textSecondary"
                  >
                    Helps AI understand patterns from this successful
                    conversation
                  </Typography>
                </Typography>
              }
            />
          )}
        </DialogContent>

        <DialogActions>
          <Button onClick={handleClose}>Cancel</Button>
          <Button
            onClick={handleCreate}
            variant="contained"
            startIcon={<AutoAwesomeIcon />}
            disabled={!promptDescription.trim()}
          >
            Generate with AI
          </Button>
        </DialogActions>
      </Dialog>

      {/* AI Generation Modal */}
      <OneShotChatModal
        open={aiModalOpen}
        onClose={() => setAiModalOpen(false)}
        prompt={buildAIPrompt()}
        onResult={handleAIResult}
        title="Generating Prompt YAML"
      />
    </>
  )
}

CreatePromptDialog.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onYAMLGenerated: PropTypes.func.isRequired,
}
