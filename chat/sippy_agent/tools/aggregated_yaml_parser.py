"""
Tool for parsing aggregated test results from YAML URLs.
"""

import yaml
import logging
from typing import Any, Dict, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class AggregatedYAMLParserTool(SippyBaseTool):
    """Tool for parsing aggregated test results from YAML URLs."""

    name: str = "parse_aggregated_yaml"
    description: str = "Parse aggregated test results from a YAML URL to analyze job runs and failure patterns"

    class AggregatedYAMLInput(SippyToolInput):
        yaml_url: str = Field(description="URL to the aggregated YAML file (e.g., from job artifacts)")

    args_schema: Type[SippyToolInput] = AggregatedYAMLInput

    def _run(self, yaml_url: str) -> Dict[str, Any]:
        """Parse aggregated YAML and return structured data."""
        if not yaml_url or not yaml_url.startswith(("http://", "https://")):
            return {"error": "Invalid URL provided. Please provide a valid HTTP/HTTPS URL to a YAML file."}

        try:
            # Fetch the YAML content
            with httpx.Client(timeout=30.0) as client:
                response = client.get(yaml_url)
                response.raise_for_status()

                # Parse YAML content
                try:
                    data = yaml.safe_load(response.text)
                except yaml.YAMLError as e:
                    logger.error(f"YAML parsing error: {e}")
                    return {"error": f"Invalid YAML format - {str(e)}"}

                # Validate that we got a dictionary
                if not isinstance(data, dict):
                    return {"error": "Expected YAML data to be a dictionary"}

                # Return the raw data for LLM processing
                return data

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error fetching YAML: {e}")
            if e.response.status_code == 404:
                return {"error": f"YAML file not found at {yaml_url}. The URL may be incorrect or the file may have been moved."}
            else:
                return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error fetching YAML: {e}")
            return {"error": f"Failed to connect to {yaml_url} - {str(e)}"}
        except Exception as e:
            logger.error(f"Unexpected error parsing aggregated YAML: {e}")
            return {"error": f"Unexpected error - {str(e)}"}
