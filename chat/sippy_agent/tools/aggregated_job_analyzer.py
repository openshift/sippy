"""
Tool for analyzing aggregated prow jobs.
"""

import json
import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class AggregatedJobAnalyzerTool(SippyBaseTool):
    """Tool for getting aggregated test results URLs (YAML format) from aggregated prow jobs."""
    
    name: str = "get_aggregated_results_url"
    description: str = "Get the direct URL to aggregated test results (YAML format) for detailed analysis of aggregated prow jobs. Only use this when specifically asked for detailed aggregated job analysis or underlying job investigation. For basic aggregated job information, use get_prow_job_summary first. Input: numeric job ID only."
    
    # Add sippy_api_url as a proper field
    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL")
    
    class AggregatedJobInput(SippyToolInput):
        prow_job_run_id: str = Field(description="Numeric prow job run ID only (e.g., 1934795512955801600)")
        sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL (optional, uses config if not provided)")
    
    args_schema: Type[SippyToolInput] = AggregatedJobInput
    
    def _run(self, prow_job_run_id: str, sippy_api_url: Optional[str] = None) -> str:
        """Get the direct URL to the aggregated test results (YAML format) for an aggregated job."""
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
                                result = f"**Aggregated JUnit XML URL Found:**\n"
                                result += f"🔗 **URL:** {artifact_url}\n\n"
                                result += f"**For detailed analysis:**\n"
                                result += f"Use the JUnit parser tool with this URL to analyze the aggregated test results.\n"
                                result += f"The JUnit parser will show which tests failed and provide links to underlying job runs.\n\n"
                                result += f"**Note:** Aggregated jobs contain both successful and failing underlying jobs - focus on the failed ones.\n\n"
                                result += f"**Example command:** `parse_junit_xml` with junit_xml_url: {artifact_url}"
                                return result
                            else:
                                return "Error: No artifact URL found in the response."
                        else:
                            return "Error: No junit-aggregated.xml artifacts found for this job. This may not be an aggregated job or the artifacts may not be available yet."
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
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return f"Error: Invalid JSON response from Sippy API"
        except Exception as e:
            logger.error(f"Unexpected error fetching aggregated JUnit URL: {e}")
            return f"Error: Unexpected error - {str(e)}"
