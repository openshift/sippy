package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
)

// RegisterTools registers all available MCP tools with the server
func RegisterTools(mcpServer *server.MCPServer, dbClient *db.DB, bigQueryClient *bigquery.Client, cacheClient cache.Cache) {
	// Create tool dependencies
	deps := &ToolDependencies{
		DBClient:       dbClient,
		BigQueryClient: bigQueryClient,
		CacheClient:    cacheClient,
	}

	// Register all tools
	tools := []MCPTool{
		NewReleasesTool(deps),
		NewHealthTool(deps), // Example tool demonstrating the pattern
		// Add new tools here following the same pattern
	}

	for _, tool := range tools {
		mcpServer.AddTool(tool.GetDefinition(), tool.GetHandler())
		log.WithField("tool", tool.GetDefinition().Name).Info("Registered MCP tool")
	}
}

// ToolDependencies holds common dependencies that tools may need
type ToolDependencies struct {
	DBClient       *db.DB
	BigQueryClient *bigquery.Client
	CacheClient    cache.Cache
}

// MCPTool defines the interface that all MCP tools must implement
type MCPTool interface {
	// GetDefinition returns the MCP tool definition
	GetDefinition() mcp.Tool

	// GetHandler returns the tool's request handler function
	GetHandler() func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// BaseTool provides common functionality for MCP tools
type BaseTool struct {
	deps *ToolDependencies
}

// NewBaseTool creates a new base tool with the given dependencies
func NewBaseTool(deps *ToolDependencies) *BaseTool {
	return &BaseTool{deps: deps}
}

// GetDependencies returns the tool dependencies
func (bt *BaseTool) GetDependencies() *ToolDependencies {
	return bt.deps
}

// CreateJSONResponse is a helper function to create a JSON response
func (bt *BaseTool) CreateJSONResponse(data interface{}) (*mcp.CallToolResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("error marshaling response: %w", err)
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

// CreateTextResponse is a helper function to create a plain text response
func (bt *BaseTool) CreateTextResponse(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: text,
			},
		},
	}
}

// CreateErrorResponse is a helper function to create an error response
func (bt *BaseTool) CreateErrorResponse(err error) (*mcp.CallToolResult, error) {
	return nil, err
}
