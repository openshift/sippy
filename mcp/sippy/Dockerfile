# Sippy MCP server ("sippy", triage management). Build from the repository root with
# `make sippy-mcp-image`, or: podman build -f mcp/sippy/Dockerfile mcp/sippy
#
# The image contains only the Sippy MCP server and its locked runtime dependencies. It contains no
# development-workflow tooling, build tools, credentials, tokens, or trusted-delegator values; supply
# SIPPY_MCP_API_URL and SIPPY_MCP_AUTH_TOKEN_FILE (a mounted service-account token) at runtime.
# See mcp/sippy/README.md.

# The builder image carries compilers and development packages; it only installs the hash-verified
# dependencies into a virtualenv that the runtime stage copies.
FROM registry.access.redhat.com/ubi9/python-312:latest AS builder

USER 0
COPY requirements.lock /tmp/requirements.lock
# The image's own pip installs into a pip-free virtualenv, so the runtime carries no installer.
RUN /usr/bin/python3.12 -m venv --without-pip /opt/sippy-mcp/venv && \
    pip --python /opt/sippy-mcp/venv/bin/python install --no-cache-dir --require-hashes -r /tmp/requirements.lock

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Same interpreter path as the builder, so the copied virtualenv resolves to it.
RUN microdnf install -y --nodocs --setopt=install_weak_deps=0 python3.12 && \
    microdnf clean all

COPY --from=builder /opt/sippy-mcp/venv /opt/sippy-mcp/venv
COPY triage_api.py sippy_mcp_server.py /opt/sippy-mcp/

# Streamable HTTP on loopback: a same-pod authenticating proxy forwards to 127.0.0.1:8090.
# Override SIPPY_MCP_HTTP_HOST only when the listener is otherwise unreachable without authentication.
ENV PYTHONUNBUFFERED=1 \
    PATH=/opt/sippy-mcp/venv/bin:$PATH \
    SIPPY_MCP_TRANSPORT=streamable-http \
    SIPPY_MCP_HTTP_HOST=127.0.0.1 \
    SIPPY_MCP_HTTP_PORT=8090

WORKDIR /opt/sippy-mcp
USER 1001
EXPOSE 8090
ENTRYPOINT ["/opt/sippy-mcp/venv/bin/python", "/opt/sippy-mcp/sippy_mcp_server.py"]
