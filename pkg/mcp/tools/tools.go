package tools

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/openshift/sippy/pkg/db"
)

// RegisterTools registers all MCP tools with the server
func RegisterTools(mcpServer *server.MCPServer, dbClient *db.DB) {
	RegisterJiraIncidentsTools(mcpServer, dbClient)
}
