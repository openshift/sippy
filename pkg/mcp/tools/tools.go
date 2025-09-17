package tools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/db"
)

// RegisterTools registers all available MCP tools with the server
func RegisterTools(mcpServer *server.MCPServer, dbClient *db.DB) {
	// Register Jira incident tool
	jiraTool := NewJiraIncidentTool(dbClient)
	tool := jiraTool.Definition()
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jiraTool.Call(ctx, request.GetArguments())
	}
	mcpServer.AddTool(tool, handler)
	log.Debug("registered Jira incident tool")
}
