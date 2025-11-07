"""
Tool for getting prow job run payload information from Sippy API.
"""

import json
import logging
from typing import Any, Dict, Optional, Type
from pydantic import Field
import httpx

from .base_tool import SippyBaseTool, SippyToolInput

logger = logging.getLogger(__name__)


class SippyProwJobPayloadTool(SippyBaseTool):
    """Tool for getting the payload a prow job used from the Sippy API. Useful for determining what changes were in a payload where a problem first appeared."""

    name: str = "get_prow_job_payload"
    description: str = "Get payload information for a Prow job run including the payload tag (may be null) and job name. Input: just the numeric job ID (e.g., 1934795512955801600)"

    # Add sippy_api_url as a proper field
    sippy_api_url: Optional[str] = Field(default=None, description="Sippy API base URL")

    class ProwJobPayloadInput(SippyToolInput):
        prow_job_run_id: str = Field(description="Numeric prow job run ID only (e.g., 1934795512955801600)")

    args_schema: Type[SippyToolInput] = ProwJobPayloadInput

    def _run(self, *args, **kwargs: Any) -> Dict[str, Any]:
        """Get prow job run payload information from Sippy API."""

        input_data = {}
        if args and isinstance(args[0], dict):
            input_data.update(args[0])
        input_data.update(kwargs)

        # Pydantic model will have validated and filled in defaults
        args = self.ProwJobPayloadInput(**input_data)

        # Use provided URL or fall back to instance URL
        api_url = self.sippy_api_url

        if not api_url:
            return {
                "error": "No Sippy API URL configured. Please set SIPPY_API_URL environment variable or provide sippy_api_url parameter."
            }

        # Clean and validate the job ID - extract just the numeric part
        clean_job_id = str(args.prow_job_run_id).strip()
        # Extract just the numeric part if there's extra text
        import re

        job_id_match = re.search(r"\b(\d{10,})\b", clean_job_id)
        if job_id_match:
            clean_job_id = job_id_match.group(1)
        elif not clean_job_id.isdigit():
            return {"error": f"Invalid job ID format. Expected numeric ID, got: {args.prow_job_run_id}"}

        # Construct the API endpoint
        endpoint = f"{api_url.rstrip('/')}/api/job/run/payload"

        try:
            # Make the API request
            params = {"prow_job_run_id": clean_job_id}
            logger.info(f"Making request to {endpoint} with params: {params}")

            with httpx.Client(timeout=30.0) as client:
                response = client.get(endpoint, params=params)
                response.raise_for_status()

                data = response.json()

                # Return the raw JSON data
                return data

        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP error getting job payload: {e}")
            return {"error": f"HTTP {e.response.status_code} - {e.response.text}"}
        except httpx.RequestError as e:
            logger.error(f"Request error getting job payload: {e}")
            return {"error": f"Failed to connect to Sippy API at {api_url} - {str(e)}"}
        except json.JSONDecodeError as e:
            logger.error(f"JSON decode error: {e}")
            return {"error": "Invalid JSON response from Sippy API"}
        except Exception as e:
            logger.error(f"Unexpected error getting job payload: {e}")
            return {"error": f"Unexpected error - {str(e)}"}
