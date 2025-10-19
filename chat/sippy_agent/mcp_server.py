"""
MCP Server implementation for Sippy Chat.

This module exposes the Sippy LangGraph agent as an MCP tool, allowing
other systems to interact with Sippy's CI/CD analysis capabilities through
the Model Context Protocol.
"""

import logging
import os
from pathlib import Path
from typing import Optional, List, Dict, Any
import yaml

from mcp.server.fastmcp import FastMCP
from mcp import types

from .agent import SippyAgent
from .config import Config
from .api_models import ChatMessage

logger = logging.getLogger(__name__)


def load_prompts_from_directory(prompts_dir: Path) -> Dict[str, Dict[str, Any]]:
    """
    Load all prompt definitions from YAML files in the prompts directory.
    
    Args:
        prompts_dir: Path to the directory containing prompt YAML files
        
    Returns:
        Dictionary mapping prompt names to their definitions
    """
    prompts = {}
    
    if not prompts_dir.exists():
        logger.warning(f"Prompts directory not found: {prompts_dir}")
        return prompts
    
    for yaml_file in prompts_dir.glob("*.yaml"):
        try:
            with open(yaml_file, "r") as f:
                prompt_data = yaml.safe_load(f)
                
            if not prompt_data or "name" not in prompt_data:
                logger.warning(f"Invalid prompt file {yaml_file}: missing 'name' field")
                continue
                
            prompt_name = prompt_data["name"]
            prompts[prompt_name] = prompt_data
            logger.info(f"Loaded prompt: {prompt_name} from {yaml_file.name}")
            
        except Exception as e:
            logger.error(f"Error loading prompt from {yaml_file}: {e}", exc_info=True)
            
    return prompts


class SippyMCPServer:
    """MCP Server wrapper for Sippy Agent."""

    def __init__(self, config: Config, prompts_dir: Optional[Path] = None):
        """Initialize the Sippy MCP server."""
        self.config = config
        self.agent = SippyAgent(config)
        self.mcp = FastMCP("sippy-chat")
        
        # Load prompts from directory
        if prompts_dir is None:
            # Default to prompts directory relative to the chat module
            chat_dir = Path(__file__).parent.parent
            prompts_dir = chat_dir / "prompts"
        
        self.prompts = load_prompts_from_directory(prompts_dir)
        logger.info(f"Loaded {len(self.prompts)} prompts from {prompts_dir}")
        
        self._setup_handlers()

    def _setup_handlers(self):
        """Set up MCP server handlers for tools and prompts."""

        @self.mcp.tool()
        async def sippy_chat(message: str, chat_history: Optional[List[Dict[str, str]]] = None) -> str:
            """
            Chat with Sippy AI agent to analyze CI job failures, test results, and release payloads.
            
            Sippy can investigate Prow job runs, analyze test failures, examine release payload health,
            correlate failures with known incidents tracked in Jira, and provide detailed analysis of
            CI/CD pipeline issues in the OpenShift ecosystem. The agent has access to job logs, test
            results, payload changelogs, and historical CI data.
            
            Args:
                message: The user's message or question about CI/CD issues
                chat_history: Previous conversation context (optional), list of {role, content} dicts
            
            Returns:
                The AI agent's response as text
            """
            # Parse chat history if provided
            parsed_history: Optional[List[ChatMessage]] = None
            if chat_history:
                parsed_history = [
                    ChatMessage(role=msg["role"], content=msg["content"])
                    for msg in chat_history
                ]

            try:
                # Call the agent
                result = await self.agent.achat(message, parsed_history)

                # Extract response text
                if isinstance(result, dict):
                    return result.get("output", str(result))
                else:
                    return str(result)

            except Exception as e:
                logger.error(f"Error processing sippy_chat tool call: {e}", exc_info=True)
                return f"Error: {str(e)}"

        # Dynamically register prompts from YAML files
        for prompt_name, prompt_data in self.prompts.items():
            self._register_prompt(prompt_name, prompt_data)
    
    def _register_prompt(self, prompt_name: str, prompt_data: Dict[str, Any]):
        """Register a single prompt dynamically."""
        # For each prompt, we need to create a function that FastMCP/Pydantic can handle
        # We'll use exec to create a properly typed function
        
        # Build function parameters from YAML arguments
        params = []
        param_names = []
        for arg in prompt_data.get("arguments", []):
            arg_name = arg["name"]
            param_names.append(arg_name)
            if arg.get("required", False):
                params.append(f"{arg_name}: str")
            else:
                params.append(f"{arg_name}: Optional[str] = None")
        
        # Build function code
        func_name = prompt_name.replace("-", "_")
        params_str = ", ".join(params) if params else ""
        
        # Create the function body that performs substitution
        func_code = f"""
async def {func_name}({params_str}) -> str:
    '''
    {prompt_data.get("description", "")}
    '''
    messages = []
    prompt_data = _prompt_data_registry["{prompt_name}"]
    kwargs = {{{', '.join(f'"{name}": {name}' for name in param_names)}}}
    
    for msg_data in prompt_data.get("messages", []):
        content = msg_data.get("content", "")
        
        # Substitute arguments if provided
        for arg_name, arg_value in kwargs.items():
            if arg_value is not None:
                content = content.replace(f"{{{{{arg_name}}}}}", arg_value)
        
        messages.append(f"{{msg_data.get('role', 'user')}}: {{content}}")
    
    return "\\n\\n".join(messages)
"""
        
        # Store prompt data in a registry that the generated function can access
        if not hasattr(self, '_prompt_data_registry'):
            self._prompt_data_registry = {}
        self._prompt_data_registry[prompt_name] = prompt_data
        
        # Execute the function definition
        exec_globals = {
            "Optional": Optional,
            "_prompt_data_registry": self._prompt_data_registry
        }
        exec(func_code, exec_globals)
        
        # Get the created function and register it
        prompt_func = exec_globals[func_name]
        self.mcp.prompt()(prompt_func)

    def get_mcp(self) -> FastMCP:
        """Get the underlying FastMCP instance."""
        return self.mcp


def create_mcp_server(config: Optional[Config] = None) -> SippyMCPServer:
    """
    Create and configure an MCP server for Sippy Chat.

    Args:
        config: Optional configuration. If not provided, will load from environment.

    Returns:
        Configured SippyMCPServer instance.
    """
    if config is None:
        config = Config.from_env()

    return SippyMCPServer(config)


if __name__ == "__main__":
    """Run the MCP server standalone."""
    import sys
    
    # Parse command line arguments
    port = 8001  # Default MCP server port
    if len(sys.argv) > 1:
        try:
            port = int(sys.argv[1])
        except ValueError:
            print(f"Invalid port: {sys.argv[1]}")
            sys.exit(1)
    
    # Create and run the server
    config = Config.from_env()
    mcp_server = create_mcp_server(config)
    
    logger.info(f"Starting Sippy MCP Server on port {port}")
    logger.info(f"SSE endpoint will be available at: http://localhost:{port}/sse")
    logger.info(f"Tools: sippy_chat")
    logger.info(f"Prompts: {list(mcp_server.prompts.keys())}")
    
    # Run the FastMCP server
    mcp_server.get_mcp().run(port=port)

