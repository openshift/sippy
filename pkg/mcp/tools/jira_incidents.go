package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

// JiraIncidentTool provides access to known open TRT incidents from the database
type JiraIncidentTool struct {
	dbClient *db.DB
}

// NewJiraIncidentTool creates a new Jira incident tool
func NewJiraIncidentTool(dbClient *db.DB) *JiraIncidentTool {
	return &JiraIncidentTool{
		dbClient: dbClient,
	}
}

// Definition returns the tool definition for MCP
func (j *JiraIncidentTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name:        "check_known_incidents",
		Description: "Check for known open TRT incidents in Sippy's database. ONLY use this when job errors suggest a correlation. Use specific keywords that match actual errors found in logs.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"search_terms": map[string]interface{}{
					"type":        "string",
					"description": "Optional search terms to filter incidents (e.g., 'registry', 'build11', 'timeout')",
				},
			},
		},
	}
}

// Call executes the tool with the given arguments
func (j *JiraIncidentTool) Call(ctx context.Context, args map[string]interface{}) (*mcp.CallToolResult, error) {
	var searchTerms string
	if terms, ok := args["search_terms"].(string); ok {
		searchTerms = terms
	}

	log.WithField("search_terms", searchTerms).Debug("checking known incidents")

	// Query open incidents from database
	incidents, err := j.queryOpenIncidents(ctx, searchTerms)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("Error querying incidents: %v", err),
				},
			},
		}, nil
	}

	// Format response
	response := j.formatIncidents(incidents, searchTerms)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: response,
			},
		},
	}, nil
}

// queryOpenIncidents retrieves open TRT incidents from the database
func (j *JiraIncidentTool) queryOpenIncidents(ctx context.Context, searchTerms string) ([]models.JiraIncident, error) {
	var incidents []models.JiraIncident

	// Base query for open incidents (no resolution time)
	query := j.dbClient.DB.WithContext(ctx).Where("resolution_time IS NULL")

	// Apply search terms if provided
	if searchTerms != "" {
		searchTerms = strings.TrimSpace(searchTerms)
		if searchTerms != "" {
			// Create a search condition for summary and key fields
			searchPattern := fmt.Sprintf("%%%s%%", searchTerms)
			query = query.Where("summary ILIKE ? OR key ILIKE ?", searchPattern, searchPattern)

			log.WithField("search_pattern", searchPattern).Debug("applying search filter")
		}
	}

	// Order by most recent first, limit results
	if err := query.Order("start_time DESC").Limit(20).Find(&incidents).Error; err != nil {
		return nil, fmt.Errorf("failed to query incidents: %w", err)
	}

	return incidents, nil
}

// formatIncidents formats the incidents for display
func (j *JiraIncidentTool) formatIncidents(incidents []models.JiraIncident, searchTerms string) string {
	if len(incidents) == 0 {
		if searchTerms != "" {
			return fmt.Sprintf("No open TRT incidents found matching search terms: %s", searchTerms)
		}
		return "No open TRT incidents found in database"
	}

	result := "**Known Open Incidents**\n\n"
	if searchTerms != "" {
		result += fmt.Sprintf("**Search Terms:** %s\n", searchTerms)
	}
	result += fmt.Sprintf("**Found %d incidents:**\n\n", len(incidents))

	for _, incident := range incidents {
		result += fmt.Sprintf("**🎫 %s** - %s\n", incident.Key, incident.Summary)

		// Format dates
		if incident.StartTime != nil {
			startDate := incident.StartTime.Format("2006-01-02")
			daysSince := int(time.Since(*incident.StartTime).Hours() / 24)
			result += fmt.Sprintf("📅 Created: %s (%d days ago)\n", startDate, daysSince)
		}

		// Add Jira link
		result += fmt.Sprintf("🔗 [View in Jira](https://issues.redhat.com/browse/%s)\n\n", incident.Key)
	}

	return result
}
