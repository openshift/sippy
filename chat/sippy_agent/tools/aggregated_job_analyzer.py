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
    description: str = "Get a JSON object with the direct URL to aggregated test results (in JUnit XML format) for detailed analysis of aggregated Prow jobs. Only use this when specifically asked for detailed aggregated job analysis. Input: numeric job ID only."

    # Add sippy_api_url as a proper field
    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL")

    class AggregatedJobInput(SippyToolInput):
        prow_job_run_id: str = Field(description="Numeric prow job run ID only (e.g., 1934795512955801600)")
        sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL (optional, uses config if not provided)")

    args_schema: Type[SippyToolInput] = AggregatedJobInput

    def _run(self, *args, **kwargs: Any) -> Dict[str, Any]:
        """Get the direct URL to the aggregated test results (YAML format) for an aggregated job."""

        input_data = {}
        if args and isinstance(args[0], dict):
            input_data.update(args[0])
        input_data.update(kwargs)

        try:
            params = self.AggregatedJobInput(**input_data)
        except Exception as e:
            return {"error": f"Invalid input parameters: {e}"}

        # Use provided URL or fall back to instance URL
        api_url = params.sippy_api_url or self.sippy_api_url

        if not api_url:
            return {
                "error": "No Sippy API URL configured. Please set SIPPY_API_URL environment variable or provide sippy_api_url parameter."
            }

        # Clean and validate the job ID
        clean_job_id = str(params.prow_job_run_id).strip()
        import re

        job_id_match = re.search(r"\b(\d{10,})\b", clean_job_id)
        if job_id_match:
            clean_job_id = job_id_match.group(1)
        elif not clean_job_id.isdigit():
            return {"error": f"Invalid job ID format. Expected numeric ID, got: {params.prow_job_run_id}"}

        # Construct the API endpoint for aggregated JUnit artifacts
        endpoint = f"{api_url.rstrip('/')}/api/jobs/artifacts"

        try:
            # Make the API request specifically for junit-aggregated.xml
            params = {"prowJobRuns": clean_job_id, "pathGlob": "artifacts/**/junit-aggregated.xml"}

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
                                return {
                                    "aggregated_junit_xml_url": artifact_url,
                                    "next_step": "Use the parse_junit_xml tool with the returned URL to analyze the aggregated test results.",
                                }
                            else:
                                return {"error": "No artifact URL found in the response."}
                        else:
                            return {
                                "error": "No junit-aggregated.xml artifacts found for this job. This may not be an aggregated job or the artifacts may not be available yet."
                            }
                    else:
                        return {"error": "No job runs found in the response."}
                else:
                    return {"error": "Unexpected response format from Sippy API."}

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error fetching aggregated JUnit URL: {e}")
            return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error fetching aggregated JUnit URL: {e}")
            return {"error": f"Failed to connect to Sippy API at {api_url} - {str(e)}"}
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return {"error": "Invalid JSON response from Sippy API"}
        except Exception as e:
            logger.error(f"Unexpected error fetching aggregated JUnit URL: {e}")
            return {"error": f"Unexpected error - {str(e)}"}
