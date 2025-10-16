"""
Standalone metrics server for Prometheus metrics.

This module provides a separate HTTP server for exposing Prometheus metrics
on a different port than the main API server, which is a common production practice.
"""

import logging
import threading
from typing import Optional
from prometheus_client import generate_latest, CONTENT_TYPE_LATEST
from http.server import HTTPServer, BaseHTTPRequestHandler

logger = logging.getLogger(__name__)


class MetricsHandler(BaseHTTPRequestHandler):
    """HTTP handler for serving Prometheus metrics."""
    
    def do_GET(self):
        """Handle GET requests."""
        if self.path == '/metrics':
            # Serve Prometheus metrics
            self.send_response(200)
            self.send_header('Content-Type', CONTENT_TYPE_LATEST)
            self.end_headers()
            self.wfile.write(generate_latest())
        elif self.path == '/health' or self.path == '/healthz':
            # Health check endpoint
            self.send_response(200)
            self.send_header('Content-Type', 'text/plain')
            self.end_headers()
            self.wfile.write(b'OK')
        else:
            # Not found
            self.send_response(404)
            self.send_header('Content-Type', 'text/plain')
            self.end_headers()
            self.wfile.write(b'Not Found')
    
    def log_message(self, format, *args):
        """Override to use Python logging instead of printing to stderr."""
        logger.debug("%s - - [%s] %s" % (
            self.address_string(),
            self.log_date_time_string(),
            format % args
        ))


class MetricsServer:
    """Standalone HTTP server for Prometheus metrics."""
    
    def __init__(self, host: str = "0.0.0.0", port: int = 9090):
        """
        Initialize the metrics server.
        
        Args:
            host: Host address to bind to (default: 0.0.0.0)
            port: Port to listen on (default: 9090)
        """
        self.host = host
        self.port = port
        self.server: Optional[HTTPServer] = None
        self.thread: Optional[threading.Thread] = None
    
    def start(self):
        """Start the metrics server in a background thread."""
        if self.server is not None:
            logger.warning("Metrics server is already running")
            return
        
        try:
            self.server = HTTPServer((self.host, self.port), MetricsHandler)
            self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
            self.thread.start()
            logger.info(f"Metrics server started on http://{self.host}:{self.port}/metrics")
        except Exception as e:
            logger.error(f"Failed to start metrics server: {e}")
            raise
    
    def stop(self):
        """Stop the metrics server."""
        if self.server is not None:
            logger.info("Stopping metrics server...")
            self.server.shutdown()
            self.server = None
            self.thread = None
            logger.info("Metrics server stopped")
    
    def is_running(self) -> bool:
        """Check if the server is running."""
        return self.server is not None and self.thread is not None and self.thread.is_alive()


# Global metrics server instance
_metrics_server: Optional[MetricsServer] = None


def start_metrics_server(host: str = "0.0.0.0", port: int = 9090):
    """
    Start the global metrics server.
    
    Args:
        host: Host address to bind to
        port: Port to listen on
    """
    global _metrics_server
    if _metrics_server is None or not _metrics_server.is_running():
        _metrics_server = MetricsServer(host, port)
        _metrics_server.start()


def stop_metrics_server():
    """Stop the global metrics server."""
    global _metrics_server
    if _metrics_server is not None:
        _metrics_server.stop()
        _metrics_server = None

