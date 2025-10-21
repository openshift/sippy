# Prompts

This directory contains prompt definitions in YAML format. These prompts
can be used by the frontend to perform specific actions (drafting a jira
card, etc.)

*TODO*: These can also be exposed as MCP tools in the future, so a tool
like `Claude Code` could consume our prompts, e.g. to produce a payload
report.

## Prompt YAML Format

Each prompt is defined in a separate YAML file with the following structure:

```yaml
# Prompt name (required) - used to identify the prompt
name: prompt-name

# Description (required) - explains what the prompt does
description: A description of what this prompt does

# Hide (optional) - whether to hide this prompt from the UI slash command list
# Default: false. Set to true to hide from "/" command list while still being usable programmatically
hide: false

# Arguments (optional) - list of parameters the prompt accepts
arguments:
  - name: argument_name
    description: What this argument is for
    required: true  # or false
    type: string  # or array
    autocomplete: field_name  # optional, references /api/autocomplete/{field_name}
    default: value  # optional, default value for this argument

# Prompt (required) - the Jinja2 template
prompt: |
  The prompt text here.
  Use {{ argument_name }} for argument substitution.
  Arrays can be formatted: {{ argument_name | join(', ') }}
```

## Templating with Jinja2

Prompts use **Jinja2 templating** for argument substitution and formatting.

### Basic Substitution

```jinja2
Analyze the job: {{ job_url_or_id }}
```

### Formatting Arrays

```jinja2
Versions: {{ releases | join(', ') }}
Streams: {{ streams | join(' and ') }}
```

### Default Values

Defaults are defined in the arguments section (not inline in the template):

```yaml
arguments:
  - name: streams
    type: array
    default: ["nightly", "ci"]

prompt: |
  Streams: {{ streams | join(', ') }}
```

When the prompt is rendered:
- If the argument is provided, its value is used
- If not provided, the default from the arguments section is used
- Defaults in the arguments section are also exposed to the UI for pre-filling form fields

### Conditionals (optional)

```jinja2
{% if detailed_analysis %}
Perform a detailed analysis including full logs.
{% endif %}
```

### Jinja2 Features Available

- Variables: `{{ variable }}`
- Filters: `{{ list | join(', ') }}`, `{{ text | upper }}`
- Conditionals: `{% if condition %}...{% endif %}`
- Loops: `{% for item in items %}...{% endfor %}`
- See [Jinja2 documentation](https://jinja.palletsprojects.com/) for full reference

### Hierarchical Organization

Prompts can be organized into subdirectories for better structure:

```
prompts/
├── component-readiness/
│   └── test-regression.yaml      # name: component-readiness-regression-analysis
├── test-analysis.yaml             # name: test-analysis
└── payload-report.yaml            # name: payload-report
```
