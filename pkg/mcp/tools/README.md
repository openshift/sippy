# Sippy MCP Tools

This directory contains Model Context Protocol (MCP) tools for the Sippy application. These tools allow AI agents to interact with Sippy's database and functionality through a standardized protocol.

## Available Tools

### query_jira_incidents

Query JIRA incidents from the database with optional filtering capabilities.

**Parameters:**
- `created_after` (string, optional): Filter incidents created after this date (RFC3339 format, e.g., '2024-01-01T00:00:00Z')
- `created_before` (string, optional): Filter incidents created before this date (RFC3339 format, e.g., '2024-12-31T23:59:59Z')
- `unresolved_only` (boolean, optional): If true, only return unresolved incidents (those without resolution_time)
- `limit` (number, optional): Maximum number of incidents to return (default: 100, max: 1000)

**Example Usage:**

```json
{
  "tool": "query_jira_incidents",
  "arguments": {
    "unresolved_only": true,
    "limit": 50
  }
}
```

**Response Format:**
Returns a JSON array of incident objects with the following structure:
```json
[
  {
    "key": "TRT-162",
    "summary": "Description of the incident",
    "start_time": "2024-01-15T10:30:00Z",
    "resolution_time": null,
    "is_resolved": false,
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
]
```

## Usage

The MCP tools are automatically registered when the Sippy server starts with a database connection. They can be accessed through the MCP endpoint at `/mcp/v1/`.

## Development

To add new MCP tools:

1. Create a new file in this directory (e.g., `new_tool.go`)
2. Implement the tool using the `mcp.NewTool()` function
3. Create a handler function that implements the tool logic
4. Register the tool in the `RegisterTools()` function in `tools.go`
5. Add tests for your tool

## Testing

Run tests for the MCP tools:

```bash
go test ./pkg/mcp/tools/
```

## Dependencies

- `github.com/mark3labs/mcp-go` - MCP Go implementation
- `github.com/openshift/sippy/pkg/db` - Sippy database models and client
