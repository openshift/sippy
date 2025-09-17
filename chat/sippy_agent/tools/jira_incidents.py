"""
MCP-based tool for querying Jira for known open incidents in the TRT project.
This version connects to Sippy's Go MCP server instead of directly calling Jira APIs.
"""

import logging
from typing import Any, Dict, Optional, Type, List
from pydantic import Field
import asyncio

from langchain_mcp_adapters.tools import load_mcp_tools
from langchain_mcp_adapters.sessions import StreamableHttpConnection
from langchain_core.tools import BaseTool

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class SippyJiraIncidentMCPTool(SippyBaseTool):
    """MCP-based tool for querying Jira for known open incidents in the TRT project."""
    
    name: str = "check_known_incidents"
    description: str = "Check Jira for known open TRT incidents. ONLY use this when job errors suggest a correlation. Use specific keywords that match actual errors found in logs."
    
    # MCP server configuration
    mcp_server_url: str = Field(default="http://localhost:8080/mcp/v1/", description="MCP server URL")
    
    class JiraIncidentInput(SippyToolInput):
        search_terms: Optional[str] = Field(
            default=None,
            description="Optional search terms to filter incidents (e.g., 'registry', 'build11', 'timeout')"
        )
    
    args_schema: Type[SippyToolInput] = JiraIncidentInput
    
    def __init__(self, mcp_server_url: Optional[str] = None, **data):
        super().__init__(**data)
        if mcp_server_url:
            self.mcp_server_url = mcp_server_url
        self._mcp_tools = None
        self._jira_tool = None
    
    async def _get_mcp_tools(self) -> List[BaseTool]:
        """Get or create the MCP tools connection."""
        if self._mcp_tools is None:
            logger.debug(f"Loading MCP tools from {self.mcp_server_url}")
            
            # Create connection configuration for streamable HTTP
            connection: StreamableHttpConnection = {
                "transport": "streamable_http",
                "url": self.mcp_server_url,
            }
            
            try:
                self._mcp_tools = await load_mcp_tools(session=None, connection=connection)
                logger.debug(f"MCP tools loaded successfully: {len(self._mcp_tools)} tools available")
            except Exception as e:
                logger.error(f"Failed to load MCP tools: {e}")
                raise
        return self._mcp_tools
    
    async def _get_jira_tool(self) -> BaseTool:
        """Get the Jira incident tool from the MCP server."""
        if self._jira_tool is None:
            tools = await self._get_mcp_tools()
            
            # Find the check_known_incidents tool
            jira_tool = None
            for tool in tools:
                if tool.name == "check_known_incidents":
                    jira_tool = tool
                    break
            
            if jira_tool is None:
                available_tools = [tool.name for tool in tools]
                raise RuntimeError(f"check_known_incidents tool not found in MCP server. Available tools: {available_tools}")
            
            self._jira_tool = jira_tool
            logger.debug("Found check_known_incidents tool in MCP server")
        
        return self._jira_tool
    
    def _run(self, search_terms: Optional[str] = None) -> str:
        """Query Jira for known open incidents via MCP server."""
        try:
            # Run async code in sync context
            return asyncio.run(self._arun(search_terms))
        except Exception as e:
            logger.error(f"Error querying incidents via MCP: {e}")
            return f"Error: Failed to query incidents via MCP server - {str(e)}"
    
    async def _arun(self, search_terms: Optional[str] = None) -> str:
        """Async version of the tool execution."""
        try:
            logger.info(f"Querying MCP server for incidents with search terms: {search_terms}")
            
            # Get the MCP tool
            jira_tool = await self._get_jira_tool()
            
            # Prepare arguments for the MCP tool
            tool_args = {}
            if search_terms:
                tool_args["search_terms"] = search_terms
            
            # Call the MCP tool
            result = await jira_tool.ainvoke(tool_args)
            
            logger.debug("Successfully received response from MCP server")
            return result
            
        except Exception as e:
            logger.error(f"Error in async MCP call: {e}")
            return f"Error: Failed to query incidents via MCP server - {str(e)}"
    
    async def cleanup(self):
        """Clean up MCP connection."""
        # For langchain-mcp-adapters, cleanup is handled automatically
        # when the tools go out of scope or the session ends
        self._mcp_tools = None
        self._jira_tool = None
        logger.debug("MCP tool references cleared")


# Backwards compatibility alias
SippyJiraIncidentTool = SippyJiraIncidentMCPTool
