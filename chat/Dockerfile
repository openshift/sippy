# Use Red Hat Universal Base Image 10 with Python 3.11
FROM registry.access.redhat.com/ubi9/python-311:latest

# Set metadata
LABEL name="sippy-chat" \
      version="1.0.0" \
      description="Sippy AI Agent - LangChain Re-Act agent for CI/CD analysis" \
      maintainer="Sippy Team" \
      vendor="Red Hat" \
      summary="AI-powered CI/CD analysis tool with web API and CLI interfaces" \
      io.k8s.description="Sippy AI Agent provides intelligent analysis of CI/CD pipelines, test failures, and build issues using LangChain and various LLM providers" \
      io.k8s.display-name="Sippy AI Agent" \
      io.openshift.tags="ai,ci-cd,analysis,langchain,python"

# Switch to root to install system packages
USER root

# Install system dependencies
RUN dnf update -y && \
    dnf install -y \
        git \
        curl-minimal \
        ca-certificates \
    && dnf clean all \
    && rm -rf /var/cache/dnf

# Create application directory
WORKDIR /opt/app-root/src

# Create non-root user for running the application
RUN groupadd -r sippy && \
    useradd -r -g sippy -d /opt/app-root/src -s /sbin/nologin \
    -c "Sippy AI Agent user" sippy

# Copy requirements first for better Docker layer caching
COPY requirements.txt ./

# Upgrade pip and install Python dependencies
RUN python -m pip install --upgrade pip && \
    python -m pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY . .

# Copy environment template and create default .env
COPY .env.example .env

# Set proper ownership
RUN chown -R sippy:sippy /opt/app-root/src

# Switch to non-root user
USER sippy

# Set environment variables
ENV PYTHONPATH=/opt/app-root/src \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PATH="/opt/app-root/src/.local/bin:$PATH"

# Default environment variables for the application
ENV LLM_ENDPOINT=http://localhost:11434/v1 \
    MODEL_NAME=granite3.3:8b \
    SIPPY_API_URL=https://sippy.dptools.openshift.org \
    JIRA_URL=https://issues.redhat.com

# Expose the web server port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Default command runs the web server
CMD ["python", "web_main.py", "--host", "0.0.0.0", "--port", "8000"]
