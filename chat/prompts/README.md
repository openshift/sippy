# MCP Prompts

This directory contains MCP prompt definitions in YAML format. Prompts are automatically loaded by the MCP server at startup.

## Prompt YAML Format

Each prompt is defined in a separate YAML file with the following structure:

```yaml
# Prompt name (required) - used to identify the prompt
name: prompt-name

# Description (required) - explains what the prompt does
description: A description of what this prompt does

# Arguments (optional) - list of parameters the prompt accepts
arguments:
  - name: argument_name
    description: What this argument is for
    required: true  # or false

# Messages (required) - list of messages that make up the prompt
messages:
  - role: user  # or assistant, system
    content: |
      The prompt text here.
      Use {argument_name} for argument substitution.
```

## Argument Substitution

Arguments can be embedded in message content using curly braces: `{argument_name}`

When the prompt is retrieved via MCP, the provided argument values will replace the placeholders.

## Example

See `hello-world.yaml` for a complete example:

```yaml
name: hello-world
description: A simple hello world prompt for testing
arguments:
  - name: topic
    description: The topic to ask Sippy about
    required: true
messages:
  - role: user
    content: |
      Hello! Can you tell me about {topic}?
```

## Adding New Prompts

1. Create a new `.yaml` file in this directory
2. Define your prompt following the format above
3. Restart the Sippy Chat server to load the new prompt
4. The prompt will be automatically available via the MCP server

## Testing Prompts

You can test prompts by:
1. Using an MCP client to call `prompts/list` to see all available prompts
2. Using `prompts/get` with the prompt name and arguments to retrieve the rendered prompt

