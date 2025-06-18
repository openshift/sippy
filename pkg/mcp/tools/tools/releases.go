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
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
)

// RegisterReleasesTool registers the get_releases MCP tool with the server
func RegisterReleasesTool(mcpServer *server.MCPServer, dbClient *db.DB, bigQueryClient *bigquery.Client) {
	releasesTool := mcp.Tool{
		Name:        "get_releases",
		Description: "Get all OpenShift releases with their GA dates, development start dates, and capabilities",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
		},
	}

	releasesHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Debug("Handling get_releases tool call")

		// Get releases from BigQuery (never force refresh for MCP)
		releases, err := api.GetReleases(ctx, bigQueryClient, false)
		if err != nil {
			log.WithError(err).Error("error querying releases")
			return nil, fmt.Errorf("error querying releases: %w", err)
		}

		// Get last updated time from database if available
		var lastUpdated time.Time
		if dbClient != nil && dbClient.DB != nil {
			type LastUpdatedQuery struct {
				Max time.Time
			}
			var result LastUpdatedQuery
			// Assume our last update is the last time we inserted a prow job run.
			if err := dbClient.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&result).Error; err == nil {
				lastUpdated = result.Max
			}
		}

		// Build response using shared function
		response := api.BuildReleasesResponse(releases, lastUpdated)

		// Marshal to JSON for response
		jsonData, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("error marshaling releases response: %w", err)
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

	mcpServer.AddTool(releasesTool, releasesHandler)
	log.Info("Registered get_releases MCP tool")
}

