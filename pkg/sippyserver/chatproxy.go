package sippyserver

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

// rewriteChatPath rewrites /api/chat paths to /chat for the target service
func rewriteChatPath(path string) string {
	// Only replace /api/chat when it's followed by end of string or a slash
	if path == "/api/chat" {
		return "/chat"
	}
	if strings.HasPrefix(path, "/api/chat/") {
		return "/chat" + strings.TrimPrefix(path, "/api/chat")
	}
	return path
}

// ChatProxy handles proxying HTTP and WebSocket requests to the sippy-chat service
type ChatProxy struct {
	chatAPIURL string
	httpProxy  *httputil.ReverseProxy
	wsUpgrader websocket.Upgrader
}

// NewChatProxy creates a new chat proxy instance
func NewChatProxy(chatAPIURL string) (*ChatProxy, error) {
	targetURL, err := url.Parse(chatAPIURL)
	if err != nil {
		return nil, err
	}

	// Create HTTP reverse proxy
	httpProxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Modify the director to handle the path rewriting
	originalDirector := httpProxy.Director
	httpProxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Rewrite /api/chat paths for the target service
		req.URL.Path = rewriteChatPath(req.URL.Path)
		req.Host = targetURL.Host
	}

	// Configure WebSocket upgrader
	wsUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// Allow all origins for now - in production you might want to be more restrictive
			return true
		},
	}

	return &ChatProxy{
		chatAPIURL: chatAPIURL,
		httpProxy:  httpProxy,
		wsUpgrader: wsUpgrader,
	}, nil
}

// ServeHTTP handles both HTTP and WebSocket requests
func (cp *ChatProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if this is a WebSocket upgrade request
	if isWebSocketUpgrade(r) {
		cp.handleWebSocket(w, r)
		return
	}

	// Handle regular HTTP request
	cp.httpProxy.ServeHTTP(w, r)
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade request
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Connection")) == "upgrade" &&
		strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}

// handleWebSocket handles WebSocket proxy connections
func (cp *ChatProxy) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Parse target URL
	targetURL, err := url.Parse(cp.chatAPIURL)
	if err != nil {
		log.WithError(err).Error("Failed to parse chat API URL")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Build target WebSocket URL
	wsScheme := "ws"
	if targetURL.Scheme == "https" {
		wsScheme = "wss"
	}

	targetPath := rewriteChatPath(r.URL.Path)
	targetWSURL := wsScheme + "://" + targetURL.Host + targetPath
	if r.URL.RawQuery != "" {
		targetWSURL += "?" + r.URL.RawQuery
	}

	// Upgrade the client connection
	clientConn, err := cp.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithError(err).Error("Failed to upgrade client connection")
		return
	}
	defer clientConn.Close()

	// Connect to the target WebSocket
	targetConn, _, err := websocket.DefaultDialer.Dial(targetWSURL, nil)
	if err != nil {
		log.WithError(err).Error("Failed to connect to target WebSocket")
		return
	}
	defer targetConn.Close()

	// Start proxying messages in both directions
	errChan := make(chan error, 2)

	// Proxy messages from client to target
	go func() {
		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}

			if err := targetConn.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Proxy messages from target to client
	go func() {
		for {
			messageType, message, err := targetConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}

			if err := clientConn.WriteMessage(messageType, message); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Wait for either connection to close or error
	<-errChan
	log.Debug("WebSocket proxy connection closed")
}
