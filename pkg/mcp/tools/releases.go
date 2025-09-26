package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api"
)

// ReleasesTool implements the get_releases MCP tool
type ReleasesTool struct {
	*BaseTool
}

// NewReleasesTool creates a new releases tool instance
func NewReleasesTool(deps *ToolDependencies) *ReleasesTool {
	return &ReleasesTool{
		BaseTool: NewBaseTool(deps),
	}
}

// GetDefinition returns the MCP tool definition for the releases tool
func (rt *ReleasesTool) GetDefinition() mcp.Tool {
	return mcp.Tool{
		Name:        "get_releases",
		Description: "Get current and past OpenShift release information including GA dates, development start dates, etc. You can use this to determine the current development release (the newest one without a GA date), or to determine the GA date of previous releases.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			// No properties needed - this tool takes no parameters
		},
	}
}

// GetHandler returns the request handler for the releases tool
func (rt *ReleasesTool) GetHandler() func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Debug("Handling get_releases tool call")

		// Get releases from BigQuery (never force refresh for MCP)
		releases, err := api.GetReleases(ctx, rt.deps.BigQueryClient, false)
		if err != nil {
			log.WithError(err).Error("error querying releases")
			return rt.CreateErrorResponse(fmt.Errorf("error querying releases: %w", err))
		}

		// Get last updated time from database if available
		var lastUpdated time.Time
		if rt.deps.DBClient != nil && rt.deps.DBClient.DB != nil {
			type LastUpdatedQuery struct {
				Max time.Time
			}
			var result LastUpdatedQuery
			// Assume our last update is the last time we inserted a prow job run.
			if err := rt.deps.DBClient.DB.Raw("SELECT MAX(created_at) FROM prow_job_runs").Scan(&result).Error; err == nil {
				lastUpdated = result.Max
			}
		}

		// Build response using shared function
		response := api.BuildReleasesResponse(releases, lastUpdated)

		// Return JSON response
		return rt.CreateJSONResponse(response)
	}
}
