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

**Optional: Database Access**

To enable direct database queries (fallback tool for when standard tools don't provide enough information), set:

```bash
SIPPY_READ_ONLY_DATABASE_DSN=postgresql://readonly_user:password@host:5432/sippy
```

**Important:** Use a read-only database user for security. The tool enforces read-only queries at the application level as well.

### 3. Run the Agent

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
