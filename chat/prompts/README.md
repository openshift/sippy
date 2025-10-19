# MCP Prompts

This directory contains prompt definitions in YAML format that are automatically exposed as MCP tools. Each prompt is loaded by the MCP server at startup and registered as a callable tool.

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
    type: string  # or array
    autocomplete: field_name  # optional, references /api/autocomplete/{field_name}

# Messages (required) - list of messages that make up the prompt
messages:
  - role: user  # or assistant, system
    content: |
      The prompt text here.
      Use {argument_name} for argument substitution.
```

## How Prompts Work

When a prompt YAML file is loaded, it's exposed in TWO ways:

### 1. As an MCP Tool (for direct execution)
The MCP server creates a tool with the same name as the prompt (with hyphens converted to underscores).

When the tool is called:
1. Arguments are substituted into the prompt template
2. The rendered prompt is sent to the Sippy agent
3. The agent's response is returned to the caller

**Use case:** Direct programmatic execution, automated workflows

### 2. As an MCP Prompt (slash command for LLMs)
The MCP server also creates a prompt with the original name from the YAML.

When the prompt is retrieved:
1. It returns instructions telling an LLM to use the corresponding tool
2. The instructions include the parameter values that were provided
3. The LLM can then decide to call the tool with those arguments

**Use case:** LLM-driven interactions where the LLM needs to understand what tools are available

## Argument Substitution

Arguments can be embedded in message content using curly braces: `{argument_name}`

You can also specify default values: `{argument_name|default:default_value}`

When the tool is called, argument values replace the placeholders:
- If an argument is provided, its value is used
- If an argument is not provided but has a default, the default is used
- Array arguments are converted to comma-separated strings

## Example

See `hello-world.yaml` for a complete example:

```yaml
name: hello-world
description: A simple hello world prompt for testing
arguments:
  - name: topic
    description: The topic to ask Sippy about
    required: true
    type: string
messages:
  - role: user
    content: |
      Hello! Can you tell me about {topic}?
```

## Adding New Prompts

1. Create a new `.yaml` file in this directory
2. Define your prompt following the format above
3. Restart the Sippy Chat MCP server to load the new prompt
4. The prompt will be automatically registered as an MCP tool

For example, if you create `my-analysis.yaml` with name `my-analysis`, it will be available as the MCP tool `my_analysis`.

## Testing Prompts

### Testing as a Tool (direct execution)
Use an MCP client to call the tool directly:

```json
{
  "method": "tools/call",
  "params": {
    "name": "test_analysis",
    "arguments": {
      "test_name": "my-test",
      "release": "4.20",
      "variants": ["Platform:gcp"],
      "days": "7"
    }
  }
}
```

This will execute the prompt immediately and return the agent's analysis.

### Testing as a Prompt (slash command for LLM)
Use an MCP client to retrieve the prompt:

```json
{
  "method": "prompts/get",
  "params": {
    "name": "test-analysis",
    "arguments": {
      "test_name": "my-test",
      "release": "4.20"
    }
  }
}
```

This will return instructions like:
```
Use the test_analysis tool to analyze why a specific test is failing...

Call it with the following arguments:
  - test_name: my-test
  - release: 4.20
  - variants: <optional, not provided>
  - days: <optional, not provided>
```

The LLM can then use these instructions to understand when and how to call the tool.

