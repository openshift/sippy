# Sippy Agent Tools

This directory contains all the tools available to the Sippy AI Agent for analyzing CI/CD pipelines and job failures.

## Directory Structure

```
tools/
├── __init__.py                 # Package exports
├── README.md                   # This file
├── base_tool.py               # Base classes for all tools
├── sippy_job_summary.py       # Job run summary tool
├── sippy_log_analyzer.py      # Log analysis tool
├── jira_incidents.py          # Jira incident tracking tool
├── release_payloads.py        # OpenShift release payload tool
├── payload_details.py         # Detailed payload analysis tool
├── sippy_releases.py          # OpenShift release information tool
├── junit_parser.py            # JUnit XML parser for test failures and flakes
├── placeholder_tools.py       # Placeholder tools for future features
├── test_analysis_helpers.py   # Helper functions for test failure analysis
└── log_analysis_helpers.py    # Helper functions for log pattern analysis
```

## Tool Categories

### Core Sippy Tools
- **SippyProwJobSummaryTool** (`sippy_job_summary.py`): Gets comprehensive job run summaries including timing, results, test failures, and configuration details
- **SippyLogAnalyzerTool** (`sippy_log_analyzer.py`): Analyzes job artifacts and logs for error patterns, with intelligent categorization of issues

### Release Management Tools
- **SippyReleasePayloadTool** (`release_payloads.py`): Queries OpenShift release controller API for payload information, including nightly and CI streams with status tracking
- **SippyPayloadDetailsTool** (`payload_details.py`): Gets detailed information about specific payloads including blocking jobs, PRs, upgrade results, and failure analysis

### External Integration Tools
- **SippyJiraIncidentTool** (`jira_incidents.py`): Queries Jira for known open incidents in the TRT project to correlate with job failures

### Test Analysis Tools
- **JUnitParserTool** (`junit_parser.py`): Parses JUnit XML files from URLs to extract test failures and flakes with intelligent flake detection. Also handles aggregated job results embedded as YAML in JUnit XML system-out sections.
- **AggregatedJobAnalyzerTool** (`aggregated_job_analyzer.py`): Gets direct URLs to aggregated test results for jobs that start with 'aggregated-'
- **AggregatedYAMLParserTool** (`aggregated_yaml_parser.py`): Parses aggregated test results in pure YAML format with underlying job links

### Utility Tools
- **ExampleTool** (`base_tool.py`): Simple example tool for testing and demonstration

### Placeholder Tools
- **SippyJobAnalysisTool** (`placeholder_tools.py`): Placeholder for future job analysis features
- **SippyTestFailureTool** (`placeholder_tools.py`): Placeholder for future test failure analysis features

## Special Job Types

### Aggregated Jobs
Jobs that start with "aggregated-" are statistical aggregations that run multiple instances (typically 10) of the same test. These jobs have special handling:

1. **Detection**: The job summary tool automatically detects aggregated jobs by name prefix
2. **Analysis Workflow**:
   - Use `get_aggregated_results_url` to get the junit-aggregated.xml URL
   - Use `parse_junit_xml` with that URL to extract YAML data embedded in `<system-out>` sections
   - The parser shows passing/failing underlying jobs with direct links
   - Only analyze individual underlying jobs if specifically requested for deep analysis

3. **Data Format**: Aggregated results are stored as YAML within JUnit XML `<system-out>` sections, containing:
   - Test suite name and summary with historical pass rates
   - Arrays of passing, failing, and skipped job runs
   - Each job entry includes `jobrunid`, `humanurl`, and `gcsartifacturl`

## Helper Modules

### Test Analysis Helpers (`test_analysis_helpers.py`)
Functions for analyzing test failure patterns:
- `analyze_test_failures()`: Categorizes test failures by sig and error patterns
- `extract_test_category()`: Extracts test categories from test names
- `clean_failure_message()`: Cleans and formats failure messages
- `generate_test_insights()`: Provides insights based on failure patterns

### Log Analysis Helpers (`log_analysis_helpers.py`)
Functions for analyzing log patterns and errors:
- `analyze_error_patterns()`: Categorizes errors by type (operator, installation, network, etc.)
- `format_log_analysis()`: Formats log analysis results for display

## Base Classes

### SippyBaseTool (`base_tool.py`)
Abstract base class that all Sippy tools inherit from. Provides:
- Consistent interface for tool execution
- Pydantic-based input validation
- Async support
- Error handling patterns

### SippyToolInput (`base_tool.py`)
Base input schema for all Sippy tools using Pydantic for validation.

## Adding New Tools

To add a new tool:

1. Create a new file in this directory (e.g., `my_new_tool.py`)
2. Import the base classes:
   ```python
   from .base_tool import SippyBaseTool, SippyToolInput
   ```
3. Define your tool class:
   ```python
   class MyNewTool(SippyBaseTool):
       name: str = "my_new_tool"
       description: str = "Description of what this tool does"
       
       class MyInput(SippyToolInput):
           param: str = Field(description="Parameter description")
       
       args_schema: Type[SippyToolInput] = MyInput
       
       def _run(self, param: str) -> str:
           # Implement your tool logic here
           return f"Result for {param}"
   ```
4. Add the import to `__init__.py`
5. Add the tool to the agent in `../agent.py`

## Tool Design Principles

1. **Single Responsibility**: Each tool should have a clear, focused purpose
2. **Consistent Interface**: All tools inherit from `SippyBaseTool` for consistency
3. **Input Validation**: Use Pydantic schemas for robust input validation
4. **Error Handling**: Provide clear error messages and graceful failure handling
5. **Documentation**: Include clear descriptions and parameter documentation
6. **Modularity**: Break complex functionality into helper modules

## Configuration

Tools that need external API access (like Sippy API or Jira) receive configuration through their constructor from the main agent configuration. This allows for:
- Environment-based configuration
- Easy testing with mock endpoints
- Consistent credential management

## Testing

When adding new tools, consider:
- Unit tests for the tool logic
- Integration tests with actual APIs (when appropriate)
- Mock tests for external dependencies
- Error condition testing
