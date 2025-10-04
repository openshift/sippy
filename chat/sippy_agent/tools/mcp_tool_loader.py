"""
Load tools from MCP (Model Context Protocol) servers.
"""

import json
import logging
import os
import re
from typing import List, Dict, Any

from langchain.tools import BaseTool
from langchain_mcp_adapters.client import MultiServerMCPClient


logger = logging.getLogger(__name__)


def _replace_env_vars(config: Any) -> Any:
    """Recursively replace ${env:VAR} with environment variable values."""
    if isinstance(config, dict):
        return {k: _replace_env_vars(v) for k, v in config.items()}
    elif isinstance(config, list):
        return [_replace_env_vars(i) for i in config]
    elif isinstance(config, str):
        # Use regex to find all instances of ${env:VAR} and replace them.
        return re.sub(r"\$\{env:([_A-Za-z0-9]+)\}", lambda m: os.getenv(m.group(1), ""), config)
    return config


def _adapt_server_config(servers: Dict[str, Any]) -> Dict[str, Any]:
    """Adapt the server config from the user's JSON format to what MultiServerMCPClient expects."""
    adapted_servers = {}
    for name, config in servers.items():
        adapted_config = config.copy()
        if "type" in adapted_config:
            transport = adapted_config.pop("type")
            # The library example uses 'streamable_http' for http transport.
            if transport == "http":
                transport = "streamable_http"
            adapted_config["transport"] = transport
        adapted_servers[name] = adapted_config
    return adapted_servers


async def load_tools_from_mcp(mcp_config_file: str) -> List[BaseTool]:
    """
    Load tools from MCP servers defined in a JSON configuration file.

    Args:
        mcp_config_file: Path to the MCP configuration file.

    Returns:
        A list of BaseTool instances loaded from the MCP servers.
    """
    if not os.path.exists(mcp_config_file):
        logger.warning(f"MCP config file not found: {mcp_config_file}")
        return []

    try:
        with open(mcp_config_file, "r") as f:
            config = json.load(f)
    except json.JSONDecodeError:
        logger.error(f"Invalid JSON in MCP config file: {mcp_config_file}")
        return []

    mcp_servers = config.get("mcpServers", {})
    if not mcp_servers:
        logger.info("No mcpServers found in the configuration.")
        return []

    try:
        # Replace environment variables in the configuration
        processed_servers = _replace_env_vars(mcp_servers)

        # Adapt config to what MultiServerMCPClient expects ('transport' key)
        adapted_servers = _adapt_server_config(processed_servers)

        # Use MultiServerMCPClient to load all tools at once
        client = MultiServerMCPClient(adapted_servers)
        tools = await client.get_tools()
        logger.info(f"Loaded {len(tools)} tools from {len(mcp_servers)} MCP servers.")
        return tools
    except Exception as e:
        logger.error(f"Failed to load tools from MCP servers: {e}")
        return []
