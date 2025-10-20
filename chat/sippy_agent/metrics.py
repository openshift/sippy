"""
Prometheus metrics for Sippy Agent.
"""

from prometheus_client import Counter, Histogram, Gauge, Info

# Total messages received counter
messages_received_total = Counter(
    "sippy_chat_messages_received_total",
    "Total number of chat messages received",
    ["endpoint"]  # websocket or http
)

# Total sessions started counter
sessions_started_total = Counter(
    "sippy_chat_sessions_started_total",
    "Total number of chat sessions started"
)

# Total errors counter
errors_total = Counter(
    "sippy_chat_errors_total",
    "Total number of errors encountered",
    ["error_type"]  # e.g., processing_error, websocket_error, agent_error
)

# Response duration histogram (in seconds)
response_duration_seconds = Histogram(
    "sippy_chat_response_duration_seconds",
    "Time taken to process a message and issue a response",
    ["endpoint"],  # websocket or http
    buckets=[0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0]
)

# Active sessions gauge
active_sessions = Gauge(
    "sippy_chat_active_sessions",
    "Number of currently active chat sessions"
)

# Tool calls counter
tool_calls_total = Counter(
    "sippy_chat_tool_calls_total",
    "Total number of tool calls made",
    ["tool_name"]
)

# Message size histogram (in bytes)
message_size_bytes = Histogram(
    "sippy_chat_message_size_bytes",
    "Size of messages in bytes",
    ["direction"],  # request or response
    buckets=[100, 500, 1000, 5000, 10000, 50000, 100000]
)

# Cancelled requests counter
cancelled_requests_total = Counter(
    "sippy_chat_cancelled_requests_total",
    "Number of cancelled/interrupted requests",
    ["endpoint"]  # websocket or http
)

# Agent info
agent_info = Info(
    "sippy_chat_agent",
    "Information about the Sippy Chat agent"
)

