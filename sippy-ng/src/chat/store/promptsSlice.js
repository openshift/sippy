import { promptToYAML, validatePromptName } from '../promptSchema'
import nunjucks from 'nunjucks'

// Configure nunjucks for template rendering
const nunjucksEnv = new nunjucks.Environment(null, { autoescape: false })

/**
 * Zustand slice for managing slash command prompts
 */
export const createPromptsSlice = (set, get) => ({
  // State
  serverPrompts: [], // Prompts fetched from the server
  localPrompts: [], // User-created prompts stored locally
  promptsLoading: false,
  promptsError: null,

  // Function that merges server and local prompts
  getPrompts: () => {
    const state = get()
    const server = (state.serverPrompts || []).map((p) => ({
      ...p,
      source: 'server',
    }))
    const local = (state.localPrompts || []).map((p) => ({
      ...p,
      source: 'local',
    }))
    return [...server, ...local].sort((a, b) => a.name.localeCompare(b.name))
  },

  // Fetch prompts from the server
  fetchPrompts: async () => {
    set({ promptsLoading: true, promptsError: null })

    try {
      const response = await fetch(
        (process.env.REACT_APP_CHAT_API_URL || '/api/chat') + '/prompts'
      )

      if (!response.ok) {
        throw new Error('Failed to fetch prompts')
      }

      const data = await response.json()
      set({
        serverPrompts: data.prompts || [],
        promptsLoading: false,
      })
    } catch (error) {
      set({
        promptsLoading: false,
        promptsError: error.message,
      })
    }
  },

  // Render a prompt with arguments
  renderPrompt: async (promptName, args) => {
    const state = get()
    const allPrompts = state.getPrompts()

    // Find the prompt
    const prompt = allPrompts.find((p) => p.name === promptName)

    if (!prompt) {
      throw new Error(`Prompt "${promptName}" not found`)
    }

    // If it's a local prompt, render it client-side
    if (prompt.source === 'local') {
      try {
        // Fill in default values for missing arguments
        const filledArgs = { ...args }
        if (prompt.arguments) {
          prompt.arguments.forEach((arg) => {
            if (
              filledArgs[arg.name] === undefined &&
              arg.default !== undefined
            ) {
              filledArgs[arg.name] = arg.default
            }
          })
        }

        // Render the template using nunjucks
        const rendered = nunjucksEnv.renderString(prompt.prompt, filledArgs)
        return rendered
      } catch (error) {
        console.error('Error rendering local prompt:', error)
        throw new Error(`Failed to render local prompt: ${error.message}`)
      }
    }

    // For server prompts, use the server API
    try {
      const response = await fetch(
        (process.env.REACT_APP_CHAT_API_URL || '/api/chat') + '/prompts/render',
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            prompt_name: promptName,
            arguments: args,
          }),
        }
      )

      if (!response.ok) {
        throw new Error('Failed to render prompt')
      }

      const data = await response.json()
      return data.rendered
    } catch (error) {
      console.error('Error rendering server prompt:', error)
      throw error
    }
  },

  // Save a new local prompt
  saveLocalPrompt: (promptData) => {
    const state = get()

    // Validate prompt name doesn't conflict with server prompts
    const nameValidation = validatePromptName(
      promptData.name,
      state.serverPrompts
    )
    if (!nameValidation.valid) {
      throw new Error(nameValidation.error)
    }

    // Check for duplicate local prompt names
    const existingLocal = state.localPrompts.find(
      (p) => p.name === promptData.name
    )
    if (existingLocal) {
      throw new Error(
        `A local prompt with name "${promptData.name}" already exists`
      )
    }

    // Add metadata
    const newPrompt = {
      ...promptData,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    }

    set({
      localPrompts: [...state.localPrompts, newPrompt],
    })

    return newPrompt
  },

  // Update an existing local prompt
  updateLocalPrompt: (promptName, promptData) => {
    const state = get()

    const index = state.localPrompts.findIndex((p) => p.name === promptName)
    if (index === -1) {
      throw new Error(`Local prompt "${promptName}" not found`)
    }

    // If name is changing, validate the new name
    if (promptData.name && promptData.name !== promptName) {
      const nameValidation = validatePromptName(
        promptData.name,
        state.serverPrompts,
        promptName
      )
      if (!nameValidation.valid) {
        throw new Error(nameValidation.error)
      }

      // Check for duplicate in other local prompts
      const duplicateLocal = state.localPrompts.find(
        (p) => p.name === promptData.name && p.name !== promptName
      )
      if (duplicateLocal) {
        throw new Error(
          `A local prompt with name "${promptData.name}" already exists`
        )
      }
    }

    // Update the prompt
    const updatedPrompts = [...state.localPrompts]
    updatedPrompts[index] = {
      ...updatedPrompts[index],
      ...promptData,
      updatedAt: new Date().toISOString(),
    }

    set({ localPrompts: updatedPrompts })

    return updatedPrompts[index]
  },

  // Delete a local prompt
  deleteLocalPrompt: (promptName) => {
    const state = get()

    const filtered = state.localPrompts.filter((p) => p.name !== promptName)
    if (filtered.length === state.localPrompts.length) {
      throw new Error(`Local prompt "${promptName}" not found`)
    }

    set({ localPrompts: filtered })
  },

  // Get a local prompt by name
  getLocalPrompt: (promptName) => {
    const state = get()
    return state.localPrompts.find((p) => p.name === promptName)
  },

  // Export local prompts as YAML
  exportLocalPromptsAsYAML: () => {
    const state = get()
    return state.localPrompts.map((prompt) => {
      // Remove metadata fields for clean export
      const { createdAt, updatedAt, source, ...cleanPrompt } = prompt
      return promptToYAML(cleanPrompt)
    })
  },
})
