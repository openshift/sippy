"""
Command-line interface for Sippy Agent.
"""

import logging
import sys
from typing import Optional, List, Dict
import asyncio
import click
from rich.console import Console
from rich.panel import Panel
from rich.prompt import Prompt
from rich.text import Text
from rich.logging import RichHandler

from .agent import SippyAgent
from .api_models import ChatMessage
from .config import Config

console = Console()


def setup_logging(verbose: bool = False) -> None:
    """Setup logging with Rich handler."""
    level = logging.DEBUG if verbose else logging.INFO
    logging.basicConfig(level=level, format="%(message)s", datefmt="[%X]", handlers=[RichHandler(console=console, rich_tracebacks=True)])


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

        welcome_panel = Panel(welcome_text, title="Welcome", border_style="blue", padding=(1, 2))
        console.print(welcome_panel)
        console.print()

        # Display available tools
        tools = self.agent.list_tools()
        tools_text = "Available tools: " + ", ".join(f"[bold green]{tool}[/bold green]" for tool in tools)
        console.print(tools_text)
        console.print()

        # Show thinking status and persona
        thinking_status = "enabled" if self.config.show_thinking else "disabled"
        console.print(f"[dim]Thinking display: {thinking_status} (use 'thinking' to toggle)[/dim]")
        console.print(f"[dim]Current persona: {self.config.persona} (use 'personas' to see available)[/dim]")
        console.print("[dim]Type 'help' for commands, 'quit' or 'exit' to leave[/dim]")
        console.print()

    def display_help(self) -> None:
        """Display help information."""
        help_text = """
[bold cyan]Sippy AI Agent Commands:[/bold cyan]

[bold green]help[/bold green]     - Show this help message
[bold green]tools[/bold green]    - List available tools
[bold green]personas[/bold green] - List available AI personas
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

    def display_personas(self) -> None:
        """Display available personas."""
        from .personas import PERSONAS

        personas_text = ""
        for name, persona in PERSONAS.items():
            current = " [bold yellow](current)[/bold yellow]" if name == self.config.persona else ""
            personas_text += f"• [bold green]{name}[/bold green]{current}\n"
            personas_text += f"  {persona.description}\n"
            if persona.style_instructions:
                personas_text += f"  [dim]{persona.style_instructions}[/dim]\n"
            personas_text += "\n"

        console.print(Panel(personas_text.strip(), title="Available Personas", border_style="magenta"))

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
            console.print(Panel(step_content, title=f"Step {i}", border_style="dim", padding=(0, 1)))

        console.print()

    async def streaming_thinking_callback(self, thought: str, action: str, action_input: str, observation: str) -> None:
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
            console.print(Panel(step_content, title=f"Step {self.current_step} - Thinking", border_style="cyan", padding=(0, 1)))

            # Store for potential observation update
            self.streaming_steps.append(
                {"step": self.current_step, "thought": thought, "action": action, "action_input": action_input, "observation": ""}
            )

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

                if last_step["action_input"]:
                    step_content.append("📝 Input: ", style="bold yellow")
                    step_content.append(f"{last_step['action_input']}\n\n", style="yellow")

                step_content.append("👁️ Observation: ", style="bold magenta")
                step_content.append(observation, style="magenta")

                # Display updated step panel
                console.print(Panel(step_content, title=f"Step {last_step['step']} - Complete", border_style="green", padding=(0, 1)))

    async def process_user_input(self, user_input: str) -> bool:
        """Process user input and return False if should exit."""
        user_input = user_input.strip()

        # Handle special commands
        if user_input.lower() in ["quit", "exit"]:
            return False
        elif user_input.lower() == "help":
            self.display_help()
            return True
        elif user_input.lower() == "tools":
            self.display_tools()
            return True
        elif user_input.lower() == "personas":
            self.display_personas()
            return True
        elif user_input.lower() == "history":
            self.display_history()
            return True
        elif user_input.lower() == "clear":
            self.clear_history()
            return True
        elif user_input.lower() == "thinking":
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
            history_messages: List[ChatMessage] = []
            for user_msg, agent_msg in self.chat_history[-3:]:  # Last 3 exchanges
                history_messages.append(ChatMessage(role="user", content=user_msg))
                history_messages.append(ChatMessage(role="assistant", content=agent_msg))

            if self.config.show_thinking:
                # Show thinking header
                console.print()
                console.print(Panel("🧠 Agent's Thinking Process", title="Reasoning", border_style="cyan"))

                # Use streaming callback
                response = await self.agent.achat(user_input, history_messages, self.streaming_thinking_callback)
            else:
                with console.status("[bold green]Thinking...", spinner="dots"):
                    response = await self.agent.achat(user_input, history_messages)

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
            else:
                # Regular response without thinking
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

    def run(self) -> None:
        """Run the interactive CLI."""

        async def run_async():
            self.display_welcome()

            try:
                while True:
                    user_input = await asyncio.to_thread(Prompt.ask, "[bold blue]You")

                    if not await self.process_user_input(user_input):
                        break

            except KeyboardInterrupt:
                console.print("\n[yellow]Goodbye![/yellow]")
            except EOFError:
                console.print("\n[yellow]Goodbye![/yellow]")

        asyncio.run(run_async())
