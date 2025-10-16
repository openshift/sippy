#!/usr/bin/env python3
"""
Unified entry point for Sippy AI Agent - CLI and Web Server.
"""

import logging
import sys
from functools import wraps
from typing import Optional
import click
from rich.console import Console
from rich.logging import RichHandler

from sippy_agent.config import Config
from sippy_agent.cli import SippyCLI
from sippy_agent.web_server import SippyWebServer

console = Console()


def setup_logging(verbose: bool = False) -> None:
    """Setup logging with Rich handler."""
    level = logging.DEBUG if verbose else logging.INFO
    logging.basicConfig(level=level, format="%(message)s", datefmt="[%X]", handlers=[RichHandler(console=console, rich_tracebacks=True)])


def common_options(f):
    """Decorator to add common options shared between chat and serve commands."""
    options = [
        click.option("--verbose", "-v", is_flag=True, help="Enable verbose logging"),
        click.option("--thinking", "-t", is_flag=True, help="Enable thinking display"),
        click.option("--persona", default=None, help="AI persona to use (default, zorp, etc.)"),
        click.option("--model", default=None, help="Model name to use (e.g., llama3.1:8b, gpt-4)"),
        click.option("--endpoint", default=None, help="LLM API endpoint"),
        click.option("--temperature", default=None, type=float, help="Temperature for the model"),
        click.option("--max-iterations", default=None, type=int, help="Maximum number of agent iterations (default: 25)"),
        click.option("--timeout", default=None, type=int, help="Maximum execution time in seconds (default: 1800 = 30 minutes)"),
        click.option("--google-credentials", default=None, help="Path to Google service account credentials JSON file"),
        click.option("--mcp-config", default=None, help="Path to MCP servers config file"),
    ]
    for option in reversed(options):
        f = option(f)
    return f


def apply_config_overrides(config: Config, **kwargs) -> None:
    """Apply command-line overrides to configuration."""
    config.verbose = kwargs.get("verbose", False)
    config.show_thinking = kwargs.get("thinking", False)

    # Only override .env values if explicitly provided via CLI
    if kwargs.get("persona") is not None:
        config.persona = kwargs["persona"]
    if kwargs.get("model") is not None:
        config.model_name = kwargs["model"]
    if kwargs.get("endpoint") is not None:
        config.llm_endpoint = kwargs["endpoint"]
    if kwargs.get("temperature") is not None:
        config.temperature = kwargs["temperature"]
    if kwargs.get("max_iterations") is not None:
        config.max_iterations = kwargs["max_iterations"]
    if kwargs.get("timeout") is not None:
        config.max_execution_time = kwargs["timeout"]
    if kwargs.get("google_credentials") is not None:
        config.google_credentials_file = kwargs["google_credentials"]
    if kwargs.get("mcp_config") is not None:
        config.mcp_config_file = kwargs["mcp_config"]


@click.group()
@click.version_option(version="1.0.0", prog_name="Sippy AI Agent")
def cli():
    """
    Sippy AI Agent - Your CI/CD Analysis Assistant

    Use 'chat' for interactive CLI or 'serve' for the web server.
    """
    pass


@cli.command()
@common_options
def chat(**kwargs) -> None:
    """
    Start the interactive chat CLI.

    Examples:
      python main.py chat
      python main.py chat --verbose --thinking
      python main.py chat --model gpt-4 --temperature 0.7
    """
    setup_logging(kwargs.get("verbose", False))

    try:
        # Create and configure
        config = Config.from_env()
        apply_config_overrides(config, **kwargs)
        config.validate_required_settings()

        # Start CLI
        cli_app = SippyCLI(config)
        cli_app.run()

    except ValueError as e:
        console.print(f"[red]Configuration error: {e}[/red]")
        sys.exit(1)
    except Exception as e:
        console.print(f"[red]Unexpected error: {e}[/red]")
        sys.exit(1)


@cli.command()
@click.option("--host", default="0.0.0.0", help="Host to bind the server to")
@click.option("--port", default=8000, type=int, help="Port to bind the server to")
@click.option("--metrics-port", default=None, type=int, help="Port for Prometheus metrics (if not set, metrics available on main port at /metrics)")
@click.option("--reload", is_flag=True, help="Enable auto-reload for development")
@common_options
def serve(host: str, port: int, metrics_port: Optional[int], reload: bool, **kwargs) -> None:
    """
    Start the web server with REST API.

    Examples:
      python main.py serve
      python main.py serve --port 8000 --metrics-port 9090
      python main.py serve --port 8080 --reload
      python main.py serve --model gpt-4 --thinking
    """
    setup_logging(kwargs.get("verbose", False))

    try:
        # Create and configure
        config = Config.from_env()
        apply_config_overrides(config, **kwargs)
        config.validate_required_settings()

        # Create and run web server
        console.print(f"[green]Starting Sippy AI Agent Web Server...[/green]")
        console.print(f"[blue]Server will be available at: http://{host}:{port}[/blue]")
        console.print(f"[blue]API documentation at: http://{host}:{port}/docs[/blue]")
        if metrics_port:
            console.print(f"[blue]Metrics will be available at: http://0.0.0.0:{metrics_port}/metrics[/blue]")
        else:
            console.print(f"[blue]Metrics available at: http://{host}:{port}/metrics[/blue]")
        console.print(f"[dim]Model: {config.model_name}[/dim]")
        console.print(f"[dim]Endpoint: {config.llm_endpoint}[/dim]")
        console.print(f"[dim]Thinking enabled: {config.show_thinking}[/dim]")
        console.print(f"[dim]Persona: {config.persona}[/dim]")
        console.print()

        server = SippyWebServer(config, metrics_port=metrics_port)
        server.run(host=host, port=port, reload=reload)

    except ValueError as e:
        console.print(f"[red]Configuration error: {e}[/red]")
        sys.exit(1)
    except Exception as e:
        console.print(f"[red]Unexpected error: {e}[/red]")
        sys.exit(1)


if __name__ == "__main__":
    cli()
