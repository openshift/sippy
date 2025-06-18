package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/filter"
)

// RegisterJobInfoTool registers the get_job_info MCP tool with the server
func RegisterJobInfoTool(mcpServer *server.MCPServer, dbClient *db.DB) {
	jobInfoTool := mcp.Tool{
		Name:        "get_job_info",
		Description: "Get information about a specific CI job including current and previous run statistics",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"job_name": map[string]any{
					"type":        "string",
					"description": "The name of the CI job to get information about",
				},
			},
			Required: []string{"job_name"},
		},
	}

	jobInfoHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Debug("Handling get_job_info tool call")

		// Parse arguments using helper methods
		jobName, err := request.RequireString("job_name")
		if err != nil {
			return nil, fmt.Errorf("job_name is required: %w", err)
		}

		// Use filter options to search for the specific job
		filterOpts := &filter.FilterOptions{
			Filter: &filter.Filter{
				Items: []filter.FilterItem{
					{
						Field:    "name",
						Operator: filter.OperatorContains,
						Value:    jobName,
					},
				},
			},
		}

		// Get current time for report bounds
		reportEnd := time.Now()
		start := reportEnd.Add(-14 * 24 * time.Hour)   // Last 14 days
		boundary := reportEnd.Add(-7 * 24 * time.Hour) // Last 7 days boundary
		end := reportEnd

		// Query across all releases by passing empty string (API now supports this)
		jobs, err := api.JobReportsFromDB(dbClient, "", "", filterOpts, start, boundary, end, reportEnd)
		if err != nil {
			log.WithError(err).Error("error querying job reports")
			return nil, fmt.Errorf("error querying job reports: %w", err)
		}

		// Filter for exact job name match
		var matchedJob *apitype.Job
		for _, job := range jobs {
			if job.Name == jobName {
				matchedJob = &job
				break
			}
		}

		if matchedJob == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("No job found with name: %s", jobName),
					},
				},
			}, nil
		}

		// Marshal to JSON for response
		jsonData, err := json.Marshal(matchedJob)
		if err != nil {
			return nil, fmt.Errorf("error marshaling job info response: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(jsonData),
				},
			},
		}, nil
	}

	mcpServer.AddTool(jobInfoTool, jobInfoHandler)
	log.Info("Registered get_job_info MCP tool")
}
