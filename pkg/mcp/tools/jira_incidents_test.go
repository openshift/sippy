package tools

import (
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func TestJiraIncidentResult_Structure(t *testing.T) {
	// Test that the JiraIncidentResult struct has the expected fields
	now := time.Now()
	result := JiraIncidentResult{
		Key:            "TRT-001",
		Summary:        "Test incident",
		StartTime:      &now,
		ResolutionTime: nil,
		IsResolved:     false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	assert.Equal(t, "TRT-001", result.Key)
	assert.Equal(t, "Test incident", result.Summary)
	assert.NotNil(t, result.StartTime)
	assert.Nil(t, result.ResolutionTime)
	assert.False(t, result.IsResolved)
}

func TestToolRegistration(t *testing.T) {
	// Test that the tool can be created without errors
	tool := mcp.NewTool("query_jira_incidents",
		mcp.WithDescription("Query JIRA incidents from the database with optional filtering"),
		mcp.WithString("created_after",
			mcp.Description("Filter incidents created after this date (RFC3339 format, e.g., '2024-01-01T00:00:00Z')"),
		),
		mcp.WithString("created_before",
			mcp.Description("Filter incidents created before this date (RFC3339 format, e.g., '2024-12-31T23:59:59Z')"),
		),
		mcp.WithBoolean("unresolved_only",
			mcp.Description("If true, only return unresolved incidents (those without resolution_time)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of incidents to return (default: 100, max: 1000)"),
		),
	)

	assert.Equal(t, "query_jira_incidents", tool.Name)
	assert.Contains(t, tool.Description, "Query JIRA incidents")
	assert.NotNil(t, tool.InputSchema)
}
