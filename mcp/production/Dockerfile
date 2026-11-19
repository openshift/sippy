# Production Sippy MCP server (triage management). Build from the repository root with
# `make mcp-production-image`, or: podman build -f mcp/production/Dockerfile mcp/production
#
# The image contains only the production server and its locked runtime dependencies. It contains no
# development-workflow tooling, credentials, tokens, or trusted-delegator values; supply
# SIPPY_MCP_API_URL and SIPPY_MCP_AUTH_TOKEN_FILE (a mounted service-account token) at runtime.
# See mcp/production/README.md.
FROM registry.access.redhat.com/ubi9/python-312:latest

WORKDIR /opt/app-root/src

COPY requirements.lock ./
RUN pip install --no-cache-dir --require-hashes -r requirements.lock

COPY triage_api.py sippy_mcp_server.py ./

# Streamable HTTP on loopback: a same-pod authenticating proxy forwards to 127.0.0.1:8090.
# Override SIPPY_MCP_HTTP_HOST only when the listener is otherwise unreachable without authentication.
ENV PYTHONUNBUFFERED=1 \
    SIPPY_MCP_TRANSPORT=streamable-http \
    SIPPY_MCP_HTTP_HOST=127.0.0.1 \
    SIPPY_MCP_HTTP_PORT=8090

USER 1001
EXPOSE 8090
ENTRYPOINT ["python", "/opt/app-root/src/sippy_mcp_server.py"]
