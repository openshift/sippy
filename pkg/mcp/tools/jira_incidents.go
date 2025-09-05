package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

// JiraIncidentResult represents a JIRA incident for MCP response
type JiraIncidentResult struct {
	Key            string     `json:"key"`
	Summary        string     `json:"summary"`
	StartTime      *time.Time `json:"start_time"`
	ResolutionTime *time.Time `json:"resolution_time"`
	IsResolved     bool       `json:"is_resolved"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// RegisterJiraIncidentsTools registers JIRA incident related tools
func RegisterJiraIncidentsTools(mcpServer *server.MCPServer, dbClient *db.DB) {
	// Register the query JIRA incidents tool
	queryTool := mcp.NewTool("query_jira_incidents",
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

	mcpServer.AddTool(queryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleQueryJiraIncidents(ctx, request, dbClient)
	})

	log.Debug("Registered JIRA incidents tools")
}

// handleQueryJiraIncidents handles the query_jira_incidents tool call
func handleQueryJiraIncidents(ctx context.Context, request mcp.CallToolRequest, dbClient *db.DB) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Parse optional parameters
	var createdAfter, createdBefore *time.Time
	var unresolvedOnly bool
	limit := 100 // default limit

	// Parse created_after
	if createdAfterStr, ok := args["created_after"].(string); ok && createdAfterStr != "" {
		if t, err := time.Parse(time.RFC3339, createdAfterStr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid created_after format: %v. Use RFC3339 format like '2024-01-01T00:00:00Z'", err)), nil
		} else {
			createdAfter = &t
		}
	}

	// Parse created_before
	if createdBeforeStr, ok := args["created_before"].(string); ok && createdBeforeStr != "" {
		if t, err := time.Parse(time.RFC3339, createdBeforeStr); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid created_before format: %v. Use RFC3339 format like '2024-12-31T23:59:59Z'", err)), nil
		} else {
			createdBefore = &t
		}
	}

	// Parse unresolved_only
	if unresolvedOnlyVal, ok := args["unresolved_only"].(bool); ok {
		unresolvedOnly = unresolvedOnlyVal
	}

	// Parse limit
	if limitVal, ok := args["limit"].(float64); ok {
		limit = int(limitVal)
		if limit <= 0 {
			limit = 100
		} else if limit > 1000 {
			limit = 1000
		}
	}

	// Build the database query
	query := dbClient.DB.Model(&models.JiraIncident{})

	// Apply filters
	if createdAfter != nil {
		query = query.Where("created_at >= ?", *createdAfter)
	}
	if createdBefore != nil {
		query = query.Where("created_at <= ?", *createdBefore)
	}
	if unresolvedOnly {
		query = query.Where("resolution_time IS NULL")
	}

	// Apply limit and ordering
	query = query.Order("created_at DESC").Limit(limit)

	// Execute the query
	var incidents []models.JiraIncident
	if err := query.Find(&incidents).Error; err != nil {
		log.WithError(err).Error("Failed to query JIRA incidents")
		return mcp.NewToolResultError(fmt.Sprintf("Database query failed: %v", err)), nil
	}

	// Convert to result format
	results := make([]JiraIncidentResult, len(incidents))
	for i, incident := range incidents {
		results[i] = JiraIncidentResult{
			Key:            incident.Key,
			Summary:        incident.Summary,
			StartTime:      incident.StartTime,
			ResolutionTime: incident.ResolutionTime,
			IsResolved:     incident.ResolutionTime != nil,
			CreatedAt:      incident.CreatedAt,
			UpdatedAt:      incident.UpdatedAt,
		}
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.WithError(err).Error("Failed to marshal JIRA incidents to JSON")
		return mcp.NewToolResultError(fmt.Sprintf("Failed to format results: %v", err)), nil
	}

	// Create summary message
	summary := fmt.Sprintf("Found %d JIRA incidents", len(results))
	if createdAfter != nil || createdBefore != nil || unresolvedOnly {
		summary += " (filtered)"
	}
	if unresolvedOnly {
		unresolvedCount := 0
		for _, result := range results {
			if !result.IsResolved {
				unresolvedCount++
			}
		}
		summary += fmt.Sprintf(" - %d unresolved", unresolvedCount)
	}

	return mcp.NewToolResultText(fmt.Sprintf("%s:\n\n%s", summary, string(jsonData))), nil
}
