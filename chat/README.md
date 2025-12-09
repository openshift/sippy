# Sippy AI Agent

A LangGraph ReAct AI Agent for the Sippy platform.

## Features

- 🤖 **LangGraph ReAct Agent**: State-based reasoning with explicit control flow
- 🧠 **Thinking Display**: Optional visualization of the agent's thought process
- 🔧 **CI/CD Analysis**: Tools for analyzing jobs, test failures, and build patterns
- 💬 **Interactive CLI**: Rich command-line interface with chat functionality
- 🌐 **Web API**: REST and WebSocket endpoints for web frontend integration
- 🛠️ **Extensible Tools**: Modular tool system ready for Sippy API integration
- ⚙️ **Configurable**: Environment-based configuration management

## Quick Start

### 1. Installation

```bash
$ cd chat
$ python -m venv .venv && source .venv/bin/activate
$ pip install -r requirements.txt
```

### 2. Configuration

Create a `.env` file from the example:

```bash
cp .env.example .env
```

Edit `.env` for your LLM setup, according to the instructions in the
.env file.

#### Optional: Database Access

To enable direct database queries (fallback tool for when standard tools don't provide enough information), set:

```bash
SIPPY_READ_ONLY_DATABASE_DSN=postgresql://readonly_user:password@host:5432/sippy
```

**Important:** Use a read-only database user for security. The tool enforces read-only queries at the application level as well.

#### Optional: Claude Models via Google Vertex AI

To use Claude models through Google's Vertex AI, you need:

1. A Google Cloud project with Vertex AI API enabled
2. Authentication via `gcloud auth` OR service account credentials
3. Claude models enabled in your project (requires allowlist access)

**Option 1: Using gcloud auth (recommended for local development):**

```bash
# Login with your Google Cloud account
gcloud auth application-default login

# Set required environment variables
MODEL_NAME=claude-sonnet-4-5
GOOGLE_PROJECT_ID=your-gcp-project-id
GOOGLE_LOCATION=us-central1  # Optional, defaults to us-central1
```

**Option 2: Using service account credentials:**

```bash
MODEL_NAME=claude-sonnet-4-5
GOOGLE_PROJECT_ID=your-gcp-project-id
GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
GOOGLE_LOCATION=us-central1  # Optional, defaults to us-central1
```

**Claude Extended Thinking:**
When using Claude with `--thinking` enabled, the model can use its extended thinking feature to show detailed reasoning. You can control the token budget:
```bash
# Use extended thinking with custom budget (if supported by your model/region)
python main.py chat --model claude-sonnet-4-5 --thinking --thinking-budget 15000

# Or set via environment variable
export EXTENDED_THINKING_BUDGET=15000

# If you encounter 400 errors, extended thinking may not be available
# Disable it by setting the budget to 0:
python main.py chat --model claude-sonnet-4-5 --thinking --thinking-budget 0
```

**Important Notes:**
- Extended thinking **automatically sets temperature to 1.0** (required by Claude API)
- Extended thinking availability may vary by Claude model version and Vertex AI region
- If you encounter errors, you can still use `--thinking` to see the agent's tool usage and reasoning without Claude's extended thinking by setting budget to 0

### 3. Multiple Model Configuration (Optional)

Sippy Chat supports running with multiple AI models that users can switch between via the web UI. This is configured using a `models.yaml` file.

**Create models.yaml:**

```bash
cp models.yaml.example models.yaml
# Edit models.yaml to configure your models
```

**Configuration Options:**

- `id`: Unique identifier for the model (required)
- `name`: Display name shown in the UI (required)
- `description`: Brief description shown in the UI (optional)
- `model_name`: The actual model name to use with the provider (required)
- `endpoint`: API endpoint URL (required for OpenAI-compatible APIs, empty for Vertex AI)
- `temperature`: Temperature setting for the model (optional, default: 0.0)
- `extended_thinking_budget`: Token budget for Claude's extended thinking (optional, default: 0)
- `default`: Set to true to make this the default model (optional, only one should be true)

**Important Notes:**

- Environment variables (API keys, credentials) are still required and shared across all models
- Users can switch models mid-conversation via the Settings panel in the web UI
- If `models.yaml` doesn't exist, the system falls back to using a single model from environment variables

**Start the server with models.yaml:**

```bash
python main.py serve --models-config models.yaml
```

If `models.yaml` exists in the `chat/` directory, it will be loaded automatically without the `--models-config` flag.

### 4. Run the Agent

**Interactive Chat CLI:**
```bash
python main.py chat
```

**Web Server (REST API):**
```bash
python main.py serve
```

**With options:**

```bash
# Interactive CLI with options
python main.py chat --verbose --thinking --model llama3.1:70b --temperature 0.2

# Web server with custom port and thinking enabled
python main.py serve --port 8080 --thinking --reload

# Using OpenAI with thinking process visible
python main.py chat --thinking --model gpt-4 --endpoint https://api.openai.com/v1

# Using Google Gemini with API key
python main.py chat --model gemini-1.5-pro

# Using Google Gemini with service account
python main.py serve --model gemini-1.5-pro --google-credentials /path/to/credentials.json

# Using Claude models via Google Vertex AI
python main.py serve --model claude-3-5-sonnet@20240620
```

**Get help:**
```bash
python main.py --help        # Show main help
python main.py chat --help   # Show chat-specific options
python main.py serve --help  # Show server-specific options
```

## Thinking Display

The agent supports a "thinking display" mode that shows the LLM's reasoning process:

```bash
# Enable thinking display from command line
python main.py chat --thinking

# Or toggle it during runtime in chat mode
> thinking
```

## Web Server

The Sippy AI Agent can run as a web API server for integration with web frontends:

```bash
# Start the web server
python main.py serve

# With options
python main.py serve --port 8080 --thinking --verbose --reload
```

The web server provides:
- **REST API** at `http://localhost:8000` for chat interactions
- **WebSocket streaming** at `ws://localhost:8000/chat/stream` for real-time responses
- **Interactive API docs** at `http://localhost:8000/docs`
- **Health check** at `http://localhost:8000/health`
- **Prometheus metrics** at `http://localhost:8000/metrics`
