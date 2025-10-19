# Component Readiness Prompts

This directory contains prompts specifically for Component Readiness workflows.

## Available Prompts

### regression-analysis
Performs a comprehensive analysis of a Component Readiness test regression, including:
- Regression overview with status codes and timeline
- Statistical analysis of pass rate changes
- Root cause investigation using failed job runs
- Failure pattern analysis
- Triage status and recommendations

**Usage:**
```
/component-readiness-regression-analysis url=<test_details_url>
```

### jira-description
Generates a Jira bug description for a Component Readiness test regression. This prompt is used by the "File a new bug" dialog to create AI-enhanced bug descriptions.

The prompt:
- Fetches test details and regression data
- Analyzes failure patterns across multiple jobs
- Generates properly formatted Jira markup
- Includes statistics, failure outputs, and relevant links

**Usage:**
```
/component-readiness-jira-description test_name=<test_name> url=<test_details_url>
```

**Integration:**
This prompt is automatically invoked when a user clicks the "Generate AI-enhanced Description" button in the File Bug dialog on test details pages.

## Jira Markup Guidelines

When creating Jira descriptions, the prompts use Jira's markup syntax:
- Links: `[link text|url]`
- Code blocks: `{code:none}text{code}`
- Panels: `{panel:title=Title}content{panel}`
- Bold: `*text*`
- Headings: `h3. Heading Text`

The AI is instructed to output ONLY Jira markup with no preamble or explanations, making the output directly pastable into Jira tickets.

