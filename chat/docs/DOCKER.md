# Docker Deployment Guide

This guide explains how to build and run the Sippy AI Agent using Docker with RHEL UBI 10.

## Quick Start

### 1. Build the Image

```bash
# Build the Docker image
docker build -t sippy-chat:latest .

# Or with a specific tag
docker build -t sippy-chat:v1.0.0 .
```

### 2. Run the Container

**Web Server Mode (default):**
```bash
# Run with default settings (web server on port 8000)
docker run -p 8000:8000 sippy-chat:latest

# Run with environment variables
docker run -p 8000:8000 \
  -e LLM_ENDPOINT=https://api.openai.com/v1 \
  -e MODEL_NAME=gpt-4 \
  -e OPENAI_API_KEY=your_api_key_here \
  sippy-chat:latest
```

**CLI Mode:**
```bash
# Run in CLI mode (interactive)
docker run -it sippy-chat:latest python main.py

# Run CLI with specific options
docker run -it sippy-chat:latest python main.py --thinking --verbose
```

## Configuration

### Environment Variables

The container supports all the same environment variables as the native application:

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_ENDPOINT` | `http://localhost:11434/v1` | LLM API endpoint |
| `MODEL_NAME` | `granite3.3:8b` | Model name to use |
| `OPENAI_API_KEY` | - | OpenAI API key (if using OpenAI) |
| `GOOGLE_API_KEY` | - | Google API key (if using Gemini) |
| `SIPPY_API_URL` | `https://sippy.dptools.openshift.org` | Sippy API base URL |
| `JIRA_URL` | `https://issues.redhat.com` | Jira instance URL |

### Using External LLM Services

**OpenAI:**
```bash
docker run -p 8000:8000 \
  -e LLM_ENDPOINT=https://api.openai.com/v1 \
  -e MODEL_NAME=gpt-4 \
  -e OPENAI_API_KEY=sk-your-key-here \
  sippy-chat:latest
```

**Google Gemini (API Key):**
```bash
docker run -p 8000:8000 \
  -e MODEL_NAME=gemini-1.5-pro \
  -e GOOGLE_API_KEY=your-google-api-key \
  sippy-chat:latest
```

**Google Gemini (Service Account):**
```bash
# Mount the credentials file
docker run -p 8000:8000 \
  -v /path/to/credentials.json:/opt/app-root/src/credentials.json \
  -e MODEL_NAME=gemini-1.5-pro \
  -e GOOGLE_APPLICATION_CREDENTIALS=/opt/app-root/src/credentials.json \
  sippy-chat:latest
```

**Local Ollama (requires network access):**
```bash
# If Ollama is running on the host
docker run -p 8000:8000 \
  -e LLM_ENDPOINT=http://host.docker.internal:11434/v1 \
  -e MODEL_NAME=llama3.1:8b \
  sippy-chat:latest
```

## Docker Compose

Create a `docker-compose.yml` file for easier management:

```yaml
version: '3.8'

services:
  sippy-chat:
    build: .
    ports:
      - "8000:8000"
    environment:
      - LLM_ENDPOINT=https://api.openai.com/v1
      - MODEL_NAME=gpt-4
      - OPENAI_API_KEY=${OPENAI_API_KEY}
    volumes:
      # Optional: mount credentials file
      - ./credentials.json:/opt/app-root/src/credentials.json:ro
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

Run with:
```bash
# Set your API key
export OPENAI_API_KEY=your_api_key_here

# Start the service
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the service
docker-compose down
```

## Advanced Usage

### Custom Command Line Options

```bash
# Run web server with custom options
docker run -p 8080:8080 sippy-chat:latest \
  python web_main.py --host 0.0.0.0 --port 8080 --thinking --verbose

# Run CLI with specific model
docker run -it sippy-chat:latest \
  python main.py --model gpt-4 --endpoint https://api.openai.com/v1 --thinking
```

### Volume Mounts

```bash
# Mount configuration directory
docker run -p 8000:8000 \
  -v /host/config:/opt/app-root/src/config:ro \
  sippy-chat:latest

# Mount logs directory
docker run -p 8000:8000 \
  -v /host/logs:/opt/app-root/src/logs \
  sippy-chat:latest
```

### Development Mode

```bash
# Mount source code for development
docker run -p 8000:8000 \
  -v $(pwd):/opt/app-root/src \
  -e PYTHONPATH=/opt/app-root/src \
  sippy-chat:latest \
  python web_main.py --reload --verbose
```

## Security Considerations

### Non-Root User

The container runs as a non-root user (`sippy`) for security:
- UID/GID: Automatically assigned by the system
- Home directory: `/opt/app-root/src`
- No shell access: `/sbin/nologin`

### Secrets Management

**Don't include secrets in the image:**
```bash
# ❌ Bad - secrets in environment
docker run -e OPENAI_API_KEY=sk-secret sippy-chat:latest

# ✅ Good - secrets from file
docker run --env-file .env.secrets sippy-chat:latest

# ✅ Good - secrets from external system
docker run -e OPENAI_API_KEY="$(cat /secure/path/api-key)" sippy-chat:latest
```

### Network Security

```bash
# Bind to localhost only
docker run -p 127.0.0.1:8000:8000 sippy-chat:latest

# Use custom network
docker network create sippy-network
docker run --network sippy-network sippy-chat:latest
```

## Troubleshooting

### Health Check

```bash
# Check container health
docker ps
docker inspect <container-id> | grep -A 10 Health

# Manual health check
docker exec <container-id> curl -f http://localhost:8000/health
```

### Logs

```bash
# View container logs
docker logs <container-id>

# Follow logs
docker logs -f <container-id>

# View last 100 lines
docker logs --tail 100 <container-id>
```

### Debug Mode

```bash
# Run with debug shell
docker run -it --entrypoint /bin/bash sippy-chat:latest

# Check Python environment
docker run sippy-chat:latest python -c "import sys; print(sys.path)"

# Check installed packages
docker run sippy-chat:latest pip list
```

### Common Issues

1. **Port already in use:**
   ```bash
   docker run -p 8001:8000 sippy-chat:latest
   ```

2. **Permission denied:**
   ```bash
   # Check if running as non-root
   docker run sippy-chat:latest id
   ```

3. **Module not found:**
   ```bash
   # Check PYTHONPATH
   docker run sippy-chat:latest python -c "import sys; print(sys.path)"
   ```

## Production Deployment

### Resource Limits

```bash
# Set memory and CPU limits
docker run -p 8000:8000 \
  --memory=2g \
  --cpus=1.0 \
  sippy-chat:latest
```

### Restart Policies

```bash
# Auto-restart on failure
docker run -p 8000:8000 \
  --restart=unless-stopped \
  sippy-chat:latest
```

### Multi-stage Build (Optional)

For smaller production images, consider a multi-stage build approach in the Dockerfile.

## Integration with OpenShift

The image is designed to work with OpenShift:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sippy-chat
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sippy-chat
  template:
    metadata:
      labels:
        app: sippy-chat
    spec:
      containers:
      - name: sippy-chat
        image: sippy-chat:latest
        ports:
        - containerPort: 8000
        env:
        - name: LLM_ENDPOINT
          value: "https://api.openai.com/v1"
        - name: MODEL_NAME
          value: "gpt-4"
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: openai-secret
              key: api-key
        livenessProbe:
          httpGet:
            path: /health
            port: 8000
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 8000
          initialDelaySeconds: 5
          periodSeconds: 10
```
