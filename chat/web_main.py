#!/usr/bin/env python3
"""
Entry point for the Sippy AI Agent web server.
"""

import logging
import sys
import click
from rich.console import Console
from rich.logging import RichHandler

from sippy_agent.config import Config
from sippy_agent.web_server import SippyWebServer

console = Console()


def setup_logging(verbose: bool = False) -> None:
    """Setup logging with Rich handler."""
    level = logging.DEBUG if verbose else logging.INFO
    logging.basicConfig(
        level=level,
        format="%(message)s",
        datefmt="[%X]",
        handlers=[RichHandler(console=console, rich_tracebacks=True)]
    )


@click.command()
@click.option('--host', default='0.0.0.0', help='Host to bind the server to')
@click.option('--port', default=8000, type=int, help='Port to bind the server to')
@click.option('--reload', is_flag=True, help='Enable auto-reload for development')
@click.option('--verbose', '-v', is_flag=True, help='Enable verbose logging')
@click.option('--thinking', '-t', is_flag=True, help='Enable thinking display by default')
@click.option('--model', default=None, help='Model name to use (e.g., llama3.1:8b, gpt-4)')
@click.option('--endpoint', default=None, help='LLM API endpoint')
@click.option('--temperature', default=None, type=float, help='Temperature for the model')
@click.option('--max-iterations', default=None, type=int, help='Maximum number of agent iterations (default: 25)')
@click.option('--timeout', default=None, type=int, help='Maximum execution time in seconds (default: 1800 = 30 minutes)')
@click.option('--google-credentials', default=None, help='Path to Google service account credentials JSON file')
def main(host: str, port: int, reload: bool, verbose: bool, thinking: bool,
         model: str, endpoint: str, temperature: float, max_iterations: int, timeout: int, google_credentials: str) -> None:
    """Sippy AI Agent Web Server - REST API for CI/CD Analysis."""
    setup_logging(verbose)
    
    try:
        # Create configuration
        config = Config.from_env()
        config.verbose = verbose
        config.show_thinking = thinking

        # Only override .env values if explicitly provided via CLI
        if model is not None:
            config.model_name = model
        if endpoint is not None:
            config.llm_endpoint = endpoint
        if temperature is not None:
            config.temperature = temperature
        if max_iterations is not None:
            config.max_iterations = max_iterations
        if timeout is not None:
            config.max_execution_time = timeout
        if google_credentials is not None:
            config.google_credentials_file = google_credentials

        # Create and run web server
        console.print(f"[green]Starting Sippy AI Agent Web Server...[/green]")
        console.print(f"[blue]Server will be available at: http://{host}:{port}[/blue]")
        console.print(f"[blue]API documentation at: http://{host}:{port}/docs[/blue]")
        console.print(f"[dim]Model: {config.model_name}[/dim]")
        console.print(f"[dim]Endpoint: {config.llm_endpoint}[/dim]")
        console.print(f"[dim]Thinking enabled: {config.show_thinking}[/dim]")
        console.print()
        
        server = SippyWebServer(config)
        server.run(host=host, port=port, reload=reload)
        
    except ValueError as e:
        console.print(f"[red]Configuration error: {e}[/red]")
        sys.exit(1)
    except Exception as e:
        console.print(f"[red]Unexpected error: {e}[/red]")
        sys.exit(1)


if __name__ == '__main__':
    main()
