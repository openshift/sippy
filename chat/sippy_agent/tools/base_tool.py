"""
Base classes and interfaces for Sippy Agent tools.
"""

import json
import logging
from abc import ABC, abstractmethod
from typing import Any, Optional, Type, ClassVar
from pydantic import BaseModel, Field
from langchain.tools import BaseTool

logger = logging.getLogger(__name__)


class SippyToolInput(BaseModel):
    """Base input schema for Sippy tools."""
    pass


class SippyBaseTool(BaseTool, ABC):
    """Base class for all Sippy Agent tools."""

    name: str = Field(..., description="Name of the tool")
    description: str = Field(..., description="Description of what the tool does")
    args_schema: Type[BaseModel] = SippyToolInput

    # Maximum output size in bytes (150KB)
    MAX_OUTPUT_SIZE: ClassVar[int] = 150 * 1024

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def _truncate_output_if_needed(self, output: str) -> str:
        """Truncate output if it exceeds the maximum size limit."""
        if not isinstance(output, str):
            output = str(output)

        # Check if output exceeds the limit
        output_bytes = output.encode('utf-8')
        if len(output_bytes) <= self.MAX_OUTPUT_SIZE:
            return output

        # Calculate how much we can keep (leave room for truncation message)
        truncation_message = "\n\n⚠️ **Tool output truncated** - Tool produced too much data (>150KB). Use more specific queries or filters to get focused results."
        truncation_bytes = truncation_message.encode('utf-8')
        available_bytes = self.MAX_OUTPUT_SIZE - len(truncation_bytes)

        # Truncate at character boundary to avoid encoding issues
        truncated_output = output.encode('utf-8')[:available_bytes].decode('utf-8', errors='ignore')

        # Try to truncate at a reasonable boundary (end of line)
        last_newline = truncated_output.rfind('\n')
        if last_newline > available_bytes * 0.8:  # If we can keep 80% of content
            truncated_output = truncated_output[:last_newline]

        result = truncated_output + truncation_message

        # Log the truncation
        original_size = len(output_bytes)
        final_size = len(result.encode('utf-8'))
        logger.warning(f"Tool {self.name} output truncated: {original_size:,} bytes -> {final_size:,} bytes")

        return result

    def run(self, *args, **kwargs) -> str:
        """Override run to add output size limiting."""
        try:
            # Filter out LangChain-specific kwargs that tools don't need
            langchain_params = {
                'verbose', 'callbacks', 'tags', 'metadata', 'run_name',
                'color', 'llm_prefix', 'observation_prefix', 'return_intermediate_steps'
            }
            filtered_kwargs = {k: v for k, v in kwargs.items() if k not in langchain_params}

            # Call the original _run method with filtered kwargs
            result = self._run(*args, **filtered_kwargs)
            # Apply size limiting
            return self._truncate_output_if_needed(result)
        except Exception as e:
            logger.error(f"Error in tool {self.name}: {e}")
            return f"Error in {self.name}: {str(e)}"

    @abstractmethod
    def _run(self, **kwargs: Any) -> str:
        """Execute the tool with the given arguments."""
        pass

    async def _arun(self, **kwargs: Any) -> str:
        """Async version of _run. Default implementation calls _run."""
        return self._run(**kwargs)


class ExampleTool(SippyBaseTool):
    """Example tool to demonstrate the structure."""
    
    name: str = "example_tool"
    description: str = "An example tool that echoes back the input"
    
    class ExampleInput(SippyToolInput):
        message: str = Field(description="Message to echo back")
    
    args_schema: Type[BaseModel] = ExampleInput
    
    def _run(self, message: str) -> str:
        """Echo back the input message."""
        return f"Echo: {message}"
