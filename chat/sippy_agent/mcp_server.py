"""
MCP Server implementation for Sippy Chat.

This module exposes the Sippy LangGraph agent through the Model Context Protocol,
allowing other systems to interact with Sippy's CI/CD analysis capabilities.

Prompts defined in YAML files are automatically loaded and exposed in two ways:

1. As MCP Tools: When called, the tool renders the prompt with the provided
   arguments, sends it to the Sippy agent, and returns the agent's response.
   This is useful for direct programmatic execution.

2. As MCP Prompts (slash commands): When retrieved, the prompt returns instructions
   for an LLM to call the corresponding tool with the appropriate arguments.
   This is useful for LLM-driven interactions where the LLM decides when to use the tool.
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

        # Dynamically register prompts from YAML files as both tools and prompts
        for prompt_name, prompt_data in self.prompts.items():
            self._register_prompt_as_tool(prompt_name, prompt_data)
            self._register_prompt_as_prompt(prompt_name, prompt_data)
    
    def _register_prompt_as_tool(self, prompt_name: str, prompt_data: Dict[str, Any]):
        """Register a prompt as an MCP tool that renders and executes it."""
        # For each prompt, we need to create a function that FastMCP/Pydantic can handle
        # We'll use exec to create a properly typed function
        
        # Build function parameters from YAML arguments
        params = []
        param_names = []
        param_types = []
        for arg in prompt_data.get("arguments", []):
            arg_name = arg["name"]
            arg_type = arg.get("type", "string")
            param_names.append(arg_name)
            param_types.append(arg_type)
            
            # Handle array types
            if arg_type == "array":
                if arg.get("required", False):
                    params.append(f"{arg_name}: List[str]")
                else:
                    params.append(f"{arg_name}: Optional[List[str]] = None")
            else:
                if arg.get("required", False):
                    params.append(f"{arg_name}: str")
                else:
                    params.append(f"{arg_name}: Optional[str] = None")
        
        # Build function code
        func_name = prompt_name.replace("-", "_")
        params_str = ", ".join(params) if params else ""
        
        # Create the function body that renders the prompt and sends it to the agent
        # Escape single quotes in description to avoid breaking the docstring
        description = prompt_data.get("description", "").replace("'", "\\'")
        
        # Build parameter documentation
        param_docs = []
        for arg in prompt_data.get("arguments", []):
            arg_name = arg["name"]
            arg_desc = arg.get("description", "").replace("'", "\\'")
            required_str = " (required)" if arg.get("required", False) else " (optional)"
            param_docs.append(f"    {arg_name}: {arg_desc}{required_str}")
        
        params_doc_str = "\\n".join(param_docs) if param_docs else "    No parameters"
        
        func_code = f"""
async def {func_name}({params_str}) -> str:
    '''
    {description}
    
    Args:
{params_doc_str}
    
    Returns:
        The Sippy agent's analysis response as text
    '''
    import re
    prompt_data = _prompt_data_registry["{prompt_name}"]
    agent = _agent_registry["{prompt_name}"]
    kwargs = {{{', '.join(f'"{name}": {name}' for name in param_names)}}}
    param_types = {{{', '.join(f'"{name}": "{ptype}"' for name, ptype in zip(param_names, param_types))}}}
    
    # Render the prompt messages with argument substitution
    rendered_content = []
    for msg_data in prompt_data.get("messages", []):
        content = msg_data.get("content", "")
        
        # Substitute arguments with support for defaults: {{arg|default:value}}
        def substitute_arg(match):
            arg_expr = match.group(1)
            parts = arg_expr.split('|default:', 1)
            arg_name = parts[0].strip()
            default_value = parts[1].strip() if len(parts) > 1 else None
            
            arg_value = kwargs.get(arg_name)
            
            # Handle array types - convert to comma-separated string
            if param_types.get(arg_name) == "array" and isinstance(arg_value, list):
                arg_value = ", ".join(arg_value)
            
            if arg_value is not None:
                return str(arg_value)
            elif default_value is not None:
                return default_value
            else:
                return ""
        
        content = re.sub(r'\\{{\\s*([^}}]+?)\\s*\\}}', substitute_arg, content)
        rendered_content.append(content)
    
    # Combine all message content (assuming single user message for now)
    full_prompt = "\\n\\n".join(rendered_content)
    
    # Send to agent and get response
    try:
        result = await agent.achat(full_prompt, None)
        
        # Extract response text
        if isinstance(result, dict):
            return result.get("output", str(result))
        else:
            return str(result)
    except Exception as e:
        logger.error(f"Error executing prompt tool {{func_name}}: {{e}}", exc_info=True)
        return f"Error: {{str(e)}}"
"""
        
        # Store prompt data and agent reference in registries
        if not hasattr(self, '_prompt_data_registry'):
            self._prompt_data_registry = {}
        if not hasattr(self, '_agent_registry'):
            self._agent_registry = {}
        self._prompt_data_registry[prompt_name] = prompt_data
        self._agent_registry[prompt_name] = self.agent
        
        # Execute the function definition
        exec_globals = {
            "Optional": Optional,
            "List": List,
            "logger": logger,
            "_prompt_data_registry": self._prompt_data_registry,
            "_agent_registry": self._agent_registry
        }
        exec(func_code, exec_globals)
        
        # Get the created function and register it as a tool
        prompt_func = exec_globals[func_name]
        self.mcp.tool()(prompt_func)
    
    def _register_prompt_as_prompt(self, prompt_name: str, prompt_data: Dict[str, Any]):
        """Register a prompt as an MCP prompt that instructs the LLM to call the tool."""
        # Build function parameters from YAML arguments
        # For prompts, we accept strings for all parameters (more flexible for slash commands)
        # and convert arrays as needed
        params = []
        param_names = []
        param_types = []
        for arg in prompt_data.get("arguments", []):
            arg_name = arg["name"]
            arg_type = arg.get("type", "string")
            param_names.append(arg_name)
            param_types.append(arg_type)
            
            # For prompts (slash commands), accept strings for everything to be more flexible
            if arg.get("required", False):
                params.append(f"{arg_name}: str")
            else:
                params.append(f"{arg_name}: Optional[str] = None")
        
        # Build function code
        # Use a unique internal function name but we'll register it with the original prompt name
        func_name = f"_prompt_impl_{prompt_name.replace('-', '_')}"
        params_str = ", ".join(params) if params else ""
        
        # Escape single quotes in description
        description = prompt_data.get("description", "").replace("'", "\\'")
        
        # Build the instruction message that tells the LLM to call the tool
        tool_name = prompt_name.replace("-", "_")
        
        # Build parameter documentation for the instruction
        param_docs = []
        for arg in prompt_data.get("arguments", []):
            arg_name = arg["name"]
            arg_desc = arg.get("description", "").replace("'", "\\'")
            required_str = " (required)" if arg.get("required", False) else " (optional)"
            param_docs.append(f"  - {arg_name}: {arg_desc}{required_str}")
        
        params_doc_str = "\\n".join(param_docs) if param_docs else "  No parameters"
        
        func_code = f"""
async def {func_name}({params_str}) -> str:
    '''
    {description}
    
    Args:
{params_doc_str if params else "    No parameters"}
    '''
    # Build the instruction message
    instruction = "Use the {tool_name} tool to {description.lower()}\\n\\n"
    instruction += "Call it with the following arguments:\\n"
    
    prompt_data = _prompt_data_registry["{prompt_name}"]
    kwargs = {{{', '.join(f'"{name}": {name}' for name in param_names)}}}
    param_types = {{{', '.join(f'"{name}": "{ptype}"' for name, ptype in zip(param_names, param_types))}}}
    
    # List each argument with its value or placeholder
    for arg in prompt_data.get("arguments", []):
        arg_name = arg["name"]
        arg_value = kwargs.get(arg_name)
        arg_type = param_types.get(arg_name, "string")
        
        if arg_value is not None:
            # Convert string to list format if this is an array type
            if arg_type == "array" and isinstance(arg_value, str):
                # Split on commas or spaces for slash command inputs
                list_value = [v.strip() for v in arg_value.replace(',', ' ').split() if v.strip()]
                instruction += f"  - {{arg_name}}: {{list_value}}\\n"
            elif isinstance(arg_value, list):
                instruction += f"  - {{arg_name}}: {{arg_value}}\\n"
            else:
                instruction += f"  - {{arg_name}}: {{arg_value}}\\n"
        elif arg.get("required", False):
            instruction += f"  - {{arg_name}}: <required, not provided>\\n"
        else:
            instruction += f"  - {{arg_name}}: <optional, not provided>\\n"
    
    return instruction
"""
        
        # Store prompt data in registry
        if not hasattr(self, '_prompt_data_registry'):
            self._prompt_data_registry = {}
        if prompt_name not in self._prompt_data_registry:
            self._prompt_data_registry[prompt_name] = prompt_data
        
        # Execute the function definition
        exec_globals = {
            "Optional": Optional,
            "List": List,
            "_prompt_data_registry": self._prompt_data_registry
        }
        exec(func_code, exec_globals)
        
        # Get the created function and register it as a prompt with the original name
        prompt_func = exec_globals[func_name]
        # Set the function name to match the YAML prompt name (with underscores for Python compatibility)
        # but the MCP prompt will use the original hyphenated name
        prompt_func.__name__ = prompt_name.replace("-", "_")
        # Note: FastMCP uses the function name, so we need to keep hyphens converted to underscores
        # The prompt name in MCP will be the underscored version (e.g., "test_analysis")
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
    
    # List all available tools and prompts
    tool_names = ["sippy_chat"] + [p.replace("-", "_") for p in mcp_server.prompts.keys()]
    prompt_names = list(mcp_server.prompts.keys())
    logger.info(f"Tools: {', '.join(tool_names)}")
    logger.info(f"Prompts: {', '.join(prompt_names)}")
    
    # Run the FastMCP server
    mcp_server.get_mcp().run(port=port)

