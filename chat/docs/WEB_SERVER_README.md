# Sippy Agent Web Server

This document describes how to run and use the Sippy Agent as a web API server for integration with web frontends.

## Quick Start

1. **Install dependencies:**
   ```bash
   pip install -r requirements.txt
   ```

2. **Start the web server:**
   ```bash
   python web_main.py
   ```

3. **Access the API:**
   - Server: http://localhost:8000
   - API Documentation: http://localhost:8000/docs
   - Health Check: http://localhost:8000/health

## Command Line Options

```bash
python web_main.py --help
```

Available options:
- `--host` - Host to bind to (default: 0.0.0.0)
- `--port` - Port to bind to (default: 8000)
- `--reload` - Enable auto-reload for development
- `--verbose` - Enable verbose logging
- `--thinking` - Enable thinking display by default
- `--model` - Override model name
- `--endpoint` - Override LLM endpoint
- `--temperature` - Override temperature
- `--max-iterations` - Override max iterations
- `--timeout` - Override timeout

## API Endpoints

### REST Endpoints

#### `GET /health`
Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "agent_ready": true
}
```

#### `GET /status`
Get agent status and configuration.

**Response:**
```json
{
  "available_tools": ["get_prow_job_summary", "analyze_job_logs", ...],
  "model_name": "granite-3.2-8b-instruct",
  "endpoint": "https://...",
  "thinking_enabled": false
}
```

#### `POST /chat`
Send a message to the agent and get a response.

**Request:**
```json
{
  "message": "Analyze job 1234567890",
  "chat_history": [
    {
      "role": "user",
      "content": "Previous message",
      "timestamp": "2024-01-01T12:00:00Z"
    },
    {
      "role": "assistant", 
      "content": "Previous response",
      "timestamp": "2024-01-01T12:00:01Z"
    }
  ],
  "show_thinking": true
}
```

**Response:**
```json
{
  "response": "The job analysis shows...",
  "thinking_steps": [
    {
      "step_number": 1,
      "thought": "I need to get the job summary first",
      "action": "get_prow_job_summary",
      "action_input": "1234567890",
      "observation": "Job summary retrieved successfully"
    }
  ],
  "tools_used": ["get_prow_job_summary"],
  "error": null
}
```

### WebSocket Endpoint

#### `WebSocket /chat/stream`
Real-time streaming chat with thinking process.

**Send message:**
```json
{
  "message": "Analyze job 1234567890",
  "chat_history": [...],
  "show_thinking": true
}
```

**Receive messages:**

Thinking step:
```json
{
  "type": "thinking_step",
  "data": {
    "step_number": 1,
    "thought": "I need to analyze this job",
    "action": "get_prow_job_summary",
    "action_input": "1234567890",
    "observation": "",
    "complete": false
  }
}
```

Step completion:
```json
{
  "type": "thinking_step", 
  "data": {
    "step_number": 1,
    "thought": "",
    "action": "",
    "action_input": "",
    "observation": "Job summary retrieved successfully",
    "complete": true
  }
}
```

Final response:
```json
{
  "type": "final_response",
  "data": {
    "response": "The job analysis shows...",
    "tools_used": ["get_prow_job_summary"],
    "timestamp": "2024-01-01T12:00:00Z"
  }
}
```

Error:
```json
{
  "type": "error",
  "data": {
    "error": "Error message",
    "timestamp": "2024-01-01T12:00:00Z"
  }
}
```

## Configuration

The web server uses the same configuration as the CLI version:

1. **Environment variables** (`.env` file)
2. **Command line arguments** (override .env values)

See the main README for configuration details.

## CORS Configuration

The server is configured to allow all origins by default for development. For production, update the CORS settings in `sippy_agent/web_server.py`:

```python
self.app.add_middleware(
    CORSMiddleware,
    allow_origins=["https://your-frontend-domain.com"],  # Specific domains
    allow_credentials=True,
    allow_methods=["GET", "POST"],
    allow_headers=["*"],
)
```

## Development

For development with auto-reload:

```bash
python web_main.py --reload --verbose
```

## Production Deployment

For production, consider using a production ASGI server:

```bash
# Install production server
pip install gunicorn

# Run with gunicorn
gunicorn sippy_agent.web_server:app -w 4 -k uvicorn.workers.UvicornWorker --bind 0.0.0.0:8000
```

Or use the provided uvicorn server with appropriate settings:

```bash
python web_main.py --host 0.0.0.0 --port 8000
```

## Integration Examples

See `examples/react_integration.md` for detailed examples of integrating with React frontends.

## Troubleshooting

1. **Port already in use:**
   ```bash
   python web_main.py --port 8001
   ```

2. **CORS issues:**
   - Check the CORS configuration in `web_server.py`
   - Ensure your frontend URL is allowed

3. **WebSocket connection issues:**
   - Check that your frontend is connecting to the correct WebSocket URL
   - Ensure the server is running and accessible

4. **Model/endpoint issues:**
   - Check your `.env` configuration
   - Verify the model endpoint is accessible
   - Use `--verbose` for detailed logging
