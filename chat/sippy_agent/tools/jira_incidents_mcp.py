"""
MCP-based tool for querying Jira for known open incidents in the TRT project.
This version connects to Sippy's Go MCP server instead of directly calling Jira APIs.
"""

import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import asyncio

from langchain_mcp_adapters import MCPToolkit
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
        self._mcp_toolkit = None
        self._mcp_tool = None
    
    async def _get_mcp_toolkit(self) -> MCPToolkit:
        """Get or create the MCP toolkit connection."""
        if self._mcp_toolkit is None:
            logger.debug(f"Creating MCP toolkit for {self.mcp_server_url}")
            self._mcp_toolkit = MCPToolkit(server_url=self.mcp_server_url)
            try:
                await self._mcp_toolkit.initialize()
                logger.debug("MCP toolkit initialized successfully")
            except Exception as e:
                logger.error(f"Failed to initialize MCP toolkit: {e}")
                raise
        return self._mcp_toolkit
    
    async def _get_jira_tool(self) -> BaseTool:
        """Get the Jira incident tool from the MCP server."""
        if self._mcp_tool is None:
            toolkit = await self._get_mcp_toolkit()
            tools = await toolkit.get_tools()
            
            # Find the check_known_incidents tool
            jira_tool = None
            for tool in tools:
                if tool.name == "check_known_incidents":
                    jira_tool = tool
                    break
            
            if jira_tool is None:
                available_tools = [tool.name for tool in tools]
                raise RuntimeError(f"check_known_incidents tool not found in MCP server. Available tools: {available_tools}")
            
            self._mcp_tool = jira_tool
            logger.debug("Found check_known_incidents tool in MCP server")
        
        return self._mcp_tool
    
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
        if self._mcp_toolkit:
            try:
                await self._mcp_toolkit.close()
                logger.debug("MCP toolkit connection closed")
            except Exception as e:
                logger.warning(f"Error closing MCP toolkit: {e}")
            finally:
                self._mcp_toolkit = None
                self._mcp_tool = None


# Backwards compatibility alias
SippyJiraIncidentTool = SippyJiraIncidentMCPTool
