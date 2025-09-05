package mcp

import (
	"context"
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/apis/cache"
	"github.com/openshift/sippy/pkg/bigquery"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/mcp/tools"
)

type MCPServer struct {
	mcpServer *server.MCPServer
	streamSrv *server.StreamableHTTPServer
}

func NewMCPServer(ctx context.Context, sippyServer *http.Server, dbClient *db.DB, bigQueryClient *bigquery.Client, cacheClient cache.Cache) *MCPServer {
	hooks := &server.Hooks{}

	hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		log.WithField("session_id", session.SessionID()).Info("MCP client session registered")
	})

	hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		log.WithField("session_id", session.SessionID()).Info("MCP client session unregistered")
	})

	mcpServer := server.NewMCPServer(
		"Sippy MCP Server",
		"0.0.1",
		server.WithLogging(),
		server.WithToolCapabilities(true),
		server.WithPromptCapabilities(false),
		server.WithRecovery(),
		server.WithHooks(hooks),
	)
	log.Debug("Created MCP server instance")

	// Register tools
	if dbClient != nil {
		tools.RegisterTools(mcpServer, dbClient, bigQueryClient, cacheClient)
		log.Debug("Registered MCP tools")
	}

	streamSrv := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithStreamableHTTPServer(sippyServer),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context { return ctx }),
	)

	return &MCPServer{
		mcpServer: mcpServer,
		streamSrv: streamSrv,
	}
}

func (m *MCPServer) Handler() http.Handler {
	return m.streamSrv
}
