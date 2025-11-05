import yaml from 'js-yaml'

/**
 * Validates a prompt YAML string and returns the parsed object or validation errors
 * @param {string} yamlString - The YAML string to validate
 * @returns {{valid: boolean, prompt?: object, errors?: string[]}}
 */
export function validatePromptYAML(yamlString) {
  const errors = []

  if (!yamlString || yamlString.trim() === '') {
    return { valid: false, errors: ['YAML content is empty'] }
  }

  let parsed
  try {
    parsed = yaml.load(yamlString)
  } catch (error) {
    return {
      valid: false,
      errors: [`Invalid YAML syntax: ${error.message}`],
    }
  }

  // Validate required fields
  if (!parsed || typeof parsed !== 'object') {
    errors.push('YAML must contain a valid object')
    return { valid: false, errors }
  }

  if (!parsed.name || typeof parsed.name !== 'string') {
    errors.push('Missing or invalid "name" field (must be a non-empty string)')
  } else if (!/^[a-z0-9-]+$/.test(parsed.name)) {
    errors.push(
      'Invalid "name" format (must contain only lowercase letters, numbers, and hyphens)'
    )
  }

  if (!parsed.description || typeof parsed.description !== 'string') {
    errors.push(
      'Missing or invalid "description" field (must be a non-empty string)'
    )
  }

  if (!parsed.prompt || typeof parsed.prompt !== 'string') {
    errors.push(
      'Missing or invalid "prompt" field (must be a non-empty string)'
    )
  }

  // Validate optional arguments field
  if (parsed.arguments !== undefined) {
    if (!Array.isArray(parsed.arguments)) {
      errors.push('"arguments" field must be an array')
    } else {
      parsed.arguments.forEach((arg, index) => {
        if (!arg.name || typeof arg.name !== 'string') {
          errors.push(
            `Argument at index ${index} is missing or has invalid "name" field`
          )
        }
        if (!arg.description || typeof arg.description !== 'string') {
          errors.push(
            `Argument "${
              arg.name || index
            }" is missing or has invalid "description" field`
          )
        }
        if (arg.type && !['string', 'array'].includes(arg.type)) {
          errors.push(
            `Argument "${
              arg.name || index
            }" has invalid "type" (must be "string" or "array")`
          )
        }
        if (arg.required !== undefined && typeof arg.required !== 'boolean') {
          errors.push(
            `Argument "${
              arg.name || index
            }" has invalid "required" field (must be boolean)`
          )
        }
        if (
          arg.autocomplete !== undefined &&
          typeof arg.autocomplete !== 'string'
        ) {
          errors.push(
            `Argument "${
              arg.name || index
            }" has invalid "autocomplete" field (must be string)`
          )
        }
      })
    }
  }

  // Validate optional hide field
  if (parsed.hide !== undefined && typeof parsed.hide !== 'boolean') {
    errors.push('"hide" field must be a boolean')
  }

  if (errors.length > 0) {
    return { valid: false, errors }
  }

  return { valid: true, prompt: parsed }
}

/**
 * Converts a prompt object to YAML string
 * @param {object} promptObject - The prompt object to convert
 * @returns {string} YAML string representation
 */
export function promptToYAML(promptObject) {
  return yaml.dump(promptObject, {
    indent: 2,
    lineWidth: -1, // No line wrapping
    noRefs: true,
  })
}

/**
 * Extracts YAML code blocks from markdown/text content
 * @param {string} content - Text content that may contain YAML code blocks
 * @returns {string[]} Array of YAML strings found in code blocks
 */
export function extractYAMLFromText(content) {
  const yamlBlocks = []

  // Find all code block starts marked as yaml or yml
  const yamlStartRegex = /```(?:yaml|yml)\s*\n/gi
  let startMatch

  while ((startMatch = yamlStartRegex.exec(content)) !== null) {
    const startPos = startMatch.index + startMatch[0].length

    // Find the matching closing ``` by counting nested blocks
    let depth = 1
    let pos = startPos
    let endPos = -1

    while (pos < content.length && depth > 0) {
      const nextBackticks = content.indexOf('```', pos)
      if (nextBackticks === -1) {
        break
      }

      // Check if this is a new opening block
      const beforeBackticks = content.substring(
        Math.max(0, nextBackticks - 50),
        nextBackticks
      )
      if (/\n```[a-z]*\s*$/.test(beforeBackticks)) {
        depth++
      } else {
        depth--
        if (depth === 0) {
          endPos = nextBackticks
          break
        }
      }

      pos = nextBackticks + 3
    }

    if (endPos !== -1) {
      const yamlContent = content.substring(startPos, endPos).trim()
      console.log('Extracted YAML block length:', yamlContent.length)
      yamlBlocks.push(yamlContent)
    }
  }

  console.log('Total YAML blocks found:', yamlBlocks.length)
  return yamlBlocks
}

/**
 * Creates a default prompt template
 * @returns {string} Default YAML template string
 */
export function getDefaultPromptTemplate() {
  return `name: my-custom-prompt
description: A brief description of what this prompt does
arguments:
  - name: example_arg
    description: Description of this argument
    required: true
    type: string
prompt: |
  This is your prompt template.
  You can use {{ example_arg }} for variable substitution.
`
}

/**
 * Validates that a prompt name doesn't conflict with server prompts
 * @param {string} name - The prompt name to check
 * @param {Array} serverPrompts - Array of server prompts
 * @param {string} currentName - Current name if editing (to allow keeping same name)
 * @returns {{valid: boolean, error?: string}}
 */
export function validatePromptName(name, serverPrompts, currentName = null) {
  if (!name || name.trim() === '') {
    return { valid: false, error: 'Prompt name cannot be empty' }
  }

  if (!/^[a-z0-9-]+$/.test(name)) {
    return {
      valid: false,
      error:
        'Prompt name must contain only lowercase letters, numbers, and hyphens',
    }
  }

  // Allow keeping the same name when editing
  if (currentName && name === currentName) {
    return { valid: true }
  }

  // Check for conflicts with server prompts
  const conflictingServerPrompt = serverPrompts.find((p) => p.name === name)
  if (conflictingServerPrompt) {
    return {
      valid: false,
      error: `A server prompt with name "${name}" already exists. Please choose a different name.`,
    }
  }

  return { valid: true }
}
