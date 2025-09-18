"""
Tool for analyzing job artifacts and logs from Sippy API.
"""

import json
import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput
from .log_analysis_helpers import format_log_analysis

logger = logging.getLogger(__name__)


class SippyLogAnalyzerTool(SippyBaseTool):
    """Tool for analyzing job artifacts and logs from Sippy API using the /api/jobs/artifacts endpoint."""

    name: str = "analyze_job_logs"
    description: str = "Search job artifacts for patterns. Input: numeric job ID, optional path_glob and text_regex"

    # Add sippy_api_url as a proper field
    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL")

    # Simple cache to prevent redundant API calls
    _cache: Dict[str, str] = {}
    
    class LogAnalyzerInput(SippyToolInput):
        prow_job_run_id: str = Field(description="Numeric prow job run ID only (e.g., 1934795512955801600)")
        path_glob: str = Field(
            default="*build-log*", 
            description="Path glob pattern to match artifacts (e.g., '*build-log*', '*.log', '**/junit*.xml')"
        )
        text_regex: str = Field(
            default="[Ee]rror|[Ff]ail", 
            description="Regex pattern to search for in the artifacts (e.g., '[Ee]rror', 'timeout', 'panic')"
        )
        sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL (optional, uses config if not provided)")
    
    args_schema: Type[SippyToolInput] = LogAnalyzerInput
    
    def _run(self, prow_job_run_id: str, path_glob: str = "*build-log*",
             text_regex: str = "[Ee]rror|[Ff]ail",
             sippy_api_url: Optional[str] = None) -> str:
        """Fetch and analyze job artifacts from Sippy API."""
        # Use provided URL or fall back to instance URL
        api_url = sippy_api_url or self.sippy_api_url

        if not api_url:
            return "Error: No Sippy API URL configured. Please set SIPPY_API_URL environment variable or provide sippy_api_url parameter."

        # Clean and validate the job ID - ensure it's just the numeric ID
        clean_job_id = str(prow_job_run_id).strip()
        # Extract just the numeric part if there's extra text
        import re
        job_id_match = re.search(r'\b(\d{10,})\b', clean_job_id)
        if job_id_match:
            clean_job_id = job_id_match.group(1)
        elif not clean_job_id.isdigit():
            return f"Error: Invalid job ID format. Expected numeric ID, got: {prow_job_run_id}"

        # Create cache key to prevent redundant calls
        cache_key = f"{clean_job_id}:{path_glob}:{text_regex}"
        if cache_key in self._cache:
            logger.info(f"Returning cached result for {cache_key}")
            return f"[CACHED RESULT]\n{self._cache[cache_key]}"
        
        # Construct the API endpoint
        endpoint = f"{api_url.rstrip('/')}/api/jobs/artifacts"
        
        try:
            # Make the API request with correct parameter names
            params = {
                "prowJobRuns": clean_job_id,  # Just the numeric ID
                "pathGlob": path_glob,
                "textRegex": text_regex
            }
            
            logger.info(f"Making request to {endpoint} with params: {params}")
            
            with httpx.Client(timeout=60.0) as client:  # Longer timeout for log analysis
                response = client.get(endpoint, params=params)
                response.raise_for_status()
                
                # The response should be JSON containing the matched artifacts
                data = response.json()

                # Format the response for better readability
                result = format_log_analysis(data, clean_job_id, path_glob, text_regex)

                # Cache the result to prevent redundant calls
                self._cache[cache_key] = result

                return result
                
        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error analyzing logs: {e}")
            return f"Error: HTTP {e.response.status_code} - {e.response.text}"
        except httpx.RequestError as e:
            logger.error(f"Request error analyzing logs: {e}")
            return f"Error: Failed to connect to Sippy API at {api_url} - {str(e)}"
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return f"Error: Invalid JSON response from Sippy API"
        except Exception as e:
            logger.error(f"Unexpected error analyzing logs: {e}")
            return f"Error: Unexpected error - {str(e)}"

    def get_aggregated_junit_url(self, prow_job_run_id: str, sippy_api_url: Optional[str] = None) -> str:
        """Get the direct URL to the junit-aggregated.xml file for an aggregated job."""
        # Use provided URL or fall back to instance URL
        api_url = sippy_api_url or self.sippy_api_url

        if not api_url:
            return "Error: No Sippy API URL configured. Please set SIPPY_API_URL environment variable or provide sippy_api_url parameter."

        # Clean and validate the job ID
        clean_job_id = str(prow_job_run_id).strip()
        import re
        job_id_match = re.search(r'\b(\d{10,})\b', clean_job_id)
        if job_id_match:
            clean_job_id = job_id_match.group(1)
        elif not clean_job_id.isdigit():
            return f"Error: Invalid job ID format. Expected numeric ID, got: {prow_job_run_id}"

        # Construct the API endpoint for aggregated JUnit artifacts
        endpoint = f"{api_url.rstrip('/')}/api/jobs/artifacts"

        try:
            # Make the API request specifically for junit-aggregated.xml
            params = {
                "prowJobRuns": clean_job_id,
                "pathGlob": "artifacts/**/junit-aggregated.xml"
            }

            logger.info(f"Fetching aggregated JUnit URL from {endpoint} with params: {params}")

            with httpx.Client(timeout=30.0) as client:
                response = client.get(endpoint, params=params)
                response.raise_for_status()

                data = response.json()

                # Extract the artifact URL from the response
                if isinstance(data, dict) and "job_runs" in data:
                    job_runs = data.get("job_runs", [])
                    if job_runs:
                        artifacts = job_runs[0].get("artifacts", [])
                        if artifacts:
                            artifact_url = artifacts[0].get("artifact_url", "")
                            if artifact_url:
                                return f"**Aggregated JUnit XML URL Found:**\n{artifact_url}\n\nUse the JUnit parser tool with this URL to analyze the aggregated test results."
                            else:
                                return "Error: No artifact URL found in the response."
                        else:
                            return "Error: No junit-aggregated.xml artifacts found for this job."
                    else:
                        return "Error: No job runs found in the response."
                else:
                    return "Error: Unexpected response format from Sippy API."

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error fetching aggregated JUnit URL: {e}")
            return f"Error: HTTP {e.response.status_code} - {e.response.text}"
        except httpx.RequestError as e:
            logger.error(f"Request error fetching aggregated JUnit URL: {e}")
            return f"Error: Failed to connect to Sippy API at {api_url} - {str(e)}"
        except Exception as e:
            logger.error(f"Unexpected error fetching aggregated JUnit URL: {e}")
            return f"Error: Unexpected error - {str(e)}"
