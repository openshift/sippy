"""
Command-line interface for Sippy Agent.
"""

import logging
import sys
from typing import Optional, List, Dict
import click
from rich.console import Console
from rich.panel import Panel
from rich.prompt import Prompt
from rich.text import Text
from rich.logging import RichHandler

from .agent import SippyAgent
from .config import Config

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


class SippyCLI:
    """Command-line interface for the Sippy Agent."""
    
    def __init__(self, config: Config):
        """Initialize the CLI with configuration."""
        self.config = config
        self.agent = SippyAgent(config)
        self.chat_history = []
        self.current_step = 0
        self.streaming_steps = []
    
    def display_welcome(self) -> None:
        """Display welcome message."""
        welcome_text = Text()
        welcome_text.append("🔧 ", style="bold blue")
        welcome_text.append("Sippy AI Agent", style="bold cyan")
        welcome_text.append(" - Your CI/CD Analysis Assistant", style="bold white")
        
        welcome_panel = Panel(
            welcome_text,
            title="Welcome",
            border_style="blue",
            padding=(1, 2)
        )
        console.print(welcome_panel)
        console.print()
        
        # Display available tools
        tools = self.agent.list_tools()
        tools_text = "Available tools: " + ", ".join(f"[bold green]{tool}[/bold green]" for tool in tools)
        console.print(tools_text)
        console.print()

        # Show thinking status
        thinking_status = "enabled" if self.config.show_thinking else "disabled"
        console.print(f"[dim]Thinking display: {thinking_status} (use 'thinking' to toggle)[/dim]")
        console.print("[dim]Type 'help' for commands, 'quit' or 'exit' to leave[/dim]")
        console.print()
    
    def display_help(self) -> None:
        """Display help information."""
        help_text = """
[bold cyan]Sippy AI Agent Commands:[/bold cyan]

[bold green]help[/bold green]     - Show this help message
[bold green]tools[/bold green]    - List available tools
[bold green]history[/bold green]  - Show chat history
[bold green]clear[/bold green]    - Clear chat history
[bold green]thinking[/bold green] - Toggle showing the agent's thinking process
[bold green]quit[/bold green]     - Exit the application
[bold green]exit[/bold green]     - Exit the application

[bold cyan]Example queries:[/bold cyan]
• "Analyze job 12345 for failures"
• "What are the common test failures for test_login?"
• "Show me patterns in recent CI failures"
"""
        console.print(Panel(help_text, title="Help", border_style="green"))
    
    def display_tools(self) -> None:
        """Display available tools."""
        tools = self.agent.list_tools()
        tools_text = "\n".join(f"• [bold green]{tool}[/bold green]" for tool in tools)
        console.print(Panel(tools_text, title="Available Tools", border_style="blue"))
    
    def display_history(self) -> None:
        """Display chat history."""
        if not self.chat_history:
            console.print("[dim]No chat history yet.[/dim]")
            return
        
        history_text = ""
        for i, (user_msg, agent_msg) in enumerate(self.chat_history, 1):
            history_text += f"[bold blue]{i}. User:[/bold blue] {user_msg}\n"
            history_text += f"[bold green]   Agent:[/bold green] {agent_msg}\n\n"
        
        console.print(Panel(history_text.strip(), title="Chat History", border_style="yellow"))
    
    def clear_history(self) -> None:
        """Clear chat history."""
        self.chat_history.clear()
        console.print("[green]Chat history cleared.[/green]")

    def toggle_thinking(self) -> None:
        """Toggle showing the agent's thinking process."""
        self.config.show_thinking = not self.config.show_thinking
        # Update the agent's configuration
        self.agent.config.show_thinking = self.config.show_thinking

        status = "enabled" if self.config.show_thinking else "disabled"
        console.print(f"[green]Thinking display {status}.[/green]")

    def display_thinking_steps(self, thinking_steps: List[Dict[str, str]]) -> None:
        """Display the agent's thinking process in a nicely formatted way."""
        if not thinking_steps:
            return

        console.print(Panel("🧠 Agent's Thinking Process", title="Reasoning", border_style="cyan"))

        for i, step in enumerate(thinking_steps, 1):
            # Create thinking step panel
            step_content = Text()

            # Add thought
            if step.get("thought"):
                step_content.append("💭 Thought: ", style="bold blue")
                step_content.append(f"{step['thought']}\n\n", style="white")

            # Add action
            if step.get("action"):
                step_content.append("🔧 Action: ", style="bold green")
                step_content.append(f"{step['action']}\n", style="green")

                if step.get("action_input"):
                    step_content.append("📝 Input: ", style="bold yellow")
                    step_content.append(f"{step['action_input']}\n\n", style="yellow")

            # Add observation
            if step.get("observation"):
                step_content.append("👁️ Observation: ", style="bold magenta")
                observation = step["observation"]
                step_content.append(observation, style="magenta")

            # Display step panel
            console.print(Panel(
                step_content,
                title=f"Step {i}",
                border_style="dim",
                padding=(0, 1)
            ))

        console.print()

    def streaming_thinking_callback(self, thought: str, action: str, action_input: str, observation: str) -> None:
        """Callback for streaming thinking process."""
        if thought and action:
            # New step starting
            self.current_step += 1

            # Create step content
            step_content = Text()
            step_content.append("💭 Thought: ", style="bold blue")
            step_content.append(f"{thought}\n\n", style="white")
            step_content.append("🔧 Action: ", style="bold green")
            step_content.append(f"{action}\n", style="green")

            if action_input:
                step_content.append("📝 Input: ", style="bold yellow")
                step_content.append(f"{action_input}\n", style="yellow")

            # Display step panel immediately
            console.print(Panel(
                step_content,
                title=f"Step {self.current_step} - Thinking",
                border_style="cyan",
                padding=(0, 1)
            ))

            # Store for potential observation update
            self.streaming_steps.append({
                "step": self.current_step,
                "thought": thought,
                "action": action,
                "action_input": action_input,
                "observation": ""
            })

        elif observation:
            # Update the last step with observation
            if self.streaming_steps:
                last_step = self.streaming_steps[-1]

                # Create updated content with observation
                step_content = Text()
                step_content.append("💭 Thought: ", style="bold blue")
                step_content.append(f"{last_step['thought']}\n\n", style="white")
                step_content.append("🔧 Action: ", style="bold green")
                step_content.append(f"{last_step['action']}\n", style="green")

                if last_step['action_input']:
                    step_content.append("📝 Input: ", style="bold yellow")
                    step_content.append(f"{last_step['action_input']}\n\n", style="yellow")

                step_content.append("👁️ Observation: ", style="bold magenta")
                step_content.append(observation, style="magenta")

                # Display updated step panel
                console.print(Panel(
                    step_content,
                    title=f"Step {last_step['step']} - Complete",
                    border_style="green",
                    padding=(0, 1)
                ))

    def process_user_input(self, user_input: str) -> bool:
        """Process user input and return False if should exit."""
        user_input = user_input.strip()
        
        # Handle special commands
        if user_input.lower() in ['quit', 'exit']:
            return False
        elif user_input.lower() == 'help':
            self.display_help()
            return True
        elif user_input.lower() == 'tools':
            self.display_tools()
            return True
        elif user_input.lower() == 'history':
            self.display_history()
            return True
        elif user_input.lower() == 'clear':
            self.clear_history()
            return True
        elif user_input.lower() == 'thinking':
            self.toggle_thinking()
            return True
        elif not user_input:
            return True
        
        # Process with agent
        try:
            # Reset streaming state
            self.current_step = 0
            self.streaming_steps = []

            # Prepare chat history for context
            history_context = "\n".join([
                f"User: {user_msg}\nAssistant: {agent_msg}"
                for user_msg, agent_msg in self.chat_history[-3:]  # Last 3 exchanges
            ])

            if self.config.show_thinking:
                # Show thinking header
                console.print()
                console.print(Panel("🧠 Agent's Thinking Process", title="Reasoning", border_style="cyan"))

                # Use streaming callback
                response = self.agent.chat(user_input, history_context, self.streaming_thinking_callback)
            else:
                with console.status("[bold green]Thinking...", spinner="dots"):
                    response = self.agent.chat(user_input, history_context)

            # Display response
            console.print()

            if isinstance(response, dict) and "thinking_steps" in response:
                # Check if there are any actual thinking steps to show
                thinking_steps = response["thinking_steps"]

                if thinking_steps and len(thinking_steps) > 0:
                    # For streaming thinking, we already showed the steps, just show final answer
                    if not self.config.show_thinking or not self.streaming_steps:
                        # Fallback to non-streaming display if streaming didn't work
                        self.display_thinking_steps(thinking_steps)

                    # Display final answer
                    console.print(Panel(response["output"], title="Sippy AI - Final Answer", border_style="green"))
                else:
                    # No thinking steps, just show regular response
                    console.print(Panel(response["output"], title="Sippy AI", border_style="green"))

                # Store only the final output in history
                final_output = response["output"]

                # Display token usage if available
                token_usage = response.get("token_usage")
                if token_usage and token_usage.get('total_tokens', 0) > 0:
                    self._display_token_usage(token_usage)

            elif isinstance(response, dict) and "token_usage" in response:
                # Response with token usage but no thinking steps
                console.print(Panel(response["output"], title="Sippy AI", border_style="green"))
                final_output = response["output"]

                # Display token usage
                token_usage = response.get("token_usage")
                if token_usage and token_usage.get('total_tokens', 0) > 0:
                    self._display_token_usage(token_usage)
            else:
                # Regular response without thinking or token usage
                console.print(Panel(response, title="Sippy AI", border_style="green"))
                final_output = response

            console.print()

            # Add to history
            self.chat_history.append((user_input, final_output))
            
        except KeyboardInterrupt:
            console.print("\n[yellow]Interrupted by user.[/yellow]")
        except Exception as e:
            console.print(f"\n[red]Error: {str(e)}[/red]")
        
        return True

    def _display_token_usage(self, token_usage: Dict[str, int]) -> None:
        """Display token usage information."""
        token_info = Text()
        token_info.append("📊 Token Usage: ", style="bold cyan")
        token_info.append(f"Total: {token_usage['total_tokens']}, ", style="white")
        token_info.append(f"Prompt: {token_usage['prompt_tokens']}, ", style="yellow")
        token_info.append(f"Completion: {token_usage['completion_tokens']}, ", style="green")
        token_info.append(f"LLM Calls: {token_usage['call_count']}", style="blue")

        # Add warning for high usage
        if token_usage['total_tokens'] > 100000:
            token_info.append(" ⚠️ HIGH USAGE", style="bold red")
        elif token_usage['total_tokens'] > 50000:
            token_info.append(" ⚠️ MODERATE USAGE", style="bold yellow")

        console.print(Panel(token_info, title="Token Usage", border_style="cyan"))

    def run(self) -> None:
        """Run the interactive CLI."""
        self.display_welcome()
        
        try:
            while True:
                user_input = Prompt.ask("[bold blue]You")
                
                if not self.process_user_input(user_input):
                    break
                    
        except KeyboardInterrupt:
            console.print("\n[yellow]Goodbye![/yellow]")
        except EOFError:
            console.print("\n[yellow]Goodbye![/yellow]")


@click.command()
@click.option('--verbose', '-v', is_flag=True, help='Enable verbose logging')
@click.option('--thinking', '-t', is_flag=True, help='Show the agent\'s thinking process')
@click.option('--model', default=None, help='Model name to use (e.g., llama3.1:8b, gpt-4)')
@click.option('--endpoint', default=None, help='LLM API endpoint')
@click.option('--temperature', default=None, type=float, help='Temperature for the model')
@click.option('--max-iterations', default=None, type=int, help='Maximum number of agent iterations (default: 25)')
@click.option('--timeout', default=None, type=int, help='Maximum execution time in seconds (default: 1800 = 30 minutes)')
@click.option('--google-credentials', default=None, help='Path to Google service account credentials JSON file')
def main(verbose: bool, thinking: bool, model: str, endpoint: str, temperature: float, max_iterations: int, timeout: int, google_credentials: str) -> None:
    """Sippy AI Agent - Your CI/CD Analysis Assistant."""
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

        # Create and run CLI
        cli = SippyCLI(config)
        cli.run()
        
    except ValueError as e:
        console.print(f"[red]Configuration error: {e}[/red]")
        sys.exit(1)
    except Exception as e:
        console.print(f"[red]Unexpected error: {e}[/red]")
        sys.exit(1)


if __name__ == '__main__':
    main()
